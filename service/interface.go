package service

import (
	"context"

	"github.com/memoio/go-mefs-v2/lib/pb"
)

// protoMessenger -> msgSender -> (send)

//所有角色都需要的

// send/handle msg directly over network
type CoreService interface {
	SendMetaMessage(ctx context.Context, to uint64, mes_typ pb.NetMessage_MsgType, val []byte) error
	SendMetaRequest(ctx context.Context, to uint64, mes_typ pb.NetMessage_MsgType, val []byte) (*pb.NetMessage, error)
}
