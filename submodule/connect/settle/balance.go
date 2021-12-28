package settle

import (
	"context"
	"math/big"
)

func (cm *ContractMgr) GetBalance(ctx context.Context, roleID uint64) (*big.Int, error) {
	avil, tmp, err := cm.iFS.GetBalance(roleID, cm.tIndex)
	avil.Add(avil, tmp)
	return avil, err
}

func (cm *ContractMgr) Withdraw(ctx context.Context, val, penalty *big.Int) error {
	ksigns := make([][]byte, 10)
	err := cm.iRFS.ProWithdraw(cm.rAddr, cm.rtAddr, cm.roleID, cm.tIndex, val, penalty, ksigns)
	return err
}
