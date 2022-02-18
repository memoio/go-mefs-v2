package settle

import (
	"context"
	"math/big"

	"github.com/memoio/go-mefs-v2/api"
)

func (cm *ContractMgr) ProWithdraw(proID uint64, pay, lost *big.Int, kindex []uint64, ksigns [][]byte) error {
	err := cm.iRFS.ProWithdraw(cm.rAddr, cm.rtAddr, proID, cm.tIndex, pay, lost, kindex, ksigns)
	if err != nil {
		return err
	}
	if err = <-cm.status; err != nil {
		logger.Fatal("pro withdraw fail: ", err)
	}
	return err
}

// return time, size, price
func (cm *ContractMgr) SettleGetStoreInfo(ctx context.Context, userID, proID uint64) (*api.StoreInfo, error) {
	ti, size, price, err := cm.iFS.GetStoreInfo(userID, proID, cm.tIndex)
	if err != nil {
		return nil, err
	}

	addNonce, subNonce, err := cm.iFS.GetFsInfoAggOrder(userID, proID)
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
