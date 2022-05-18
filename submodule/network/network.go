package network

import (
	"context"
	"sort"
	"strings"
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
	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/api"
	"github.com/memoio/go-mefs-v2/build"
	"github.com/memoio/go-mefs-v2/config"
	"github.com/memoio/go-mefs-v2/lib/repo"
	"github.com/memoio/go-mefs-v2/lib/utils/net"
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
	Repo() repo.Repo
}

type blankValidator struct{}

func (blankValidator) Validate(_ string, _ []byte) error        { return nil }
func (blankValidator) Select(_ string, _ [][]byte) (int, error) { return 0, nil }

// NewNetworkSubmodule creates a new network submodule.
func NewNetworkSubmodule(ctx context.Context, nconfig networkConfig, networkName string) (*NetworkSubmodule, error) {
	bandwidthTracker := metrics.NewBandwidthCounter()

	libP2pOpts := append(nconfig.Libp2pOpts(), Transport())
	libP2pOpts = append(libP2pOpts, libp2p.BandwidthReporter(bandwidthTracker))
	libP2pOpts = append(libP2pOpts, libp2p.EnableNATService())
	libP2pOpts = append(libP2pOpts, libp2p.NATPortMap())
	libP2pOpts = append(libP2pOpts, Peerstore())
	libP2pOpts = append(libP2pOpts, makeSmuxTransportOption())
	libP2pOpts = append(libP2pOpts, Security(true, false))

	// set up host
	rawHost, err := libp2p.New(
		ctx,
		libp2p.UserAgent("memoriae-"+build.UserVersion()),
		libp2p.ChainOptions(libP2pOpts...),
		libp2p.Ping(true),
	)
	if err != nil {
		return nil, err
	}

	// setup dht
	validator := blankValidator{}
	bootNodes, err := net.ParseAddresses(nconfig.Repo().Config().Bootstrap.Addresses)
	if err != nil {
		return nil, err
	}

	cbootNodes, err := net.ParseAddresses(config.DefaultBootstrapConfig.Addresses)
	if err != nil {
		return nil, err
	}

	for _, cbn := range cbootNodes {
		has := false
		for _, bn := range bootNodes {
			if bn.ID == cbn.ID {
				has = true
				break
			}
		}

		if !has {
			bootNodes = append(bootNodes, cbn)
		}
	}

	dhtopts := []dht.Option{dht.Mode(dht.ModeAutoServer),
		dht.Datastore(nconfig.Repo().DhtStore()),
		dht.Validator(validator),
		dht.ProtocolPrefix(build.MemoriaeDHT(networkName)),
		// uncomment these in mainnet
		//dht.QueryFilter(dht.PublicQueryFilter),
		//dht.RoutingTableFilter(dht.PublicRoutingTableFilter),
		dht.BootstrapPeers(bootNodes...),
		dht.DisableProviders(),
		dht.DisableValues(),
	}

	router, err := dht.New(ctx, rawHost, dhtopts...)
	if err != nil {
		return nil, xerrors.Errorf("failed to setup routing %w", err)
	}

	peerHost := routed.Wrap(rawHost, router)

	peerMgr, err := NewPeerMgr(networkName, peerHost, router, bootNodes)
	if err != nil {
		return nil, err
	}

	go peerMgr.Run(ctx)

	mdnsdisc, err := SetupDiscovery(ctx, peerHost, DiscoveryHandler(ctx, peerHost))
	if err != nil {
		logger.Error("Setup Discovery falied:", err)
		return nil, err
	}

	// Set up pubsub
	topicdisc, err := TopicDiscovery(ctx, peerHost, router)
	if err != nil {
		return nil, err
	}

	allowTopics := []string{
		build.MsgTopic(networkName),
		build.BlockTopic(networkName),
		build.HSMsgTopic(networkName),
		build.EventTopic(networkName),
	}

	pubsub.GossipSubHeartbeatInterval = 100 * time.Millisecond
	options := []pubsub.Option{
		// Gossipsubv1.1 configuration
		pubsub.WithFloodPublish(true),
		pubsub.WithMessageIdFn(HashMsgId),
		pubsub.WithDiscovery(topicdisc),
		// public bootstrap node
		pubsub.WithDirectPeers(bootNodes),
		// set allow topics
		pubsub.WithSubscriptionFilter(
			pubsub.WrapLimitSubscriptionFilter(
				pubsub.NewAllowlistSubscriptionFilter(allowTopics...),
				100)),
	}

	gsub, err := pubsub.NewGossipSub(ctx, peerHost, options...)
	if err != nil {
		return nil, xerrors.Errorf("failed to set up gossip %w", err)
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
	err := ns.Host.Close()
	if err != nil {
		logger.Errorf("error closing host: %s", err)
	}
	err = ns.Discovery.Close()
	if err != nil {
		logger.Errorf("error closing Discovery: %s", err)
	}
	err = ns.PeerMgr.Stop(ctx)
	if err != nil {
		logger.Errorf("error closing PeerMgr: %s", err)
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
			return err
		}
		pai = npi
	}

	swrm, ok := ns.Host.Network().(*swarm.Swarm)
	if !ok {
		return xerrors.Errorf("peerhost network was not a swarm")
	}

	swrm.Backoff().Clear(pai.ID)
	err := ns.Host.Connect(ctx, pai)
	if err != nil {
		return err
	}

	protos, err := ns.Host.Peerstore().GetProtocols(pai.ID)
	if err != nil {
		return err
	}

	for _, pro := range protos {
		if strings.Contains(pro, ns.NetworkName) {
			return nil
		}
	}

	return ns.Host.Network().ClosePeer(pai.ID)
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
		return nil, xerrors.New("underlying routing should be pointer of IpfsDHT")
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
	out := make([]peer.AddrInfo, 0, len(conns))

	for _, conn := range conns {
		id := conn.RemotePeer()
		protos, err := ns.Host.Peerstore().GetProtocols(id)
		if err != nil {
			continue
		}

		has := false
		for _, pro := range protos {
			if strings.Contains(pro, ns.NetworkName) {
				has = true
				break
			}
		}

		if !has {
			ns.Host.Network().ClosePeer(id)
			continue
		}

		out = append(out, peer.AddrInfo{
			ID: id,
			Addrs: []ma.Multiaddr{
				conn.RemoteMultiaddr(),
			},
		})
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
