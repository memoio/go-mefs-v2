package v2

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
	"github.com/memoio/go-mefs-v2/submodule/connect/v2/impl"
	inter "github.com/memoio/go-mefs-v2/submodule/connect/v2/interface"
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
func NewContractMgr(ctx context.Context, endPoint, baseAddr string, sk []byte) (*ContractMgr, error) {
	logger.Debug("create contract mgr: ", endPoint, ", ", baseAddr)

	client, err := ethclient.DialContext(context.TODO(), endPoint)
	if err != nil {
		return nil, xerrors.Errorf("get client from %s fail: %s", endPoint, err)
	}
	defer client.Close()

	// convert key
	hexSk := hex.EncodeToString(sk)

	eAddr, err := impl.SkToAddr(hexSk)
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
		return nil, err
	}
	geti, err := impl.NewGetter(endPoint, hexSk, getAddr)
	if err != nil {
		return nil, err
	}

	erc20Addr, err := geti.GetToken(0)
	if err != nil {
		return nil, err
	}
	erc20i, err := impl.NewErc20(endPoint, hexSk, erc20Addr)
	if err != nil {
		return nil, err
	}

	proxyAddr, err := insti.Instances(&bind.CallOpts{
		From: eAddr,
	}, 100)
	if err != nil {
		return nil, err
	}
	proxyi, err := impl.NewProxy(endPoint, hexSk, proxyAddr)
	if err != nil {
		return nil, err
	}

	cm := &ContractMgr{
		ctx:      ctx,
		endPoint: endPoint,

		eAddr:    eAddr,
		baseAddr: base,

		ercIns:   erc20i,
		getIns:   geti,
		proxyIns: proxyi,
	}

	// getInfo
	ri, err := cm.getRoleInfo(cm.eAddr)
	if err != nil {
		return cm, err
	}

	cm.roleID = ri.RoleID
	cm.typ = ri.Type
	cm.groupID = ri.GroupID

	return cm, nil
}

// register account, type and group
func (cm *ContractMgr) Start(typ pb.RoleInfo_Type, gIndex uint64) error {
	if cm.groupID > 0 {
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

	// register account
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
	}

	// register role
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
	}

	// add to group
	if ri.GIndex == 0 && gIndex > 0 {
		err := cm.AddToGroup(gIndex)
		if err != nil {
			return err
		}

		time.Sleep(5 * time.Second)

		ri, err = cm.getIns.GetRoleInfo(cm.eAddr)
		if err != nil {
			return err
		}

		cm.groupID = ri.GIndex

		if cm.groupID != gIndex {
			return xerrors.Errorf("add to group fails")
		}
	}

	return nil
}

func GetTokenAddr(endPoint string, baseAddr common.Address, hexSk string) (common.Address, error) {
	var res common.Address
	client, err := ethclient.DialContext(context.TODO(), endPoint)
	if err != nil {
		return res, xerrors.Errorf("get client from %s fail: %s", endPoint, err)
	}
	defer client.Close()

	eAddr, err := impl.SkToAddr(hexSk)
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
	geti, err := impl.NewGetter(endPoint, hexSk, getAddr)
	if err != nil {
		return res, err
	}

	return geti.GetToken(0)
}
