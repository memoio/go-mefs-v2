package v2

import (
	"context"
	"encoding/hex"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/memoio/go-mefs-v2/api"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/submodule/connect/v2/impl"
	inter "github.com/memoio/go-mefs-v2/submodule/connect/v2/interface"

	"golang.org/x/xerrors"
)

var _ api.ISettle = &ContractMgr{}

type ContractMgr struct {
	ctx context.Context

	// chain related
	endPoint string
	chainID  *big.Int

	// role related
	roleID uint64
	typ    pb.RoleInfo_Type

	// group related
	groupID uint64
	level   int

	tIndex uint8

	sk    string
	eAddr common.Address

	proxyAddr common.Address

	proxyIns inter.IProxy  //
	getIns   inter.IGetter // addr is get from proxy
	ercIns   inter.IERC20  //
}

// create and verify
func NewContractMgr(ctx context.Context, endPoint, proxyAddr string, sk []byte) (*ContractMgr, error) {
	logger.Debug("create contract mgr: ", endPoint, ", ", proxyAddr)

	client, err := ethclient.DialContext(context.TODO(), endPoint)
	if err != nil {
		return nil, xerrors.Errorf("get client from %s fail: %s", endPoint, err)
	}

	chainID, err := client.NetworkID(context.TODO())
	if err != nil {
		return nil, xerrors.Errorf("get networkID from %s fail: %s", endPoint, err)
	}

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

	// ins

	cm := &ContractMgr{
		ctx:      ctx,
		endPoint: endPoint,
		chainID:  chainID,

		eAddr: eAddr,
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
