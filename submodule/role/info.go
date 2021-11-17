package role

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/memoio/go-mefs-v2/api"
	"github.com/memoio/go-mefs-v2/lib/address"
	pdpcommon "github.com/memoio/go-mefs-v2/lib/crypto/pdp/common"
	pdpv2 "github.com/memoio/go-mefs-v2/lib/crypto/pdp/version2"
	logging "github.com/memoio/go-mefs-v2/lib/log"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/types/store"
)

var logger = logging.Logger("roleinfo")

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
		logger.Debug("roleID not equal")
	}

	localAddr, _ := address.NewAddress(ri.ChainVerifyKey)
	blsAddr, _ := address.NewAddress(ri.BlsVerifyKey)

	rm := &RoleMgr{
		IWallet:   iw,
		ctx:       ctx,
		roleID:    roleID,
		groupID:   groupID,
		infos:     make(map[uint64]pb.RoleInfo),
		ds:        ds,
		localAddr: localAddr,
		blsAddr:   blsAddr,
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
	logger.Debug("add role info for: ", ri.ID)
	_, ok := rm.infos[ri.ID]
	if !ok {
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

func (rm *RoleMgr) RoleGetKeyset() (pdpcommon.KeySet, error) {
	ki, err := rm.WalletExport(rm.ctx, rm.localAddr)
	if err != nil {
		return nil, err
	}

	privBytes := ki.SecretKey

	keyset, err := pdpv2.GenKeySetWithSeed(privBytes, pdpv2.SCount)
	if err != nil {
		return nil, err
	}
	return keyset, nil
}
