package net

import (
	"context"
	"fmt"
	"sort"

	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/metrics"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/routing"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	swarm "github.com/libp2p/go-libp2p-swarm"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/pkg/errors"
)

// Network is a unified interface for dealing with libp2p
type Network struct {
	metrics.Reporter
	host host.Host
	dht  routing.Routing
}

// New returns a new Network
func New(
	host host.Host,
	router routing.Routing,
	reporter metrics.Reporter,
) *Network {
	return &Network{
		Reporter: reporter,
		host:     host,
		dht:      router,
	}
}

// GetPeerAddresses gets the current addresses of the node
func (network *Network) GetPeerAddresses() []ma.Multiaddr {
	return network.host.Addrs()
}

// GetPeerID gets the current peer id from libp2p-host
func (network *Network) GetPeerID() peer.ID {
	return network.host.ID()
}

// GetBandwidthStats gets stats on the current bandwidth usage of the network
func (network *Network) GetBandwidthStats() metrics.Stats {
	return network.Reporter.GetBandwidthTotals()
}

// ConnectionResult represents the result of an attempted connection from the
// Connect method.
type ConnectionResult struct {
	PeerID peer.ID
	Err    string
}

// Connect connects to peers at the given addresses. Does not retry.
func (network *Network) Connect(ctx context.Context, pinfo peer.AddrInfo) error {
	swrm, ok := network.host.Network().(*swarm.Swarm)
	if !ok {
		return fmt.Errorf("peerhost network was not a swarm")
	}

	swrm.Backoff().Clear(pinfo.ID)
	return network.host.Connect(ctx, pinfo)
}

// SwarmConnInfo represents details about a single swarm connection.
type SwarmConnInfo struct {
	Addr    string
	Peer    string
	Latency string
	Muxer   string
	Streams []SwarmStreamInfo
}

// SwarmStreamInfo represents details about a single swarm stream.
type SwarmStreamInfo struct {
	Protocol string
}

func (ci *SwarmConnInfo) Less(i, j int) bool {
	return ci.Streams[i].Protocol < ci.Streams[j].Protocol
}

func (ci *SwarmConnInfo) Len() int {
	return len(ci.Streams)
}

func (ci *SwarmConnInfo) Swap(i, j int) {
	ci.Streams[i], ci.Streams[j] = ci.Streams[j], ci.Streams[i]
}

// SwarmConnInfos represent details about a list of swarm connections.
type SwarmConnInfos struct {
	Peers []SwarmConnInfo
}

func (ci SwarmConnInfos) Less(i, j int) bool {
	return ci.Peers[i].Addr < ci.Peers[j].Addr
}

func (ci SwarmConnInfos) Len() int {
	return len(ci.Peers)
}

func (ci SwarmConnInfos) Swap(i, j int) {
	ci.Peers[i], ci.Peers[j] = ci.Peers[j], ci.Peers[i]
}

// Peers lists peers currently available on the network
func (network *Network) Peers(ctx context.Context, verbose, latency, streams bool) (*SwarmConnInfos, error) {
	if network.host == nil {
		return nil, errors.New("node must be online")
	}

	conns := network.host.Network().Conns()

	out := SwarmConnInfos{
		Peers: []SwarmConnInfo{},
	}
	for _, c := range conns {
		pid := c.RemotePeer()
		addr := c.RemoteMultiaddr()

		ci := SwarmConnInfo{
			Addr: addr.String(),
			Peer: pid.Pretty(),
		}

		if verbose || latency {
			lat := network.host.Peerstore().LatencyEWMA(pid)
			if lat == 0 {
				ci.Latency = "n/a"
			} else {
				ci.Latency = lat.String()
			}
		}
		if verbose || streams {
			strs := c.GetStreams()

			for _, s := range strs {
				ci.Streams = append(ci.Streams, SwarmStreamInfo{Protocol: string(s.Protocol())})
			}
		}
		sort.Sort(&ci)
		out.Peers = append(out.Peers, ci)
	}

	sort.Sort(&out)
	return &out, nil
}

// FindPeer searches the libp2p router for a given peer id
func (network *Network) FindPeer(ctx context.Context, peerID peer.ID) (peer.AddrInfo, error) {
	return network.dht.FindPeer(ctx, peerID)
}

// GetClosestPeers returns a channel of the K closest peers  to the given key,
// K is the 'K Bucket' parameter of the Kademlia DHT protocol.
func (network *Network) GetClosestPeers(ctx context.Context, key string) ([]peer.ID, error) {
	ipfsDHT, ok := network.dht.(*dht.IpfsDHT)
	if !ok {
		return nil, errors.New("underlying routing should be pointer of IpfsDHT")
	}
	return ipfsDHT.GetClosestPeers(ctx, key)
}
