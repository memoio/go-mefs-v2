package role

import (
	"context"
	"encoding/binary"
	"sync"

	"github.com/gogo/protobuf/proto"
	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/api"
	"github.com/memoio/go-mefs-v2/lib/address"
	"github.com/memoio/go-mefs-v2/lib/crypto/pdp"
	pdpcommon "github.com/memoio/go-mefs-v2/lib/crypto/pdp/common"
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
	ri, err := is.GetRoleInfoAt(ctx, roleID)
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

	rm.loadkeeper()
	rm.loadpro()

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

	go rm.syncFromChain()
}

func (rm *RoleMgr) syncFromChain() {
	rm.is.GetAllAddrs(rm.ds)

	rm.loadkeeper()
	rm.loadpro()
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

	ri, err = rm.is.GetRoleInfoAt(rm.ctx, roleID)
	if err != nil {
		return nil, err
	}

	if ri.GroupID != rm.groupID {
		return nil, xerrors.Errorf("not my group")
	}

	rm.addRoleInfo(ri, true)

	return ri, nil
}

func (rm *RoleMgr) addRoleInfo(ri *pb.RoleInfo, save bool) {
	_, ok := rm.infos[ri.ID]
	if !ok {
		logger.Debug("add role info for: ", ri.ID)
		rm.infos[ri.ID] = ri

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

		if save {
			key := store.NewKey(pb.MetaType_RoleInfoKey, ri.ID)

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

func (rm *RoleMgr) RoleGetKeyset(roleID uint64) (pdpcommon.KeySet, error) {
	addr, err := rm.GetPubKey(roleID, types.Secp256k1)
	if err != nil {
		return nil, err
	}

	ki, err := rm.WalletExport(rm.ctx, addr)
	if err != nil {
		return nil, err
	}

	privBytes := ki.SecretKey

	privBytes = append(privBytes, byte(types.PDP))

	keyset, err := pdp.GenerateKeyWithSeed(pdpcommon.PDPV2, privBytes)
	if err != nil {
		return nil, err
	}
	return keyset, nil
}
