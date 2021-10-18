package network

import (
	"context"
	"fmt"
	"runtime"
	"time"

	"github.com/pkg/errors"

	"github.com/libp2p/go-libp2p"
	host "github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/metrics"
	"github.com/libp2p/go-libp2p-core/routing"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
	routed "github.com/libp2p/go-libp2p/p2p/host/routed"

	config "github.com/memoio/go-mefs-v2/config"
	logging "github.com/memoio/go-mefs-v2/lib/log"
	"github.com/memoio/go-mefs-v2/lib/net"
	"github.com/memoio/go-mefs-v2/lib/repo"
)

var networkLogger = logging.Logger("network_module")

// NetworkSubmodule enhances the `Node` with networking capabilities.
type NetworkSubmodule struct { //nolint
	NetworkName string

	RawHost RawHost
	Host    host.Host

	// Router is a router from IPFS
	Router routing.Routing

	Pubsub *pubsub.PubSub

	Network *net.Network

	PeerMgr net.IPeerMgr

	Discovery mdns.Service `optional:"true"`

	ShutdownChan chan struct{}
}

func (networkSubmodule *NetworkSubmodule) API() *NetworkAPI {
	return &NetworkAPI{networkSubmodule}
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
	IsRelay() bool
	Libp2pOpts() []libp2p.Option
}

type networkRepo interface {
	Config() *config.Config
	DhtDatastore() repo.Datastore
	Path() (string, error)
}

type blankValidator struct{}

func (blankValidator) Validate(_ string, _ []byte) error        { return nil }
func (blankValidator) Select(_ string, _ [][]byte) (int, error) { return 0, nil }

// NewNetworkSubmodule creates a new network submodule.
func NewNetworkSubmodule(ctx context.Context, config networkConfig, rep networkRepo) (*NetworkSubmodule, error) {
	cfg := rep.Config()
	var bandwidthTracker metrics.Reporter
	var err error

	libP2pOpts := append(config.Libp2pOpts(), libp2p.BandwidthReporter(bandwidthTracker))
	libP2pOpts = Transports(cfg.Swarm.Transports, libP2pOpts)
	libP2pOpts = Security(cfg.Swarm.Transports, libP2pOpts)
	libP2pOpts = append(libP2pOpts, libp2p.EnableNATService())
	libP2pOpts = append(libP2pOpts, libp2p.NATPortMap())
	libP2pOpts = append(libP2pOpts, Peerstore())
	libP2pOpts = append(libP2pOpts, makeSmuxTransportOption(cfg.Swarm.Transports))

	libP2pOpts, bandwidthTracker = BandwidthCounter(libP2pOpts)

	var networkName string
	if cfg.NetworkParams.DevNet {
		networkName = "testnet"
	} else {
		networkName = "devnet"
	}

	// peer manager
	bootNodes, err := net.ParseAddresses(ctx, cfg.Bootstrap.Addresses)
	if err != nil {
		return nil, err
	}

	// set up host
	var rawHost RawHost
	var peerHost host.Host
	var router routing.Routing
	var pubsubMessageSigning bool
	var peerMgr net.IPeerMgr

	validator := blankValidator{}

	makeDHT := func(h host.Host) (routing.Routing, error) {
		mode := dht.ModeAutoServer
		opts := []dht.Option{dht.Mode(mode),
			dht.Datastore(rep.DhtDatastore()),
			dht.NamespacedValidator("v", validator),
			dht.ProtocolPrefix(net.MemoriaeDHT(networkName)),
			// dht.QueryFilter(dht.PublicQueryFilter),
			// dht.RoutingTableFilter(dht.PublicRoutingTableFilter),
			dht.BootstrapPeers(bootNodes...),
			dht.DisableProviders(),
			dht.DisableValues(),
		}
		r, err := dht.New(
			ctx, h, opts...,
		)

		if err != nil {
			return nil, errors.Wrap(err, "failed to setup routing")
		}
		return r, err
	}

	rawHost, err = buildHost(ctx, config, libP2pOpts, rep, makeDHT)
	if err != nil {
		return nil, err
	}

	// Node must build a host acting as a libp2p relay.  Additionally it
	// runs the autoNAT service which allows other nodes to check for their
	// own dialability by having this node attempt to dial them.
	router, err = makeDHT(rawHost)
	if err != nil {
		return nil, err
	}

	peerHost = routed.Wrap(rawHost, router)

	// require message signing in online mode when we have priv key
	pubsubMessageSigning = true

	period, err := time.ParseDuration(cfg.Bootstrap.Period)
	if err != nil {
		return nil, err
	}

	peerMgr, err = net.NewPeerMgr(peerHost, router.(*dht.IpfsDHT), period, bootNodes)
	if err != nil {
		return nil, err
	}

	// do NOT start `peerMgr` in `offline` mode
	if !config.OfflineMode() {
		go peerMgr.Run(ctx)
	}

	// Set up libp2p network
	// The gossipsub heartbeat timeout needs to be set sufficiently low
	// to enable publishing on first connection.  The default of one
	// second is not acceptable for tests.
	pubsub.GossipSubHeartbeatInterval = 100 * time.Millisecond
	options := []pubsub.Option{
		// Gossipsubv1.1 configuration
		pubsub.WithFloodPublish(true),

		//  buffer, 32 -> 10K
		pubsub.WithValidateQueueSize(10 << 10),
		//  worker, 1x cpu -> 2x cpu
		pubsub.WithValidateWorkers(runtime.NumCPU() * 2),
		//  goroutine, 8K -> 16K
		pubsub.WithValidateThrottle(16 << 10),

		pubsub.WithMessageSigning(pubsubMessageSigning),
	}

	topicdisc, err := TopicDiscovery(ctx, peerHost, router)
	if err != nil {
		return nil, err
	}

	gsub, err := GossipSub(ctx, peerHost, topicdisc, options...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to set up network")
	}

	mdnsdisc := SetupDiscovery(peerHost, DiscoveryHandler(ctx, peerHost))

	// build network
	network := net.New(peerHost, net.NewRouter(router), bandwidthTracker)

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
