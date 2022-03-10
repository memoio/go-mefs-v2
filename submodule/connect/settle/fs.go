package settle

import (
	"context"
	"log"
	"math/big"
	filesys "memoc/contracts/filesystem"
	"memoc/contracts/rolefs"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/memoio/go-mefs-v2/api"
	"golang.org/x/xerrors"
)

// GetStoreInfo Get information of storage order. return repairFs info when uIndex is 0
func (cm *ContractMgr) getStoreInfo(uIndex uint64, pIndex uint64, tIndex uint32) (uint64, uint64, *big.Int, error) {
	var _time, size uint64
	var price *big.Int

	client := getClient(cm.endPoint)
	defer client.Close()
	fsIns, err := filesys.NewFileSys(cm.fsAddr, client)
	if err != nil {
		return _time, size, price, err
	}

	retryCount := 0
	for {
		retryCount++
		_time, size, price, err = fsIns.GetStoreInfo(
			&bind.CallOpts{From: cm.eAddr},
			uIndex,
			pIndex,
			tIndex,
		)
		if err != nil {
			if retryCount > sendTransactionRetryCount {
				return _time, size, price, err
			}
			time.Sleep(retryGetInfoSleepTime)
			continue
		}

		return _time, size, price, nil
	}
}

// GetFsInfoAggOrder Get information of aggOrder. return repairFs info when uIndex is 0
func (cm *ContractMgr) getFsInfoAggOrder(uIndex uint64, pIndex uint64) (uint64, uint64, error) {
	var nonce uint64
	var subNonce uint64

	client := getClient(cm.endPoint)
	defer client.Close()
	fsIns, err := filesys.NewFileSys(cm.fsAddr, client)
	if err != nil {
		return nonce, subNonce, err
	}

	retryCount := 0
	for {
		retryCount++
		nonce, subNonce, err = fsIns.GetFsInfoAggOrder(&bind.CallOpts{
			From: cm.eAddr,
		}, uIndex, pIndex)
		if err != nil {
			if retryCount > sendTransactionRetryCount {
				return nonce, subNonce, err
			}
			time.Sleep(retryGetInfoSleepTime)
			continue
		}

		return nonce, subNonce, nil
	}
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

// AddOrder called by keeper? Add the storage order in the FileSys.
// hash(uIndex, pIndex, _start, end, _size, nonce, tIndex, sPrice)?
// 目前合约中还未对签名进行判断处理
// nonce需要从0开始依次累加
// 调用该函数前，需要admin为RoleFS合约账户赋予MINTER_ROLE权限
func (cm *ContractMgr) addOrder(roleAddr, rTokenAddr common.Address, uIndex, pIndex, start, end, size, nonce uint64, tIndex uint32, sprice *big.Int, usign, psign []byte) error {
	client := getClient(cm.endPoint)
	defer client.Close()

	roleFSIns, err := rolefs.NewRoleFS(cm.rfsAddr, client)
	if err != nil {
		return err
	}

	// check start,end,size
	if size == 0 {
		return xerrors.Errorf("size is zero")
	}
	if end <= start {
		return xerrors.Errorf("start should after end")
	}
	if (end/86400)*86400 != end {
		return xerrors.Errorf("end %d should be aligned to 86400(one day)", end)
	}
	// todo: check uIndex,pIndex,gIndex,tIndex

	// check balance

	// check nonce

	// check start
	_time, _, _, err := cm.getStoreInfo(uIndex, pIndex, tIndex)
	if err != nil {
		return err
	}
	if end < _time {
		log.Println("end:", start, " should be more than time:", _time)
		return xerrors.New("end error")
	}
	// check whether rolefsAddr has Minter-Role

	log.Println("begin AddOrder in RoleFS contract...")

	// txopts.gasPrice参数赋值为nil
	auth, errMA := makeAuth(cm.hexSK, nil, nil)
	if errMA != nil {
		return errMA
	}

	// 构建交易，通过 sendTransaction 将交易发送至 pending pool
	// tx, err := roleFSIns.AddOrder(auth, uIndex, pIndex, start, end, size, nonce, tIndex, sprice, usign, psign, ksigns)
	// use struct to call addOder
	ps := rolefs.AOParams{
		UIndex: uIndex,
		PIndex: pIndex,
		Start:  start,
		End:    end,
		Size:   size,
		Nonce:  nonce,
		TIndex: tIndex,
		SPrice: sprice,
		Usign:  usign,
		Psign:  psign,
	}
	tx, err := roleFSIns.AddOrder(auth, ps)
	if err != nil {
		log.Println("AddOrder Err:", err)
		return err
	}

	return checkTx(cm.endPoint, tx, "AddOrder")
}

// SubOrder called by keeper? Reduce the storage order in the FileSys.
// hash(uIndex, pIndex, _start, end, _size, nonce, tIndex, sPrice)?
// 目前合约中还未对签名信息做判断处理
func (cm *ContractMgr) subOrder(roleAddr, rTokenAddr common.Address, uIndex, pIndex, start, end, size, nonce uint64, tIndex uint32, sprice *big.Int, usign, psign []byte) error {
	client := getClient(cm.endPoint)
	defer client.Close()
	roleFSIns, err := rolefs.NewRoleFS(cm.rfsAddr, client)
	if err != nil {
		return err
	}

	// check size,start.end

	// check uIndex,pIndex,gIndex,tIndex

	// check nonce

	// check size

	log.Println("begin SubOrder in RoleFS contract...")

	// txopts.gasPrice参数赋值为nil
	auth, err := makeAuth(cm.hexSK, nil, nil)
	if err != nil {
		return err
	}

	// prepair params for subOrder
	ps := rolefs.SOParams{
		KIndex: 0,
		UIndex: uIndex,
		PIndex: pIndex,
		Start:  start,
		End:    end,
		Size:   size,
		Nonce:  nonce,
		TIndex: tIndex,
		SPrice: sprice,
		Usign:  usign,
		Psign:  psign,
	}
	tx, err := roleFSIns.SubOrder(auth, ps)
	if err != nil {
		return err
	}

	return checkTx(cm.endPoint, tx, "SubOrder")
}
