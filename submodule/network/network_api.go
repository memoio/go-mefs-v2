package network

import (
	"context"
	"sort"
	"time"

	"github.com/ipfs/go-cid"
	metrics "github.com/libp2p/go-libp2p-core/metrics"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
	basichost "github.com/libp2p/go-libp2p/p2p/host/basic"
	"github.com/memoio/go-mefs-v2/lib/net"
	ma "github.com/multiformats/go-multiaddr"
)

type ExtendedPeerInfo struct {
	ID          peer.ID
	Agent       string
	Addrs       []string
	Protocols   []string
	ConnMgrMeta *ConnMgrInfo
}

type ConnMgrInfo struct {
	FirstSeen time.Time
	Value     int
	Tags      map[string]int
	Conns     map[string]time.Time
}

type NatInfo struct {
	Reachability network.Reachability
	PublicAddr   string
}

type NetworkAPI struct { //nolint
	*NetworkSubmodule
}

func (networkAPI *NetworkAPI) NetConnectedness(ctx context.Context, pid peer.ID) (network.Connectedness, error) {
	return networkAPI.Host.Network().Connectedness(pid), nil
}

// NetworkGetBandwidthStats gets stats on the current bandwidth usage of the network
func (networkAPI *NetworkAPI) NetBandwidthStats(ctx context.Context) (metrics.Stats, error) {
	return networkAPI.Network.Reporter.GetBandwidthTotals(), nil
}

func (networkAPI *NetworkAPI) NetBandwidthStatsByPeer(ctx context.Context) (map[string]metrics.Stats, error) {
	out := make(map[string]metrics.Stats)
	for p, s := range networkAPI.Network.Reporter.GetBandwidthByPeer() {
		out[p.String()] = s
	}
	return out, nil
}

func (networkAPI *NetworkAPI) NetBandwidthStatsByProtocol(ctx context.Context) (map[protocol.ID]metrics.Stats, error) {
	return networkAPI.Network.Reporter.GetBandwidthByProtocol(), nil
}

// ID gets the current peer id of the node
func (networkAPI *NetworkAPI) ID(context.Context) (peer.ID, error) {
	return networkAPI.Host.ID(), nil
}

// NetworkGetPeerAddresses gets the current addresses of the node
func (networkAPI *NetworkAPI) Addrs() ([]string, error) {
	addrs := networkAPI.Host.Addrs()
	strs := make([]string, 0, len(addrs))
	for _, addr := range addrs {
		strs = append(strs, addr.String())
	}
	return strs, nil
}

// NetworkFindProvidersAsync issues a findProviders query to the filecoin network content router.
func (networkAPI *NetworkAPI) NetworkFindProvidersAsync(ctx context.Context, key cid.Cid, count int) <-chan peer.AddrInfo {
	return networkAPI.Network.Router.FindProvidersAsync(ctx, key, count)
}

// NetworkGetClosestPeers issues a getClosestPeers query to the filecoin network.
func (networkAPI *NetworkAPI) NetworkGetClosestPeers(ctx context.Context, key string) ([]peer.ID, error) {
	return networkAPI.Network.GetClosestPeers(ctx, key)
}

// NetworkConnect connects to peers at the given addresses
func (networkAPI *NetworkAPI) NetworkConnect(ctx context.Context, addrs []string) (<-chan net.ConnectionResult, error) {
	return networkAPI.Network.Connect(ctx, addrs)
}

func (networkAPI *NetworkAPI) NetDisconnect(ctx context.Context, p peer.ID) error {
	return networkAPI.Host.Network().ClosePeer(p)
}

func (networkAPI *NetworkAPI) NetPeerInfo(_ context.Context, p peer.ID) (*ExtendedPeerInfo, error) {
	info := &ExtendedPeerInfo{ID: p}

	agent, err := networkAPI.Host.Peerstore().Get(p, "AgentVersion")
	if err == nil {
		info.Agent = agent.(string)
	}

	for _, a := range networkAPI.Host.Peerstore().Addrs(p) {
		info.Addrs = append(info.Addrs, a.String())
	}
	sort.Strings(info.Addrs)

	protocols, err := networkAPI.Host.Peerstore().GetProtocols(p)
	if err == nil {
		sort.Strings(protocols)
		info.Protocols = protocols
	}

	if cm := networkAPI.Host.ConnManager().GetTagInfo(p); cm != nil {
		info.ConnMgrMeta = &ConnMgrInfo{
			FirstSeen: cm.FirstSeen,
			Value:     cm.Value,
			Tags:      cm.Tags,
			Conns:     cm.Conns,
		}
	}

	return info, nil
}

func (networkAPI *NetworkAPI) NetPeers(context.Context) ([]peer.AddrInfo, error) {
	conns := networkAPI.Host.Network().Conns()
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

func (networkAPI *NetworkAPI) NetAutoNatStatus(ctx context.Context) (i NatInfo, err error) {
	autonat := networkAPI.RawHost.(*basichost.BasicHost).GetAutoNat()

	if autonat == nil {
		return NatInfo{
			Reachability: network.ReachabilityUnknown,
		}, nil
	}

	var maddr string
	if autonat.Status() == network.ReachabilityPublic {
		pa, err := autonat.PublicAddr()
		if err != nil {
			return NatInfo{}, err
		}
		maddr = pa.String()
	}

	return NatInfo{
		Reachability: autonat.Status(),
		PublicAddr:   maddr,
	}, nil
}

// NetworkPeers lists peers currently available on the network
func (networkAPI *NetworkAPI) NetworkPeers(ctx context.Context, verbose, latency, streams bool) (*net.SwarmConnInfos, error) {
	return networkAPI.Network.Peers(ctx, verbose, latency, streams)
}

func (networkAPI *NetworkAPI) FindPeer(ctx context.Context, p peer.ID) (peer.AddrInfo, error) {
	return networkAPI.Router.FindPeer(ctx, p)
}

func (networkAPI *NetworkAPI) Shutdown(ctx context.Context) error {
	networkAPI.ShutdownChan <- struct{}{}
	return nil
}

func (networkAPI *NetworkAPI) Closing(ctx context.Context) (<-chan struct{}, error) {
	return make(chan struct{}), nil // relies on jsonrpc closing
}

// Version provides various build-time information
type Version struct {
	Version string
}

func (networkAPI *NetworkAPI) Version(context.Context) (Version, error) {
	return Version{
		Version: "2.0",
	}, nil
}

func (networkAPI *NetworkAPI) NetAddrsListen(context.Context) (peer.AddrInfo, error) {
	return peer.AddrInfo{
		ID:    networkAPI.Host.ID(),
		Addrs: networkAPI.Host.Addrs(),
	}, nil
}
