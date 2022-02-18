package settle

import (
	"context"
	"encoding/binary"
	"math/big"

	"github.com/memoio/go-mefs-v2/api"
	"github.com/memoio/go-mefs-v2/lib/address"
	"github.com/memoio/go-mefs-v2/lib/pb"
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

func (f *fakeSettle) SettleGetRoleID(ctx context.Context) uint64 {
	return binary.BigEndian.Uint64(f.localAddr.Bytes())
}

func (f *fakeSettle) SettleGetGroupID(ctx context.Context) uint64 {
	return 0
}

func (f *fakeSettle) SettleGetThreshold(ctx context.Context) int {
	return 7
}

func (f *fakeSettle) SettleGetRoleInfoAt(ctx context.Context, rid uint64) (*pb.RoleInfo, error) {
	return nil, nil
}

func (f *fakeSettle) SettleGetGroupInfoAt(ctx context.Context, rid uint64) (*api.GroupInfo, error) {
	return nil, nil
}

func (f *fakeSettle) SettleGetBalanceInfo(context.Context, uint64) (*api.BalanceInfo, error) {
	return nil, nil
}

func (f *fakeSettle) SettleGetPledgeInfo(context.Context, uint64) (*api.PledgeInfo, error) {
	return nil, nil
}

func (f *fakeSettle) SettleGetStoreInfo(context.Context, uint64, uint64) (*api.StoreInfo, error) {
	return nil, nil
}

func (f *fakeSettle) SettleWithdraw(context.Context, *big.Int, *big.Int, []uint64, [][]byte) error {
	return nil
}

func (f *fakeSettle) SettlePledge(context.Context, *big.Int) error {
	return nil
}

func (f *fakeSettle) SettleCanclePledge(context.Context, *big.Int) error {
	return nil
}
