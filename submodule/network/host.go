package network

import (
	"context"

	"github.com/libp2p/go-libp2p"
	circuit "github.com/libp2p/go-libp2p-circuit"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/routing"
	ma "github.com/multiformats/go-multiaddr"
)

type RawHost host.Host

// address determines if we are publically dialable.  If so use public
// address, if not configure node to announce relay address.
func buildHost(ctx context.Context, config networkConfig, libP2pOpts []libp2p.Option, repo networkRepo, makeDHT func(host host.Host) (routing.Routing, error)) (host.Host, error) {

	// Node must build a host acting as a libp2p relay.  Additionally it
	// runs the autoNAT service which allows other nodes to check for their
	// own dialability by having this node attempt to dial them.
	makeDHTRightType := func(h host.Host) (routing.PeerRouting, error) {
		return makeDHT(h)
	}

	if config.IsRelay() {
		cfg := repo.Config()
		publicAddr, err := ma.NewMultiaddr(cfg.Net.PublicRelayAddress)
		if err != nil {
			return nil, err
		}
		publicAddrFactory := func(lc *libp2p.Config) error {
			lc.AddrsFactory = func(addrs []ma.Multiaddr) []ma.Multiaddr {
				if cfg.Net.PublicRelayAddress == "" {
					return addrs
				}
				return append(addrs, publicAddr)
			}
			return nil
		}

		relayHost, err := libp2p.New(
			ctx,
			libp2p.EnableRelay(circuit.OptHop),
			libp2p.EnableAutoRelay(),
			libp2p.Routing(makeDHTRightType),
			publicAddrFactory,
			libp2p.ChainOptions(libP2pOpts...),
			libp2p.Ping(true),
			libp2p.EnableNATService(),
		)
		if err != nil {
			return nil, err
		}
		return relayHost, nil
	}

	return libp2p.New(
		ctx,
		libp2p.UserAgent("memoriae"),
		libp2p.ChainOptions(libP2pOpts...),
		libp2p.Ping(true),
		libp2p.DisableRelay(),
	)
}
