package settle

import (
	"context"
	"encoding/binary"
	"math/big"

	"github.com/memoio/go-mefs-v2/api"
	"github.com/memoio/go-mefs-v2/lib/address"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/types/store"
)

var _ api.ISettle = &fakeSettle{}

type fakeSettle struct {
	localAddr address.Address
}

func NewFakeSettle(lAddr address.Address) api.ISettle {
	return &fakeSettle{
		localAddr: lAddr,
	}
}

func (f *fakeSettle) GetRoleID(ctx context.Context) uint64 {
	return binary.BigEndian.Uint64(f.localAddr.Bytes())
}

func (f *fakeSettle) GetGroupID(ctx context.Context) uint64 {
	return 0
}

func (f *fakeSettle) GetThreshold(ctx context.Context) int {
	return 7
}

func (f *fakeSettle) GetRoleInfoAt(ctx context.Context, rid uint64) (*pb.RoleInfo, error) {
	return nil, nil
}

func (f *fakeSettle) GetAllAddrs(context.Context, store.KVStore) {
}

func (f *fakeSettle) GetBalance(context.Context, uint64) (*big.Int, error) {
	return nil, nil
}

func (f *fakeSettle) Withdraw(context.Context, *big.Int) error {
	return nil
}
