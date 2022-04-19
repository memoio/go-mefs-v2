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
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
)

// common API permissions constraints
type CommonStruct struct {
	Internal struct {
		Version func(context.Context) (string, error) `perm:"admin"`

		LogSetLevel func(ctx context.Context, l string) error `perm:"write"`

		AuthVerify func(ctx context.Context, token string) ([]auth.Permission, error) `perm:"read"`
		AuthNew    func(ctx context.Context, erms []auth.Permission) ([]byte, error)  `perm:"admin"`

		ConfigSet func(context.Context, string, string) error        `perm:"write"`
		ConfigGet func(context.Context, string) (interface{}, error) `perm:"read"`

		LocalStoreGetMeta func(context.Context) (store.DiskStats, error) `perm:"read"`
		LocalStoreGetData func(context.Context) (store.DiskStats, error) `perm:"read"`

		WalletNew    func(context.Context, types.KeyType) (address.Address, error)          `perm:"write"`
		WalletSign   func(context.Context, address.Address, []byte) ([]byte, error)         `perm:"sign"`
		WalletList   func(context.Context) ([]address.Address, error)                       `perm:"write"`
		WalletHas    func(context.Context, address.Address) (bool, error)                   `perm:"write"`
		WalletDelete func(context.Context, address.Address) error                           `perm:"write"`
		WalletExport func(context.Context, address.Address, string) (*types.KeyInfo, error) `perm:"admin"`
		WalletImport func(context.Context, *types.KeyInfo) (address.Address, error)         `perm:"write"`

		NetAddrInfo      func(context.Context) (peer.AddrInfo, error)                  `perm:"read"`
		NetAutoNatStatus func(context.Context) (NatInfo, error)                        `perm:"read"`
		NetConnectedness func(context.Context, peer.ID) (network.Connectedness, error) `perm:"read"`
		NetConnect       func(context.Context, peer.AddrInfo) error                    `perm:"write"`
		NetDisconnect    func(context.Context, peer.ID) error                          `perm:"write"`
		NetFindPeer      func(context.Context, peer.ID) (peer.AddrInfo, error)         `perm:"read"`
		NetPeerInfo      func(context.Context, peer.ID) (*ExtendedPeerInfo, error)     `perm:"read"`
		NetPeers         func(context.Context) ([]peer.AddrInfo, error)                `perm:"read"`

		RoleSelf        func(context.Context) (*pb.RoleInfo, error)                                   `perm:"read"`
		RoleGet         func(context.Context, uint64) (*pb.RoleInfo, error)                           `perm:"read"`
		RoleGetRelated  func(context.Context, pb.RoleInfo_Type) ([]uint64, error)                     `perm:"read"`
		RoleSign        func(context.Context, uint64, []byte, types.SigType) (types.Signature, error) `perm:"write"`
		RoleVerify      func(context.Context, uint64, []byte, types.Signature) (bool, error)          `perm:"read"`
		RoleVerifyMulti func(context.Context, []byte, types.MultiSignature) (bool, error)             `perm:"read"`
		RoleSanityCheck func(context.Context, *tx.SignedMessage) (bool, error)                        `perm:"read"`

		StateGetInfo            func(context.Context) (*StateInfo, error)               `perm:"read"`
		StateGetChalEpochInfo   func(context.Context) (*types.ChalEpoch, error)         `perm:"read"`
		StateGetChalEpochInfoAt func(context.Context, uint64) (*types.ChalEpoch, error) `perm:"read"`

		StateGetNonce   func(context.Context, uint64) uint64                 `perm:"read"`
		StateGetNetInfo func(context.Context, uint64) (peer.AddrInfo, error) `perm:"read"`

		StateGetAllKeepers   func(context.Context) []uint64         `perm:"read"`
		StateGetAllUsers     func(context.Context) []uint64         `perm:"read"`
		StateGetAllProviders func(context.Context) []uint64         `perm:"read"`
		StateGetUsersAt      func(context.Context, uint64) []uint64 `perm:"read"`
		StateGetProsAt       func(context.Context, uint64) []uint64 `perm:"read"`

		StateGetPDPPublicKey func(context.Context, uint64) (pdpcommon.PublicKey, error) `perm:"read"`

		StateGetOrderState      func(context.Context, uint64, uint64) *types.NonceSeq                         `perm:"read"`
		StateGetOrder           func(context.Context, uint64, uint64, uint64) (*types.OrderFull, error)       `perm:"read"`
		StateGetOrderSeq        func(context.Context, uint64, uint64, uint64, uint32) (*types.SeqFull, error) `perm:"read"`
		StateGetPostIncome      func(context.Context, uint64, uint64) (*types.PostIncome, error)              `perm:"read"`
		StateGetPostIncomeAt    func(context.Context, uint64, uint64, uint64) (*types.PostIncome, error)      `perm:"read"`
		StateGetAccPostIncome   func(context.Context, uint64) (*types.SignedAccPostIncome, error)             `perm:"read"`
		StateGetAccPostIncomeAt func(context.Context, uint64, uint64) (*types.AccPostIncome, error)           `perm:"read"`

		SettleGetRoleID      func(context.Context) uint64                                        `perm:"read"`
		SettleGetGroupID     func(context.Context) uint64                                        `perm:"read"`
		SettleGetThreshold   func(context.Context) int                                           `perm:"read"`
		SettleGetRoleInfoAt  func(context.Context, uint64) (*pb.RoleInfo, error)                 `perm:"read"`
		SettleGetGroupInfoAt func(context.Context, uint64) (*GroupInfo, error)                   `perm:"read"`
		SettleGetBalanceInfo func(context.Context, uint64) (*BalanceInfo, error)                 `perm:"read"`
		SettleGetPledgeInfo  func(context.Context, uint64) (*PledgeInfo, error)                  `perm:"read"`
		SettleGetStoreInfo   func(context.Context, uint64, uint64) (*StoreInfo, error)           `perm:"read"`
		SettleWithdraw       func(context.Context, *big.Int, *big.Int, []uint64, [][]byte) error `perm:"write"`
		SettlePledge         func(context.Context, *big.Int) error                               `perm:"write"`
		SettleCanclePledge   func(context.Context, *big.Int) error                               `perm:"write"`

		SyncGetInfo        func(context.Context) (*SyncInfo, error)                 `perm:"read"`
		SyncGetTxMsgStatus func(context.Context, types.MsgID) (*tx.MsgState, error) `perm:"read"`

		PushGetPendingNonce func(context.Context, uint64) uint64                          `perm:"read"`
		PushMessage         func(context.Context, *tx.Message) (types.MsgID, error)       `perm:"write"`
		PushSignedMessage   func(context.Context, *tx.SignedMessage) (types.MsgID, error) `perm:"write"`

		Ready    func(context.Context) bool  `perm:"read"`
		Shutdown func(context.Context) error `perm:"admin"`
	}
}

func (s *CommonStruct) Version(ctx context.Context) (string, error) {
	return s.Internal.Version(ctx)
}

func (s *CommonStruct) LogSetLevel(ctx context.Context, l string) error {
	return s.Internal.LogSetLevel(ctx, l)
}

func (s *CommonStruct) AuthVerify(ctx context.Context, token string) ([]auth.Permission, error) {
	return s.Internal.AuthVerify(ctx, token)
}

func (s *CommonStruct) AuthNew(ctx context.Context, perms []auth.Permission) ([]byte, error) {
	return s.Internal.AuthNew(ctx, perms)
}

func (s *CommonStruct) ConfigSet(ctx context.Context, key, val string) error {
	return s.Internal.ConfigSet(ctx, key, val)
}

func (s *CommonStruct) ConfigGet(ctx context.Context, key string) (interface{}, error) {
	return s.Internal.ConfigGet(ctx, key)
}

func (s *CommonStruct) LocalStoreGetMeta(ctx context.Context) (store.DiskStats, error) {
	return s.Internal.LocalStoreGetMeta(ctx)
}

func (s *CommonStruct) LocalStoreGetData(ctx context.Context) (store.DiskStats, error) {
	return s.Internal.LocalStoreGetData(ctx)
}

func (s *CommonStruct) WalletNew(ctx context.Context, typ types.KeyType) (address.Address, error) {
	return s.Internal.WalletNew(ctx, typ)
}

func (s *CommonStruct) WalletSign(ctx context.Context, addr address.Address, msg []byte) ([]byte, error) {
	return s.Internal.WalletSign(ctx, addr, msg)
}

func (s *CommonStruct) WalletHas(ctx context.Context, addr address.Address) (bool, error) {
	return s.Internal.WalletHas(ctx, addr)
}

func (s *CommonStruct) WalletDelete(ctx context.Context, addr address.Address) error {
	return s.Internal.WalletDelete(ctx, addr)
}

func (s *CommonStruct) WalletList(ctx context.Context) ([]address.Address, error) {
	return s.Internal.WalletList(ctx)
}

func (s *CommonStruct) WalletExport(ctx context.Context, addr address.Address, pw string) (*types.KeyInfo, error) {
	return s.Internal.WalletExport(ctx, addr, pw)
}

func (s *CommonStruct) WalletImport(ctx context.Context, ki *types.KeyInfo) (address.Address, error) {
	return s.Internal.WalletImport(ctx, ki)
}

func (s *CommonStruct) NetAddrInfo(ctx context.Context) (peer.AddrInfo, error) {
	return s.Internal.NetAddrInfo(ctx)
}

func (s *CommonStruct) NetAutoNatStatus(ctx context.Context) (NatInfo, error) {
	return s.Internal.NetAutoNatStatus(ctx)
}

func (s *CommonStruct) NetConnectedness(ctx context.Context, p peer.ID) (network.Connectedness, error) {
	return s.Internal.NetConnectedness(ctx, p)
}

func (s *CommonStruct) NetConnect(ctx context.Context, p peer.AddrInfo) error {
	return s.Internal.NetConnect(ctx, p)
}

func (s *CommonStruct) NetDisconnect(ctx context.Context, p peer.ID) error {
	return s.Internal.NetDisconnect(ctx, p)
}

func (s *CommonStruct) NetFindPeer(ctx context.Context, p peer.ID) (peer.AddrInfo, error) {
	return s.Internal.NetFindPeer(ctx, p)
}

func (s *CommonStruct) NetPeerInfo(ctx context.Context, p peer.ID) (*ExtendedPeerInfo, error) {
	return s.Internal.NetPeerInfo(ctx, p)
}

func (s *CommonStruct) NetPeers(ctx context.Context) ([]peer.AddrInfo, error) {
	return s.Internal.NetPeers(ctx)
}

func (s *CommonStruct) RoleSelf(ctx context.Context) (*pb.RoleInfo, error) {
	return s.Internal.RoleSelf(ctx)
}

func (s *CommonStruct) RoleGet(ctx context.Context, id uint64) (*pb.RoleInfo, error) {
	return s.Internal.RoleGet(ctx, id)
}

func (s *CommonStruct) RoleGetRelated(ctx context.Context, typ pb.RoleInfo_Type) ([]uint64, error) {
	return s.Internal.RoleGetRelated(ctx, typ)
}

func (s *CommonStruct) RoleSign(ctx context.Context, id uint64, msg []byte, typ types.SigType) (types.Signature, error) {
	return s.Internal.RoleSign(ctx, id, msg, typ)
}

func (s *CommonStruct) RoleVerify(ctx context.Context, id uint64, msg []byte, sig types.Signature) (bool, error) {
	return s.Internal.RoleVerify(ctx, id, msg, sig)
}

func (s *CommonStruct) RoleVerifyMulti(ctx context.Context, msg []byte, sig types.MultiSignature) (bool, error) {
	return s.Internal.RoleVerifyMulti(ctx, msg, sig)
}

func (s *CommonStruct) RoleSanityCheck(ctx context.Context, msg *tx.SignedMessage) (bool, error) {
	return s.Internal.RoleSanityCheck(ctx, msg)
}

func (s *CommonStruct) StateGetInfo(ctx context.Context) (*StateInfo, error) {
	return s.Internal.StateGetInfo(ctx)
}

func (s *CommonStruct) StateGetChalEpochInfo(ctx context.Context) (*types.ChalEpoch, error) {
	return s.Internal.StateGetChalEpochInfo(ctx)
}

func (s *CommonStruct) StateGetChalEpochInfoAt(ctx context.Context, epoch uint64) (*types.ChalEpoch, error) {
	return s.Internal.StateGetChalEpochInfoAt(ctx, epoch)
}

func (s *CommonStruct) StateGetNonce(ctx context.Context, roleID uint64) uint64 {
	return s.Internal.StateGetNonce(ctx, roleID)
}

func (s *CommonStruct) StateGetNetInfo(ctx context.Context, roleID uint64) (peer.AddrInfo, error) {
	return s.Internal.StateGetNetInfo(ctx, roleID)
}

func (s *CommonStruct) StateGetUsersAt(ctx context.Context, proID uint64) []uint64 {
	return s.Internal.StateGetUsersAt(ctx, proID)
}

func (s *CommonStruct) StateGetProsAt(ctx context.Context, userID uint64) []uint64 {
	return s.Internal.StateGetProsAt(ctx, userID)
}

func (s *CommonStruct) StateGetAllUsers(ctx context.Context) []uint64 {
	return s.Internal.StateGetAllUsers(ctx)
}

func (s *CommonStruct) StateGetAllProviders(ctx context.Context) []uint64 {
	return s.Internal.StateGetAllProviders(ctx)
}

func (s *CommonStruct) StateGetAllKeepers(ctx context.Context) []uint64 {
	return s.Internal.StateGetAllKeepers(ctx)
}

func (s *CommonStruct) StateGetPDPPublicKey(ctx context.Context, userID uint64) (pdpcommon.PublicKey, error) {
	return s.Internal.StateGetPDPPublicKey(ctx, userID)
}

func (s *CommonStruct) StateGetOrderState(ctx context.Context, userID, proID uint64) *types.NonceSeq {
	return s.Internal.StateGetOrderState(ctx, userID, proID)
}

func (s *CommonStruct) StateGetOrder(ctx context.Context, userID, proID, nonce uint64) (*types.OrderFull, error) {
	return s.Internal.StateGetOrder(ctx, userID, proID, nonce)
}

func (s *CommonStruct) StateGetOrderSeq(ctx context.Context, userID, proID, nonce uint64, seqNum uint32) (*types.SeqFull, error) {
	return s.Internal.StateGetOrderSeq(ctx, userID, proID, nonce, seqNum)
}

func (s *CommonStruct) StateGetPostIncome(ctx context.Context, userID, proID uint64) (*types.PostIncome, error) {
	return s.Internal.StateGetPostIncome(ctx, userID, proID)
}

func (s *CommonStruct) StateGetPostIncomeAt(ctx context.Context, userID, proID, epoch uint64) (*types.PostIncome, error) {
	return s.Internal.StateGetPostIncomeAt(ctx, userID, proID, epoch)
}

func (s *CommonStruct) StateGetAccPostIncome(ctx context.Context, proID uint64) (*types.SignedAccPostIncome, error) {
	return s.Internal.StateGetAccPostIncome(ctx, proID)
}

func (s *CommonStruct) StateGetAccPostIncomeAt(ctx context.Context, proID, epoch uint64) (*types.AccPostIncome, error) {
	return s.Internal.StateGetAccPostIncomeAt(ctx, proID, epoch)
}

func (s *CommonStruct) SettleGetRoleID(ctx context.Context) uint64 {
	return s.Internal.SettleGetRoleID(ctx)
}

func (s *CommonStruct) SettleGetGroupID(ctx context.Context) uint64 {
	return s.Internal.SettleGetGroupID(ctx)
}

func (s *CommonStruct) SettleGetThreshold(ctx context.Context) int {
	return s.Internal.SettleGetThreshold(ctx)
}

func (s *CommonStruct) SettleGetRoleInfoAt(ctx context.Context, rid uint64) (*pb.RoleInfo, error) {
	return s.Internal.SettleGetRoleInfoAt(ctx, rid)
}

func (s *CommonStruct) SettleGetGroupInfoAt(ctx context.Context, gid uint64) (*GroupInfo, error) {
	return s.Internal.SettleGetGroupInfoAt(ctx, gid)
}

func (s *CommonStruct) SettleGetBalanceInfo(ctx context.Context, rid uint64) (*BalanceInfo, error) {
	return s.Internal.SettleGetBalanceInfo(ctx, rid)
}

func (s *CommonStruct) SettleGetPledgeInfo(ctx context.Context, rid uint64) (*PledgeInfo, error) {
	return s.Internal.SettleGetPledgeInfo(ctx, rid)
}

func (s *CommonStruct) SettleGetStoreInfo(ctx context.Context, uid, pid uint64) (*StoreInfo, error) {
	return s.Internal.SettleGetStoreInfo(ctx, uid, pid)
}

func (s *CommonStruct) SettleWithdraw(ctx context.Context, val, penlty *big.Int, kind []uint64, sig [][]byte) error {
	return s.Internal.SettleWithdraw(ctx, val, penlty, kind, sig)
}

func (s *CommonStruct) SettlePledge(ctx context.Context, val *big.Int) error {
	return s.Internal.SettlePledge(ctx, val)
}

func (s *CommonStruct) SettleCanclePledge(ctx context.Context, val *big.Int) error {
	return s.Internal.SettleCanclePledge(ctx, val)
}

func (s *CommonStruct) SyncGetInfo(ctx context.Context) (*SyncInfo, error) {
	return s.Internal.SyncGetInfo(ctx)
}

func (s *CommonStruct) SyncGetTxMsgStatus(ctx context.Context, mid types.MsgID) (*tx.MsgState, error) {
	return s.Internal.SyncGetTxMsgStatus(ctx, mid)
}

func (s *CommonStruct) PushGetPendingNonce(ctx context.Context, rid uint64) uint64 {
	return s.Internal.PushGetPendingNonce(ctx, rid)
}

func (s *CommonStruct) PushMessage(ctx context.Context, msg *tx.Message) (types.MsgID, error) {
	return s.Internal.PushMessage(ctx, msg)
}

func (s *CommonStruct) PushSignedMessage(ctx context.Context, smsg *tx.SignedMessage) (types.MsgID, error) {
	return s.Internal.PushSignedMessage(ctx, smsg)
}

func (s *CommonStruct) Ready(ctx context.Context) bool {
	return s.Internal.Ready(ctx)
}

func (s *CommonStruct) Shutdown(ctx context.Context) error {
	return s.Internal.Shutdown(ctx)
}

type FullNodeStruct struct {
	CommonStruct
}

type ProviderNodeStruct struct {
	CommonStruct

	Internal struct {
		OrderList         func(ctx context.Context) ([]uint64, error)                    `perm:"read"`
		OrderGetJobInfo   func(ctx context.Context) ([]*OrderJobInfo, error)             `perm:"read"`
		OrderGetJobInfoAt func(ctx context.Context, proID uint64) (*OrderJobInfo, error) `perm:"read"`
		OrderGetPayInfo   func(context.Context) ([]*types.OrderPayInfo, error)           `perm:"read"`
		OrderGetPayInfoAt func(context.Context, uint64) (*types.OrderPayInfo, error)     `perm:"read"`

		OrderGetDetail func(ctx context.Context, proID, nonce uint64, seqNum uint32) (*types.SignedOrderSeq, error) `perm:"read"`
	}
}

func (s *ProviderNodeStruct) OrderGetJobInfo(ctx context.Context) ([]*OrderJobInfo, error) {
	return s.Internal.OrderGetJobInfo(ctx)
}

func (s *ProviderNodeStruct) OrderGetJobInfoAt(ctx context.Context, proID uint64) (*OrderJobInfo, error) {
	return s.Internal.OrderGetJobInfoAt(ctx, proID)
}

func (s *ProviderNodeStruct) OrderGetPayInfo(ctx context.Context) ([]*types.OrderPayInfo, error) {
	return s.Internal.OrderGetPayInfo(ctx)
}

func (s *ProviderNodeStruct) OrderGetPayInfoAt(ctx context.Context, proID uint64) (*types.OrderPayInfo, error) {
	return s.Internal.OrderGetPayInfoAt(ctx, proID)
}

func (s *ProviderNodeStruct) OrderList(ctx context.Context) ([]uint64, error) {
	return s.Internal.OrderList(ctx)
}

func (s *ProviderNodeStruct) OrderGetDetail(ctx context.Context, id, nonce uint64, seqNum uint32) (*types.SignedOrderSeq, error) {
	return s.Internal.OrderGetDetail(ctx, id, nonce, seqNum)
}

type UserNodeStruct struct {
	CommonStruct

	Internal struct {
		CreateBucket func(ctx context.Context, bucketName string, opts pb.BucketOption) (types.BucketInfo, error)                                      `perm:"write"`
		DeleteBucket func(ctx context.Context, bucketName string) error                                                                                `perm:"write"`
		PutObject    func(ctx context.Context, bucketName, objectName string, reader io.Reader, opts types.PutObjectOptions) (types.ObjectInfo, error) `perm:"write"`
		DeleteObject func(ctx context.Context, bucketName, objectName string) error                                                                    `perm:"write"`

		HeadBucket  func(ctx context.Context, bucketName string) (types.BucketInfo, error) `perm:"read"`
		ListBuckets func(ctx context.Context, prefix string) ([]types.BucketInfo, error)   `perm:"read"`

		GetObject   func(ctx context.Context, bucketName, objectName string, opts types.DownloadObjectOptions) ([]byte, error) `perm:"read"`
		HeadObject  func(ctx context.Context, bucketName, objectName string) (types.ObjectInfo, error)                         `perm:"read"`
		ListObjects func(ctx context.Context, bucketName string, opts types.ListObjectsOptions) (types.ListObjectsInfo, error) `perm:"read"`

		LfsGetInfo func(context.Context, bool) (types.LfsInfo, error) `perm:"read"`

		ShowStorage       func(ctx context.Context) (uint64, error)                    `perm:"read"`
		ShowBucketStorage func(ctx context.Context, bucketName string) (uint64, error) `perm:"read"`

		OrderList         func(ctx context.Context) ([]uint64, error)                    `perm:"read"`
		OrderGetJobInfo   func(ctx context.Context) ([]*OrderJobInfo, error)             `perm:"read"`
		OrderGetJobInfoAt func(ctx context.Context, proID uint64) (*OrderJobInfo, error) `perm:"read"`
		OrderGetPayInfo   func(context.Context) ([]*types.OrderPayInfo, error)           `perm:"read"`
		OrderGetPayInfoAt func(context.Context, uint64) (*types.OrderPayInfo, error)     `perm:"read"`

		OrderGetDetail func(ctx context.Context, proID, nonce uint64, seqNum uint32) (*types.SignedOrderSeq, error) `perm:"read"`
	}
}

func (s *UserNodeStruct) CreateBucket(ctx context.Context, bucketName string, options pb.BucketOption) (types.BucketInfo, error) {
	return s.Internal.CreateBucket(ctx, bucketName, options)
}

func (s *UserNodeStruct) DeleteBucket(ctx context.Context, bucketName string) error {
	return s.Internal.DeleteBucket(ctx, bucketName)
}

func (s *UserNodeStruct) PutObject(ctx context.Context, bucketName, objectName string, reader io.Reader, opts types.PutObjectOptions) (types.ObjectInfo, error) {
	return s.Internal.PutObject(ctx, bucketName, objectName, reader, opts)
}

func (s *UserNodeStruct) DeleteObject(ctx context.Context, bucketName, objectName string) error {
	return s.Internal.DeleteObject(ctx, bucketName, objectName)
}

func (s *UserNodeStruct) HeadBucket(ctx context.Context, bucketName string) (types.BucketInfo, error) {
	return s.Internal.HeadBucket(ctx, bucketName)
}

func (s *UserNodeStruct) ListBuckets(ctx context.Context, prefix string) ([]types.BucketInfo, error) {
	return s.Internal.ListBuckets(ctx, prefix)
}

func (s *UserNodeStruct) GetObject(ctx context.Context, bucketName, objectName string, opts types.DownloadObjectOptions) ([]byte, error) {
	return s.Internal.GetObject(ctx, bucketName, objectName, opts)
}

func (s *UserNodeStruct) HeadObject(ctx context.Context, bucketName, objectName string) (types.ObjectInfo, error) {
	return s.Internal.HeadObject(ctx, bucketName, objectName)
}

func (s *UserNodeStruct) ListObjects(ctx context.Context, bucketName string, opts types.ListObjectsOptions) (types.ListObjectsInfo, error) {
	return s.Internal.ListObjects(ctx, bucketName, opts)
}

func (s *UserNodeStruct) LfsGetInfo(ctx context.Context, update bool) (types.LfsInfo, error) {
	return s.Internal.LfsGetInfo(ctx, update)
}

func (s *UserNodeStruct) ShowStorage(ctx context.Context) (uint64, error) {
	return s.Internal.ShowStorage(ctx)
}

func (s *UserNodeStruct) ShowBucketStorage(ctx context.Context, bucketName string) (uint64, error) {
	return s.Internal.ShowBucketStorage(ctx, bucketName)
}

func (s *UserNodeStruct) OrderList(ctx context.Context) ([]uint64, error) {
	return s.Internal.OrderList(ctx)
}

func (s *UserNodeStruct) OrderGetJobInfo(ctx context.Context) ([]*OrderJobInfo, error) {
	return s.Internal.OrderGetJobInfo(ctx)
}

func (s *UserNodeStruct) OrderGetJobInfoAt(ctx context.Context, proID uint64) (*OrderJobInfo, error) {
	return s.Internal.OrderGetJobInfoAt(ctx, proID)
}

func (s *UserNodeStruct) OrderGetPayInfo(ctx context.Context) ([]*types.OrderPayInfo, error) {
	return s.Internal.OrderGetPayInfo(ctx)
}

func (s *UserNodeStruct) OrderGetPayInfoAt(ctx context.Context, proID uint64) (*types.OrderPayInfo, error) {
	return s.Internal.OrderGetPayInfoAt(ctx, proID)
}

func (s *UserNodeStruct) OrderGetDetail(ctx context.Context, id, nonce uint64, seqNum uint32) (*types.SignedOrderSeq, error) {
	return s.Internal.OrderGetDetail(ctx, id, nonce, seqNum)
}

type KeeperNodeStruct struct {
	CommonStruct
}
