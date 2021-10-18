package config

import "time"

// DefaultConnMgrHighWater is the default value for the connection managers
// 'high water' mark
const DefaultConnMgrHighWater = 900

// DefaultConnMgrLowWater is the default value for the connection managers 'low
// water' mark
const DefaultConnMgrLowWater = 600

// DefaultConnMgrGracePeriod is the default value for the connection managers
// grace period
const DefaultConnMgrGracePeriod = time.Second * 20

type SwarmConfig struct {
	// addresses for the swarm to listen on
	Addresses []string `json:"addresses"`

	// AddrFilters specifies a set libp2p addresses that we should never
	// dial or receive connections from.
	AddrFilters []string `json:"addrFilters"`

	// DisableBandwidthMetrics disables recording of bandwidth metrics for a
	// slight reduction in memory usage. You probably don't need to set this
	// flag.
	DisableBandwidthMetrics bool `json:"disableBandwidthMetrics"`

	// DisableNatPortMap turns off NAT port mapping (UPnP, etc.).
	DisableNatPortMap bool `json:"disableNatPortmap"`

	// DisableRelay explicitly disables the relay transport.
	//
	// Deprecated: This flag is deprecated and is overridden by
	// `Transports.Relay` if specified.
	DisableRelay bool `json:",omitempty"`

	// EnableRelayHop makes this node act as a public relay, relaying
	// traffic between other nodes.
	EnableRelayHop bool `json:"enableRelayHop"`

	// EnableAutoRelay enables the "auto relay" feature.
	//
	// When both EnableAutoRelay and EnableRelayHop are set, this go-ipfs node
	// will advertise itself as a public relay. Otherwise it will find and use
	// advertised public relays when it determines that it's not reachable
	// from the public internet.
	EnableAutoRelay bool `json:"enableAutoRelay"`

	// Transports contains flags to enable/disable libp2p transports.
	Transports Transports `json:"transports"`

	// ConnMgr configures the connection manager.
	ConnMgr ConnMgr `json:"connMgr"`

	PublicRelayAddress string `json:"public_relay_address,omitempty"`
}

type Transports struct {
	// Network specifies the base transports we'll use for dialing. To
	// listen on a transport, add the transport to your Addresses.Swarm.
	Network struct {
		// All default to on.
		QUIC      Flag `json:",omitempty"`
		TCP       Flag `json:",omitempty"`
		Websocket Flag `json:",omitempty"`
		Relay     Flag `json:",omitempty"`
	}

	// Security specifies the transports used to encrypt insecure network
	// transports.
	Security struct {
		// Defaults to 100.
		TLS Priority `json:",omitempty"`
		// Defaults to 200.
		SECIO Priority `json:",omitempty"`
		// Defaults to 300.
		Noise Priority `json:",omitempty"`
	}

	// Multiplexers specifies the transports used to multiplex multiple
	// connections over a single duplex connection.
	Multiplexers struct {
		// Defaults to 100.
		Yamux Priority `json:",omitempty"`
		// Defaults to 200.
		Mplex Priority `json:",omitempty"`
	}
}

// ConnMgr defines configuration options for the libp2p connection manager
type ConnMgr struct {
	Type        string
	LowWater    int
	HighWater   int
	GracePeriod string
}

func newDefaultSwarmConfig() SwarmConfig {
	return SwarmConfig{
		Addresses: []string{
			"/ip4/0.0.0.0/tcp/7001",
			"/ip6/::/tcp/7001",
			"/ip4/0.0.0.0/udp/7001/quic",
			"/ip6/::/udp/7001/quic",
		},
		ConnMgr: ConnMgr{
			LowWater:    DefaultConnMgrLowWater,
			HighWater:   DefaultConnMgrHighWater,
			GracePeriod: DefaultConnMgrGracePeriod.String(),
			Type:        "basic",
		},
	}
}
