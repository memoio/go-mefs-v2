package settle

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/api"
	"github.com/memoio/go-mefs-v2/lib/address"
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
	return cm.rAddr.Bytes()
}

func (cm *ContractMgr) SettleGetAddrCnt(ctx context.Context) uint64 {
	tc, _ := cm.getAddrCount()
	return tc
}

// get info
func (cm *ContractMgr) SettleGetRoleInfo(addr address.Address) (*api.RoleInfo, error) {
	eAddr := common.BytesToAddress(utils.ToEthAddress(addr.Bytes()))
	ri, err := cm.getRoleInfo(eAddr)
	if err != nil {
		return nil, err
	}

	return ri, nil
}

func (cm *ContractMgr) SettleGetRoleInfoAt(ctx context.Context, rid uint64) (*api.RoleInfo, error) {
	gotAddr, err := cm.getAddrAt(rid)
	if err != nil {
		return nil, err
	}

	ri, err := cm.getRoleInfo(gotAddr)
	if err != nil {
		return nil, err
	}
	return ri, nil
}

func (cm *ContractMgr) SettleGetGroupInfoAt(ctx context.Context, gIndex uint64) (*api.GroupInfo, error) {
	isActive, isBanned, isReady, level, size, price, fsAddr, err := cm.getGroupInfo(gIndex)
	if err != nil {
		return nil, err
	}

	logger.Debugf("group %d, state %v %v %v, level %d, fsAddr %s", gIndex, isActive, isBanned, isReady, level, fsAddr)

	kc, err := cm.getKNumAtGroup(cm.groupID)
	if err != nil {
		return nil, err
	}

	uc, pc, err := cm.getUPNumAtGroup(cm.groupID)
	if err != nil {
		return nil, err
	}

	gi := &api.GroupInfo{
		EndPoint: cm.endPoint,
		BaseAddr: cm.rAddr.String(),
		ID:       gIndex,
		Level:    uint8(level),
		FsAddr:   fsAddr.String(),
		Size:     size.Uint64(),
		Price:    new(big.Int).Set(price),
		KCount:   kc,
		UCount:   uc,
		PCount:   pc,
	}

	return gi, nil
}

func (cm *ContractMgr) SettleGetPledgeInfo(ctx context.Context, roleID uint64) (*api.PledgeInfo, error) {
	tp, err := cm.getTotalPledge()
	if err != nil {
		return nil, err
	}

	ep, err := cm.getPledgeAmount(cm.tIndex)
	if err != nil {
		return nil, err
	}

	pv, err := cm.getBalanceInPPool(roleID, cm.tIndex)
	if err != nil {
		return nil, err
	}

	pi := &api.PledgeInfo{
		Value:    pv,
		ErcTotal: ep,
		Total:    tp,
		
		PledgeTime: big.NewInt(0),
	}
	return pi, nil
}

func (cm *ContractMgr) SettleGetBalanceInfo(ctx context.Context, roleID uint64) (*api.BalanceInfo, error) {
	gotAddr, err := cm.getAddrAt(roleID)
	if err != nil {
		return nil, err
	}

	avil, tmp, err := cm.getBalanceInFs(roleID, cm.tIndex)
	if err != nil {
		return nil, err
	}

	avil.Add(avil, tmp)

	bi := &api.BalanceInfo{
		Value:    getBalance(cm.endPoint, gotAddr),
		ErcValue: cm.getBalanceInErc(gotAddr),
		FsValue:  avil,
	}

	return bi, nil
}

// return time, size, price
func (cm *ContractMgr) SettleGetStoreInfo(ctx context.Context, userID, proID uint64) (*api.StoreInfo, error) {
	ti, size, price, err := cm.getStoreInfo(userID, proID, cm.tIndex)
	if err != nil {
		return nil, err
	}

	addNonce, subNonce, err := cm.getFsInfoAggOrder(userID, proID)
	if err != nil {
		return nil, err
	}

	si := &api.StoreInfo{
		Time:     int64(ti),
		Nonce:    addNonce,
		SubNonce: subNonce,
		Size:     size,
		Price:    price,
	}

	return si, nil
}

func (cm *ContractMgr) SettlePledge(ctx context.Context, val *big.Int) error {
	logger.Debugf("%d pledge %d", cm.roleID, val)
	return cm.pledge(val)
}

func (cm *ContractMgr) SettleCharge(ctx context.Context, val *big.Int) error {
	logger.Debugf("%d charge %d", cm.roleID, val)
	return cm.Recharge(val)
}

func (cm *ContractMgr) SettlePledgeWithdraw(ctx context.Context, val *big.Int) error {
	logger.Debugf("%d cancle pledge %d", cm.roleID, val)

	err := cm.canclePledge(cm.rAddr, cm.rtAddr, cm.roleID, cm.tIndex, val, nil)
	if err != nil {
		return err
	}

	return nil
}

func (cm *ContractMgr) SettlePledgeRewardWithdraw(ctx context.Context, val *big.Int) error {
	return nil
}

func (cm *ContractMgr) SettleProIncome(ctx context.Context, val, penalty *big.Int, kindex []uint64, ksigns [][]byte) error {
	logger.Debugf("%d pro income", cm.roleID)

	err := cm.proWithdraw(cm.rAddr, cm.rtAddr, cm.roleID, cm.tIndex, val, penalty, kindex, ksigns)
	if err != nil {
		return xerrors.Errorf("%d pro income fail %s", cm.roleID, err)
	}

	return nil
}

func (cm *ContractMgr) SettleWithdraw(ctx context.Context, val *big.Int) error {
	logger.Debugf("%d withdraw", cm.roleID)

	err := cm.withdrawFromFs(cm.rtAddr, cm.roleID, cm.tIndex, val, nil)
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
	return nil
}

func (cm *ContractMgr) SettleQuitRole(ctx context.Context) error {
	return nil
}

func (cm *ContractMgr) SettleAlterPayee(ctx context.Context, p string) error {
	return nil
}
