package generic_service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/jbenet/goprocess"

	goprocessctx "github.com/jbenet/goprocess/context"
	host "github.com/libp2p/go-libp2p-core/host"
	peer "github.com/libp2p/go-libp2p-core/peer"
	protocol "github.com/libp2p/go-libp2p-core/protocol"

	"github.com/memoio/go-mefs-v2/lib/log"
	"github.com/memoio/go-mefs-v2/service/core/generic/internal/net"
	"github.com/memoio/go-mefs-v2/service/core/instance"
)

var (
	logger = log.Logger("generic")
)

type mode int

const (
	modeServer mode = iota + 1
	modeClient
)

const DefaultPrefix protocol.ID = "/memo"

const (
	version1 protocol.ID = "/1.0.0"
)

type GenericService struct {
	host host.Host // the network services we need
	self peer.ID   // Local peer (yourself)

	birth time.Time // When this peer started up

	ctx  context.Context
	proc goprocess.Process

	msgSender net.MessageSender

	plk sync.Mutex

	// DHT protocols we query with. We'll only add peers to our routing
	// table if they speak these protocols.
	protocols     []protocol.ID
	protocolsStrs []string

	// DHT protocols we can respond to.
	serverProtocols []protocol.ID

	mode mode

	sub instance.Subscriber
}

func New(ctx context.Context, h host.Host, s instance.Subscriber) (*GenericService, error) {
	service, err := makeService(ctx, h, s)
	if err != nil {
		return nil, fmt.Errorf("failed to create Service, err=%s", err)
	}

	service.msgSender = net.NewMessageSenderImpl(h, service.protocols)

	service.mode = modeServer
	for _, p := range service.serverProtocols {
		service.host.SetStreamHandler(p, service.handleNewStream)
	}

	// register for event bus and network notifications
	sn, err := newSubscriberNotifiee(service)
	if err != nil {
		return nil, err
	}

	// register for network notifications
	service.host.Network().Notify(sn)

	return service, nil
}

func makeService(ctx context.Context, h host.Host, sub instance.Subscriber) (*GenericService, error) {
	var protocols, serverProtocols []protocol.ID

	v1proto := DefaultPrefix + version1

	protocols = []protocol.ID{v1proto}
	serverProtocols = []protocol.ID{v1proto}

	service := &GenericService{
		self:            h.ID(),
		host:            h,
		birth:           time.Now(),
		protocols:       protocols,
		protocolsStrs:   protocol.ConvertToStrings(protocols),
		serverProtocols: serverProtocols,
		sub:             sub,
	}

	// create a DHT proc with the given context
	service.proc = goprocessctx.WithContextAndTeardown(ctx, func() error {
		return nil
	})

	// the DHT context should be done when the process is closed
	service.ctx = goprocessctx.WithProcessClosing(ctx, service.proc)

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
	return service.self
}

// Host returns the libp2p host this DHT is operating with.
func (service *GenericService) Host() host.Host {
	return service.host
}

func (service *GenericService) Close() error {
	return service.proc.Close()
}
