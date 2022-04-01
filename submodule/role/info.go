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
	"github.com/memoio/go-mefs-v2/submodule/connect/settle"
)

type RoleMgr struct {
	sync.RWMutex
	api.IWallet

	is *settle.ContractMgr

	ctx     context.Context
	roleID  uint64
	groupID uint64

	infos map[uint64]*pb.RoleInfo // get from chain

	users     []uint64 // related role
	keepers   []uint64
	providers []uint64

	ds store.KVStore
}

func New(ctx context.Context, roleID, groupID uint64, ds store.KVStore, iw api.IWallet, is *settle.ContractMgr) (*RoleMgr, error) {
	// cureent node is registered
	ri, err := is.SettleGetRoleInfoAt(ctx, roleID)
	if err != nil {
		return nil, err
	}

	rm := &RoleMgr{
		IWallet: iw,
		is:      is,
		ctx:     ctx,
		roleID:  roleID,
		groupID: groupID,
		infos:   make(map[uint64]*pb.RoleInfo),
		ds:      ds,
	}

	rm.addRoleInfo(ri, true)

	return rm, nil
}

func (rm *RoleMgr) API() *roleAPI {
	return &roleAPI{rm}
}

func (rm *RoleMgr) loadpro() {
	key := store.NewKey(pb.MetaType_RoleInfoKey, rm.groupID, pb.RoleInfo_Provider.String())
	val, err := rm.ds.Get(key)
	if err != nil {
		return
	}

	for i := 0; i < len(val)/8; i++ {
		pid := binary.BigEndian.Uint64(val[8*i : 8*(i+1)])
		rm.get(pid)
	}
}

func (rm *RoleMgr) loadkeeper() {
	key := store.NewKey(pb.MetaType_RoleInfoKey, rm.groupID, pb.RoleInfo_Keeper.String())
	val, err := rm.ds.Get(key)
	if err != nil {
		return
	}

	for i := 0; i < len(val)/8; i++ {
		pid := binary.BigEndian.Uint64(val[8*i : 8*(i+1)])
		rm.get(pid)
	}
}

func (rm *RoleMgr) Start() {
	logger.Debug("start sync from remote chain")

	rm.loadkeeper()
	rm.loadpro()

	go rm.syncFromChain()
}

func (rm *RoleMgr) syncFromChain() {
	//get all addrs and added it into roleMgr
	tc := time.NewTicker(30 * time.Second)
	defer tc.Stop()

	key := store.NewKey(pb.MetaType_RoleInfoKey)

	kCnt := uint64(0)
	pCnt := uint64(0)
	uCnt := uint64(0)

	val, _ := rm.ds.Get(key)
	if len(val) >= 24 {
		kCnt = binary.BigEndian.Uint64(val[:8])
		pCnt = binary.BigEndian.Uint64(val[8:16])
		uCnt = binary.BigEndian.Uint64(val[16:24])
	}

	for {
		select {
		case <-rm.ctx.Done():
			return
		case <-tc.C:
			if rm.groupID == 0 {
				continue
			}

			kcnt, err := rm.is.GetGKNum()
			if err != nil {
				logger.Debugf("get group %d keeper count fail %s", rm.groupID, err)
				continue
			}
			if kcnt > kCnt {
				nkey := store.NewKey(pb.MetaType_RoleInfoKey, rm.groupID, pb.RoleInfo_Keeper.String())
				data, _ := rm.ds.Get(nkey)
				for i := kCnt; i < kcnt; i++ {
					kindex, err := rm.is.GetGroupK(rm.groupID, i)
					if err != nil {
						logger.Debugf("get group %d keeper %d fail %s", rm.groupID, i, err)
						continue
					}

					buf := make([]byte, 8)
					binary.BigEndian.PutUint64(buf, kindex)
					data = append(data, buf...)

					pri, err := rm.is.SettleGetRoleInfoAt(context.TODO(), kindex)
					if err != nil {
						continue
					}

					rm.AddRoleInfo(pri)
				}
				kCnt = kcnt

				rm.ds.Put(nkey, data)
			} else {
				if len(rm.keepers) != int(kcnt) {
					// reload
					logger.Debug("reload keepers: ", kcnt)
					data := make([]byte, int(kcnt)*8)
					for i := uint64(0); i < kcnt; i++ {
						kindex, err := rm.is.GetGroupK(rm.groupID, i)
						if err != nil {
							logger.Debugf("get group %d keeper %d fail %s", rm.groupID, i, err)
							continue
						}
						binary.BigEndian.PutUint64(data[i*8:(i+1)*8], kindex)

						rm.Lock()
						rm.get(kindex)
						rm.Unlock()
					}
					nkey := store.NewKey(pb.MetaType_RoleInfoKey, rm.groupID, pb.RoleInfo_Keeper.String())
					rm.ds.Put(nkey, data)
				}
			}

			ucnt, pcnt, err := rm.is.GetGUPNum()
			if err != nil {
				logger.Debugf("get group %d user-pro count fail %s", rm.groupID, err)
				continue
			}
			if pcnt > pCnt {
				nkey := store.NewKey(pb.MetaType_RoleInfoKey, rm.groupID, pb.RoleInfo_Provider.String())
				data, _ := rm.ds.Get(nkey)
				for i := pCnt; i < pcnt; i++ {
					pindex, err := rm.is.GetGroupP(rm.groupID, i)
					if err != nil {
						logger.Debugf("get group %d pro %d fail %s", rm.groupID, i, err)
						continue
					}

					buf := make([]byte, 8)
					binary.BigEndian.PutUint64(buf, pindex)
					data = append(data, buf...)

					pri, err := rm.is.SettleGetRoleInfoAt(context.TODO(), pindex)
					if err != nil {
						continue
					}

					rm.AddRoleInfo(pri)
				}
				pCnt = pcnt

				rm.ds.Put(nkey, data)
			} else {
				if len(rm.providers) != int(pcnt) {
					// reload
					logger.Debug("reload providers: ", len(rm.providers), pcnt)
					data := make([]byte, int(pcnt)*8)
					for i := uint64(0); i < pcnt; i++ {
						pindex, err := rm.is.GetGroupP(rm.groupID, i)
						if err != nil {
							logger.Debugf("get group %d pro %d fail %s", rm.groupID, i, err)
							continue
						}

						logger.Debugf("load provider: %d %d %d", rm.groupID, i, pindex)

						binary.BigEndian.PutUint64(data[i*8:(i+1)*8], pindex)
						rm.Lock()
						rm.get(pindex)
						rm.Unlock()
						nkey := store.NewKey(pb.MetaType_RoleInfoKey, rm.groupID, pb.RoleInfo_Provider.String())
						rm.ds.Put(nkey, data)
					}
				}
			}

			if ucnt > uCnt {
				for i := uCnt; i < ucnt; i++ {
					uindex, err := rm.is.GetGroupU(rm.groupID, i)
					if err != nil {
						logger.Debugf("get group %d user %d fail %s", rm.groupID, i, err)
						continue
					}

					pri, err := rm.is.SettleGetRoleInfoAt(context.TODO(), uindex)
					if err != nil {
						continue
					}

					rm.AddRoleInfo(pri)
				}
				uCnt = ucnt
			}

			logger.Debug("sync from chain: ", rm.groupID, kCnt, pCnt, uCnt)

			buf := make([]byte, 24)
			binary.BigEndian.PutUint64(buf[:8], kCnt)
			binary.BigEndian.PutUint64(buf[8:16], pCnt)
			binary.BigEndian.PutUint64(buf[16:24], uCnt)
			rm.ds.Put(key, buf)
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

		if save {
			key := store.NewKey(pb.MetaType_RoleInfoKey, ri.RoleID)

			pbyte, err := proto.Marshal(ri)
			if err != nil {
				return
			}
			rm.ds.Put(key, pbyte)
		}
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
