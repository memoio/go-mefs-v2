package api

import (
	"math/big"
	"time"

	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/memoio/go-mefs-v2/lib/types"
)

type GroupInfo struct {
	EndPoint string
	RoleAddr string
	ID       uint64
	Level    uint16
	Size     uint64
	Price    *big.Int
	KCount   uint64
	PCount   uint64
	UCount   uint64
	FsAddr   string
}

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

type BalanceInfo struct {
	Value    *big.Int
	FsValue  *big.Int
	ErcValue *big.Int // should be map?
}

type PledgeInfo struct {
	Value    *big.Int
	ErcTotal *big.Int
	Total    *big.Int
}

type StoreInfo struct {
	Time     int64
	Nonce    uint64
	SubNonce uint64
	Size     uint64
	Price    *big.Int
}

type OrderJobInfo struct {
	ID uint64

	PeerID string

	AvailTime int64

	Nonce      uint64
	OrderTime  int64
	OrderState string

	SeqNum   uint32
	SeqTime  int64
	SeqState string

	Jobs int

	Ready  bool
	InStop bool
}

type SyncInfo struct {
	Status       bool
	SyncedHeight uint64
	RemoteHeight uint64
}

type StateInfo struct {
	Version uint32
	Height  uint64      // block next height
	Slot    uint64      // distance from basetime
	Epoch   uint64      // challenge
	Root    types.MsgID // state root
	BlockID types.MsgID
}
