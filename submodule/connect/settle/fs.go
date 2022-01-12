package settle

import "math/big"

func (cm *ContractMgr) ProWithdraw(proID uint64, pay, lost *big.Int, ksigns [][]byte) error {
	err := cm.iRFS.ProWithdraw(cm.rAddr, cm.rtAddr, proID, cm.tIndex, pay, lost, ksigns)
	if err != nil {
		return err
	}
	if err = <-cm.status; err != nil {
		logger.Fatal("pro withdraw fail: ", err)
	}
	return err
}

// return nonce, subNonce
func (cm *ContractMgr) GetOrderInfo(userID, proID uint64) (uint64, uint64, error) {
	return cm.iFS.GetFsInfoAggOrder(userID, proID)
}

// return time, size, price
func (cm *ContractMgr) GetStoreInfo(userID, proID uint64) (uint64, uint64, *big.Int, error) {
	return cm.iFS.GetStoreInfo(userID, proID, cm.tIndex)
}
