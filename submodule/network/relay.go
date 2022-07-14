package network

import (
	"log"
	"time"

	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/client"
	ma "github.com/multiformats/go-multiaddr"
)

func (ns *NetworkSubmodule) startRelay(sa ma.Multiaddr) {
	sinfo, err := peer.AddrInfoFromP2pAddr(sa)
	if err != nil {
		return
	}

	for {
		rsvp, err := client.Reserve(ns.ctx, ns.RawHost, *sinfo)
		if err != nil {
			logger.Warn("reserve error", err)
			return
		}
		if rsvp.Voucher == nil {
			log.Println("no reservation voucher")
		}

		logger.Info("got relay addr: ", rsvp.Addrs)

		timer := time.NewTimer(50 * time.Minute)
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
