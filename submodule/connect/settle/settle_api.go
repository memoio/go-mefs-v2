package settle

import (
	"encoding/binary"

	"github.com/memoio/go-mefs-v2/lib/address"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/types/store"
)

type ISettle interface {
	GetRoleID() uint64
	GetGroupID() uint64
	GetThreshold() int

	GetRoleInfoAt(uint64) (*pb.RoleInfo, error)

	GetAllAddrs(ds store.KVStore)
}

var _ ISettle = &fakeSettle{}

type fakeSettle struct {
	localAddr address.Address
}

func NewFakeSettle(lAddr address.Address) ISettle {
	return &fakeSettle{
		localAddr: lAddr,
	}
}

func (f *fakeSettle) GetRoleID() uint64 {
	return binary.BigEndian.Uint64(f.localAddr.Bytes())
}

func (f *fakeSettle) GetGroupID() uint64 {
	return 0
}

func (f *fakeSettle) GetThreshold() int {
	return 7
}

func (f *fakeSettle) GetRoleInfoAt(rid uint64) (*pb.RoleInfo, error) {
	return nil, nil
}

func (f *fakeSettle) GetAllAddrs(ds store.KVStore) {
}
