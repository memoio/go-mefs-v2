package settle

import (
	"context"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/gogo/protobuf/proto"
	"golang.org/x/xerrors"

	callconts "memoContract/callcontracts"
	iface "memoContract/interfaces"

	"github.com/memoio/go-mefs-v2/lib/address"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/types/store"
	"github.com/memoio/go-mefs-v2/lib/utils"
)

type ContractMgr struct {
	ctx       context.Context
	ds        store.KVStore
	roleID    uint64
	groupID   uint64
	threshold int
	tIndex    uint32

	eAddr  common.Address // local address
	rAddr  common.Address // role contract address
	tAddr  common.Address // token address
	fsAddr common.Address // fs contract addr

	// Role caller for user
	rUser iface.RoleInfo
}

func NewContractMgr(ctx context.Context, addr address.Address, hexSk string, typ pb.RoleInfo_Type, gIndex uint64, ds store.KVStore) (*ContractMgr, error) {
	// set endpoint

	txopts := &callconts.TxOpts{
		Nonce:    nil,
		GasPrice: big.NewInt(callconts.DefaultGasPrice),
		GasLimit: callconts.DefaultGasLimit,
	}

	eAddr := common.BytesToAddress(utils.ToEthAddress(addr.Bytes()))

	rUser := callconts.NewR(eAddr, hexSk, txopts)

	tokenAddr, err := rUser.RToken(roleAddr)
	if err != nil {
		return nil, err
	}

	cm := &ContractMgr{
		ctx:   ctx,
		ds:    ds,
		eAddr: eAddr,
		rAddr: roleAddr,
		tAddr: tokenAddr,
		rUser: rUser,
	}

	_, _, _, rid, _, _, err := cm.rUser.GetRoleInfo(eAddr, eAddr)
	if err != nil {
		return nil, err
	}

	if rid == 0 {
		// registerRole
		err := cm.RegisterRole()
		if err != nil {
			return nil, err
		}
	}

	_, _, _, rid, gid, _, err := cm.rUser.GetRoleInfo(eAddr, eAddr)
	if err != nil {
		return nil, err
	}

	if rid == 0 {
		return nil, xerrors.Errorf("register fails")
	}

	if gid == 0 {
		switch typ {
		case pb.RoleInfo_Provider:
			// provider: register,register; add to group
			err = cm.RegisterProvider()
			if err != nil {
				return nil, err
			}

			err = cm.AddProviderToGroup(gIndex)
			if err != nil {
				return nil, err
			}

		case pb.RoleInfo_User:
			// user: resgister user
			// get extra byte
			err = cm.RegisterUser(gIndex, nil)
			if err != nil {
				return nil, err
			}

		}
	}

	if rid == 0 {
		return nil, xerrors.Errorf("register group fails")
	}

	err = cm.getGroupInfo(gid)
	if err != nil {
		return nil, err
	}

	cm.roleID = rid
	cm.groupID = gid

	go cm.getAllAddrs()

	return cm, nil
}

// get info
func (cm *ContractMgr) GetRoleInfo(addr address.Address) (*pb.RoleInfo, error) {
	eAddr := common.BytesToAddress(utils.ToEthAddress(addr.Bytes()))
	return cm.getRoleInfo(eAddr)
}

func (cm *ContractMgr) GetRoleInfoAt(rid uint64) (*pb.RoleInfo, error) {
	gotAddr, err := cm.rUser.GetAddr(cm.eAddr, rid)
	if err != nil {
		return nil, err
	}

	return cm.getRoleInfo(gotAddr)
}

func (cm *ContractMgr) getRoleInfo(eAddr common.Address) (*pb.RoleInfo, error) {
	_, _, rType, rid, gid, extra, err := cm.rUser.GetRoleInfo(eAddr, eAddr)
	if err != nil {
		return nil, err
	}

	if len(extra) < 48 {
		return nil, xerrors.Errorf("bls public key length short, expected 48, got %d", len(extra))
	}

	pri := new(pb.RoleInfo)
	pri.ID = rid
	pri.GroupID = gid
	pri.ChainVerifyKey = eAddr.Bytes()

	switch rType {
	case 0:
		pri.Type = pb.RoleInfo_Unknown
	case 1:
		pri.Type = pb.RoleInfo_User
		pri.Extra = extra
	case 2:
		pri.Type = pb.RoleInfo_Provider
	case 3:
		pri.Type = pb.RoleInfo_Keeper
		pri.BlsVerifyKey = extra
	default:
		return nil, xerrors.Errorf("roletype %d unsupported", rType)
	}

	cm.groupID = gid
	cm.roleID = rid

	return pri, nil
}

func (cm *ContractMgr) RegisterRole() error {
	return cm.rUser.Register(cm.rAddr, cm.eAddr, nil)
}

func (cm *ContractMgr) RegisterProvider() error {
	return cm.rUser.RegisterProvider(cm.rAddr, cm.roleID, nil)
}

func (cm *ContractMgr) AddProviderToGroup(gIndex uint64) error {
	return cm.rUser.AddProviderToGroup(cm.rAddr, cm.roleID, gIndex, nil)
}

func (cm *ContractMgr) RegisterUser(gIndex uint64, extra []byte) error {
	return cm.rUser.RegisterUser(cm.rAddr, cm.eAddr, cm.roleID, gIndex, cm.tIndex, extra, nil)
}

func (cm *ContractMgr) getGroupInfo(gIndex uint64) error {
	_, _, _, level, _, _, fsAddr, err := cm.rUser.GetGroupInfo(cm.rAddr, gIndex)
	if err != nil {
		return err
	}

	cm.threshold = int(level)
	cm.fsAddr = fsAddr
	return nil
}

func (cm *ContractMgr) getAllAddrs() {
	//get all addrs and added it into roleMgr
	tc := time.NewTicker(15 * time.Second)
	defer tc.Stop()

	kCnt := uint64(0)
	pCnt := uint64(0)
	uCnt := uint64(0)

	for {
		select {
		case <-cm.ctx.Done():
			return
		case <-tc.C:
			if cm.groupID == 0 {
				continue
			}

			kcnt, err := cm.rUser.GetGKNum(cm.eAddr, cm.groupID)
			if err != nil {
				continue
			}
			if kcnt > kCnt {
				for i := kCnt; i <= kcnt; i++ {
					kindex, err := cm.rUser.GetGroupK(cm.eAddr, cm.groupID, i)
					if err != nil {
						continue
					}

					gotAddr, err := cm.rUser.GetAddr(cm.eAddr, kindex)
					if err != nil {
						continue
					}

					pri, err := cm.getRoleInfo(gotAddr)
					if err != nil {
						continue
					}

					// should not
					if pri.ID == 0 {
						continue
					}
					if pri.GroupID == 0 {
						continue
					}
					if pri.GroupID != cm.groupID {
						continue
					}

					val, err := proto.Marshal(pri)
					if err != nil {
						continue
					}

					// save to local
					key := store.NewKey(pb.MetaType_RoleInfoKey, pri.ID)
					cm.ds.Put(key, val)
				}
				kCnt = kcnt
			}

			ucnt, pcnt, err := cm.rUser.GetGUPNum(cm.eAddr, cm.groupID)
			if err != nil {
				continue
			}
			if pcnt > pCnt {
				for i := pCnt; i <= pcnt; i++ {
					pindex, err := cm.rUser.GetGroupP(cm.eAddr, cm.groupID, i)
					if err != nil {
						continue
					}

					gotAddr, err := cm.rUser.GetAddr(cm.eAddr, pindex)
					if err != nil {
						continue
					}

					pri, err := cm.getRoleInfo(gotAddr)
					if err != nil {
						continue
					}

					// should not
					if pri.ID == 0 {
						continue
					}
					if pri.GroupID == 0 {
						continue
					}
					if pri.GroupID != cm.groupID {
						continue
					}

					val, err := proto.Marshal(pri)
					if err != nil {
						continue
					}

					// save to local
					key := store.NewKey(pb.MetaType_RoleInfoKey, pri.ID)
					cm.ds.Put(key, val)
				}
				pCnt = pcnt
			}

			if ucnt > uCnt {
				for i := uCnt; i <= ucnt; i++ {
					pindex, err := cm.rUser.GetGroupU(cm.eAddr, cm.groupID, i)
					if err != nil {
						continue
					}

					gotAddr, err := cm.rUser.GetAddr(cm.eAddr, pindex)
					if err != nil {
						continue
					}

					pri, err := cm.getRoleInfo(gotAddr)
					if err != nil {
						continue
					}

					// should not
					if pri.ID == 0 {
						continue
					}
					if pri.GroupID == 0 {
						continue
					}
					if pri.GroupID != cm.groupID {
						continue
					}

					val, err := proto.Marshal(pri)
					if err != nil {
						continue
					}

					// save to local
					key := store.NewKey(pb.MetaType_RoleInfoKey, pri.ID)
					cm.ds.Put(key, val)
				}
				uCnt = ucnt
			}
		}
	}
}
