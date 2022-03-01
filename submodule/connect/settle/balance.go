package settle

import (
	"context"
	"math/big"

	"github.com/memoio/go-mefs-v2/api"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"golang.org/x/xerrors"
)

func (cm *ContractMgr) pledge(val *big.Int) error {
	bal, err := cm.iErc.BalanceOf(cm.eAddr)
	if err != nil {
		logger.Debug(err)
		return err
	}

	logger.Debugf("role pledge %d, has %d", val, bal)

	err = cm.iPP.Pledge(cm.tAddr, cm.rAddr, cm.roleID, val, nil)
	if err != nil {
		return err
	}
	if err = <-cm.status; err != nil {
		logger.Fatal("keeper pledge fail: ", err)
		return err
	}

	return nil
}

func (cm *ContractMgr) SettleGetBalanceInfo(ctx context.Context, roleID uint64) (*api.BalanceInfo, error) {
	gotAddr, err := cm.iRole.GetAddr(roleID)
	if err != nil {
		return nil, err
	}

	avil, tmp, err := cm.iFS.GetBalance(roleID, cm.tIndex)
	if err != nil {
		return nil, err
	}

	avil.Add(avil, tmp)

	ercVal, err := cm.iErc.BalanceOf(gotAddr)
	if err != nil {
		return nil, err
	}

	bi := &api.BalanceInfo{
		Value:    QueryBalance(gotAddr),
		ErcValue: ercVal,
		FsValue:  avil,
	}

	return bi, nil
}

func (cm *ContractMgr) SettleWithdraw(ctx context.Context, val, penalty *big.Int, kindex []uint64, ksigns [][]byte) error {
	logger.Debugf("%d withdraw", cm.roleID)

	ri, err := cm.SettleGetRoleInfoAt(ctx, cm.roleID)
	if err != nil {
		return err
	}

	switch ri.Type {
	case pb.RoleInfo_Provider:
		err := cm.iRFS.ProWithdraw(cm.rAddr, cm.rtAddr, cm.roleID, cm.tIndex, val, penalty, kindex, ksigns)
		if err != nil {
			return xerrors.Errorf("%d withdraw fail %s", cm.roleID, err)
		}
		if err = <-cm.status; err != nil {
			return xerrors.Errorf("%d withdraw fail %s", cm.roleID, err)
		}
	default:
		err = cm.iRole.WithdrawFromFs(cm.rtAddr, cm.roleID, cm.tIndex, val, nil)
		if err != nil {
			return xerrors.Errorf("%d withdraw fail %s", cm.roleID, err)
		}

		if err = <-cm.status; err != nil {
			return xerrors.Errorf("%d withdraw fail %s", cm.roleID, err)
		}
	}

	return nil
}

func (cm *ContractMgr) SettlePledge(ctx context.Context, val *big.Int) error {
	logger.Debugf("%d pledge %d", cm.roleID, val)
	return cm.pledge(val)
}

func (cm *ContractMgr) SettleGetPledgeInfo(ctx context.Context, roleID uint64) (*api.PledgeInfo, error) {
	tp, err := cm.iPP.TotalPledge()
	if err != nil {
		return nil, err
	}

	ep, err := cm.iPP.GetPledge(cm.tIndex)
	if err != nil {
		return nil, err
	}

	pv, err := cm.iPP.GetBalanceInPPool(roleID, cm.tIndex)
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

func (cm *ContractMgr) SettleCanclePledge(ctx context.Context, val *big.Int) error {
	logger.Debugf("%d cancle pledge %d", cm.roleID, val)

	err := cm.iPP.Withdraw(cm.rAddr, cm.rtAddr, cm.roleID, cm.tIndex, val, nil)
	if err != nil {
		return err
	}

	if err = <-cm.status; err != nil {
		return xerrors.Errorf("%d cancle pledge fail %s", cm.roleID, err)
	}

	return nil
}
