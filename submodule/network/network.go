package network

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/metrics"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
	"github.com/libp2p/go-libp2p-core/routing"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	swarm "github.com/libp2p/go-libp2p-swarm"
	"github.com/libp2p/go-libp2p/p2p/discovery"
	basichost "github.com/libp2p/go-libp2p/p2p/host/basic"
	routed "github.com/libp2p/go-libp2p/p2p/host/routed"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/pkg/errors"

	"github.com/memoio/go-mefs-v2/api"
	"github.com/memoio/go-mefs-v2/build"
	config "github.com/memoio/go-mefs-v2/config"
	"github.com/memoio/go-mefs-v2/lib/types/store"
	"github.com/memoio/go-mefs-v2/lib/utils/net"
	"github.com/memoio/go-mefs-v2/lib/utils/storeutil"
)

// NetworkSubmodule enhances the `Node` with networking capabilities.
type NetworkSubmodule struct { //nolint
	NetworkName string

	RawHost host.Host
	Host    host.Host

	// dht related
	Router routing.Routing

	// pub/sub topics
	Pubsub *pubsub.PubSub

	// connect bootstrap peers first
	// peer manager
	PeerMgr IPeerMgr

	// find peer in local net
	Discovery discovery.Service

	// metrics info
	Reporter *metrics.BandwidthCounter
}

type networkConfig interface {
	Libp2pOpts() []libp2p.Option
}

type blankValidator struct{}

func (blankValidator) Validate(_ string, _ []byte) error        { return nil }
func (blankValidator) Select(_ string, _ [][]byte) (int, error) { return 0, nil }

// NewNetworkSubmodule creates a new network submodule.
func NewNetworkSubmodule(ctx context.Context, config networkConfig, cfg *config.Config, ds store.KVStore, networkName string) (*NetworkSubmodule, error) {
	bandwidthTracker := metrics.NewBandwidthCounter()

	libP2pOpts := append(config.Libp2pOpts(), Transport())
	libP2pOpts = append(libP2pOpts, libp2p.BandwidthReporter(bandwidthTracker))
	libP2pOpts = append(libP2pOpts, libp2p.EnableNATService())
	libP2pOpts = append(libP2pOpts, libp2p.NATPortMap())
	libP2pOpts = append(libP2pOpts, Peerstore())
	libP2pOpts = append(libP2pOpts, makeSmuxTransportOption())
	libP2pOpts = append(libP2pOpts, Security(true, false))

	nds, err := storeutil.NewDatastore("dht", ds)
	if err != nil {
		return nil, err
	}

	// set up host
	rawHost, err := libp2p.New(
		ctx,
		libp2p.UserAgent("memoriae"),
		libp2p.ChainOptions(libP2pOpts...),
		libp2p.Ping(true),
	)
	if err != nil {
		return nil, err
	}

	// setup dht
	validator := blankValidator{}
	bootNodes, err := net.ParseAddresses(ctx, cfg.Bootstrap.Addresses)
	if err != nil {
		return nil, err
	}

	dhtopts := []dht.Option{dht.Mode(dht.ModeAutoServer),
		dht.Datastore(nds),
		dht.Validator(validator),
		dht.ProtocolPrefix(build.MemoriaeDHT(networkName)),
		dht.QueryFilter(dht.PublicQueryFilter),
		dht.RoutingTableFilter(dht.PublicRoutingTableFilter),
		dht.BootstrapPeers(bootNodes...),
		dht.DisableProviders(),
		dht.DisableValues(),
	}

	router, err := dht.New(ctx, rawHost, dhtopts...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to setup routing")
	}

	peerHost := routed.Wrap(rawHost, router)

	peerMgr, err := NewPeerMgr(peerHost, router, bootNodes)
	if err != nil {
		return nil, err
	}

	go peerMgr.Run(ctx)

	mdnsdisc, err := SetupDiscovery(ctx, peerHost, DiscoveryHandler(ctx, peerHost))
	if err != nil {
		log.Error("Setup Discovery falied, error:", err)
	}

	// Set up pubsub
	topicdisc, err := TopicDiscovery(ctx, peerHost, router)
	if err != nil {
		return nil, err
	}

	pubsub.GossipSubHeartbeatInterval = 100 * time.Millisecond
	options := []pubsub.Option{
		// Gossipsubv1.1 configuration
		pubsub.WithFloodPublish(true),
		pubsub.WithDiscovery(topicdisc),
	}

	gsub, err := pubsub.NewGossipSub(ctx, peerHost, options...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to set up network")
	}

	// build the network submdule
	return &NetworkSubmodule{
		NetworkName: networkName,
		RawHost:     rawHost,
		Host:        peerHost,
		Router:      router,
		Pubsub:      gsub,
		Reporter:    bandwidthTracker,
		PeerMgr:     peerMgr,
		Discovery:   mdnsdisc,
	}, nil
}

func (ns *NetworkSubmodule) API() *networkAPI {
	return &networkAPI{ns}
}

func (ns *NetworkSubmodule) Stop(ctx context.Context) {
	if err := ns.Host.Close(); err != nil {
		fmt.Printf("error closing host: %s\n", err)
	}
	if err := ns.Discovery.Close(); err != nil {
		fmt.Printf("error closing Discovery: %s\n", err)
	}
	if err := ns.PeerMgr.Stop(ctx); err != nil {
		fmt.Printf("error closing PeerMgr: %s\n", err)
	}
}

// info
func (ns *NetworkSubmodule) NetName(context.Context) string {
	return ns.NetworkName
}

func (ns *NetworkSubmodule) NetID(context.Context) peer.ID {
	return ns.Host.ID()
}

func (ns *NetworkSubmodule) NetAddrInfo(context.Context) (peer.AddrInfo, error) {
	return peer.AddrInfo{
		ID:    ns.Host.ID(),
		Addrs: ns.Host.Addrs(),
	}, nil
}

// connect
func (ns *NetworkSubmodule) NetConnectedness(ctx context.Context, pid peer.ID) (network.Connectedness, error) {
	return ns.Host.Network().Connectedness(pid), nil
}

func (ns *NetworkSubmodule) NetConnect(ctx context.Context, pai peer.AddrInfo) error {
	if ns.Host.Network().Connectedness(pai.ID) == network.Connected {
		return nil
	}

	if len(pai.Addrs) == 0 {
		// find peer first
		npi, err := ns.NetFindPeer(ctx, pai.ID)
		if err != nil {
			return nil
		}
		pai = npi
	}

	swrm, ok := ns.Host.Network().(*swarm.Swarm)
	if !ok {
		return fmt.Errorf("peerhost network was not a swarm")
	}

	swrm.Backoff().Clear(pai.ID)
	return ns.Host.Connect(ctx, pai)
}

func (ns *NetworkSubmodule) NetDisconnect(ctx context.Context, p peer.ID) error {
	return ns.Host.Network().ClosePeer(p)
}

func (ns *NetworkSubmodule) NetFindPeer(ctx context.Context, p peer.ID) (peer.AddrInfo, error) {
	return ns.Router.FindPeer(ctx, p)
}

func (ns *NetworkSubmodule) NetGetClosestPeers(ctx context.Context, key string) ([]peer.ID, error) {

	ipfsDHT, ok := ns.Router.(*dht.IpfsDHT)
	if !ok {
		return nil, errors.New("underlying routing should be pointer of IpfsDHT")
	}
	return ipfsDHT.GetClosestPeers(ctx, key)
}

func (ns *NetworkSubmodule) NetPeerInfo(ctx context.Context, p peer.ID) (*api.ExtendedPeerInfo, error) {
	info := &api.ExtendedPeerInfo{ID: p}

	agent, err := ns.Host.Peerstore().Get(p, "AgentVersion")
	if err == nil {
		info.Agent = agent.(string)
	}

	for _, a := range ns.Host.Peerstore().Addrs(p) {
		info.Addrs = append(info.Addrs, a.String())
	}
	sort.Strings(info.Addrs)

	protocols, err := ns.Host.Peerstore().GetProtocols(p)
	if err == nil {
		sort.Strings(protocols)
		info.Protocols = protocols
	}

	if cm := ns.Host.ConnManager().GetTagInfo(p); cm != nil {
		info.ConnMgrMeta = &api.ConnMgrInfo{
			FirstSeen: cm.FirstSeen,
			Value:     cm.Value,
			Tags:      cm.Tags,
			Conns:     cm.Conns,
		}
	}

	return info, nil
}

func (ns *NetworkSubmodule) NetPeers(context.Context) ([]peer.AddrInfo, error) {
	conns := ns.Host.Network().Conns()
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

func (ns *NetworkSubmodule) NetSwarmPeers(ctx context.Context, verbose, latency, streams bool) (*api.SwarmConnInfos, error) {
	conns := ns.Host.Network().Conns()

	out := api.SwarmConnInfos{
		Peers: []api.SwarmConnInfo{},
	}
	for _, c := range conns {
		pid := c.RemotePeer()
		addr := c.RemoteMultiaddr()

		ci := api.SwarmConnInfo{
			Addr: addr.String(),
			Peer: pid.Pretty(),
		}

		if verbose || latency {
			lat := ns.Host.Peerstore().LatencyEWMA(pid)
			if lat == 0 {
				ci.Latency = "n/a"
			} else {
				ci.Latency = lat.String()
			}
		}
		if verbose || streams {
			strs := c.GetStreams()

			for _, s := range strs {
				ci.Streams = append(ci.Streams, api.SwarmStreamInfo{Protocol: string(s.Protocol())})
			}
		}
		sort.Sort(&ci)
		out.Peers = append(out.Peers, ci)
	}

	sort.Sort(&out)
	return &out, nil
}

// stats
func (ns *NetworkSubmodule) NetBandwidthStats(ctx context.Context) (metrics.Stats, error) {
	return ns.Reporter.GetBandwidthTotals(), nil
}

func (ns *NetworkSubmodule) NetBandwidthStatsByPeer(ctx context.Context) (map[string]metrics.Stats, error) {
	out := make(map[string]metrics.Stats)
	for p, s := range ns.Reporter.GetBandwidthByPeer() {
		out[p.String()] = s
	}
	return out, nil
}

func (ns *NetworkSubmodule) NetBandwidthStatsByProtocol(ctx context.Context) (map[protocol.ID]metrics.Stats, error) {
	return ns.Reporter.GetBandwidthByProtocol(), nil
}

func (ns *NetworkSubmodule) NetAutoNatStatus(ctx context.Context) (i api.NatInfo, err error) {
	autonat := ns.RawHost.(*basichost.BasicHost).GetAutoNat()

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
