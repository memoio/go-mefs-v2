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
	Name string `json:"name"`
	// addresses for the swarm to listen on
	Addresses []string `json:"addresses"`

	EnableRelay bool

	PublicRelayAddress string `json:"public_relay_address,omitempty"`
}

func newDefaultSwarmConfig() SwarmConfig {
	return SwarmConfig{
		Name: "devnet",
		Addresses: []string{
			"/ip4/0.0.0.0/tcp/7001",
			"/ip6/::/tcp/7001",
			"/ip4/0.0.0.0/udp/7001/quic",
			"/ip6/::/udp/7001/quic",
		},
	}
}
