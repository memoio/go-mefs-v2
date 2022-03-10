package settle

import (
	"context"
	"log"
	"math/big"
	"memoc/contracts/erc20"
	filesys "memoc/contracts/filesystem"
	"memoc/contracts/pledgepool"
	"memoc/contracts/role"
	"memoc/contracts/rolefs"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/memoio/go-mefs-v2/api"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"golang.org/x/xerrors"
)

func (cm *ContractMgr) getBalanceInErc(addr common.Address) *big.Int {
	balance := new(big.Int)

	client := getClient(cm.endPoint)
	defer client.Close()

	erc20Ins, err := erc20.NewERC20(cm.tAddr, client)
	if err != nil {
		return balance
	}

	retryCount := 0
	for {
		retryCount++
		balance, err = erc20Ins.BalanceOf(&bind.CallOpts{
			From: cm.eAddr,
		}, addr)
		if err != nil {
			if retryCount > sendTransactionRetryCount {
				return balance
			}
			time.Sleep(retryGetInfoSleepTime)
			continue
		}

		return balance
	}

	return balance
}

func (cm *ContractMgr) getAllowance(sender, reciver common.Address) (*big.Int, error) {
	var allowance *big.Int

	client := getClient(cm.endPoint)
	defer client.Close()

	erc20Ins, err := erc20.NewERC20(cm.tAddr, client)
	if err != nil {
		return allowance, err
	}

	retryCount := 0
	for {
		retryCount++
		allowance, err = erc20Ins.Allowance(&bind.CallOpts{
			From: cm.eAddr,
		}, sender, reciver)
		if err != nil {
			if retryCount > sendTransactionRetryCount {
				return allowance, err
			}
			time.Sleep(retryGetInfoSleepTime)
			continue
		}

		return allowance, nil
	}
}

func (cm *ContractMgr) increaseAllowance(recipient common.Address, value *big.Int) error {
	if recipient.Hex() == InvalidAddr {
		return xerrors.Errorf("recipient is an invalid addr")
	}

	client := getClient(cm.endPoint)
	defer client.Close()

	erc20Ins, err := erc20.NewERC20(cm.tAddr, client)
	if err != nil {
		return err
	}

	// the given allowance shouldn't exceeds the balance
	bal := cm.getBalanceInErc(cm.eAddr)

	allo, err := cm.getAllowance(cm.eAddr, recipient)
	if err != nil {
		return err
	}
	sum := big.NewInt(0)
	sum.Add(value, allo)
	if bal.Cmp(sum) < 0 {
		return xerrors.Errorf("%s balance %d is lower than %d", cm.eAddr, bal, sum)
	}

	logger.Debug("begin IncreaseAllowance to", recipient.Hex(), " with value", value, " in ERC20 contract...")

	// txopts.gasPrice参数赋值为nil
	auth, err := makeAuth(cm.hexSK, nil, nil)
	if err != nil {
		return err
	}
	// 构建交易，通过 sendTransaction 将交易发送至 pending pool
	tx, err := erc20Ins.IncreaseAllowance(auth, recipient, value)
	// ====面临的失败场景====
	// 交易参数通过abi打包失败;payable检测失败;构造types.Transaction结构体时遇到的失败问题（opt默认值字段通过预言机获取）；
	// 交易发送失败，直接返回错误
	if err != nil {
		return err
	}
	// 交易成功发送至 pending pool , 后台检查交易是否成功执行,执行失败则将错误传入 ContractModule 中的 status 通道
	// 交易若由于链上拥堵而短时间无法被打包，不再增加gasPrice重新发送
	return checkTx(cm.endPoint, tx, "IncreaseAllowance")
}

func (cm *ContractMgr) approve(addr common.Address, value *big.Int) error {
	client := getClient(cm.endPoint)
	defer client.Close()
	erc20Ins, err := erc20.NewERC20(cm.tAddr, client)
	if err != nil {
		return err
	}

	if addr.Hex() == InvalidAddr {
		return xerrors.Errorf("invlaid address")
	}

	// need to determine whether the account balance is enough to approve.
	bal := cm.getBalanceInErc(cm.eAddr)
	if bal.Cmp(value) < 0 {
		return xerrors.Errorf("Balance %d is not enough %d", bal, value)
	}

	logger.Debug("begin Approve", addr.Hex(), " with value", value, " in ERC20 contract...")

	auth, err := makeAuth(cm.hexSK, nil, nil)
	if err != nil {
		return err
	}
	tx, err := erc20Ins.Approve(auth, addr, value)
	if err != nil {
		return err
	}

	return checkTx(cm.endPoint, tx, "Approve")
}

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

func (cm *ContractMgr) SettleGetBalanceInfo(ctx context.Context, roleID uint64) (*api.BalanceInfo, error) {
	gotAddr, err := cm.getAddrAt(roleID)
	if err != nil {
		return nil, err
	}

	avil, tmp, err := cm.getBalanceInFs(roleID, cm.tIndex)
	if err != nil {
		return nil, err
	}

	avil.Add(avil, tmp)

	bi := &api.BalanceInfo{
		Value:    getBalance(cm.endPoint, gotAddr),
		ErcValue: cm.getBalanceInErc(gotAddr),
		FsValue:  avil,
	}

	return bi, nil
}

func (cm *ContractMgr) pledge(val *big.Int) error {
	bal := cm.getBalanceInErc(cm.eAddr)
	// check balance
	if bal.Cmp(val) < 0 {
		return xerrors.Errorf("addr %s balance %d is lower than pledge %d", cm.eAddr, bal, val)
	}

	logger.Debugf("role pledge %d, has %d", val, bal)

	//err = cm.iPP.Pledge(cm.tAddr, cm.rAddr, cm.roleID, val, nil)

	client := getClient(cm.endPoint)
	defer client.Close()

	addr, err := cm.getAddrAt(cm.roleID)
	if err != nil {
		return err
	}

	ri, err := cm.getRoleInfo(addr)
	if err != nil {
		return err
	}

	if ri.isBanned {
		return xerrors.Errorf("%d is banned", cm.roleID)
	}

	// check whether the allowance[addr][pledgePoolAddr] is not less than value, if not, will approve automatically by code.
	erc20Ins, err := erc20.NewERC20(cm.tAddr, client)
	if err != nil {
		return err
	}

	allo, err := erc20Ins.Allowance(&bind.CallOpts{
		From: cm.eAddr,
	}, addr, cm.ppAddr)
	if err != nil {
		return err
	}
	if allo.Cmp(val) < 0 {
		tmp := big.NewInt(0)
		tmp.Sub(val, allo)
		logger.Debug("The allowance of ", addr, " to ", cm.ppAddr, " is not enough, also need to add allowance", tmp)
		// if called by the account itself， then call IncreaseAllowance directly.
		err = cm.increaseAllowance(cm.ppAddr, tmp)
		if err != nil {
			return err
		}
	}

	logger.Debug("begin Pledge in PledgePool contract with value", val, " and rindex", ri.pri.ID)

	ppIns, err := pledgepool.NewPledgePool(cm.ppAddr, client)
	if err != nil {
		return err
	}

	// txopts.gasPrice参数赋值为nil
	auth, errMA := makeAuth(cm.hexSK, nil, nil)
	if errMA != nil {
		return errMA
	}
	// 构建交易，通过 sendTransaction 将交易发送至 pending pool
	tx, err := ppIns.Pledge(auth, ri.pri.ID, val, nil)
	// ====面临的失败场景====
	// 交易参数通过abi打包失败;payable检测失败;构造types.Transaction结构体时遇到的失败问题（opt默认值字段通过预言机获取）；
	// 交易发送失败，直接返回错误
	if err != nil {
		return err
	}
	// 交易成功发送至 pending pool , 后台检查交易是否成功执行,执行失败则将错误传入 ContractModule 中的 status 通道
	// 交易若由于链上拥堵而短时间无法被打包，不再增加gasPrice重新发送

	return checkTx(cm.endPoint, tx, "Pledge")
}

func (cm *ContractMgr) SettlePledge(ctx context.Context, val *big.Int) error {
	logger.Debugf("%d pledge %d", cm.roleID, val)
	return cm.pledge(val)
}

func (cm *ContractMgr) getTotalPledge() (*big.Int, error) {
	var amount *big.Int

	client := getClient(cm.endPoint)
	defer client.Close()

	ppIns, err := pledgepool.NewPledgePool(cm.ppAddr, client)
	if err != nil {
		return amount, err
	}

	retryCount := 0
	for {
		retryCount++
		amount, err = ppIns.TotalPledge(&bind.CallOpts{
			From: cm.eAddr,
		})
		if err != nil {
			if retryCount > sendTransactionRetryCount {
				return amount, err
			}
			time.Sleep(retryGetInfoSleepTime)
			continue
		}

		return amount, nil
	}
}

// GetPledge Get all pledge amount in specified token.
func (cm *ContractMgr) GetPledge(tindex uint32) (*big.Int, error) {
	var amount *big.Int

	client := getClient(cm.endPoint)
	defer client.Close()

	ppIns, err := pledgepool.NewPledgePool(cm.ppAddr, client)
	if err != nil {
		return amount, err
	}

	retryCount := 0
	for {
		retryCount++
		amount, err = ppIns.GetPledge(&bind.CallOpts{
			From: cm.eAddr,
		}, tindex)
		if err != nil {
			if retryCount > sendTransactionRetryCount {
				return amount, err
			}
			time.Sleep(retryGetInfoSleepTime)
			continue
		}

		return amount, nil
	}
}

// GetBalanceInPPool Get balance of the account related rindex in specified token.
func (cm *ContractMgr) getBalanceInPPool(rindex uint64, tindex uint32) (*big.Int, error) {
	var amount *big.Int

	client := getClient(cm.endPoint)
	defer client.Close()

	ppIns, err := pledgepool.NewPledgePool(cm.ppAddr, client)
	if err != nil {
		return amount, err
	}

	retryCount := 0
	for {
		retryCount++
		amount, err = ppIns.GetBalance(&bind.CallOpts{
			From: cm.eAddr,
		}, rindex, tindex)
		if err != nil {
			if retryCount > sendTransactionRetryCount {
				return amount, err
			}
			time.Sleep(retryGetInfoSleepTime)
			continue
		}

		return amount, nil
	}
}

func (cm *ContractMgr) SettleGetPledgeInfo(ctx context.Context, roleID uint64) (*api.PledgeInfo, error) {
	tp, err := cm.getTotalPledge()
	if err != nil {
		return nil, err
	}

	ep, err := cm.GetPledge(cm.tIndex)
	if err != nil {
		return nil, err
	}

	pv, err := cm.getBalanceInPPool(roleID, cm.tIndex)
	if err != nil {
		return nil, err
	}

	pi := &api.PledgeInfo{
		Value:    pv,
		ErcTotal: ep,
		Total:    tp,
	}
	return pi, nil
}

func (cm *ContractMgr) canclePledge(roleAddr, rTokenAddr common.Address, rindex uint64, tindex uint32, value *big.Int, sign []byte) error {
	client := getClient(cm.endPoint)
	defer client.Close()

	ppIns, err := pledgepool.NewPledgePool(cm.ppAddr, client)
	if err != nil {
		return err
	}

	// check if rindex is banned
	addr, err := cm.getAddrAt(cm.roleID)
	if err != nil {
		return err
	}

	ri, err := cm.getRoleInfo(addr)
	if err != nil {
		return err
	}

	if ri.isBanned {
		return xerrors.Errorf("%d is banned", cm.roleID)
	}

	// check if tindex is valid
	logger.Debug("begin Withdraw in PledgePool contract with value", value, " and rindex", rindex, " and tindex", tindex, " ...")

	auth, errMA := makeAuth(cm.hexSK, nil, nil)
	if errMA != nil {
		return errMA
	}
	tx, err := ppIns.Withdraw(auth, rindex, tindex, value, sign)
	if err != nil {
		return err
	}

	return checkTx(cm.endPoint, tx, "Withdraw")
}

func (cm *ContractMgr) SettleCanclePledge(ctx context.Context, val *big.Int) error {
	logger.Debugf("%d cancle pledge %d", cm.roleID, val)

	err := cm.canclePledge(cm.rAddr, cm.rtAddr, cm.roleID, cm.tIndex, val, nil)
	if err != nil {
		return err
	}

	return nil
}

func (cm *ContractMgr) proWithdraw(roleAddr, rTokenAddr common.Address, pIndex uint64, tIndex uint32, pay, lost *big.Int, kIndexes []uint64, ksigns [][]byte) error {
	client := getClient(cm.endPoint)
	defer client.Close()
	roleFSIns, err := rolefs.NewRoleFS(cm.rfsAddr, client)
	if err != nil {
		return err
	}

	// check pIndex
	addr, err := cm.getAddrAt(pIndex)
	if err != nil {
		return err
	}
	ri, err := cm.getRoleInfo(addr)
	if err != nil {
		return err
	}
	if !ri.isActive || ri.isBanned || ri.pri.Type != pb.RoleInfo_Provider {
		return xerrors.Errorf("%d should be active, not be banned, roleType should be provider", pIndex)
	}

	// check ksigns's length
	gkNum, err := cm.getKNumAtGroup(ri.pri.GroupID)
	if err != nil {
		return err
	}
	l := int(gkNum * 2 / 3)
	le := len(ksigns)
	if le < l {
		log.Println("ksigns length", le, " shouldn't be less than", l)
		return xerrors.Errorf("ksigns length %d shouldn't be less than %d", le, l)
	}

	log.Println("begin call ProWithdraw in RoleFS contract...")

	// txopts.gasPrice参数赋值为nil
	auth, err := makeAuth(cm.hexSK, nil, nil)
	if err != nil {
		return err
	}

	// get provider address for calling proWithdraw
	proAddr, err := cm.getAddrAt(pIndex)
	if err != nil {
		return err
	}
	// get token address from tIndex for calling proWithdraw

	// prepare params for subOrder
	ps := rolefs.PWParams{
		PIndex:   pIndex,
		TIndex:   tIndex,
		PAddr:    proAddr,
		TAddr:    cm.tAddr,
		Pay:      pay,
		Lost:     lost,
		KIndexes: kIndexes,
		Ksigns:   ksigns,
	}
	tx, err := roleFSIns.ProWithdraw(auth, ps)
	if err != nil {
		log.Println("ProWithdraw Err:", err)
		return err
	}

	return checkTx(cm.endPoint, tx, "ProWithdraw")
}

// WithdrawFromFs called by memo-role or called by others.
// foundation、user、keeper withdraw money from filesystem
func (cm *ContractMgr) withdrawFromFs(rTokenAddr common.Address, rIndex uint64, tIndex uint32, amount *big.Int, sign []byte) error {
	client := getClient(cm.endPoint)
	defer client.Close()
	roleIns, err := role.NewRole(cm.rAddr, client)
	if err != nil {
		return err
	}

	// check amount
	if amount.Cmp(big.NewInt(0)) <= 0 {
		return xerrors.New("amount shouldn't be 0")
	}

	// check tindex

	// check rIndex

	// 需要存在有效的gIndex

	// 非 foundation 取回余额

	log.Println("begin WithdrawFromFs in Role contract...")

	// txopts.gasPrice参数赋值为nil
	auth, err := makeAuth(cm.hexSK, nil, nil)
	if err != nil {
		return err
	}
	tx, err := roleIns.WithdrawFromFs(auth, rIndex, tIndex, amount, sign)
	if err != nil {
		return err
	}

	return checkTx(cm.endPoint, tx, "WithdrawFromFs")
}

func (cm *ContractMgr) SettleWithdraw(ctx context.Context, val, penalty *big.Int, kindex []uint64, ksigns [][]byte) error {
	logger.Debugf("%d withdraw", cm.roleID)

	ri, err := cm.SettleGetRoleInfoAt(ctx, cm.roleID)
	if err != nil {
		return err
	}

	switch ri.Type {
	case pb.RoleInfo_Provider:
		err := cm.proWithdraw(cm.rAddr, cm.rtAddr, cm.roleID, cm.tIndex, val, penalty, kindex, ksigns)
		if err != nil {
			return xerrors.Errorf("%d withdraw fail %s", cm.roleID, err)
		}
	default:
		err = cm.withdrawFromFs(cm.rtAddr, cm.roleID, cm.tIndex, val, nil)
		if err != nil {
			return xerrors.Errorf("%d withdraw fail %s", cm.roleID, err)
		}
	}

	return nil
}
