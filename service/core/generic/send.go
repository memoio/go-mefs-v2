package generic_service

import (
	"context"

	"github.com/libp2p/go-libp2p-core/network"
	peer "github.com/libp2p/go-libp2p-core/peer"

	"github.com/memoio/go-mefs-v2/lib/pb"
)

// send
func (gs *GenericService) SendMetaMessage(ctx context.Context, p peer.ID, typ pb.NetMessage_MsgType, value []byte) error {
	nm := &pb.NetMessage{}
	return gs.msgSender.SendMessage(ctx, p, nm)
}

func (gs *GenericService) SendMetaRequest(ctx context.Context, p peer.ID, typ pb.NetMessage_MsgType, value []byte) (*pb.NetMessage, error) {
	if gs.ns.Host.Network().Connectedness(p) != network.Connected {
		return nil, nil
	}

	nm := &pb.NetMessage{
		Header: &pb.NetMessage_MsgHeader{
			Type: typ,
		},
		Data: &pb.NetMessage_MsgData{
			MsgInfo: value,
		},
	}

	return gs.msgSender.SendRequest(ctx, p, nm)
}

// find peerID according to its name
func (gs *GenericService) FindPeer(ctx context.Context, id string) {

}
