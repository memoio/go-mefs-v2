package api

import (
	"context"
	"io"
	"math/big"

	"github.com/filecoin-project/go-jsonrpc/auth"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"

	"github.com/memoio/go-mefs-v2/lib/address"
	pdpcommon "github.com/memoio/go-mefs-v2/lib/crypto/pdp/common"
	hs "github.com/memoio/go-mefs-v2/lib/hotstuff"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/segment"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
)

type FullNode interface {
	Version(context.Context) (string, error)

	ILog
	IAuth
	IConfig
	ILocalStore
	IWallet
	IRole
	IChainPush
	INetwork
	ISettle

	Ready(context.Context) bool
	Shutdown(context.Context) error
}

type UserNode interface {
	FullNode

	ILfsService
	IOrder
}

type ProviderNode interface {
	FullNode

	IOrder
}

type KeeperNode interface {
	FullNode
}

type ILog interface {
	LogSetLevel(context.Context, string) error
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

type ILocalStore interface {
	LocalStoreGetMeta(context.Context) (store.DiskStats, error)
	LocalStoreGetData(context.Context) (store.DiskStats, error)
}

// wallet ops
type IWallet interface {
	WalletNew(context.Context, types.KeyType) (address.Address, error)
	WalletSign(context.Context, address.Address, []byte) ([]byte, error)
	WalletList(context.Context) ([]address.Address, error)
	WalletHas(context.Context, address.Address) (bool, error)
	WalletDelete(context.Context, address.Address) error
	WalletExport(context.Context, address.Address, string) (*types.KeyInfo, error)
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

	NetAutoNatStatus(context.Context) (NatInfo, error)
	// status; add more

	//NetBandwidthStats(ctx context.Context) (metrics.Stats, error)
	//NetBandwidthStatsByPeer(ctx context.Context) (map[string]metrics.Stats, error)
	//NetBandwidthStatsByProtocol(ctx context.Context) (map[protocol.ID]metrics.Stats, error)
}

type IRole interface {
	RoleSelf(context.Context) (*pb.RoleInfo, error)
	RoleGet(context.Context, uint64) (*pb.RoleInfo, error)
	RoleGetRelated(context.Context, pb.RoleInfo_Type) ([]uint64, error)

	RoleSanityCheck(context.Context, *tx.SignedMessage) (bool, error)
	RoleSign(context.Context, uint64, []byte, types.SigType) (types.Signature, error)
	RoleVerify(context.Context, uint64, []byte, types.Signature) (bool, error)
	RoleVerifyMulti(context.Context, []byte, types.MultiSignature) (bool, error)
}

type INetService interface {
	// send/handle msg directly over network
	SendMetaRequest(ctx context.Context, to uint64, mes_typ pb.NetMessage_MsgType, val, sig []byte) (*pb.NetMessage, error)

	// todo: should be swap network
	Fetch(ctx context.Context, key []byte) ([]byte, error)
	GetPeerIDAt(ctx context.Context, id uint64) (peer.ID, error)

	// broadcast using pubsub
	PublishTxMsg(ctx context.Context, msg *tx.SignedMessage) error
	PublishTxBlock(ctx context.Context, msg *tx.SignedBlock) error
	PublishEvent(ctx context.Context, msg *pb.EventMessage) error
	PublishHsMsg(ctx context.Context, msg *hs.HotstuffMessage) error
}

type IDataService interface {
	PutSegmentToLocal(ctx context.Context, seg segment.Segment) error
	GetSegmentFromLocal(ctx context.Context, sid segment.SegmentID) (segment.Segment, error)
	DeleteSegment(ctx context.Context, sid segment.SegmentID) error
	HasSegment(ctx context.Context, sid segment.SegmentID) (bool, error)

	SendSegment(ctx context.Context, seg segment.Segment, to uint64) error
	SendSegmentByID(ctx context.Context, sid segment.SegmentID, to uint64) error

	GetSegmentLocation(ctx context.Context, sid segment.SegmentID) (uint64, error)
	GetSegment(ctx context.Context, sid segment.SegmentID) (segment.Segment, error)
	GetSegmentRemote(ctx context.Context, sid segment.SegmentID, from uint64, sig []byte) (segment.Segment, error)
}

type ILfsService interface {
	ListBuckets(ctx context.Context, prefix string) ([]types.BucketInfo, error)
	CreateBucket(ctx context.Context, bucketName string, options pb.BucketOption) (types.BucketInfo, error)
	HeadBucket(ctx context.Context, bucketName string) (types.BucketInfo, error)
	DeleteBucket(ctx context.Context, bucketName string) error

	ListObjects(ctx context.Context, bucketName string, opts types.ListObjectsOptions) (types.ListObjectsInfo, error)

	PutObject(ctx context.Context, bucketName, objectName string, reader io.Reader, opts types.PutObjectOptions) (types.ObjectInfo, error)
	GetObject(ctx context.Context, bucketName, objectName string, opts types.DownloadObjectOptions) ([]byte, error)
	HeadObject(ctx context.Context, bucketName, objectName string) (types.ObjectInfo, error)
	DeleteObject(ctx context.Context, bucketName, objectName string) error

	LfsGetInfo(ctx context.Context, update bool) (types.LfsInfo, error)

	ShowStorage(ctx context.Context) (uint64, error)
	ShowBucketStorage(ctx context.Context, bucketName string) (uint64, error)
}

// process chain

// push
type IChainPush interface {
	IChainSync

	PushGetPendingNonce(context.Context, uint64) uint64
	PushMessage(context.Context, *tx.Message) (types.MsgID, error)
	PushSignedMessage(context.Context, *tx.SignedMessage) (types.MsgID, error)
}

// sync status
type IChainSync interface {
	IChainState
	SyncGetInfo(context.Context) (*SyncInfo, error)
	SyncGetTxMsgStatus(context.Context, types.MsgID) (*tx.MsgState, error)
}

type IChainState interface {
	StateGetInfo(context.Context) (*StateInfo, error)
	StateGetChalEpochInfo(context.Context) (*types.ChalEpoch, error)
	StateGetChalEpochInfoAt(context.Context, uint64) (*types.ChalEpoch, error)

	StateGetNonce(context.Context, uint64) uint64
	StateGetNetInfo(context.Context, uint64) (peer.AddrInfo, error)

	StateGetAllKeepers(context.Context) []uint64
	StateGetAllUsers(context.Context) []uint64
	StateGetAllProviders(context.Context) []uint64
	StateGetUsersAt(context.Context, uint64) []uint64
	StateGetProsAt(context.Context, uint64) []uint64

	StateGetPDPPublicKey(context.Context, uint64) (pdpcommon.PublicKey, error)

	StateGetOrderState(context.Context, uint64, uint64) *types.NonceSeq
	StateGetOrder(context.Context, uint64, uint64, uint64) (*types.OrderFull, error)
	StateGetOrderSeq(context.Context, uint64, uint64, uint64, uint32) (*types.SeqFull, error)
	StateGetPostIncome(context.Context, uint64, uint64) (*types.PostIncome, error)
	StateGetPostIncomeAt(context.Context, uint64, uint64, uint64) (*types.PostIncome, error)
	StateGetAccPostIncome(context.Context, uint64) (*types.SignedAccPostIncome, error)
	StateGetAccPostIncomeAt(context.Context, uint64, uint64) (*types.AccPostIncome, error)
}

type ISettle interface {
	SettleGetRoleID(context.Context) uint64
	SettleGetGroupID(context.Context) uint64
	SettleGetThreshold(context.Context) int
	SettleGetRoleInfoAt(context.Context, uint64) (*pb.RoleInfo, error)
	SettleGetGroupInfoAt(context.Context, uint64) (*GroupInfo, error)
	SettleGetBalanceInfo(context.Context, uint64) (*BalanceInfo, error)
	SettleGetPledgeInfo(context.Context, uint64) (*PledgeInfo, error)
	SettleGetStoreInfo(context.Context, uint64, uint64) (*StoreInfo, error)

	SettleWithdraw(context.Context, *big.Int, *big.Int, []uint64, [][]byte) error
	SettlePledge(context.Context, *big.Int) error
	SettleCanclePledge(context.Context, *big.Int) error
}

type IOrder interface {
	OrderList(context.Context) ([]uint64, error)
	OrderGetJobInfo(context.Context) ([]*OrderJobInfo, error)
	OrderGetJobInfoAt(context.Context, uint64) (*OrderJobInfo, error)
	OrderGetPayInfo(context.Context) ([]*types.OrderPayInfo, error)
	OrderGetPayInfoAt(context.Context, uint64) (*types.OrderPayInfo, error)

	OrderGetDetail(ctx context.Context, proID, nonce uint64, seqNum uint32) (*types.SignedOrderSeq, error)
}
