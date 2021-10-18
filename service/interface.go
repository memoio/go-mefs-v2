package service

import (
	"context"

	"github.com/libp2p/go-libp2p-core/peer"

	pb "github.com/memoio/go-mefs-v2/lib/pb"
)

// protoMessenger -> msgSender -> (send)

//所有角色都需要的
type CoreService interface {
	SendMetaMessage(ctx context.Context, to peer.ID, mes_typ pb.NetMessage_MsgType, key string) error
	SendMetaRequest(ctx context.Context, to peer.ID, mes_typ pb.NetMessage_MsgType, key string, data []byte) ([]byte, error)

	Connect(ctx context.Context, addr string) error
	Disconnect(ctx context.Context, to peer.ID) error
	TryConnect(ctx context.Context, to peer.ID) (string, bool)
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
