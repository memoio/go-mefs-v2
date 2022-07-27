package role

import (
	"context"
	"encoding/binary"
	"sync"
	"time"

	"github.com/gogo/protobuf/proto"
	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/api"
	"github.com/memoio/go-mefs-v2/lib/address"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/store"
)

type RoleMgr struct {
	sync.RWMutex
	api.IWallet

	is api.ISettle

	ctx     context.Context
	roleID  uint64
	groupID uint64

	infos map[uint64]*pb.RoleInfo // get from chain

	users     []uint64 // related role
	keepers   []uint64
	providers []uint64

	ds store.KVStore
}

func New(ctx context.Context, roleID, groupID uint64, ds store.KVStore, iw api.IWallet, is api.ISettle) (*RoleMgr, error) {
	rm := &RoleMgr{
		IWallet:   iw,
		is:        is,
		ctx:       ctx,
		roleID:    roleID,
		groupID:   groupID,
		infos:     make(map[uint64]*pb.RoleInfo),
		ds:        ds,
		keepers:   make([]uint64, 0, 128),
		providers: make([]uint64, 0, 128),
		users:     make([]uint64, 0, 128),
	}

	ri, err := rm.is.SettleGetRoleInfoAt(rm.ctx, roleID)
	if err != nil {
		return nil, err
	}

	ri.GroupID = rm.groupID

	rm.addRoleInfo(ri, true)

	return rm, nil
}

func (rm *RoleMgr) API() *roleAPI {
	return &roleAPI{rm}
}

func (rm *RoleMgr) load() {
	key := store.NewKey(pb.MetaType_RoleInfoKey, rm.groupID, pb.RoleInfo_Keeper.String())
	val, err := rm.ds.Get(key)
	if err != nil {
		return
	}

	for i := 0; i < len(val)/8; i++ {
		pid := binary.BigEndian.Uint64(val[8*i : 8*(i+1)])
		rm.get(pid)
	}

	key = store.NewKey(pb.MetaType_RoleInfoKey, rm.groupID, pb.RoleInfo_Provider.String())
	val, err = rm.ds.Get(key)
	if err != nil {
		return
	}

	for i := 0; i < len(val)/8; i++ {
		pid := binary.BigEndian.Uint64(val[8*i : 8*(i+1)])
		rm.get(pid)
	}
}

func (rm *RoleMgr) save() {
	val := make([]byte, len(rm.keepers)*8)
	for i, pid := range rm.keepers {
		binary.BigEndian.PutUint64(val[8*i:8*(i+1)], pid)
	}

	key := store.NewKey(pb.MetaType_RoleInfoKey, rm.groupID, pb.RoleInfo_Keeper.String())
	rm.ds.Put(key, val)

	val = make([]byte, len(rm.providers)*8)
	for i, pid := range rm.providers {
		binary.BigEndian.PutUint64(val[8*i:8*(i+1)], pid)
	}

	key = store.NewKey(pb.MetaType_RoleInfoKey, rm.groupID, pb.RoleInfo_Provider.String())
	rm.ds.Put(key, val)
}

func (rm *RoleMgr) Start() {
	logger.Debug("start sync from remote chain")

	rm.load()

	go rm.sync()
}

func (rm *RoleMgr) syncFromChain(cnt, acnt uint64) uint64 {
	//get all addrs and added it into roleMgr
	logger.Debug("sync from settle chain: ", rm.groupID, cnt, acnt)
	i := cnt
	for ; i <= acnt; i++ {
		pri, err := rm.is.SettleGetRoleInfoAt(rm.ctx, i)
		if err != nil {
			break
		}

		if pri.GroupID != rm.groupID {
			continue
		}

		rm.AddRoleInfo(pri)
	}

	return i
}

func (rm *RoleMgr) sync() {
	//get all addrs and added it into roleMgr
	tc := time.NewTicker(30 * time.Second)
	defer tc.Stop()

	ltc := time.NewTicker(5 * time.Minute)
	defer ltc.Stop()

	key := store.NewKey(pb.MetaType_RoleInfoKey)

	cnt := uint64(0)

	val, _ := rm.ds.Get(key)
	if len(val) == 8 {
		cnt = binary.BigEndian.Uint64(val[:8])
	}

	logger.Debug("sync from settle chain at: ", rm.groupID, cnt)

	for {
		select {
		case <-rm.ctx.Done():
			return
		case <-tc.C:
			if rm.groupID == 0 {
				continue
			}

			acnt := rm.is.SettleGetAddrCnt(rm.ctx)
			if acnt > cnt {
				cnt = rm.syncFromChain(cnt+1, acnt)
				buf := make([]byte, 8)
				binary.BigEndian.PutUint64(buf[:8], cnt)
				rm.ds.Put(key, buf)
			}
		case <-ltc.C:
			rm.RLock()
			rm.save()
			rm.RUnlock()
		}
	}
}

func (rm *RoleMgr) get(roleID uint64) (*pb.RoleInfo, error) {
	ri, ok := rm.infos[roleID]
	if ok {
		return ri, nil
	}

	key := store.NewKey(pb.MetaType_RoleInfoKey, roleID)
	val, err := rm.ds.Get(key)
	if err == nil {
		ri = new(pb.RoleInfo)
		err = proto.Unmarshal(val, ri)
		if err == nil {
			rm.addRoleInfo(ri, false)
			return ri, nil
		}
	}

	ri, err = rm.is.SettleGetRoleInfoAt(rm.ctx, roleID)
	if err != nil {
		return nil, err
	}

	if ri.GroupID != rm.groupID {
		logger.Debugf("%d group wrong %d, need %d", roleID, ri.GroupID, rm.groupID)
		return nil, xerrors.Errorf("not my group")
	}

	rm.addRoleInfo(ri, true)

	return ri, nil
}

func (rm *RoleMgr) addRoleInfo(ri *pb.RoleInfo, save bool) {
	_, ok := rm.infos[ri.RoleID]
	if !ok {
		logger.Debug("add role info for: ", ri.RoleID, save)
		rm.infos[ri.RoleID] = ri

		switch ri.Type {
		case pb.RoleInfo_Keeper:
			rm.keepers = append(rm.keepers, ri.RoleID)
		case pb.RoleInfo_Provider:
			rm.providers = append(rm.providers, ri.RoleID)
		case pb.RoleInfo_User:
			rm.users = append(rm.users, ri.RoleID)
		default:
			return
		}
	} else {
		logger.Debug("update role info for: ", ri.RoleID, save)
		rm.infos[ri.RoleID] = ri
	}

	if save {
		key := store.NewKey(pb.MetaType_RoleInfoKey, ri.RoleID)

		pbyte, err := proto.Marshal(ri)
		if err != nil {
			return
		}
		rm.ds.Put(key, pbyte)
	}
}

func (rm *RoleMgr) AddRoleInfo(ri *pb.RoleInfo) {
	rm.Lock()
	defer rm.Unlock()
	rm.addRoleInfo(ri, true)
}

func (rm *RoleMgr) GetPubKey(roleID uint64, typ types.KeyType) (address.Address, error) {
	rm.Lock()
	defer rm.Unlock()
	ri, err := rm.get(roleID)
	if err != nil {
		return address.Undef, err
	}

	switch typ {
	case types.Secp256k1:
		return address.NewAddress(ri.ChainVerifyKey)
	case types.BLS:
		return address.NewAddress(ri.BlsVerifyKey)
	default:
		return address.Undef, xerrors.New("key not supported")
	}
}
