package api

import (
	"context"

	"github.com/filecoin-project/go-jsonrpc/auth"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/memoio/go-mefs-v2/lib/address"
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
	}
}

type FullNodeStruct struct {
	CommonStruct

	Internal struct {
		NetAddrInfo func(context.Context) (peer.AddrInfo, error) `perm:"write"`
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

func (s *FullNodeStruct) NetAddrInfo(ctx context.Context) (peer.AddrInfo, error) {
	return s.Internal.NetAddrInfo(ctx)
}
