package settle

import (
	"context"
	"math/big"

	"github.com/memoio/go-mefs-v2/api"
	"golang.org/x/xerrors"
)

func (cm *ContractMgr) GetBalance(ctx context.Context, roleID uint64) (*api.BalanceInfo, error) {
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

func (cm *ContractMgr) Withdraw(ctx context.Context, val, penalty *big.Int, ksigns [][]byte) error {
	logger.Debugf("%d withdraw", cm.roleID)
	err := cm.iRFS.ProWithdraw(cm.rAddr, cm.rtAddr, cm.roleID, cm.tIndex, val, penalty, ksigns)
	if err != nil {
		return xerrors.Errorf("%d withdraw fail %s", cm.roleID, err)
	}
	return nil
}
