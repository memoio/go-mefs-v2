package settle

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/api"
	"github.com/memoio/go-mefs-v2/lib/address"
	"github.com/memoio/go-mefs-v2/lib/pb"
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
func (cm *ContractMgr) GetRoleAddr() common.Address {
	return cm.rAddr
}

// get info
func (cm *ContractMgr) SettleGetRoleInfo(addr address.Address) (*pb.RoleInfo, error) {
	eAddr := common.BytesToAddress(utils.ToEthAddress(addr.Bytes()))
	ri, err := cm.getRoleInfo(eAddr)
	if err != nil {
		return nil, err
	}

	return ri.pri, nil
}

func (cm *ContractMgr) SettleGetRoleInfoAt(ctx context.Context, rid uint64) (*pb.RoleInfo, error) {
	gotAddr, err := cm.getAddrAt(rid)
	if err != nil {
		return nil, err
	}

	ri, err := cm.getRoleInfo(gotAddr)
	if err != nil {
		return nil, err
	}
	return ri.pri, nil
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
		RoleAddr: cm.rAddr.String(),
		ID:       gIndex,
		Level:    level,
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

func (cm *ContractMgr) SettleCanclePledge(ctx context.Context, val *big.Int) error {
	logger.Debugf("%d cancle pledge %d", cm.roleID, val)

	err := cm.canclePledge(cm.rAddr, cm.rtAddr, cm.roleID, cm.tIndex, val, nil)
	if err != nil {
		return err
	}

	return nil
}

func (cm *ContractMgr) SettleWithdraw(ctx context.Context, val, penalty *big.Int, kindex []uint64, ksigns [][]byte) error {
	logger.Debugf("%d withdraw", cm.roleID)

	ri, err := cm.SettleGetRoleInfoAt(ctx, cm.roleID)
	if err != nil {
		return err
	}

	switch ri.Type {
	case pb.RoleInfo_Provider:
		err := cm.proWithdraw(cm.rAddr, cm.rtAddr, cm.roleID, cm.tIndex, val, penalty, kindex, ksigns)
		if err != nil {
			return xerrors.Errorf("%d withdraw fail %s", cm.roleID, err)
		}
	default:
		err = cm.withdrawFromFs(cm.rtAddr, cm.roleID, cm.tIndex, val, nil)
		if err != nil {
			return xerrors.Errorf("%d withdraw fail %s", cm.roleID, err)
		}
	}

	return nil
}
