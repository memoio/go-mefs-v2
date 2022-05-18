package network

import (
	"context"
	"time"

	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p/p2p/discovery"
)

type discoveryHandler struct {
	ctx  context.Context
	host host.Host
}

func (dh *discoveryHandler) HandlePeerFound(p peer.AddrInfo) {
	if dh.host.Network().Connectedness(p.ID) == network.Connected {
		return
	}

	//logger.Debug("connecting to discovered peer: ", p)
	ctx, cancel := context.WithTimeout(dh.ctx, 5*time.Second)
	defer cancel()

	err := dh.host.Connect(ctx, p)
	if err != nil {
		//logger.Debugf("failed to connect to peer %s found by discovery: %s", p.ID, err)
	}
}

func DiscoveryHandler(ctx context.Context, host host.Host) *discoveryHandler {
	return &discoveryHandler{
		ctx:  ctx,
		host: host,
	}
}

func SetupDiscovery(ctx context.Context, host host.Host, handler *discoveryHandler) (discovery.Service, error) {
	service, err := discovery.NewMdnsService(ctx, host, 60*time.Second, "mefs-discovery")
	if err != nil {
		return service, err
	}
	service.RegisterNotifee(handler)
	return service, nil
}
