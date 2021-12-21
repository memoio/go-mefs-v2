package settle

import (
	"context"
	"log"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/gogo/protobuf/proto"
	"golang.org/x/xerrors"

	callconts "memoContract/callcontracts"
	iface "memoContract/interfaces"
	"memoContract/test"

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
	ppAddr common.Address // pledge contract addr

	// Role caller for user
	iRole iface.RoleInfo
	iErc  iface.ERC20Info
	ipp   iface.PledgePoolInfo
}

func NewContractMgr(ctx context.Context, addr address.Address, hexSk string, ds store.KVStore) (*ContractMgr, error) {
	logger.Debug("create contract mgr")
	// set endpoint
	callconts.EndPoint = "http://119.147.213.220:8191"

	txopts := &callconts.TxOpts{
		Nonce:    nil,
		GasPrice: big.NewInt(callconts.DefaultGasPrice),
		GasLimit: callconts.DefaultGasLimit,
	}

	eAddr := common.BytesToAddress(utils.ToEthAddress(addr.Bytes()))

	val := QueryBalance(eAddr.String(), callconts.EndPoint)
	if val.Cmp(big.NewInt(100_000_000_000_000_000)) < 0 {
		TransferTo(big.NewInt(1_000_000_000_000_000_000), eAddr.String(), callconts.EndPoint, callconts.EndPoint)
	}

	iRole := callconts.NewR(eAddr, hexSk, txopts)
	iERC := callconts.NewERC20(eAddr, hexSk, txopts)
	ipp := callconts.NewPledgePool(eAddr, hexSk, txopts)

	cm := &ContractMgr{
		ctx: ctx,
		ds:  ds,

		eAddr:  eAddr,
		rAddr:  callconts.RoleAddr,
		tAddr:  callconts.ERC20Addr,
		ppAddr: callconts.PledgePoolAddr,

		iRole: iRole,
		iErc:  iERC,
		ipp:   ipp,
	}

	return cm, nil
}

func (cm *ContractMgr) Start(typ pb.RoleInfo_Type, gIndex uint64) error {
	logger.Debug("start contract mgr")
	_, _, _, rid, _, _, err := cm.iRole.GetRoleInfo(cm.rAddr, cm.eAddr)
	if err != nil {
		return err
	}

	logger.Debug("get roleinfo: ", rid)

	if rid == 0 {
		// registerRole
		err := cm.RegisterRole()
		if err != nil {
			return err
		}
	}

	cm.roleID = rid

	_, _, rType, rid, gid, _, err := cm.iRole.GetRoleInfo(cm.rAddr, cm.eAddr)
	if err != nil {
		return err
	}

	logger.Debug("get roleinfo: ", rid, gid, rType)

	if rid == 0 {
		return xerrors.Errorf("register fails")
	}

	if rType == 0 {
		switch typ {
		case pb.RoleInfo_Provider:
			// provider: register,register; add to group

			err = cm.RegisterProvider()
			if err != nil {
				logger.Debug("register fail: ", err)
				return err
			}

			err = cm.AddProviderToGroup(gIndex)
			if err != nil {
				return err
			}

		case pb.RoleInfo_User:
			// user: resgister user
			// get extra byte
			err = cm.RegisterUser(gIndex, nil)
			if err != nil {
				return err
			}

		}
	}

	_, _, rType, rid, gid, _, err = cm.iRole.GetRoleInfo(cm.rAddr, cm.eAddr)
	if err != nil {
		return err
	}

	logger.Debug("get roleinfo: ", rid, gid, rType)

	if gid == 0 {
		switch typ {
		case pb.RoleInfo_Provider:
			// provider: register,register; add to group
			err = cm.AddProviderToGroup(gIndex)
			if err != nil {
				return err
			}
		}
	}

	if rid == 0 {
		return xerrors.Errorf("register group fails")
	}

	err = cm.getGroupInfo(gid)
	if err != nil {
		return err
	}

	cm.roleID = rid
	cm.groupID = gid

	go cm.getAllAddrs()

	return nil
}

// get info
func (cm *ContractMgr) GetRoleInfo(addr address.Address) (*pb.RoleInfo, error) {
	eAddr := common.BytesToAddress(utils.ToEthAddress(addr.Bytes()))
	return cm.getRoleInfo(eAddr)
}

func (cm *ContractMgr) GetRoleInfoAt(rid uint64) (*pb.RoleInfo, error) {
	gotAddr, err := cm.iRole.GetAddr(cm.eAddr, rid)
	if err != nil {
		return nil, err
	}

	return cm.getRoleInfo(gotAddr)
}

func (cm *ContractMgr) getRoleInfo(eAddr common.Address) (*pb.RoleInfo, error) {
	_, _, rType, rid, gid, extra, err := cm.iRole.GetRoleInfo(eAddr, eAddr)
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
	logger.Debug("contract mgr register")
	return cm.iRole.Register(cm.rAddr, cm.eAddr, nil)
}

func (cm *ContractMgr) RegisterProvider() error {
	logger.Debug("register provider")

	txopts := &callconts.TxOpts{
		Nonce:    nil,
		GasPrice: big.NewInt(callconts.DefaultGasPrice),
		GasLimit: callconts.DefaultGasLimit,
	}

	bal, err := cm.iErc.BalanceOf(cm.tAddr, cm.eAddr)
	if err != nil {
		logger.Debug(err)
		return err
	}

	logger.Debugf("erc20 balance is %d", bal)

	pledgep, err := cm.iRole.PledgeP(cm.rAddr) // 申请Provider最少需质押的金额
	if err != nil {
		log.Fatal(err)
	}

	if bal.Cmp(pledgep) < 0 {
		erc20 := callconts.NewERC20(callconts.AdminAddr, test.AdminSk, txopts)
		erc20.Transfer(cm.tAddr, cm.eAddr, pledgep)
	}

	err = cm.ipp.Pledge(cm.ppAddr, cm.tAddr, cm.rAddr, cm.roleID, pledgep, nil)
	if err != nil {
		log.Fatal(err)
	}

	logger.Debugf("pledge %d", pledgep)

	return cm.iRole.RegisterProvider(cm.rAddr, cm.roleID, nil)
}

func (cm *ContractMgr) AddProviderToGroup(gIndex uint64) error {
	gn, err := cm.iRole.GetGroupsNum(cm.rAddr)
	if err != nil {
		return err
	}

	logger.Debug("get group number: ", gn)

	return cm.iRole.AddProviderToGroup(cm.rAddr, cm.roleID, gIndex, nil)
}

func (cm *ContractMgr) RegisterUser(gIndex uint64, extra []byte) error {
	return cm.iRole.RegisterUser(cm.rAddr, cm.eAddr, cm.roleID, gIndex, cm.tIndex, extra, nil)
}

func (cm *ContractMgr) getGroupInfo(gIndex uint64) error {
	_, _, _, level, _, _, fsAddr, err := cm.iRole.GetGroupInfo(cm.rAddr, gIndex)
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

			kcnt, err := cm.iRole.GetGKNum(cm.eAddr, cm.groupID)
			if err != nil {
				continue
			}
			if kcnt > kCnt {
				for i := kCnt; i <= kcnt; i++ {
					kindex, err := cm.iRole.GetGroupK(cm.eAddr, cm.groupID, i)
					if err != nil {
						continue
					}

					gotAddr, err := cm.iRole.GetAddr(cm.eAddr, kindex)
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

			ucnt, pcnt, err := cm.iRole.GetGUPNum(cm.eAddr, cm.groupID)
			if err != nil {
				continue
			}
			if pcnt > pCnt {
				for i := pCnt; i <= pcnt; i++ {
					pindex, err := cm.iRole.GetGroupP(cm.eAddr, cm.groupID, i)
					if err != nil {
						continue
					}

					gotAddr, err := cm.iRole.GetAddr(cm.eAddr, pindex)
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
					pindex, err := cm.iRole.GetGroupU(cm.eAddr, cm.groupID, i)
					if err != nil {
						continue
					}

					gotAddr, err := cm.iRole.GetAddr(cm.eAddr, pindex)
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
