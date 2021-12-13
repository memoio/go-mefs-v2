package api

import (
	"context"
	"io"

	"github.com/filecoin-project/go-jsonrpc/auth"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/memoio/go-mefs-v2/lib/address"
	pdpcommon "github.com/memoio/go-mefs-v2/lib/crypto/pdp/common"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
)

// common API permissions constraints
type CommonStruct struct {
	Internal struct {
		AuthVerify func(ctx context.Context, token string) ([]auth.Permission, error) `perm:"read"`
		AuthNew    func(ctx context.Context, erms []auth.Permission) ([]byte, error)  `perm:"admin"`

		ConfigSet func(context.Context, string, string) error        `perm:"write"`
		ConfigGet func(context.Context, string) (interface{}, error) `perm:"write"`

		WalletNew    func(context.Context, types.KeyType) (address.Address, error)  `perm:"write"`
		WalletSign   func(context.Context, address.Address, []byte) ([]byte, error) `perm:"sign"`
		WalletList   func(context.Context) ([]address.Address, error)               `perm:"write"`
		WalletHas    func(context.Context, address.Address) (bool, error)           `perm:"write"`
		WalletDelete func(context.Context, address.Address) error                   `perm:"write"`
		WalletExport func(context.Context, address.Address) (*types.KeyInfo, error) `perm:"write"`
		WalletImport func(context.Context, *types.KeyInfo) (address.Address, error) `perm:"write"`

		NetAddrInfo func(context.Context) (peer.AddrInfo, error) `perm:"write"`

		RoleSelf        func(context.Context) (*pb.RoleInfo, error)                                   `perm:"read"`
		RoleGet         func(context.Context, uint64) (*pb.RoleInfo, error)                           `perm:"read"`
		RoleGetRelated  func(context.Context, pb.RoleInfo_Type) ([]uint64, error)                     `perm:"read"`
		RoleSign        func(context.Context, uint64, []byte, types.SigType) (types.Signature, error) `perm:"write"`
		RoleVerify      func(context.Context, uint64, []byte, types.Signature) (bool, error)          `perm:"read"`
		RoleVerifyMulti func(context.Context, []byte, types.MultiSignature) (bool, error)             `perm:"read"`
		RoleSanityCheck func(context.Context, *tx.SignedMessage) (bool, error)                        `perm:"read"`

		GetRoot   func(context.Context) types.MsgID `perm:"read"`
		GetHeight func(context.Context) uint64      `perm:"read"`
		GetSlot   func(context.Context) uint64      `perm:"read"`

		GetChalEpoch       func(context.Context) uint64                            `perm:"read"`
		GetChalEpochInfo   func(context.Context) *types.ChalEpoch                  `perm:"read"`
		GetChalEpochInfoAt func(context.Context, uint64) (*types.ChalEpoch, error) `perm:"read"`

		GetNonce func(context.Context, uint64) uint64 `perm:"read"`

		GetUsersForPro func(context.Context, uint64) []uint64 `perm:"read"`
		GetProsForUser func(context.Context, uint64) []uint64 `perm:"read"`
		GetAllUsers    func(context.Context) []uint64         `perm:"read"`

		GetAllKeepers   func(context.Context) []uint64                             `perm:"read"`
		GetPDPPublicKey func(context.Context, uint64) (pdpcommon.PublicKey, error) `perm:"read"`
		GetBucket       func(context.Context, uint64) uint64                       `perm:"read"`

		GetOrderState   func(context.Context, uint64, uint64) *types.NonceSeq                          `perm:"read"`
		GetPostIncome   func(context.Context, uint64, uint64) *types.PostIncome                        `perm:"read"`
		GetPostIncomeAt func(context.Context, uint64, uint64, uint64) (*types.SignedPostIncome, error) `perm:"read"`
	}
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

func (s *CommonStruct) WalletExport(ctx context.Context, addr address.Address) (*types.KeyInfo, error) {
	return s.Internal.WalletExport(ctx, addr)
}

func (s *CommonStruct) WalletImport(ctx context.Context, ki *types.KeyInfo) (address.Address, error) {
	return s.Internal.WalletImport(ctx, ki)
}

func (s *CommonStruct) NetAddrInfo(ctx context.Context) (peer.AddrInfo, error) {
	return s.Internal.NetAddrInfo(ctx)
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

func (s *CommonStruct) GetRoot(ctx context.Context) types.MsgID {
	return s.Internal.GetRoot(ctx)
}

func (s *CommonStruct) GetHeight(ctx context.Context) uint64 {
	return s.Internal.GetHeight(ctx)
}

func (s *CommonStruct) GetSlot(ctx context.Context) uint64 {
	return s.Internal.GetSlot(ctx)
}

func (s *CommonStruct) GetChalEpoch(ctx context.Context) uint64 {
	return s.Internal.GetChalEpoch(ctx)
}

func (s *CommonStruct) GetChalEpochInfo(ctx context.Context) *types.ChalEpoch {
	return s.Internal.GetChalEpochInfo(ctx)
}

func (s *CommonStruct) GetChalEpochInfoAt(ctx context.Context, epoch uint64) (*types.ChalEpoch, error) {
	return s.Internal.GetChalEpochInfoAt(ctx, epoch)
}

func (s *CommonStruct) GetNonce(ctx context.Context, roleID uint64) uint64 {
	return s.Internal.GetNonce(ctx, roleID)
}

func (s *CommonStruct) GetUsersForPro(ctx context.Context, proID uint64) []uint64 {
	return s.Internal.GetUsersForPro(ctx, proID)
}

func (s *CommonStruct) GetProsForUser(ctx context.Context, userID uint64) []uint64 {
	return s.Internal.GetProsForUser(ctx, userID)
}

func (s *CommonStruct) GetAllUsers(ctx context.Context) []uint64 {
	return s.Internal.GetAllUsers(ctx)
}

func (s *CommonStruct) GetAllKeepers(ctx context.Context) []uint64 {
	return s.Internal.GetAllKeepers(ctx)
}

func (s *CommonStruct) GetPDPPublicKey(ctx context.Context, userID uint64) (pdpcommon.PublicKey, error) {
	return s.Internal.GetPDPPublicKey(ctx, userID)
}

func (s *CommonStruct) GetBucket(ctx context.Context, userID uint64) uint64 {
	return s.Internal.GetBucket(ctx, userID)
}

func (s *CommonStruct) GetOrderState(ctx context.Context, userID, proID uint64) *types.NonceSeq {
	return s.Internal.GetOrderState(ctx, userID, proID)
}

func (s *CommonStruct) GetPostIncome(ctx context.Context, userID, proID uint64) *types.PostIncome {
	return s.Internal.GetPostIncome(ctx, userID, proID)
}

func (s *CommonStruct) GetPostIncomeAt(ctx context.Context, userID, proID, epoch uint64) (*types.SignedPostIncome, error) {
	return s.Internal.GetPostIncomeAt(ctx, userID, proID, epoch)
}

type FullNodeStruct struct {
	CommonStruct
}

type ProviderNodeStruct struct {
	CommonStruct
}

type UserNodeStruct struct {
	CommonStruct

	Internal struct {
		CreateBucket func(ctx context.Context, bucketName string, opts *pb.BucketOption) (*types.BucketInfo, error)                                      `perm:"write"`
		DeleteBucket func(ctx context.Context, bucketName string) (*types.BucketInfo, error)                                                             `perm:"write"`
		PutObject    func(ctx context.Context, bucketName, objectName string, reader io.Reader, opts *types.PutObjectOptions) (*types.ObjectInfo, error) `perm:"write"`
		DeleteObject func(ctx context.Context, bucketName, objectName string) (*types.ObjectInfo, error)                                                 `perm:"write"`

		HeadBucket  func(ctx context.Context, bucketName string) (*types.BucketInfo, error) `perm:"read"`
		ListBuckets func(ctx context.Context, prefix string) ([]*types.BucketInfo, error)   `perm:"read"`

		GetObject   func(ctx context.Context, bucketName, objectName string, opts *types.DownloadObjectOptions) ([]byte, error) `perm:"read"`
		HeadObject  func(ctx context.Context, bucketName, objectName string) (*types.ObjectInfo, error)                         `perm:"read"`
		ListObjects func(ctx context.Context, bucketName string, opts *types.ListObjectsOptions) ([]*types.ObjectInfo, error)   `perm:"read"`

		ShowStorage       func(ctx context.Context) (uint64, error)                    `perm:"read"`
		ShowBucketStorage func(ctx context.Context, bucketName string) (uint64, error) `perm:"read"`
	}
}

func (s *UserNodeStruct) CreateBucket(ctx context.Context, bucketName string, options *pb.BucketOption) (*types.BucketInfo, error) {
	return s.Internal.CreateBucket(ctx, bucketName, options)
}

func (s *UserNodeStruct) DeleteBucket(ctx context.Context, bucketName string) (*types.BucketInfo, error) {
	return s.Internal.DeleteBucket(ctx, bucketName)
}

func (s *UserNodeStruct) PutObject(ctx context.Context, bucketName, objectName string, reader io.Reader, opts *types.PutObjectOptions) (*types.ObjectInfo, error) {
	return s.Internal.PutObject(ctx, bucketName, objectName, reader, opts)
}

func (s *UserNodeStruct) DeleteObject(ctx context.Context, bucketName, objectName string) (*types.ObjectInfo, error) {
	return s.Internal.DeleteObject(ctx, bucketName, objectName)
}

func (s *UserNodeStruct) HeadBucket(ctx context.Context, bucketName string) (*types.BucketInfo, error) {
	return s.Internal.HeadBucket(ctx, bucketName)
}

func (s *UserNodeStruct) ListBuckets(ctx context.Context, prefix string) ([]*types.BucketInfo, error) {
	return s.Internal.ListBuckets(ctx, prefix)
}

func (s *UserNodeStruct) GetObject(ctx context.Context, bucketName, objectName string, opts *types.DownloadObjectOptions) ([]byte, error) {
	return s.Internal.GetObject(ctx, bucketName, objectName, opts)
}

func (s *UserNodeStruct) HeadObject(ctx context.Context, bucketName, objectName string) (*types.ObjectInfo, error) {
	return s.Internal.HeadObject(ctx, bucketName, objectName)
}

func (s *UserNodeStruct) ListObjects(ctx context.Context, bucketName string, opts *types.ListObjectsOptions) ([]*types.ObjectInfo, error) {
	return s.Internal.ListObjects(ctx, bucketName, opts)
}

func (s *UserNodeStruct) ShowStorage(ctx context.Context) (uint64, error) {
	return s.Internal.ShowStorage(ctx)
}

func (s *UserNodeStruct) ShowBucketStorage(ctx context.Context, bucketName string) (uint64, error) {
	return s.Internal.ShowBucketStorage(ctx, bucketName)
}

type KeeperNodeStruct struct {
	CommonStruct

	Internal struct {
	}
}
