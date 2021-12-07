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
	pdpcommon "github.com/memoio/go-mefs-v2/lib/crypto/pdp/common"
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
	IState
}

type UserNode interface {
	FullNode

	ILfsService
}

type ProviderNode interface {
	FullNode
}

// json api auth and verify
type IAuth interface {
	AuthVerify(context.Context, string) ([]auth.Permission, error)
	AuthNew(context.Context, []auth.Permission) ([]byte, error)
}

// config
type IConfig interface {
	ConfigSet(context.Context, string, string) error
	ConfigGet(context.Context, string) (interface{}, error)
}

// wallet ops
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
	RoleSelf(context.Context) (*pb.RoleInfo, error)
	RoleGet(context.Context, uint64) (*pb.RoleInfo, error)
	RoleGetRelated(context.Context, pb.RoleInfo_Type) ([]uint64, error)

	RoleSign(context.Context, uint64, []byte, types.SigType) (types.Signature, error)
	RoleVerify(context.Context, uint64, []byte, types.Signature) (bool, error)
	RoleVerifyMulti(context.Context, []byte, types.MultiSignature) (bool, error)
}

type INetService interface {
	// send/handle msg directly over network
	SendMetaRequest(ctx context.Context, to uint64, mes_typ pb.NetMessage_MsgType, val, sig []byte) (*pb.NetMessage, error)

	// todo: should be swap network
	Fetch(ctx context.Context, key []byte) ([]byte, error)

	// broadcast using pubsub
	PublishTxMsg(ctx context.Context, msg *tx.SignedMessage) error
	PublishTxBlock(ctx context.Context, msg *tx.Block) error
	PublishEvent(ctx context.Context, msg *pb.EventMessage) error
}

type IDataService interface {
	PutSegmentToLocal(ctx context.Context, seg segment.Segment) error
	GetSegmentFromLocal(ctx context.Context, sid segment.SegmentID) (segment.Segment, error)
	DeleteSegment(ctx context.Context, sid segment.SegmentID) error

	SendSegment(ctx context.Context, seg segment.Segment, to uint64) error
	SendSegmentByID(ctx context.Context, sid segment.SegmentID, to uint64) error

	GetSegment(ctx context.Context, sid segment.SegmentID) (segment.Segment, error)
	GetSegmentRemote(ctx context.Context, sid segment.SegmentID, from uint64) (segment.Segment, error)
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

type IChain interface {
	IState
	GetSyncHeight(context.Context) (uint64, uint64)
	GetPendingNonce(context.Context, uint64) uint64
	GetTxMsgStatus(context.Context, types.MsgID) (*tx.MsgState, error)

	PushMessage(context.Context, *tx.Message) (types.MsgID, error)
	PushSignedMessage(context.Context, *tx.SignedMessage) (types.MsgID, error)
}

type IState interface {
	GetRoot(context.Context) types.MsgID
	GetHeight(context.Context) uint64
	GetSlot(context.Context) uint64

	GetChalEpoch(context.Context) uint64
	GetChalEpochInfo(context.Context) *types.ChalEpoch
	GetChalEpochInfoAt(context.Context, uint64) (*types.ChalEpoch, error)

	GetNonce(context.Context, uint64) uint64

	GetUsersForPro(context.Context, uint64) []uint64
	GetProsForUser(context.Context, uint64) []uint64
	GetAllUsers(context.Context) []uint64

	GetPDPPublicKey(context.Context, uint64) (pdpcommon.PublicKey, error)
	GetBucket(context.Context, uint64) uint64

	GetOrderState(context.Context, uint64, uint64) *types.NonceSeq
	GetPostIncome(context.Context, uint64, uint64) *types.PostIncome

	/*
		GetRoleBaseInfo(userID uint64) (*pb.RoleInfo, error)
		GetOrderStateAt(userID, proID, epoch uint64) *types.NonceSeq
		GetOrder(userID, proID, nonce uint64) (*types.SignedOrder, []byte, uint32, error)
		GetOrderSeq(userID, proID, nonce uint64, seqNum uint32) (*types.OrderSeq, []byte, error)
		GetProof(userID, proID, epoch uint64) bool

		GetOrderDuration(userID, proID uint64) *types.OrderDuration
	*/
}
