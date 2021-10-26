package generic_service

import (
	"context"
	"sync"

	"github.com/jbenet/goprocess"

	goprocessctx "github.com/jbenet/goprocess/context"
	host "github.com/libp2p/go-libp2p-core/host"
	peer "github.com/libp2p/go-libp2p-core/peer"
	protocol "github.com/libp2p/go-libp2p-core/protocol"

	"github.com/memoio/go-mefs-v2/lib/log"
	"github.com/memoio/go-mefs-v2/service/core/generic/internal/net"
	"github.com/memoio/go-mefs-v2/service/core/instance"
	"github.com/memoio/go-mefs-v2/submodule/network"
)

var logger = log.Logger("generic_service")

const DefaultPrefix protocol.ID = "/memo"

type GenericService struct {
	ns *network.NetworkAPI

	ctx  context.Context
	proc goprocess.Process

	msgSender net.MessageSender

	// DHT protocols we query with. We'll only add peers to our routing
	// table if they speak these protocols.
	protocols     []protocol.ID
	protocolsStrs []string

	// DHT protocols we can respond to.
	serverProtocols []protocol.ID

	plk sync.Mutex

	sub instance.Subscriber
}

func New(ctx context.Context, ns *network.NetworkAPI, s instance.Subscriber) (*GenericService, error) {
	var protocols, serverProtocols []protocol.ID

	v1proto := DefaultPrefix + protocol.ID("/core/"+ns.NetworkName)

	protocols = []protocol.ID{v1proto}
	serverProtocols = []protocol.ID{v1proto}

	service := &GenericService{
		protocols:       protocols,
		protocolsStrs:   protocol.ConvertToStrings(protocols),
		serverProtocols: serverProtocols,
		ns:              ns,
		sub:             s,
	}

	// create a DHT proc with the given context
	service.proc = goprocessctx.WithContextAndTeardown(ctx, func() error {
		return nil
	})

	// the DHT context should be done when the process is closed
	service.ctx = goprocessctx.WithProcessClosing(ctx, service.proc)

	service.msgSender = net.NewMessageSenderImpl(ns.Host, service.protocols)

	for _, p := range service.serverProtocols {
		ns.Host.SetStreamHandler(p, service.handleNewStream)
	}

	// register for event bus and network notifications
	sn, err := newSubscriberNotifiee(service)
	if err != nil {
		return nil, err
	}

	// register for network notifications
	ns.Host.Network().Notify(sn)

	logger.Info("start generic service")

	return service, nil
}

// Context returns the DHT's context.
func (service *GenericService) Context() context.Context {
	return service.ctx
}

// Process returns the DHT's process.
func (service *GenericService) Process() goprocess.Process {
	return service.proc
}

// PeerID returns the DHT node's Peer ID.
func (service *GenericService) PeerID() peer.ID {
	return service.ns.Host.ID()
}

// Host returns the libp2p host this DHT is operating with.
func (service *GenericService) Host() host.Host {
	return service.ns.Host
}

func (service *GenericService) Close() error {
	return service.proc.Close()
}
