package service

import (
	"context"

	"github.com/libp2p/go-libp2p-core/peer"

	pb "github.com/memoio/go-mefs-v2/lib/pb"
)

// protoMessenger -> msgSender -> (send)

//所有角色都需要的

// send/handle msg directly over network
type CoreService interface {
	SendMetaMessage(ctx context.Context, to peer.ID, mes_typ pb.NetMessage_MsgType, val []byte) error
	SendMetaRequest(ctx context.Context, to peer.ID, mes_typ pb.NetMessage_MsgType, val []byte) (*pb.NetMessage, error)
}

type UserDataService interface {
	CoreService

	SelfInfo() (*pb.NodeInfo, error)
	GetRandomProviders(ctx context.Context, count int) ([]*pb.NodeInfo, error)
	GetRandomProvidersDedup(ctx context.Context, count int, exists []*pb.NodeInfo) ([]*pb.NodeInfo, error)
	GetSpecificProviders(ctx context.Context, addrs []string) ([]*pb.NodeInfo, error)
	GetProviderInfo(ctx context.Context, providerAddr string) (*pb.NodeInfo, error)

	GetSegment(ctx context.Context, to peer.ID, key string, sig []byte) ([]byte, error)
	PutSegment(ctx context.Context, to peer.ID, key string, data []byte) error

	GetPiece(ctx context.Context, to peer.ID, key string, sig []byte) ([]byte, error)
	PutPiece(ctx context.Context, to peer.ID, key string, data []byte) error
}

type ProviderDataService interface {
	CoreService
	GetSegment(ctx context.Context, to peer.ID, key string, sig []byte) ([]byte, error)
	PutSegment(ctx context.Context, to peer.ID, key string, data []byte) error

	GetPiece(ctx context.Context, to peer.ID, key string, sig []byte) ([]byte, error)
	PutPiece(ctx context.Context, to peer.ID, key string, data []byte) error
}

type KeeperDataService interface {
	CoreService
}
