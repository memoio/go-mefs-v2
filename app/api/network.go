package api

import (
	"context"

	"github.com/libp2p/go-libp2p-core/metrics"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
)

type INetwork interface {
	// info
	NetAddrInfo(context.Context) (peer.AddrInfo, error)

	NetConnectedness(context.Context, peer.ID) (network.Connectedness, error)

	NetConnect(context.Context, peer.AddrInfo) error

	NetDisconnect(context.Context, peer.ID) error

	NetFindPeer(context.Context, peer.ID) (peer.AddrInfo, error)

	NetPeerInfo(context.Context, peer.ID) (*ExtendedPeerInfo, error)

	NetPeers(context.Context) ([]peer.AddrInfo, error)

	// status; add more
	NetAutoNatStatus(context.Context) (NatInfo, error)
	NetBandwidthStats(ctx context.Context) (metrics.Stats, error)
	NetBandwidthStatsByPeer(ctx context.Context) (map[string]metrics.Stats, error)
	NetBandwidthStatsByProtocol(ctx context.Context) (map[protocol.ID]metrics.Stats, error)
}
