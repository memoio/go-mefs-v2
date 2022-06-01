package node

import (
	"context"
	"log"
	"sort"
	"strconv"

	"github.com/filecoin-project/go-jsonrpc"
	"github.com/gorilla/mux"
	"github.com/libp2p/go-libp2p"
	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/api/httpio"
	"github.com/memoio/go-mefs-v2/build"
	"github.com/memoio/go-mefs-v2/lib/address"
	"github.com/memoio/go-mefs-v2/lib/backend/wrap"
	"github.com/memoio/go-mefs-v2/lib/pb"
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

	roleID         uint64
	groupID        uint64
	walletPassword string // en/decrypt wallet from keystore
	authURL        string
}

// construct build ops from repo
// todo: move some ops here
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

func (b builder) GroupID() uint64 {
	return b.groupID
}

func (b builder) RoleID() uint64 {
	return b.roleID
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

func SetGroupID(gid uint64) BuilderOpt {
	return func(c *Builder) error {
		c.groupID = gid
		return nil
	}
}

func SetRoleID(rid uint64) BuilderOpt {
	return func(c *Builder) error {
		c.roleID = rid
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
		err := o(builder)
		if err != nil {
			return nil, err
		}
	}

	// build the node
	return builder.build(ctx)
}

func (b *Builder) build(ctx context.Context) (*BaseNode, error) {
	if b.repo == nil {
		return nil, xerrors.Errorf("repo is nil")
	}

	cfg := b.repo.Config()

	saddr := cfg.Wallet.DefaultAddress
	defaultAddr, err := address.NewFromString(saddr)
	if err != nil {
		return nil, err
	}

	lw := wallet.New(b.walletPassword, b.repo.KeyStore())
	ki, err := lw.WalletExport(ctx, defaultAddr, b.walletPassword)
	if err != nil {
		return nil, err
	}

	cm, err := settle.NewContractMgr(ctx, cfg.Contract.EndPoint, cfg.Contract.RoleContract, ki.SecretKey)
	if err != nil {
		return nil, err
	}

	err = cm.Start(pb.RoleInfo_Unknown, b.groupID)
	if err != nil {
		return nil, err
	}

	// create the node
	nd := &BaseNode{
		ctx:          ctx,
		Repo:         b.repo,
		roleID:       b.roleID,
		groupID:      b.groupID,
		pw:           b.walletPassword,
		version:      build.UserVersion(),
		ContractMgr:  cm,
		LocalWallet:  lw,
		shutdownChan: make(chan struct{}),
	}

	networkName := cfg.Contract.RoleContract[2:10] + "/" + strconv.FormatUint(b.groupID, 10)

	logger.Debug("networkName is: ", networkName)

	nd.NetworkSubmodule, err = network.NewNetworkSubmodule(ctx, (*builder)(b), networkName)
	if err != nil {
		return nil, xerrors.Errorf("failed to build node network %w", err)
	}

	var lisAddrs []string
	ifaceAddrs, err := nd.Host.Network().InterfaceListenAddresses()
	if err != nil {
		log.Fatalf("failed to read listening addresses: %s", err)
		return nil, err
	}
	for _, addr := range ifaceAddrs {
		lisAddrs = append(lisAddrs, addr.String())
	}
	sort.Strings(lisAddrs)
	for _, addr := range lisAddrs {
		logger.Info("Swarm listening on: ", addr)
	}

	nd.isOnline = true

	jauth, err := auth.NewJwtAuth(b.repo)
	if err != nil {
		return nil, err
	}

	nd.JwtAuth = jauth

	nd.ConfigModule = mconfig.NewConfigModule(b.repo)

	// network
	cs, err := netapp.New(ctx, b.roleID, nd.MetaStore(), nd.NetworkSubmodule)
	if err != nil {
		return nil, xerrors.Errorf("failed to create core service %w", err)
	}

	nd.NetServiceImpl = cs

	// role mgr
	rm, err := role.New(ctx, b.roleID, nd.groupID, nd.MetaStore(), nd.LocalWallet, nd.ContractMgr)
	if err != nil {
		return nil, xerrors.Errorf("failed to create role service %w", err)
	}

	nd.RoleMgr = rm

	txs, err := tx.NewTxStore(ctx, wrap.NewKVStore("tx", nd.StateStore()))
	if err != nil {
		return nil, err
	}
	nd.Store = txs

	// state mgr
	stDB := state.NewStateMgr(cm.GetRoleAddr().Bytes(), b.groupID, nd.SettleGetThreshold(ctx), nd.StateStore(), rm)

	// sync pool
	sp := txPool.NewSyncPool(ctx, b.groupID, nd.SettleGetThreshold(ctx), stDB, txs, cs)

	// push pool
	nd.PushPool = txPool.NewPushPool(ctx, sp)

	readerHandler, readerServerOpt := httpio.ReaderParamDecoder()

	nd.RPCServer = jsonrpc.NewServer(readerServerOpt)

	nd.HttpHandle = mux.NewRouter()

	nd.HttpHandle.Handle("/rpc/v0", nd.RPCServer)
	nd.HttpHandle.Handle("/rpc/streams/v0/push/{uuid}", readerHandler)

	// move to Start()
	//nd.HttpHandle.Handle("/debug/metrics", metrics.Exporter())
	//nd.HttpHandle.PathPrefix("/").Handler(http.DefaultServeMux)

	return nd, nil
}
