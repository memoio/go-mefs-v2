package settle

import (
	"math/big"
	"time"

	"memoc/contracts/role"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"golang.org/x/xerrors"
)

func (cm *ContractMgr) getGroupInfo(gIndex uint64) (bool, bool, bool, uint16, *big.Int, *big.Int, common.Address, error) {
	retryCount := 0

	var isActive, isBanned, isReady bool
	var level uint16
	var size, price *big.Int
	var fsAddr common.Address

	if gIndex == 0 {
		return isActive, isBanned, isReady, level, size, price, fsAddr, xerrors.Errorf("group is zero")
	}

	client := getClient(cm.endPoint)
	defer client.Close()
	roleIns, err := role.NewRole(cm.rAddr, client)
	if err != nil {
		return isActive, isBanned, isReady, level, size, price, fsAddr, err
	}

	gIndex-- // get group info by array index actually in contract, which is gIndex minus 1
	for {
		retryCount++
		isActive, isBanned, isReady, level, size, price, fsAddr, err = roleIns.GetGroupInfo(&bind.CallOpts{
			From: cm.eAddr,
		}, gIndex)
		if err != nil {
			if retryCount > sendTransactionRetryCount {
				return isActive, isBanned, isReady, level, size, price, fsAddr, err
			}
			time.Sleep(retryGetInfoSleepTime)
			continue
		}

		return isActive, isBanned, isReady, level, size, price, fsAddr, nil
	}
}

// GetGKNum get the number of keepers in the group.
func (cm *ContractMgr) getKNumAtGroup(gIndex uint64) (uint64, error) {
	var gkNum uint64

	if gIndex == 0 {
		return gkNum, xerrors.Errorf("group index is zero")
	}

	client := getClient(cm.endPoint)
	defer client.Close()
	roleIns, err := role.NewRole(cm.rAddr, client)
	if err != nil {
		return gkNum, err
	}

	retryCount := 0

	gIndex--
	for {
		retryCount++
		gkNum, err = roleIns.GetGKNum(&bind.CallOpts{
			From: cm.eAddr,
		}, gIndex)
		if err != nil {
			if retryCount > sendTransactionRetryCount {
				return gkNum, err
			}
			time.Sleep(retryGetInfoSleepTime)
			continue
		}

		return gkNum, nil
	}
}

// GetGUPNum get the number of user、providers in the group.
func (cm *ContractMgr) getUPNumAtGroup(gIndex uint64) (uint64, uint64, error) {
	var guNum, gpNum uint64

	if gIndex == 0 {
		return guNum, gpNum, xerrors.Errorf("group index is zero")
	}

	client := getClient(cm.endPoint)
	defer client.Close()
	roleIns, err := role.NewRole(cm.rAddr, client)
	if err != nil {
		return guNum, gpNum, err
	}

	retryCount := 0
	gIndex--
	for {
		retryCount++
		guNum, gpNum, err = roleIns.GetGUPNum(&bind.CallOpts{
			From: cm.eAddr,
		}, gIndex)
		if err != nil {
			if retryCount > sendTransactionRetryCount {
				return guNum, gpNum, err
			}
			time.Sleep(retryGetInfoSleepTime)
			continue
		}

		return guNum, gpNum, nil
	}
}

func (cm *ContractMgr) GetGKNum() (uint64, error) {
	return cm.getKNumAtGroup(cm.groupID)
}

func (cm *ContractMgr) GetGUPNum() (uint64, uint64, error) {
	return cm.getUPNumAtGroup(cm.groupID)
}

// GetGroupK get keeper role index by gIndex and keeper array index.
func (cm *ContractMgr) GetGroupK(gIndex uint64, index uint64) (uint64, error) {
	var kIndex uint64

	if gIndex == 0 {
		return kIndex, xerrors.Errorf("group index is zero")
	}

	gkNum, err := cm.getKNumAtGroup(gIndex)
	if err != nil {
		return kIndex, err
	}
	if index >= gkNum {
		return kIndex, xerrors.Errorf("index %d is larger than group keeper count %d", index, gkNum)
	}

	client := getClient(cm.endPoint)
	defer client.Close()
	roleIns, err := role.NewRole(cm.rAddr, client)
	if err != nil {
		return kIndex, err
	}

	retryCount := 0
	gIndex-- // get group info by array index actually in contract, which is gIndex minus 1
	for {
		retryCount++
		kIndex, err = roleIns.GetGroupK(&bind.CallOpts{
			From: cm.eAddr,
		}, gIndex, index)
		if err != nil {
			if retryCount > sendTransactionRetryCount {
				return kIndex, err
			}
			time.Sleep(retryGetInfoSleepTime)
			continue
		}

		return kIndex, nil
	}
}

// GetGroupP get provider role index by gIndex and provider array index.
func (cm *ContractMgr) GetGroupP(gIndex uint64, index uint64) (uint64, error) {
	var pIndex uint64

	if gIndex == 0 {
		return pIndex, xerrors.Errorf("group index is zero")
	}

	_, pCount, err := cm.getUPNumAtGroup(gIndex)
	if err != nil {
		return pIndex, err
	}
	if index >= pCount {
		return pIndex, xerrors.Errorf("index %d is larger than group provider count %d", index, pCount)
	}

	client := getClient(cm.endPoint)
	defer client.Close()
	roleIns, err := role.NewRole(cm.rAddr, client)
	if err != nil {
		return pIndex, err
	}

	retryCount := 0
	gIndex-- // get group info by array index actually in contract, which is gIndex minus 1
	for {
		retryCount++
		pIndex, err = roleIns.GetGroupP(&bind.CallOpts{
			From: cm.eAddr,
		}, gIndex, index)
		if err != nil {
			if retryCount > sendTransactionRetryCount {
				return pIndex, err
			}
			time.Sleep(retryGetInfoSleepTime)
			continue
		}

		return pIndex, nil
	}
}

// GetGroupU get user role index by gIndex and user array index.
func (cm *ContractMgr) GetGroupU(gIndex uint64, index uint64) (uint64, error) {
	var uIndex uint64

	if gIndex == 0 {
		return uIndex, xerrors.Errorf("group index is zero")
	}

	uCount, _, err := cm.getUPNumAtGroup(gIndex)
	if err != nil {
		return uIndex, err
	}
	if index >= uCount {
		return uIndex, xerrors.Errorf("index %d is larger than group user count %d", index, uCount)
	}

	client := getClient(cm.endPoint)
	defer client.Close()
	roleIns, err := role.NewRole(cm.rAddr, client)
	if err != nil {
		return uIndex, err
	}

	retryCount := 0

	gIndex-- // get group info by array index actually in contract, which is gIndex minus 1
	for {
		retryCount++
		uIndex, err = roleIns.GetGU(&bind.CallOpts{
			From: cm.eAddr,
		}, gIndex, index)
		if err != nil {
			if retryCount > sendTransactionRetryCount {
				return uIndex, err
			}
			time.Sleep(retryGetInfoSleepTime)
			continue
		}

		return uIndex, nil
	}
}
