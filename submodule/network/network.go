package network

import (
	"context"
	"fmt"
	"time"

	"github.com/pkg/errors"

	"github.com/libp2p/go-libp2p"
	host "github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/metrics"
	"github.com/libp2p/go-libp2p-core/routing"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/p2p/discovery"
	routed "github.com/libp2p/go-libp2p/p2p/host/routed"

	config "github.com/memoio/go-mefs-v2/config"
	logging "github.com/memoio/go-mefs-v2/lib/log"
	"github.com/memoio/go-mefs-v2/lib/net"
	"github.com/memoio/go-mefs-v2/lib/types/store"
	"github.com/memoio/go-mefs-v2/lib/utils/storeutil"
)

var networkLogger = logging.Logger("network_module")

// NetworkSubmodule enhances the `Node` with networking capabilities.
type NetworkSubmodule struct { //nolint
	NetworkName string

	RawHost host.Host
	Host    host.Host

	// Router is a router from IPFS
	Router routing.Routing

	Pubsub *pubsub.PubSub

	Network *net.Network

	PeerMgr net.IPeerMgr

	Discovery discovery.Service `optional:"true"`

	ShutdownChan chan struct{}
}

func (networkSubmodule *NetworkSubmodule) API() *networkAPI {
	return &networkAPI{networkSubmodule}
}

func (networkSubmodule *NetworkSubmodule) Stop(ctx context.Context) {
	if err := networkSubmodule.Host.Close(); err != nil {
		fmt.Printf("error closing host: %s\n", err)
	}
	if err := networkSubmodule.Discovery.Close(); err != nil {
		fmt.Printf("error closing Discovery: %s\n", err)
	}
	if err := networkSubmodule.PeerMgr.Stop(ctx); err != nil {
		fmt.Printf("error closing PeerMgr: %s\n", err)
	}
}

type networkConfig interface {
	OfflineMode() bool
	Libp2pOpts() []libp2p.Option
}

type blankValidator struct{}

func (blankValidator) Validate(_ string, _ []byte) error        { return nil }
func (blankValidator) Select(_ string, _ [][]byte) (int, error) { return 0, nil }

// NewNetworkSubmodule creates a new network submodule.
func NewNetworkSubmodule(ctx context.Context, config networkConfig, cfg *config.Config, ds store.KVStore) (*NetworkSubmodule, error) {
	bandwidthTracker := metrics.NewBandwidthCounter()

	libP2pOpts := append(config.Libp2pOpts(), Transport())
	libP2pOpts = append(libP2pOpts, libp2p.BandwidthReporter(bandwidthTracker))
	libP2pOpts = append(libP2pOpts, libp2p.EnableNATService())
	libP2pOpts = append(libP2pOpts, libp2p.NATPortMap())
	libP2pOpts = append(libP2pOpts, Peerstore())
	libP2pOpts = append(libP2pOpts, makeSmuxTransportOption())
	libP2pOpts = append(libP2pOpts, Security(true, false))

	// peer manager
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
	networkName := cfg.Net.Name
	validator := blankValidator{}
	bootNodes, err := net.ParseAddresses(ctx, cfg.Bootstrap.Addresses)
	if err != nil {
		return nil, err
	}

	dhtopts := []dht.Option{dht.Mode(dht.ModeAutoServer),
		dht.Datastore(nds),
		dht.NamespacedValidator("v", validator),
		dht.ProtocolPrefix(net.MemoriaeDHT(networkName)),
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

	peerMgr, err := net.NewPeerMgr(peerHost, router, bootNodes)
	if err != nil {
		return nil, err
	}

	// do NOT start `peerMgr` in `offline` mode
	if !config.OfflineMode() {
		go peerMgr.Run(ctx)
	}

	// Set up pubsub
	pubsub.GossipSubHeartbeatInterval = 100 * time.Millisecond
	options := []pubsub.Option{
		// Gossipsubv1.1 configuration
		pubsub.WithFloodPublish(true),
	}

	gsub, err := pubsub.NewGossipSub(ctx, peerHost, options...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to set up network")
	}

	mdnsdisc, err := SetupDiscovery(10, ctx, peerHost, DiscoveryHandler(ctx, peerHost))
	if err != nil {
		networkLogger.Error("Setup Discovery falied, error:", err)
	}

	// build network
	network := net.New(peerHost, router, bandwidthTracker)

	// build the network submdule
	return &NetworkSubmodule{
		NetworkName: networkName,
		RawHost:     rawHost,
		Host:        peerHost,
		Router:      router,
		Pubsub:      gsub,
		Network:     network,
		PeerMgr:     peerMgr,
		Discovery:   mdnsdisc,
	}, nil
}
