package generic

import (
	"context"
	"sync"

	"github.com/jbenet/goprocess"

	goprocessctx "github.com/jbenet/goprocess/context"
	host "github.com/libp2p/go-libp2p-core/host"
	peer "github.com/libp2p/go-libp2p-core/peer"
	protocol "github.com/libp2p/go-libp2p-core/protocol"

	"github.com/memoio/go-mefs-v2/build"
	logging "github.com/memoio/go-mefs-v2/lib/log"
	"github.com/memoio/go-mefs-v2/service/netapp/generic/internal"
	"github.com/memoio/go-mefs-v2/service/netapp/handler"
	"github.com/memoio/go-mefs-v2/submodule/network"
)

var logger = logging.Logger("net-generic")

type GenericService struct {
	handler.MsgHandle

	localID peer.ID
	ns      *network.NetworkSubmodule

	ctx  context.Context
	proc goprocess.Process

	msgSender internal.MessageSender

	// DHT protocols we query with. We'll only add peers to our routing
	// table if they speak these protocols.
	protocols     []protocol.ID
	protocolsStrs []string

	// DHT protocols we can respond to.
	serverProtocols []protocol.ID

	plk sync.Mutex
}

func New(ctx context.Context, ns *network.NetworkSubmodule) (*GenericService, error) {
	var protocols, serverProtocols []protocol.ID

	v1proto := build.MemoriaeNet(ns.NetworkName)

	protocols = []protocol.ID{v1proto}
	serverProtocols = []protocol.ID{v1proto}

	service := &GenericService{
		MsgHandle:       handler.NewMsgHandle(),
		localID:         ns.Host.ID(),
		protocols:       protocols,
		protocolsStrs:   protocol.ConvertToStrings(protocols),
		serverProtocols: serverProtocols,
		ns:              ns,
	}

	// create a DHT proc with the given context
	service.proc = goprocessctx.WithContextAndTeardown(ctx, func() error {
		return nil
	})

	// the DHT context should be done when the process is closed
	service.ctx = goprocessctx.WithProcessClosing(ctx, service.proc)

	service.msgSender = internal.NewMessageSenderImpl(ns.Host, service.protocols)

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

	logger.Info("Start generic service")

	return service, nil
}

// Context returns the DHT's context.
func (gs *GenericService) Context() context.Context {
	return gs.ctx
}

// Process returns the DHT's process.
func (gs *GenericService) Process() goprocess.Process {
	return gs.proc
}

// PeerID returns the DHT node's Peer ID.
func (gs *GenericService) PeerID() peer.ID {
	return gs.ns.Host.ID()
}

// Host returns the libp2p host this DHT is operating with.
func (gs *GenericService) Host() host.Host {
	return gs.ns.Host
}

func (gs *GenericService) Close() error {
	return gs.proc.Close()
}
