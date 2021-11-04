package generic

import (
	"context"
	"fmt"

	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"

	ma "github.com/multiformats/go-multiaddr"
)

// subscriberNotifee implements network.Notifee and also manages the subscriber to the event bus. We consume peer
// identification events to trigger inclusion in the routing table, and we consume Disconnected events to eject peers
// from it.
type subscriberNotifee struct {
	service *GenericService
}

func newSubscriberNotifiee(service *GenericService) (*subscriberNotifee, error) {
	nn := &subscriberNotifee{
		service: service,
	}

	return nn, nil
}

func (nn *subscriberNotifee) Connected(n network.Network, v network.Conn) {
	service := nn.service

	p := v.RemotePeer()

	// Lock and check to see if we're still connected. We lock to make sure
	// we don't concurrently process a connect event.
	service.plk.Lock()
	defer service.plk.Unlock()
	if service.Host().Network().Connectedness(p) == network.NotConnected {
		// We're not connected.
		return
	}

	fmt.Println("local and remote: ", v.LocalPeer().Pretty(), p.Pretty())
}

type disconnector interface {
	OnDisconnect(ctx context.Context, p peer.ID)
}

func (nn *subscriberNotifee) Disconnected(n network.Network, v network.Conn) {
	service := nn.service

	ms, ok := service.msgSender.(disconnector)
	if !ok {
		return
	}

	select {
	case <-service.Process().Closing():
		return
	default:
	}

	p := v.RemotePeer()

	// Lock and check to see if we're still connected. We lock to make sure
	// we don't concurrently process a connect event.
	service.plk.Lock()
	defer service.plk.Unlock()
	if service.ns.Host.Network().Connectedness(p) == network.Connected {
		// We're still connected.
		return
	}

	ms.OnDisconnect(service.Context(), p)
}

func (nn *subscriberNotifee) OpenedStream(network.Network, network.Stream) {}
func (nn *subscriberNotifee) ClosedStream(network.Network, network.Stream) {}
func (nn *subscriberNotifee) Listen(network.Network, ma.Multiaddr)         {}
func (nn *subscriberNotifee) ListenClose(network.Network, ma.Multiaddr)    {}
