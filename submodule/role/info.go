package role

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/memoio/go-mefs-v2/api"
	"github.com/memoio/go-mefs-v2/lib/address"
	"github.com/memoio/go-mefs-v2/lib/crypto/signature"
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

	infos map[uint64]pb.RoleInfo // get from chain

	users     []uint64 // related role
	keepers   []uint64
	providers []uint64

	ds store.KVStore
}

func New(ctx context.Context, roleID, groupID uint64, ds store.KVStore, iw api.IWallet) (*RoleMgr, error) {
	rm := &RoleMgr{
		IWallet: iw,
		ctx:     ctx,
		roleID:  roleID,
		groupID: groupID,
		infos:   make(map[uint64]pb.RoleInfo),
		ds:      ds,
	}

	data, err := ds.Get([]byte(strconv.Itoa(int(pb.MetaType_RoleInfoKey))))
	if err != nil {
		return nil, err
	}

	ri := new(pb.RoleInfo)
	err = proto.Unmarshal(data, ri)
	if err != nil {
		return nil, err
	}

	if ri.ID != roleID {
		fmt.Println("roleID not equal")
	}

	rm.infos[roleID] = *ri

	return rm, nil
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

func (rm *RoleMgr) RoleSelf() (pb.RoleInfo, error) {
	rm.RLock()
	defer rm.RUnlock()

	ri, ok := rm.infos[rm.roleID]
	if ok {
		return ri, nil
	}
	return pb.RoleInfo{}, ErrNotFound
}

func (rm *RoleMgr) RoleGet(id uint64) (pb.RoleInfo, error) {
	rm.RLock()
	defer rm.RUnlock()
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

func (rm *RoleMgr) AddRoleInfo(ri pb.RoleInfo) {
	rm.Lock()
	defer rm.Unlock()
	fmt.Println("add role info for: ", ri.ID)
	_, ok := rm.infos[ri.ID]
	if !ok {
		switch ri.Type {
		case pb.RoleInfo_Keeper:
			rm.keepers = append(rm.keepers, ri.ID)
		case pb.RoleInfo_Provider:
			rm.providers = append(rm.providers, ri.ID)
		case pb.RoleInfo_User:
			rm.users = append(rm.users, ri.ID)
		default:
			return
		}

		rm.infos[ri.ID] = ri
	}
}

func (rm *RoleMgr) GetPubKey(roleID uint64) []byte {
	rm.RLock()
	defer rm.RUnlock()
	ri, ok := rm.infos[roleID]
	if ok {
		return ri.ChainVerifyKey
	}
	return nil
}

func (rm *RoleMgr) GetBlsPubKey(roleID uint64) []byte {
	rm.RLock()
	defer rm.RUnlock()

	ri, ok := rm.infos[roleID]
	if ok {
		return ri.BlsVerifyKey
	}
	return nil
}

func (rm *RoleMgr) Sign(msg []byte, typ types.SigType) (types.Signature, error) {
	ts := types.Signature{
		Type: typ,
	}

	switch typ {
	case types.SigSecp256k1:
		sig, err := rm.WalletSign(rm.ctx, rm.localAddr, msg)
		if err != nil {
			return ts, err
		}
		ts.Data = sig
	case types.SigBLS:
		sig, err := rm.WalletSign(rm.ctx, rm.blsAddr, msg)
		if err != nil {
			return ts, err
		}
		ts.Data = sig
	default:
		return ts, ErrNotFound
	}

	return ts, nil
}

func (rm *RoleMgr) Verify(id uint64, msg []byte, sig types.Signature) bool {
	var pubByte []byte
	switch sig.Type {
	case types.SigSecp256k1:
		pubByte = rm.GetPubKey(id)
	case types.SigBLS:
		pubByte = rm.GetBlsPubKey(id)
	default:
		return false
	}

	if len(pubByte) == 0 {
		return false
	}

	ok, err := signature.Verify(pubByte, msg, sig.Data)
	if err != nil {
		return false
	}

	return ok
}
