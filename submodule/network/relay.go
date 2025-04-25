package network

import (
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/client"
	ma "github.com/multiformats/go-multiaddr"
)

func (ns *NetworkSubmodule) startRelay(sa ma.Multiaddr) {
	sinfo, err := peer.AddrInfoFromP2pAddr(sa)
	if err != nil {
		return
	}

	_, err = client.Reserve(ns.ctx, ns.RawHost, *sinfo)
	if err != nil {
		logger.Warn("reserve error", err)
	}

	for {
		timer := time.NewTimer(10 * time.Minute)
		select {
		case <-timer.C:
			_, err := client.Reserve(ns.ctx, ns.RawHost, *sinfo)
			if err != nil {
				logger.Warn("reserve error", err)
			}
		case <-ns.ctx.Done():
			return
		}
	}
}
