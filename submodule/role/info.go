package role

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/memoio/go-mefs-v2/api"
	"github.com/memoio/go-mefs-v2/lib/address"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
)

var ErrNotFound = errors.New("not found")

type RoleMgr struct {
	sync.RWMutex
	api.IWallet

	ctx     context.Context
	roleID  uint64
	groupID uint64

	localAddr address.Address
	blsAddr   address.Address

	self  pb.RoleInfo            // local node info
	infos map[uint64]pb.RoleInfo // get from chain

	users     []uint64 // related role
	keepers   []uint64
	providers []uint64

	ds store.KVStore
}

func New(ctx context.Context, roleID, groupID uint64, ds store.KVStore) *RoleMgr {
	rm := &RoleMgr{
		ctx:     ctx,
		roleID:  roleID,
		groupID: groupID,
		infos:   make(map[uint64]pb.RoleInfo),
		ds:      ds,
	}
	return rm
}

func (rm *RoleMgr) API() *roleAPI {
	return &roleAPI{rm}
}

// load infos from local store
func (rm *RoleMgr) load() {
	// load pb.NodeInfo from local
}

// save infos to local store
func (rm *RoleMgr) save() {

}

func (rm *RoleMgr) RoleGet(id uint64) (pb.RoleInfo, error) {
	ri, ok := rm.infos[id]
	if ok {
		return ri, nil
	}
	return pb.RoleInfo{}, ErrNotFound
}

func (rm *RoleMgr) RoleGetRelated(typ pb.RoleInfo_Type) []uint64 {
	rm.RLock()
	defer rm.RUnlock()

	switch typ {
	case pb.RoleInfo_Keeper:
		out := make([]uint64, len(rm.keepers))
		for i, id := range rm.keepers {
			out[i] = id
		}

		return out
	case pb.RoleInfo_Provider:
		out := make([]uint64, len(rm.providers))
		for i, id := range rm.providers {
			out[i] = id
		}

		return out
	case pb.RoleInfo_User:
		out := make([]uint64, len(rm.users))
		for i, id := range rm.users {
			out[i] = id
		}

		return out
	default:
		return nil
	}
}

func (rm *RoleMgr) Sync(ctx context.Context) {
	t := time.NewTicker(60 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			// load from chain
			rm.SyncFromChain(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (rm *RoleMgr) SyncFromChain(ctx context.Context) {

}

func (rm *RoleMgr) Sign(msg []byte, typ types.KeyType) ([]byte, error) {
	switch typ {
	case types.Secp256k1:
		return rm.WalletSign(rm.ctx, rm.localAddr, msg)
	case types.BLS:
		return rm.WalletSign(rm.ctx, rm.blsAddr, msg)
	default:
		return nil, ErrNotFound
	}
}

func (rm *RoleMgr) Verify(id uint64, msg, sig []byte, typ types.KeyType) {
}
