package build

import (
	"fmt"

	"github.com/libp2p/go-libp2p-core/protocol"

	"github.com/memoio/go-mefs-v2/lib/types"
)

// broadcast an event
func EventTopic(netName string) string { return "/memo/event/" + string(netName) }

func MsgTopic(netName string) string   { return "/memo/msg/" + string(netName) }
func BlockTopic(netName string) string { return "/memo/block/" + string(netName) }
func HSMsgTopic(netName string) string { return "/memo/hotstuff/" + string(netName) }

// MemoriaeDHT is creates a protocol for the memoriae DHT.
func MemoriaeDHT(netName string) protocol.ID {
	return protocol.ID(fmt.Sprintf("/memo/dht/%s", netName))
}

func MemoriaeNet(netName string) protocol.ID {
	return protocol.ID(fmt.Sprintf("/memo/net/%s", netName))
}

func GenesisBlockID(netName string) types.MsgID {
	return types.NewMsgID([]byte(fmt.Sprintf("/memo/genesis/%s", netName)))
}
