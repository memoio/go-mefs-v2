package network

import (
	"context"
	"crypto/rand"

	"github.com/jbenet/goprocess"
	"github.com/libp2p/go-eventbus"
	"github.com/libp2p/go-libp2p-core/connmgr"
	crypto "github.com/libp2p/go-libp2p-core/crypto"
	"github.com/libp2p/go-libp2p-core/event"
	net "github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/peerstore"
	"github.com/libp2p/go-libp2p-core/protocol"
	"github.com/libp2p/go-libp2p-peerstore/pstoremem"
	"github.com/multiformats/go-multiaddr"
	"golang.org/x/xerrors"
)

type mockLibP2PHost struct {
	peerId peer.ID //nolint
}

//nolint
func NewMockLibP2PHost() mockLibP2PHost {
	pk, _, _ := crypto.GenerateEd25519Key(rand.Reader) //nolint
	pid, _ := peer.IDFromPrivateKey(pk)
	return mockLibP2PHost{pid}
}
func (h mockLibP2PHost) ID() peer.ID {
	return h.peerId
}

func (mockLibP2PHost) Peerstore() peerstore.Peerstore {
	return pstoremem.NewPeerstore()
}

func (mockLibP2PHost) Addrs() []multiaddr.Multiaddr {
	return []multiaddr.Multiaddr{}
}

func (mockLibP2PHost) EventBus() event.Bus {
	return eventbus.NewBus()
}

func (mockLibP2PHost) Network() net.Network {
	return mockLibP2PNetwork{}
}

func (mockLibP2PHost) Mux() protocol.Switch {
	panic("implement me")
}

func (mockLibP2PHost) Connect(ctx context.Context, pi peer.AddrInfo) error {
	return xerrors.New("Connect called on mockLibP2PHost")
}

func (mockLibP2PHost) SetStreamHandler(pid protocol.ID, handler net.StreamHandler) {

}

func (mockLibP2PHost) SetStreamHandlerMatch(protocol.ID, func(string) bool, net.StreamHandler) {

}

func (mockLibP2PHost) RemoveStreamHandler(pid protocol.ID) {
	panic("implement me")
}

func (mockLibP2PHost) NewStream(ctx context.Context, p peer.ID, pids ...protocol.ID) (net.Stream, error) {
	return nil, xerrors.New("NewStream on mockLibP2PHost")
}

func (mockLibP2PHost) Close() error {
	return nil
}

func (mockLibP2PHost) ConnManager() connmgr.ConnManager {
	return &connmgr.NullConnMgr{}
}

type mockLibP2PNetwork struct{}

func (mockLibP2PNetwork) Peerstore() peerstore.Peerstore {
	panic("implement me")
}

func (mockLibP2PNetwork) LocalPeer() peer.ID {
	panic("implement me")
}

func (mockLibP2PNetwork) DialPeer(context.Context, peer.ID) (net.Conn, error) {
	panic("implement me")
}

func (mockLibP2PNetwork) ClosePeer(peer.ID) error {
	panic("implement me")
}

func (mockLibP2PNetwork) Connectedness(peer.ID) net.Connectedness {
	panic("implement me")
}

func (mockLibP2PNetwork) Peers() []peer.ID {
	return []peer.ID{}
}

func (mockLibP2PNetwork) Conns() []net.Conn {
	return []net.Conn{}
}

func (mockLibP2PNetwork) ConnsToPeer(p peer.ID) []net.Conn {
	return []net.Conn{}
}

func (mockLibP2PNetwork) Notify(net.Notifiee) {

}

func (mockLibP2PNetwork) StopNotify(net.Notifiee) {
	panic("implement me")
}

func (mockLibP2PNetwork) Close() error {
	panic("implement me")
}

func (mockLibP2PNetwork) SetStreamHandler(net.StreamHandler) {
	panic("implement me")
}

func (mockLibP2PNetwork) SetConnHandler(net.ConnHandler) {
	panic("implement me")
}

func (mockLibP2PNetwork) NewStream(context.Context, peer.ID) (net.Stream, error) {
	panic("implement me")
}

func (mockLibP2PNetwork) Listen(...multiaddr.Multiaddr) error {
	panic("implement me")
}

func (mockLibP2PNetwork) ListenAddresses() []multiaddr.Multiaddr {
	panic("implement me")
}

func (mockLibP2PNetwork) InterfaceListenAddresses() ([]multiaddr.Multiaddr, error) {
	panic("implement me")
}

func (mockLibP2PNetwork) Process() goprocess.Process {
	panic("implement me")
}

func (mockLibP2PNetwork) API() interface{} {
	return nil
}
