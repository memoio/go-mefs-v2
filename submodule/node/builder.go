package node

import (
	"context"
	"fmt"
	"strconv"

	"github.com/filecoin-project/go-jsonrpc"
	"github.com/gorilla/mux"
	"github.com/libp2p/go-libp2p"
	"github.com/pkg/errors"
	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/api/httpio"
	"github.com/memoio/go-mefs-v2/lib/address"
	"github.com/memoio/go-mefs-v2/lib/repo"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/service/netapp"
	"github.com/memoio/go-mefs-v2/submodule/auth"
	mconfig "github.com/memoio/go-mefs-v2/submodule/config"
	"github.com/memoio/go-mefs-v2/submodule/connect/settle"
	"github.com/memoio/go-mefs-v2/submodule/network"
	"github.com/memoio/go-mefs-v2/submodule/role"
	"github.com/memoio/go-mefs-v2/submodule/state"
	"github.com/memoio/go-mefs-v2/submodule/txPool"
	"github.com/memoio/go-mefs-v2/submodule/wallet"
)

// Builder
type Builder struct {
	daemon bool // daemon mod

	libp2pOpts []libp2p.Option // network ops

	repo repo.Repo

	walletPassword string // en/decrypt wallet from keystore
	authURL        string
}

// construct build ops from repo.
func OptionsFromRepo(r repo.Repo) ([]BuilderOpt, error) {
	_, sk, err := network.GetSelfNetKey(r.KeyStore())
	if err != nil {
		return nil, err
	}

	cfg := r.Config()
	cfgopts := []BuilderOpt{
		// Libp2pOptions can only be called once, so add all options here.
		Libp2pOptions(
			libp2p.ListenAddrStrings(cfg.Net.Addresses...),
			libp2p.Identity(sk),
			libp2p.DisableRelay(),
		),
	}

	dsopt := func(c *Builder) error {
		c.repo = r
		return nil
	}

	return append(cfgopts, dsopt), nil
}

// Builder private method accessors for impl's

type builder Builder

// Repo get home data repo
func (b builder) Repo() repo.Repo {
	return b.repo
}

// Libp2pOpts get libp2p option
func (b builder) Libp2pOpts() []libp2p.Option {
	return b.libp2pOpts
}

func (b builder) DaemonMode() bool {
	return b.daemon
}

type BuilderOpt func(*Builder) error

// DaemonMode enables or disables daemon mode.
func DaemonMode(daemon bool) BuilderOpt {
	return func(c *Builder) error {
		c.daemon = daemon
		return nil
	}
}

// SetPassword set wallet password
func SetPassword(password string) BuilderOpt {
	return func(c *Builder) error {
		c.walletPassword = password
		return nil
	}
}

// SetAuthURL set venus auth service URL
func SetAuthURL(url string) BuilderOpt {
	return func(c *Builder) error {
		c.authURL = url
		return nil
	}
}

// Libp2pOptions returns a builder option that sets up the libp2p node
func Libp2pOptions(opts ...libp2p.Option) BuilderOpt {
	return func(b *Builder) error {
		// Quietly having your options overridden leads to hair loss
		if len(b.libp2pOpts) > 0 {
			panic("Libp2pOptions can only be called once")
		}
		b.libp2pOpts = opts
		return nil
	}
}

// New creates a new node.
func New(ctx context.Context, opts ...BuilderOpt) (*BaseNode, error) {
	// initialize builder and set base values
	builder := &Builder{
		daemon: true,
	}

	// apply builder options
	for _, o := range opts {
		if err := o(builder); err != nil {
			return nil, err
		}
	}

	// build the node
	return builder.build(ctx)
}

func (b *Builder) build(ctx context.Context) (*BaseNode, error) {
	//
	// Set default values on un-initialized fields
	//
	if b.repo == nil {
		return nil, fmt.Errorf("no repo")
	}
	var err error
	// create the node
	nd := &BaseNode{
		ctx:  ctx,
		Repo: b.repo,
	}

	cfg := b.repo.Config()

	saddr := cfg.Wallet.DefaultAddress
	defaultAddr, err := address.NewFromString(saddr)
	if err != nil {
		return nil, err
	}

	id, err := settle.GetRoleID(defaultAddr)
	if err != nil {
		return nil, err
	}

	gid := settle.GetGroupID(id)

	networkName := cfg.Net.Name + "/group" + strconv.Itoa(int(gid))

	logger.Debug("networkName is :", networkName)

	nd.NetworkSubmodule, err = network.NewNetworkSubmodule(ctx, (*builder)(b), b.repo.Config(), b.repo.MetaStore(), networkName)
	if err != nil {
		return nil, errors.Wrap(err, "failed to build node Network")
	}
	nd.IsOnline = true

	nd.LocalWallet = wallet.New(b.walletPassword, b.repo.KeyStore())

	ok, err := nd.LocalWallet.WalletHas(ctx, defaultAddr)
	if err != nil {
		return nil, err
	}

	if !ok {
		return nil, xerrors.New("donot have default address")
	}

	cs, err := netapp.New(ctx, id, nd.MetaStore(), nd.NetworkSubmodule)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create core service")
	}

	nd.NetServiceImpl = cs

	rm, err := role.New(ctx, id, gid, nd.MetaStore(), nd.LocalWallet)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create role service")
	}

	nd.RoleMgr = rm

	jauth, err := auth.NewJwtAuth(b.repo)
	if err != nil {
		return nil, err
	}

	nd.ConfigModule = mconfig.NewConfigModule(b.repo)

	nd.JwtAuth = jauth

	txs, err := tx.NewTxStore(ctx, nd.MetaStore())
	if err != nil {
		return nil, err
	}

	sp := txPool.NewSyncPool(ctx, id, nd.MetaStore(), txs, rm, cs)

	nd.PPool = txPool.NewPushPool(ctx, sp)

	nd.StateDB = state.NewStateMgr(nd.MetaStore(), rm)

	// register apply msg
	nd.PPool.RegisterMsgFunc(nd.StateDB.AppleyMsg)
	nd.PPool.RegisterBlockFunc(nd.StateDB.ApplyBlock)

	readerHandler, readerServerOpt := httpio.ReaderParamDecoder()

	nd.RPCServer = jsonrpc.NewServer(readerServerOpt)

	muxRouter := mux.NewRouter()

	muxRouter.Handle("/rpc/v0", nd.RPCServer)
	muxRouter.Handle("/rpc/streams/v0/push/{uuid}", readerHandler)

	nd.httpHandle = muxRouter

	return nd, nil
}
