package api

import (
	"context"
	"io"

	"github.com/filecoin-project/go-jsonrpc/auth"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/memoio/go-mefs-v2/lib/address"
	mSign "github.com/memoio/go-mefs-v2/lib/multiSign"
	"github.com/memoio/go-mefs-v2/lib/pb"
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

		RoleSelf        func(context.Context) (pb.RoleInfo, error)                            `perm:"read"`
		RoleGet         func(context.Context, uint64) (pb.RoleInfo, error)                    `perm:"read"`
		RoleGetRelated  func(context.Context, pb.RoleInfo_Type) ([]uint64, error)             `perm:"read"`
		RoleSign        func(context.Context, []byte, types.SigType) (types.Signature, error) `perm:"write"`
		RoleVerify      func(context.Context, uint64, []byte, types.Signature) (bool, error)  `perm:"read"`
		RoleVerifyMulti func(context.Context, []byte, mSign.MultiSignature) (bool, error)     `perm:"read"`
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

func (s *CommonStruct) RoleSelf(ctx context.Context) (pb.RoleInfo, error) {
	return s.Internal.RoleSelf(ctx)
}

func (s *CommonStruct) RoleGet(ctx context.Context, id uint64) (pb.RoleInfo, error) {
	return s.Internal.RoleGet(ctx, id)
}

func (s *CommonStruct) RoleGetRelated(ctx context.Context, typ pb.RoleInfo_Type) ([]uint64, error) {
	return s.Internal.RoleGetRelated(ctx, typ)
}

func (s *CommonStruct) RoleSign(ctx context.Context, msg []byte, typ types.SigType) (types.Signature, error) {
	return s.Internal.RoleSign(ctx, msg, typ)
}

func (s *CommonStruct) RoleVerify(ctx context.Context, id uint64, msg []byte, sig types.Signature) (bool, error) {
	return s.Internal.RoleVerify(ctx, id, msg, sig)
}

func (s *CommonStruct) RoleVerifyMulti(ctx context.Context, msg []byte, sig mSign.MultiSignature) (bool, error) {
	return s.Internal.RoleVerifyMulti(ctx, msg, sig)
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
