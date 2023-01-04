package settle

import (
	"context"
	"encoding/hex"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"golang.org/x/xerrors"

	inst "github.com/memoio/contractsv2/go_contracts/instance"

	"github.com/memoio/go-mefs-v2/api"
	"github.com/memoio/go-mefs-v2/lib/pb"

	scom "github.com/memoio/go-mefs-v2/submodule/connect/settle/common"
	"github.com/memoio/go-mefs-v2/submodule/connect/settle/impl"
	inter "github.com/memoio/go-mefs-v2/submodule/connect/settle/interface"
	v1 "github.com/memoio/go-mefs-v2/submodule/connect/v1"
	v2impl "github.com/memoio/go-mefs-v2/submodule/connect/v2/impl"
	v3impl "github.com/memoio/go-mefs-v2/submodule/connect/v3/impl"
)

var _ api.ISettle = &ContractMgr{}

type ContractMgr struct {
	ctx context.Context

	// chain related
	endPoint string

	// role related
	roleID uint64
	typ    pb.RoleInfo_Type

	// group related
	groupID uint64
	level   int

	tIndex uint8

	sk    string
	eAddr common.Address

	baseAddr common.Address

	proxyIns inter.IProxy  //
	getIns   inter.IGetter // addr is get from base
	ercIns   inter.IERC20  //
}

// create and verify
func NewContractMgr(ctx context.Context, endPoint, baseAddr string, ver uint32, sk []byte) (scom.SettleMgr, error) {
	logger.Debug("create v2 contract mgr: ", endPoint, ", ", baseAddr)

	client, err := ethclient.DialContext(context.TODO(), endPoint)
	if err != nil {
		return nil, xerrors.Errorf("get client from %s fail: %s", endPoint, err)
	}
	defer client.Close()

	// convert key
	hexSk := hex.EncodeToString(sk)

	eAddr, err := scom.SkToAddr(hexSk)
	if err != nil {
		return nil, err
	}

	val := GetTxBalance(endPoint, eAddr)
	logger.Debugf("%s has tx fee %d", eAddr, val)
	if val.BitLen() == 0 {
		return nil, xerrors.Errorf("not have tx fee on chain")
	}

	base := common.HexToAddress(baseAddr)

	// get contract addr from instance contract and create ins
	insti, err := inst.NewInstance(base, client)
	if err != nil {
		return nil, err
	}

	getAddr, err := insti.Instances(&bind.CallOpts{
		From: eAddr,
	}, 150)
	if err != nil {
		return nil, xerrors.Errorf("get getter contract fail: %s", err)
	}
	logger.Debug("getter contract: ", getAddr)

	proxyAddr, err := insti.Instances(&bind.CallOpts{
		From: eAddr,
	}, 100)
	if err != nil {
		return nil, xerrors.Errorf("get proxy contract fail: %s", err)
	}
	logger.Debug("proxy contract: ", proxyAddr)

	cm := &ContractMgr{
		ctx:      ctx,
		endPoint: endPoint,

		sk:       hexSk,
		eAddr:    eAddr,
		baseAddr: base,
	}

	switch ver {
	case 0:
		return v1.NewContractMgr(ctx, endPoint, baseAddr, sk)
	case 2:
		geti, err := v2impl.NewGetter(endPoint, hexSk, getAddr)
		if err != nil {
			return nil, err
		}

		proxyi, err := v2impl.NewProxy(endPoint, hexSk, proxyAddr)
		if err != nil {
			return nil, err
		}

		cm.getIns = geti
		cm.proxyIns = proxyi
	case 3:
		geti, err := v3impl.NewGetter(endPoint, hexSk, getAddr)
		if err != nil {
			return nil, err
		}

		proxyi, err := v3impl.NewProxy(endPoint, hexSk, proxyAddr)
		if err != nil {
			return nil, err
		}

		cm.getIns = geti
		cm.proxyIns = proxyi
	default:
		return nil, xerrors.Errorf("unsupported contract version: ", ver)
	}

	erc20Addr, err := cm.getIns.GetToken(0)
	if err != nil {
		return nil, xerrors.Errorf("get token contract fail: %s", err)
	}
	logger.Debug("erc20 contract: ", erc20Addr)

	erc20i, err := impl.NewErc20(endPoint, hexSk, erc20Addr)
	if err != nil {
		return nil, err
	}
	logger.Debugf("%s has memo %d", eAddr, erc20i.BalanceOf(eAddr))

	cm.ercIns = erc20i

	// getInfo
	ri, err := cm.getIns.GetRoleInfo(eAddr)
	if err != nil {
		return nil, err
	}

	cm.roleID = ri.Index
	cm.groupID = ri.GIndex

	switch ri.RType {
	case 1:
		cm.typ = pb.RoleInfo_User
		if ri.GIndex > 0 && ri.State != 3 {
			return nil, xerrors.Errorf("user %d is not active in group %d", cm.roleID, cm.groupID)
		}
	case 2:
		cm.typ = pb.RoleInfo_Provider
		if ri.GIndex > 0 && ri.State != 3 {
			return nil, xerrors.Errorf("provider %d is not active in group %d", cm.roleID, cm.groupID)
		}
	case 3:
		cm.typ = pb.RoleInfo_Keeper
		if ri.GIndex > 0 && ri.State != 3 {
			logger.Debug("registered in contract: ", cm.roleID, cm.typ, cm.groupID)
			return nil, xerrors.Errorf("keeper is not active, activate first")
		}
	default:
		cm.typ = pb.RoleInfo_Unknown
	}

	return cm, nil
}

// register account, type and group
func (cm *ContractMgr) Start(typ pb.RoleInfo_Type, gIndex uint64) error {
	if cm.groupID > 0 {
		gi, err := cm.getIns.GetGroupInfo(cm.groupID)
		if err != nil {
			return err
		}

		if gi.State != 3 {
			return xerrors.Errorf("group %d is not active", cm.groupID)
		}

		cm.level = int(gi.Level)
		logger.Debug("registered in contract: ", cm.roleID, cm.typ, cm.groupID)
		return nil
	}

	logger.Debug("register in contract mgr: ", typ, gIndex)
	ri, err := cm.getIns.GetRoleInfo(cm.eAddr)
	if err != nil {
		return err
	}

	if gIndex == 0 && ri.GIndex == 0 {
		return xerrors.Errorf("group should be larger than zero")
	}

	// register account if index is 0
	if ri.Index == 0 {
		err := cm.RegisterAccount()
		if err != nil {
			return err
		}

		time.Sleep(10 * time.Second)
		ri, err = cm.getIns.GetRoleInfo(cm.eAddr)
		if err != nil {
			return err
		}

		cm.roleID = ri.Index

		if cm.roleID == 0 {
			return xerrors.Errorf("register account fails")
		}

		logger.Info("Register account successfully. Account ID: ", cm.roleID)
	}

	if typ == pb.RoleInfo_Unknown && gIndex > 0 {
		cm.typ = pb.RoleInfo_Unknown
		cm.groupID = gIndex
		return nil
	}

	// register role if no role
	if ri.RType == 0 {
		err := cm.RegisterRole(typ)
		if err != nil {
			return err
		}
		time.Sleep(5 * time.Second)

		ri, err = cm.getIns.GetRoleInfo(cm.eAddr)
		if err != nil {
			return err
		}

		cm.typ = pb.RoleInfo_Type(ri.RType)

		if cm.typ != typ {
			return xerrors.Errorf("register type fails")
		}

		logger.Info("Register role successfully. Role type: ", cm.typ)
	}

	// add to group
	if ri.GIndex == 0 && gIndex > 0 {
		_, err := cm.getIns.GetGroupInfo(gIndex)
		if err != nil {
			return xerrors.Errorf("get groupinfo %d fail %s", gIndex, err)
		}

		err = cm.AddToGroup(gIndex)
		if err != nil {
			return err
		}

		time.Sleep(5 * time.Second)

		ri, err = cm.getIns.GetRoleInfo(cm.eAddr)
		if err != nil {
			return err
		}

		cm.groupID = ri.GIndex

		// check if group is set ok
		if cm.groupID != gIndex {
			return xerrors.Errorf("add to group fails")
		}

		logger.Info("Add to group successfully. Role group id: ", ri.GIndex)

		if cm.groupID > 0 {
			gi, err := cm.getIns.GetGroupInfo(cm.groupID)
			if err != nil {
				return err
			}

			if gi.State != 3 {
				return xerrors.Errorf("group %d is not active, you need to activate some keepers.", cm.groupID)
			}

			cm.level = int(gi.Level)
		}
	}

	logger.Debug("register in contract: ", cm.roleID, cm.typ, cm.groupID)

	return nil
}

func GetTokenAddr(endPoint string, baseAddr common.Address, hexSk string, ver uint32) (common.Address, error) {
	var res common.Address
	client, err := ethclient.DialContext(context.TODO(), endPoint)
	if err != nil {
		return res, xerrors.Errorf("get client from %s fail: %s", endPoint, err)
	}
	defer client.Close()

	eAddr, err := scom.SkToAddr(hexSk)
	if err != nil {
		return res, err
	}

	val := GetTxBalance(endPoint, eAddr)
	logger.Debugf("%s has tx fee %d", eAddr, val)
	if val.BitLen() == 0 {
		return res, xerrors.Errorf("not have tx fee on chain")
	}

	// get contract addr from instance contract and create ins
	insti, err := inst.NewInstance(baseAddr, client)
	if err != nil {
		return res, err
	}

	getAddr, err := insti.Instances(&bind.CallOpts{
		From: eAddr,
	}, 150)
	if err != nil {
		return res, err
	}

	switch ver {
	case 2:
		geti, err := v2impl.NewGetter(endPoint, hexSk, getAddr)
		if err != nil {
			return res, err
		}
		return geti.GetToken(0)
	case 3:
		geti, err := v3impl.NewGetter(endPoint, hexSk, getAddr)
		if err != nil {
			return res, err
		}
		return geti.GetToken(0)
	}

	return res, xerrors.Errorf("unsupported contract version: ", ver)
}
