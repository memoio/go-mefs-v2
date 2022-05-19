package settle

import (
	"math/big"
	"time"

	filesys "memoc/contracts/filesystem"
	"memoc/contracts/role"
	"memoc/contracts/rolefs"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	etypes "github.com/ethereum/go-ethereum/core/types"
	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/types"
)

func (cm *ContractMgr) getBalanceInFs(rIndex uint64, tIndex uint32) (*big.Int, *big.Int, error) {
	var avail, tmp *big.Int

	client := getClient(cm.endPoint)
	defer client.Close()
	fsIns, err := filesys.NewFileSys(cm.fsAddr, client)
	if err != nil {
		return avail, tmp, err
	}

	retryCount := 0
	for {
		retryCount++
		avail, tmp, err = fsIns.GetBalance(&bind.CallOpts{
			From: cm.eAddr,
		}, rIndex, tIndex)
		if err != nil {
			if retryCount > sendTransactionRetryCount {
				return avail, tmp, err
			}
			time.Sleep(retryGetInfoSleepTime)
			continue
		}

		return avail, tmp, nil
	}
}

// getStoreInfo Get information of storage order. return repairFs info when uIndex is 0
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

// Recharge called by user or called by others.
func (cm *ContractMgr) recharge(rTokenAddr common.Address, uIndex uint64, tIndex uint32, money *big.Int, sign []byte) error {
	client := getClient(cm.endPoint)
	defer client.Close()
	roleIns, err := role.NewRole(cm.rAddr, client)
	if err != nil {
		return err
	}

	// uIndex need to be user
	addr, err := cm.getAddrAt(uIndex)
	if err != nil {
		return err
	}
	ri, err := cm.getRoleInfo(addr)
	if err != nil {
		return err
	}
	if ri.isBanned || ri.pri.Type != pb.RoleInfo_User {
		return xerrors.Errorf("invalid user")
	}

	// todo: check tindex

	// check allowance
	allo, err := cm.getAllowance(addr, cm.fsAddr)
	if err != nil {
		return err
	}
	if allo.Cmp(money) < 0 {
		return xerrors.Errorf("allowance is not enough")
	}

	logger.Debug("begin Recharge in Role contract...")

	auth, err := makeAuth(cm.chainID, cm.hexSK, nil, nil)
	if err != nil {
		return err
	}
	tx, err := roleIns.Recharge(auth, uIndex, tIndex, money, sign)
	if err != nil {
		logger.Debug("Recharge Err:", err)
		return err
	}

	return checkTx(cm.endPoint, tx, "Recharge")
}

func (cm *ContractMgr) Recharge(val *big.Int) error {
	logger.Debug("recharge user: ", cm.roleID, val)

	avail, _, err := cm.getBalanceInFs(cm.roleID, cm.tIndex)
	if err != nil {
		return err
	}

	if avail.Cmp(val) < 0 {
		bal := cm.getBalanceInErc(cm.eAddr)

		logger.Debugf("recharge user approve: %d, has: %d", val, bal)

		if bal.Cmp(val) < 0 {
			val.Set(bal)
		}

		err = cm.approve(cm.fsAddr, val)
		if err != nil {
			return err
		}

		logger.Debug("recharge user charge: ", val)

		err = cm.recharge(cm.rtAddr, cm.roleID, 0, val, nil)
		if err != nil {
			return err
		}

		logger.Debug("recharge user charged: ", val)
	}

	avail, _, err = cm.getBalanceInFs(cm.roleID, cm.tIndex)
	if err != nil {
		return err
	}

	logger.Debug("recharge user has balance: ", avail)

	return nil
}

// AddOrder called by keeper? Add the storage order in the FileSys.
// hash(uIndex, pIndex, _start, end, _size, nonce, tIndex, sPrice)?
// 目前合约中还未对签名进行判断处理
// nonce需要从0开始依次累加
// 调用该函数前，需要admin为RoleFS合约账户赋予MINTER_ROLE权限
func (cm *ContractMgr) addOrder(roleAddr, rTokenAddr common.Address, uIndex, pIndex, start, end, size, nonce uint64, tIndex uint32, sprice *big.Int, usign, psign []byte) (*etypes.Transaction, error) {
	client := getClient(cm.endPoint)
	defer client.Close()

	roleFSIns, err := rolefs.NewRoleFS(cm.rfsAddr, client)
	if err != nil {
		return nil, err
	}

	// check start,end,size
	if size == 0 {
		return nil, xerrors.Errorf("size is zero")
	}
	if end <= start {
		return nil, xerrors.Errorf("start should after end")
	}
	if end%types.Day != 0 {
		return nil, xerrors.Errorf("end %d should be aligned to 86400(one day)", end)
	}
	// todo: check uIndex,pIndex,gIndex,tIndex

	// check balance

	// check nonce

	// check start
	_time, _, _, err := cm.getStoreInfo(uIndex, pIndex, tIndex)
	if err != nil {
		return nil, err
	}
	if end < _time {
		logger.Debug("end:", start, " should be more than time:", _time)
		return nil, xerrors.New("end error")
	}
	// check whether rolefsAddr has Minter-Role

	logger.Debug("begin AddOrder in RoleFS contract...")

	auth, err := makeAuth(cm.chainID, cm.hexSK, nil, nil)
	if err != nil {
		return nil, err
	}

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
		return nil, err
	}

	//return checkTx(cm.endPoint, tx, "AddOrder")
	return tx, nil
}

// SubOrder called by keeper? Reduce the storage order in the FileSys.
// hash(uIndex, pIndex, _start, end, _size, nonce, tIndex, sPrice)?
// 目前合约中还未对签名信息做判断处理
func (cm *ContractMgr) subOrder(roleAddr, rTokenAddr common.Address, uIndex, pIndex, start, end, size, nonce uint64, tIndex uint32, sprice *big.Int, usign, psign []byte) (*etypes.Transaction, error) {
	client := getClient(cm.endPoint)
	defer client.Close()
	roleFSIns, err := rolefs.NewRoleFS(cm.rfsAddr, client)
	if err != nil {
		return nil, err
	}

	// check size,start.end

	// check uIndex,pIndex,gIndex,tIndex

	// check nonce

	// check size

	logger.Debug("begin SubOrder in RoleFS contract...")

	auth, err := makeAuth(cm.chainID, cm.hexSK, nil, nil)
	if err != nil {
		return nil, err
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
		return nil, err
	}

	//return checkTx(cm.endPoint, tx, "SubOrder")

	return tx, nil
}

func (cm *ContractMgr) AddOrder(so *types.SignedOrder) error {
	avil, _, err := cm.getBalanceInFs(so.UserID, so.TokenIndex)
	if err != nil {
		return err
	}

	pay := new(big.Int).Set(so.Price)
	pay.Mul(pay, big.NewInt(so.End-so.Start))
	pay.Mul(pay, big.NewInt(12))
	pay.Div(pay, big.NewInt(10))

	if pay.Cmp(avil) > 0 {
		return xerrors.Errorf("add order insufficiecnt funds user %d has balance %s, require %s", so.UserID, types.FormatMemo(avil), types.FormatMemo(pay))
	}

	tx, err := cm.addOrder(cm.rAddr, cm.rtAddr, so.UserID, so.ProID, uint64(so.Start), uint64(so.End), so.Size, so.Nonce, so.TokenIndex, so.Price, so.Usign.Data, so.Psign.Data)
	if err != nil {
		return xerrors.Errorf("add order user %d pro %d nonce %d size %d start %d end %d, price %d balance %s fail %w", so.UserID, so.ProID, so.Nonce, so.Size, so.Start, so.End, so.Price, types.FormatMemo(avil), err)
	}

	err = checkTx(cm.endPoint, tx, "AddOrder")
	if err != nil {
		logger.Warnf("add order user %d pro %d nonce %d size %d start %d end %d, price %d balance %s tx fail %w", so.UserID, so.ProID, so.Nonce, so.Size, so.Start, so.End, so.Price, types.FormatMemo(avil), err)
		return err
	} else {
		logger.Debugf("add order user %d pro %d nonce %d size %d", so.UserID, so.ProID, so.Nonce, so.Size)
	}

	return nil
}

func (cm *ContractMgr) SubOrder(so *types.SignedOrder) error {
	tx, err := cm.subOrder(cm.rAddr, cm.rtAddr, so.UserID, so.ProID, uint64(so.Start), uint64(so.End), so.Size, so.Nonce, so.TokenIndex, so.Price, so.Usign.Data, so.Psign.Data)
	if err != nil {
		return xerrors.Errorf("sub order user %d pro %d nonce %d size %d start %d end %d, price %d fail %w", so.UserID, so.ProID, so.Nonce, so.Size, so.Start, so.End, so.Price, err)
	}

	go func(rtx *etypes.Transaction) {
		err := checkTx(cm.endPoint, tx, "SubOrder")
		if err != nil {
			logger.Warnf("sub order user %d pro %d nonce %d size %d start %d end %d, price %d tx fail %w", so.UserID, so.ProID, so.Nonce, so.Size, so.Start, so.End, so.Price, err)
		} else {
			logger.Debugf("sub order user %d pro %d nonce %d size %d", so.UserID, so.ProID, so.Nonce, so.Size)
		}
	}(tx)

	return nil
}
