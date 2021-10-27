package api

import (
	"context"
	"fmt"

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
	}
}

type FullNodeStruct struct {
	CommonStruct

	Internal struct {
		NetAddrInfo func(context.Context) (peer.AddrInfo, error)
		WalletList  func(context.Context) ([]address.Address, error)
		WalletNew   func(types.KeyType) (address.Address, error)
	}
}

func (s *CommonStruct) AuthVerify(ctx context.Context, token string) ([]auth.Permission, error) {
	return s.Internal.AuthVerify(ctx, token)
}

func (s *CommonStruct) AuthNew(ctx context.Context, perms []auth.Permission) ([]byte, error) {
	return s.Internal.AuthNew(ctx, perms)
}

func (s *FullNodeStruct) NetAddrInfo(ctx context.Context) (peer.AddrInfo, error) {
	return s.Internal.NetAddrInfo(ctx)
}

func (s *FullNodeStruct) WalletList(ctx context.Context) ([]address.Address, error) {
	res, err := s.Internal.WalletList(ctx)
	fmt.Println("why", len(res))
	return res, err
}

func (s *FullNodeStruct) WalletNew(typ types.KeyType) (address.Address, error) {
	return s.Internal.WalletNew(typ)
}
