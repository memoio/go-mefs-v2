package network

import (
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-peerstore/pstoremem"
)

func Peerstore() libp2p.Option {
	return libp2p.Peerstore(pstoremem.NewPeerstore())
}
