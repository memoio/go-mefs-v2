package api

import (
	"time"

	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
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
