package generic_service

import (
	"context"

	peer "github.com/libp2p/go-libp2p-core/peer"

	"github.com/memoio/go-mefs-v2/lib/pb"
)

// send
func (service *GenericService) SendMetaMessage(ctx context.Context, p peer.ID, typ pb.NetMessage_MsgType, key string) error {
	nm := &pb.NetMessage{}
	return service.msgSender.SendMessage(ctx, p, nm)
}

func (service *GenericService) SendMetaRequest(ctx context.Context, p peer.ID, typ pb.NetMessage_MsgType, key string, value []byte) ([]byte, error) {
	nm := &pb.NetMessage{}
	pmes, err := service.msgSender.SendRequest(ctx, p, nm)
	if err != nil {
		return nil, err
	}

	return pmes.GetData().GetMsgInfo(), nil
}
