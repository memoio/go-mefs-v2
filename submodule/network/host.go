package network

import (
	"context"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/memoio/go-mefs-v2/config"
)

type RawHost host.Host

// address determines if we are publically dialable.  If so use public
// address, if not configure node to announce relay address.
func buildHost(ctx context.Context, config networkConfig, libP2pOpts []libp2p.Option, cfg *config.Config) (host.Host, error) {
	return libp2p.New(
		ctx,
		libp2p.UserAgent("memoriae"),
		libp2p.ChainOptions(libP2pOpts...),
		libp2p.Ping(true),
	)
}
