package node

import (
	"context"
	"fmt"

	"github.com/libp2p/go-libp2p"
	"github.com/pkg/errors"

	"github.com/memoio/go-mefs-v2/lib/repo"
	core_service "github.com/memoio/go-mefs-v2/service/core"
	"github.com/memoio/go-mefs-v2/submodule/auth"
	"github.com/memoio/go-mefs-v2/submodule/network"
	"github.com/memoio/go-mefs-v2/submodule/wallet"
)

// Builder is a helper to aid in the construction of a filecoin node.
type Builder struct {
	Online bool // enable network

	offlineMode bool            // need start network module
	libp2pOpts  []libp2p.Option // network ops

	repo repo.Repo

	walletPassword string // en/decrypt wallet from keystore
	authURL        string
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

// OfflineMode get the p2p network mode
func (b builder) OfflineMode() bool {
	return b.offlineMode
}

// BuilderOpt is an option for building a filecoin node.
type BuilderOpt func(*Builder) error

// offlineMode enables or disables offline mode.
func OfflineMode(offlineMode bool) BuilderOpt {
	return func(c *Builder) error {
		c.offlineMode = offlineMode
		return nil
	}
}

// SetWalletPassword set wallet password
func SetWalletPassword(password string) BuilderOpt {
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
		offlineMode: false,
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
		repo: b.repo,
	}

	nd.NetworkSubmodule, err = network.NewNetworkSubmodule(ctx, (*builder)(b), b.repo.Config(), b.repo.MetaStore())
	if err != nil {
		return nil, errors.Wrap(err, "failed to build node Network")
	}
	nd.IsOnline = true

	nd.LocalWallet = wallet.New(b.walletPassword, b.repo.KeyStore())

	cs, err := core_service.New(ctx, nd.NetworkSubmodule, nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create core service")
	}

	nd.Service = cs

	jauth, err := auth.NewJwtAuth(b.repo)
	if err != nil {
		return nil, err
	}

	nd.JwtAuth = jauth

	return nd, nil
}
