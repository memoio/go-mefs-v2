package settle

import "math/big"

func (cm *ContractMgr) GetBalanceInFs(roleID uint64) (*big.Int, *big.Int, error) {
	return cm.iFS.GetBalance(roleID, cm.tIndex)
}
