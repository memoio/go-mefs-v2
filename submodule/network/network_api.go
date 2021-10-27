package network

import (
	"context"
	"sort"

	metrics "github.com/libp2p/go-libp2p-core/metrics"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
	basichost "github.com/libp2p/go-libp2p/p2p/host/basic"
	ma "github.com/multiformats/go-multiaddr"

	"github.com/memoio/go-mefs-v2/app/api"
	"github.com/memoio/go-mefs-v2/lib/net"
)

var _ api.INetwork = &networkAPI{}

type networkAPI struct { //nolint
	*NetworkSubmodule
}

// info
func (napi *networkAPI) NetName(context.Context) string {
	return napi.NetworkName
}

func (napi *networkAPI) NetID(context.Context) peer.ID {
	return napi.Host.ID()
}

func (napi *networkAPI) NetAddrInfo(context.Context) (peer.AddrInfo, error) {
	return peer.AddrInfo{
		ID:    napi.Host.ID(),
		Addrs: napi.Host.Addrs(),
	}, nil
}

// connect
func (napi *networkAPI) NetConnectedness(ctx context.Context, pid peer.ID) (network.Connectedness, error) {
	return napi.Host.Network().Connectedness(pid), nil
}

func (napi *networkAPI) NetConnect(ctx context.Context, pai peer.AddrInfo) error {
	return napi.Network.Connect(ctx, pai)
}

func (napi *networkAPI) NetDisconnect(ctx context.Context, p peer.ID) error {
	return napi.Host.Network().ClosePeer(p)
}

func (napi *networkAPI) NetFindPeer(ctx context.Context, p peer.ID) (peer.AddrInfo, error) {
	return napi.Router.FindPeer(ctx, p)
}

func (napi *networkAPI) NetGetClosestPeers(ctx context.Context, key string) ([]peer.ID, error) {
	return napi.Network.GetClosestPeers(ctx, key)
}

func (napi *networkAPI) NetPeerInfo(ctx context.Context, p peer.ID) (*api.ExtendedPeerInfo, error) {
	info := &api.ExtendedPeerInfo{ID: p}

	agent, err := napi.Host.Peerstore().Get(p, "AgentVersion")
	if err == nil {
		info.Agent = agent.(string)
	}

	for _, a := range napi.Host.Peerstore().Addrs(p) {
		info.Addrs = append(info.Addrs, a.String())
	}
	sort.Strings(info.Addrs)

	protocols, err := napi.Host.Peerstore().GetProtocols(p)
	if err == nil {
		sort.Strings(protocols)
		info.Protocols = protocols
	}

	if cm := napi.Host.ConnManager().GetTagInfo(p); cm != nil {
		info.ConnMgrMeta = &api.ConnMgrInfo{
			FirstSeen: cm.FirstSeen,
			Value:     cm.Value,
			Tags:      cm.Tags,
			Conns:     cm.Conns,
		}
	}

	return info, nil
}

func (napi *networkAPI) NetPeers(context.Context) ([]peer.AddrInfo, error) {
	conns := napi.Host.Network().Conns()
	out := make([]peer.AddrInfo, len(conns))

	for i, conn := range conns {
		out[i] = peer.AddrInfo{
			ID: conn.RemotePeer(),
			Addrs: []ma.Multiaddr{
				conn.RemoteMultiaddr(),
			},
		}
	}

	return out, nil
}

func (napi *networkAPI) NetSwarmPeers(ctx context.Context, verbose, latency, streams bool) (*net.SwarmConnInfos, error) {
	return napi.Network.Peers(ctx, verbose, latency, streams)
}

// stats
func (napi *networkAPI) NetBandwidthStats(ctx context.Context) (metrics.Stats, error) {
	return napi.Network.Reporter.GetBandwidthTotals(), nil
}

func (napi *networkAPI) NetBandwidthStatsByPeer(ctx context.Context) (map[string]metrics.Stats, error) {
	out := make(map[string]metrics.Stats)
	for p, s := range napi.Network.Reporter.GetBandwidthByPeer() {
		out[p.String()] = s
	}
	return out, nil
}

func (napi *networkAPI) NetBandwidthStatsByProtocol(ctx context.Context) (map[protocol.ID]metrics.Stats, error) {
	return napi.Network.Reporter.GetBandwidthByProtocol(), nil
}

func (napi *networkAPI) NetAutoNatStatus(ctx context.Context) (i api.NatInfo, err error) {
	autonat := napi.RawHost.(*basichost.BasicHost).GetAutoNat()

	if autonat == nil {
		return api.NatInfo{
			Reachability: network.ReachabilityUnknown,
		}, nil
	}

	var maddr string
	if autonat.Status() == network.ReachabilityPublic {
		pa, err := autonat.PublicAddr()
		if err != nil {
			return api.NatInfo{}, err
		}
		maddr = pa.String()
	}

	return api.NatInfo{
		Reachability: autonat.Status(),
		PublicAddr:   maddr,
	}, nil
}

func (networkAPI *networkAPI) Shutdown(ctx context.Context) error {
	networkAPI.ShutdownChan <- struct{}{}
	return nil
}

func (networkAPI *networkAPI) Closing(ctx context.Context) (<-chan struct{}, error) {
	return make(chan struct{}), nil // relies on jsonrpc closing
}
