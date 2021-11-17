package api

import (
	"context"
	"io"

	"github.com/filecoin-project/go-jsonrpc/auth"
	"github.com/libp2p/go-libp2p-core/metrics"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"

	"github.com/memoio/go-mefs-v2/lib/address"
	mSign "github.com/memoio/go-mefs-v2/lib/multiSign"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/segment"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
)

type FullNode interface {
	IAuth
	IConfig
	IWallet
	IRole
}

type UserNode interface {
	FullNode

	ILfsService
}

type ProviderNode interface {
	FullNode
}

type IAuth interface {
	AuthVerify(context.Context, string) ([]auth.Permission, error)
	AuthNew(context.Context, []auth.Permission) ([]byte, error)
}

type IConfig interface {
	ConfigSet(context.Context, string, string) error
	ConfigGet(context.Context, string) (interface{}, error)
}

type IWallet interface {
	WalletNew(context.Context, types.KeyType) (address.Address, error)
	WalletSign(context.Context, address.Address, []byte) ([]byte, error)
	WalletList(context.Context) ([]address.Address, error)
	WalletHas(context.Context, address.Address) (bool, error)
	WalletDelete(context.Context, address.Address) error
	WalletExport(context.Context, address.Address) (*types.KeyInfo, error)
	WalletImport(context.Context, *types.KeyInfo) (address.Address, error)
}

type INetwork interface {
	// info
	NetAddrInfo(context.Context) (peer.AddrInfo, error)

	NetConnectedness(context.Context, peer.ID) (network.Connectedness, error)

	NetConnect(context.Context, peer.AddrInfo) error

	NetDisconnect(context.Context, peer.ID) error

	NetFindPeer(context.Context, peer.ID) (peer.AddrInfo, error)

	NetPeerInfo(context.Context, peer.ID) (*ExtendedPeerInfo, error)

	NetPeers(context.Context) ([]peer.AddrInfo, error)

	// status; add more
	NetAutoNatStatus(context.Context) (NatInfo, error)
	NetBandwidthStats(ctx context.Context) (metrics.Stats, error)
	NetBandwidthStatsByPeer(ctx context.Context) (map[string]metrics.Stats, error)
	NetBandwidthStatsByProtocol(ctx context.Context) (map[protocol.ID]metrics.Stats, error)
}

type IRole interface {
	RoleSelf(context.Context) (pb.RoleInfo, error)
	RoleGet(context.Context, uint64) (pb.RoleInfo, error)
	RoleGetRelated(context.Context, pb.RoleInfo_Type) ([]uint64, error)

	RoleSign(context.Context, []byte, types.SigType) (types.Signature, error)
	RoleVerify(context.Context, uint64, []byte, types.Signature) (bool, error)
	RoleVerifyMulti(context.Context, []byte, mSign.MultiSignature) (bool, error)
}

type INetService interface {
	// send/handle msg directly over network
	SendMetaMessage(ctx context.Context, to uint64, mes_typ pb.NetMessage_MsgType, val []byte) error
	SendMetaRequest(ctx context.Context, to uint64, mes_typ pb.NetMessage_MsgType, val, sig []byte) (*pb.NetMessage, error)

	Fetch(ctx context.Context, key []byte) ([]byte, error)

	PublishTxMsg(ctx context.Context, msg *tx.SignedMessage) error
	PublishTxBlock(ctx context.Context, msg *tx.Block) error
	PublishEvent(ctx context.Context, msg *pb.EventMessage) error
}

type IDataService interface {
	PutSegmentToLocal(ctx context.Context, seg segment.Segment) error
	GetSegmentFromLocal(ctx context.Context, sid segment.SegmentID) (segment.Segment, error)

	SendSegment(ctx context.Context, seg segment.Segment, to uint64) error
	SendSegmentByID(ctx context.Context, sid segment.SegmentID, to uint64) error

	GetSegment(ctx context.Context, sid segment.SegmentID) (segment.Segment, error)
	GetSegmentFrom(ctx context.Context, sid segment.SegmentID, from uint64) (segment.Segment, error)
}

type ILfsService interface {
	ListBuckets(ctx context.Context, prefix string) ([]*types.BucketInfo, error)
	CreateBucket(ctx context.Context, bucketName string, options *pb.BucketOption) (*types.BucketInfo, error)
	HeadBucket(ctx context.Context, bucketName string) (*types.BucketInfo, error)
	DeleteBucket(ctx context.Context, bucketName string) (*types.BucketInfo, error)

	ListObjects(ctx context.Context, bucketName string, opts *types.ListObjectsOptions) ([]*types.ObjectInfo, error)

	PutObject(ctx context.Context, bucketName, objectName string, reader io.Reader, opts *types.PutObjectOptions) (*types.ObjectInfo, error)
	GetObject(ctx context.Context, bucketName, objectName string, opts *types.DownloadObjectOptions) ([]byte, error)
	HeadObject(ctx context.Context, bucketName, objectName string) (*types.ObjectInfo, error)
	DeleteObject(ctx context.Context, bucketName, objectName string) (*types.ObjectInfo, error)

	ShowStorage(ctx context.Context) (uint64, error)
	ShowBucketStorage(ctx context.Context, bucketName string) (uint64, error)
}
