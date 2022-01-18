package settle

import (
	"context"
	"math/big"

	"github.com/memoio/go-mefs-v2/api"
)

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
func (cm *ContractMgr) GetStoreInfo(ctx context.Context, userID, proID uint64) (*api.StoreInfo, error) {
	ti, size, price, err := cm.iFS.GetStoreInfo(userID, proID, cm.tIndex)
	if err != nil {
		return nil, err
	}

	si := &api.StoreInfo{
		Time:  int64(ti),
		Size:  size,
		Price: price,
	}

	return si, nil
}
