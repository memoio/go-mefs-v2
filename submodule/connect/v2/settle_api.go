package v2

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"golang.org/x/xerrors"

	"github.com/memoio/contractsv2/go_contracts/proxy"
	"github.com/memoio/go-mefs-v2/api"
	"github.com/memoio/go-mefs-v2/lib/address"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/utils"
)

func (cm *ContractMgr) SettleGetRoleID(ctx context.Context) uint64 {
	return cm.roleID
}

func (cm *ContractMgr) SettleGetGroupID(ctx context.Context) uint64 {
	return cm.groupID
}

func (cm *ContractMgr) SettleGetThreshold(ctx context.Context) int {
	return cm.level
}

// as net prefix
func (cm *ContractMgr) SettleGetBaseAddr(ctx context.Context) []byte {
	return cm.baseAddr.Bytes()
}

func (cm *ContractMgr) SettleGetAddrCnt(ctx context.Context) uint64 {
	return cm.getIns.GetAddrCnt()
}

// get info

func (cm *ContractMgr) getRoleInfo(eAddr common.Address) (*pb.RoleInfo, error) {
	ri, err := cm.getIns.GetRoleInfo(eAddr)
	if err != nil {
		return nil, err
	}

	if ri.IsBan {
		return nil, xerrors.Errorf("%d is banned", ri.Index)
	}

	if ri.RType > 0 && !ri.IsActive {
		return nil, xerrors.Errorf("%d is not active", ri.Index)
	}

	pri := new(pb.RoleInfo)

	pri.RoleID = ri.Index
	pri.GroupID = ri.GIndex
	pri.ChainVerifyKey = eAddr.Bytes()

	switch ri.RType {
	case 1:
		pri.Type = pb.RoleInfo_User
		pri.Extra = ri.Extra
	case 2:
		pri.Type = pb.RoleInfo_Provider
	case 3:
		pri.Type = pb.RoleInfo_Keeper
		pri.BlsVerifyKey = ri.Extra
	default:
		pri.Type = pb.RoleInfo_Unknown
	}

	return pri, nil
}

func (cm *ContractMgr) SettleGetRoleInfo(addr address.Address) (*pb.RoleInfo, error) {
	eAddr := common.BytesToAddress(utils.ToEthAddress(addr.Bytes()))
	return cm.getRoleInfo(eAddr)
}

func (cm *ContractMgr) SettleGetRoleInfoAt(ctx context.Context, rid uint64) (*pb.RoleInfo, error) {
	eAddr, err := cm.getIns.GetAddrAt(rid)
	if err != nil {
		return nil, err
	}

	return cm.getRoleInfo(eAddr)
}

func (cm *ContractMgr) SettleGetGroupInfoAt(ctx context.Context, gIndex uint64) (*api.GroupInfo, error) {
	gi, err := cm.getIns.GetGroupInfo(gIndex)
	if err != nil {
		return nil, err
	}

	gi.EndPoint = cm.endPoint
	gi.BaseAddr = cm.baseAddr.String()

	return gi, nil
}

func (cm *ContractMgr) SettleGetPledgeInfo(ctx context.Context, roleID uint64) (*api.PledgeInfo, error) {
	tp := cm.getIns.GetTotalPledge()

	ep := cm.getIns.GetPledge(cm.tIndex)

	pv := cm.getIns.GetPledgeAt(roleID, cm.tIndex)

	pi := &api.PledgeInfo{
		Value:    pv,
		ErcTotal: ep,
		Total:    tp,
	}
	return pi, nil
}

func (cm *ContractMgr) SettleGetBalanceInfo(ctx context.Context, roleID uint64) (*api.BalanceInfo, error) {
	gotAddr, err := cm.getIns.GetAddrAt(roleID)
	if err != nil {
		return nil, err
	}

	avil := cm.getIns.GetBalAt(roleID, cm.tIndex)

	bi := &api.BalanceInfo{
		Value:    GetTxBalance(cm.endPoint, gotAddr),
		ErcValue: cm.ercIns.BalanceOf(gotAddr),
		FsValue:  avil,
	}

	return bi, nil
}

// return time, size, price
func (cm *ContractMgr) SettleGetStoreInfo(ctx context.Context, userID, proID uint64) (*api.StoreInfo, error) {
	so := cm.getIns.GetStoreInfo(userID, proID, cm.tIndex)
	fo := cm.getIns.GetFsInfo(userID, proID)

	si := &api.StoreInfo{
		Nonce:    fo.Nonce,
		SubNonce: fo.SubNonce,
		Time:     int64(so.Start),
		Size:     so.Size,
		Price:    so.Sprice,
	}

	return si, nil
}

func (cm *ContractMgr) SettleCharge(ctx context.Context, val *big.Int) error {
	logger.Debugf("%d charge %d", cm.roleID, val)
	return cm.Recharge(cm.roleID, cm.tIndex, false, val)
}

func (cm *ContractMgr) SettlePledge(ctx context.Context, val *big.Int) error {
	logger.Debugf("%d pledge %d", cm.roleID, val)
	return cm.Pledge(cm.roleID, val)
}

func (cm *ContractMgr) SettleCanclePledge(ctx context.Context, val *big.Int) error {
	logger.Debugf("%d cancle pledge %d", cm.roleID, val)

	err := cm.UnPledge(cm.roleID, cm.tIndex, val)
	if err != nil {
		return err
	}

	return nil
}

func (cm *ContractMgr) SettleWithdraw(ctx context.Context, val, penalty *big.Int, kindex []uint64, ksigns [][]byte) error {
	logger.Debugf("%d withdraw %d", cm.roleID, val)

	ri, err := cm.SettleGetRoleInfoAt(ctx, cm.roleID)
	if err != nil {
		return err
	}

	if ri.Type == pb.RoleInfo_Provider && val != nil && val.BitLen() > 0 {
		pi := proxy.PWIn{
			PIndex: cm.roleID,
			TIndex: cm.tIndex,
			Pay:    val,
			Lost:   penalty,
		}

		err := cm.proxyIns.ProWithdraw(pi, kindex, ksigns)
		if err != nil {
			return xerrors.Errorf("%d pro withdraw fail %s", cm.roleID, err)
		}

		return nil
	}

	err = cm.proxyIns.Withdraw(cm.roleID, cm.tIndex, val)
	if err != nil {
		return xerrors.Errorf("%d withdraw fail %s", cm.roleID, err)
	}

	return nil
}

func (cm *ContractMgr) SettleAddOrder(ctx context.Context, so *types.SignedOrder) error {
	return cm.AddOrder(so)
}

func (cm *ContractMgr) SettleSubOrder(ctx context.Context, so *types.SignedOrder) error {
	return cm.SubOrder(so)
}
