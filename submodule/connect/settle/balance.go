package settle

import (
	"context"
	"math/big"

	"golang.org/x/xerrors"
)

func (cm *ContractMgr) GetBalance(ctx context.Context, roleID uint64) (*big.Int, error) {
	avil, tmp, err := cm.iFS.GetBalance(roleID, cm.tIndex)
	avil.Add(avil, tmp)
	return avil, err
}

func (cm *ContractMgr) Withdraw(ctx context.Context, val, penalty *big.Int, ksigns [][]byte) error {
	err := cm.iRFS.ProWithdraw(cm.rAddr, cm.rtAddr, cm.roleID, cm.tIndex, val, penalty, ksigns)
	if err != nil {
		return xerrors.Errorf("%d withdraw fail %s", cm.roleID, err)
	}
	return nil
}
