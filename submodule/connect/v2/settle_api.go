package v2

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"golang.org/x/xerrors"

	inst "github.com/memoio/contractsv2/go_contracts/instance"
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

func (cm *ContractMgr) getRoleInfo(eAddr common.Address) (*api.RoleInfo, error) {
	ri, err := cm.getIns.GetRoleInfo(eAddr)
	if err != nil {
		return nil, err
	}

	//if ri.State == 1 {
	//	return nil, xerrors.Errorf("%d is banned", ri.Index)
	//}

	//if ri.RType > 0 && ri.GIndex > 0 && ri.State != 3 {
	//	return nil, xerrors.Errorf("%d is not active", ri.Index)
	//}

	pri := new(api.RoleInfo)

	pri.RoleID = ri.Index
	pri.GroupID = ri.GIndex
	pri.ChainVerifyKey = eAddr.Bytes()
	pri.Desc = ri.Desc
	pri.Owner = ri.Owner.String()

	switch ri.State {
	case 1:
		pri.State = "banned"
		pri.IsBanned = true
	case 2:
		pri.State = "inactive"
	case 3:
		pri.State = "active"
		pri.IsActive = true
	}

	switch ri.RType {
	case 1:
		pri.Type = pb.RoleInfo_User
		pri.Extra = ri.VerifyKey
	case 2:
		pri.Type = pb.RoleInfo_Provider
	case 3:
		pri.Type = pb.RoleInfo_Keeper
		pri.BlsVerifyKey = ri.VerifyKey
	default:
		pri.Type = pb.RoleInfo_Unknown
	}

	return pri, nil
}

func (cm *ContractMgr) SettleGetRoleInfo(addr address.Address) (*api.RoleInfo, error) {
	eAddr := common.BytesToAddress(utils.ToEthAddress(addr.Bytes()))
	return cm.getRoleInfo(eAddr)
}

func (cm *ContractMgr) SettleGetRoleInfoAt(ctx context.Context, rid uint64) (*api.RoleInfo, error) {
	eAddr, err := cm.getIns.GetAddrAt(rid)
	if err != nil {
		return nil, err
	}

	return cm.getRoleInfo(eAddr)
}

func (cm *ContractMgr) SettleGetGroupInfoAt(ctx context.Context, gIndex uint64) (*api.GroupInfo, error) {
	client, err := ethclient.DialContext(context.TODO(), cm.endPoint)
	if err != nil {
		return nil, xerrors.Errorf("get client from %s fail: %s", cm.endPoint, err)
	}
	defer client.Close()

	insti, err := inst.NewInstance(cm.baseAddr, client)
	if err != nil {
		return nil, err
	}

	kmAddr, err := insti.Instances(&bind.CallOpts{
		From: cm.eAddr,
	}, 13)
	if err != nil {
		return nil, xerrors.Errorf("get getter contract fail: %s", err)
	}

	gi, err := cm.getIns.GetGroupInfo(gIndex)
	if err != nil {
		return nil, err
	}

	gi.FsAddr = kmAddr.String()
	gi.EndPoint = cm.endPoint
	gi.BaseAddr = cm.baseAddr.String()

	return gi, nil
}

func (cm *ContractMgr) SettleGetPledgeInfo(ctx context.Context, roleID uint64) (*api.PledgeInfo, error) {
	// total current pledge amount of all nodes recorded in pledge contract, all pledge amount, no mint
	tp, err := cm.getIns.GetTotalPledge()
	if err != nil {
		return nil, err
	}

	// get erc20 balance of pledge pool, including pledge and mint,
	ep, err := cm.getIns.GetPledge(cm.tIndex)
	if err != nil {
		return nil, err
	}

	// get current pledge balance of a node
	bal, err := cm.getIns.GetPledgeAt(roleID, cm.tIndex)
	if err != nil {
		return nil, err
	}

	// get node pledge and node reward
	_, last, lp, lr, err := cm.getIns.GetPleRewardInfo(roleID, 0)
	if err != nil {
		return nil, err
	}

	pi := &api.PledgeInfo{
		Value:    bal,
		ErcTotal: ep,
		Total:    tp,

		Last:        last,
		LocalPledge: lp,
		LocalReward: lr,
	}

	return pi, nil
}

func (cm *ContractMgr) SettleGetBalanceInfo(ctx context.Context, roleID uint64) (*api.BalanceInfo, error) {
	gotAddr, err := cm.getIns.GetAddrAt(roleID)
	if err != nil {
		return nil, err
	}

	avail, lock, penalty, err := cm.getIns.GetBalAt(roleID, cm.tIndex)
	if err != nil {
		return nil, err
	}
	//avil.Add(avil, lock)

	bi := &api.BalanceInfo{
		Value:        GetTxBalance(cm.endPoint, gotAddr),
		ErcValue:     cm.ercIns.BalanceOf(gotAddr),
		FsValue:      avail,
		LockValue:    lock,
		PenaltyValue: penalty,
	}

	return bi, nil
}

// return time, size, price
func (cm *ContractMgr) SettleGetStoreInfo(ctx context.Context, userID, proID uint64) (*api.StoreInfo, error) {

	fo, err := cm.getIns.GetFsInfo(userID, proID)
	if err != nil {
		return nil, err
	}

	so, err := cm.getIns.GetStoreInfo(userID, proID, cm.tIndex)
	if err != nil {
		return nil, err
	}

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

func (cm *ContractMgr) SettlePledgeWithdraw(ctx context.Context, val *big.Int) error {
	logger.Debugf("%d pledge withdraw %d", cm.roleID, val)

	err := cm.proxyIns.PledgeWithdraw(cm.roleID, cm.tIndex, val)
	if err != nil {
		return err
	}

	return nil
}

func (cm *ContractMgr) SettleProIncome(ctx context.Context, val, penalty *big.Int, kindex []uint64, ksigns [][]byte) error {
	logger.Debugf("%d pro income %d", cm.roleID, val)

	pi := proxy.PWIn{
		PIndex: cm.roleID,
		TIndex: cm.tIndex,
		Pay:    val,
		Lost:   penalty,
	}

	err := cm.proxyIns.ProWithdraw(pi, kindex, ksigns)
	if err != nil {
		return xerrors.Errorf("%d pro income fail %s", cm.roleID, err)
	}

	return nil
}

func (cm *ContractMgr) SettleWithdraw(ctx context.Context, val *big.Int) error {
	logger.Debugf("%d withdraw %d", cm.roleID, val)

	err := cm.proxyIns.Withdraw(cm.roleID, cm.tIndex, val)
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

func (cm *ContractMgr) SettleSetDesc(ctx context.Context, desc []byte) error {
	return cm.proxyIns.SetDesc(desc)
}

func (cm *ContractMgr) SettleQuitRole(ctx context.Context) error {
	return cm.proxyIns.QuitRole(cm.roleID)
}

func (cm *ContractMgr) SettleAlterPayee(ctx context.Context, p string) error {
	if !common.IsHexAddress(p) {
		return xerrors.Errorf("%s is not hex address", p)
	}
	np := common.HexToAddress(p)
	return cm.proxyIns.AlterPayee(cm.roleID, np)
}
