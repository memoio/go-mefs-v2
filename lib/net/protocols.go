package net

import (
	"fmt"

	"github.com/libp2p/go-libp2p-core/protocol"
)

// MemoriaeDHT is creates a protocol for the memoriae DHT.
func MemoriaeDHT(network string) protocol.ID {
	return protocol.ID(fmt.Sprintf("/memo/dht/%s", network))
}
