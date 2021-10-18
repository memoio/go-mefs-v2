package network

import (
	"context"
	"time"

	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
)

const discoveryConnTimeout = time.Second * 30

type discoveryHandler struct {
	ctx  context.Context
	host host.Host
}

func (dh *discoveryHandler) HandlePeerFound(p peer.AddrInfo) {
	log.Info("connecting to discovered peer: ", p)
	ctx, cancel := context.WithTimeout(dh.ctx, discoveryConnTimeout)
	defer cancel()
	if err := dh.host.Connect(ctx, p); err != nil {
		log.Warnf("failed to connect to peer %s found by discovery: %s", p.ID, err)
	}
}

func DiscoveryHandler(ctx context.Context, host host.Host) *discoveryHandler {
	return &discoveryHandler{
		ctx:  ctx,
		host: host,
	}
}

func SetupDiscovery(host host.Host, handler *discoveryHandler) mdns.Service {
	service := mdns.NewMdnsService(host, mdns.ServiceName)
	service.RegisterNotifee(handler)
	return service
}
