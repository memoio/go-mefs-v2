package generic_service

import (
	"context"

	peer "github.com/libp2p/go-libp2p-core/peer"

	"github.com/memoio/go-mefs-v2/lib/pb"
)

// send
func (service *GenericService) SendMetaMessage(ctx context.Context, p peer.ID, typ pb.NetMessage_MsgType, value []byte) error {
	nm := &pb.NetMessage{}
	return service.msgSender.SendMessage(ctx, p, nm)
}

func (service *GenericService) SendMetaRequest(ctx context.Context, p peer.ID, typ pb.NetMessage_MsgType, value []byte) (*pb.NetMessage, error) {
	nm := &pb.NetMessage{
		Header: &pb.NetMessage_MsgHeader{
			Type: typ,
		},
		Data: &pb.NetMessage_MsgData{
			MsgInfo: value,
		},
	}

	return service.msgSender.SendRequest(ctx, p, nm)
}
