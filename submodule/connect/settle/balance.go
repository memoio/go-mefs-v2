package settle

import (
	"math/big"
	"time"

	"memoc/contracts/erc20"
	"memoc/contracts/role"
	"memoc/contracts/rolefs"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"golang.org/x/xerrors"
)

// erc20 related
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
	sum := new(big.Int).Add(value, allo)
	if bal.Cmp(sum) < 0 {
		return xerrors.Errorf("%s balance %d is lower than %d", cm.eAddr, bal, sum)
	}

	logger.Debug("begin IncreaseAllowance to", recipient.Hex(), " with value", value, " in ERC20 contract...")

	auth, err := makeAuth(cm.chainID, cm.hexSK, nil, nil)
	if err != nil {
		return err
	}
	tx, err := erc20Ins.IncreaseAllowance(auth, recipient, value)
	if err != nil {
		return err
	}
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

	auth, err := makeAuth(cm.chainID, cm.hexSK, nil, nil)
	if err != nil {
		return err
	}
	tx, err := erc20Ins.Approve(auth, addr, value)
	if err != nil {
		return err
	}

	return checkTx(cm.endPoint, tx, "Approve")
}

func (cm *ContractMgr) proWithdraw(roleAddr, rTokenAddr common.Address, pIndex uint64, tIndex uint32, pay, lost *big.Int, kIndexes []uint64, ksigns [][]byte) error {
	if tIndex != cm.tIndex {
		return xerrors.Errorf("token index %d is not supported", tIndex)
	}

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
		logger.Debug("ksigns length", le, " shouldn't be less than", l)
		return xerrors.Errorf("ksigns length %d shouldn't be less than %d", le, l)
	}

	logger.Debug("begin call ProWithdraw in RoleFS contract...")

	auth, err := makeAuth(cm.chainID, cm.hexSK, nil, nil)
	if err != nil {
		return err
	}

	proAddr, err := cm.getAddrAt(pIndex)
	if err != nil {
		return err
	}
	// get token address from tIndex for calling proWithdraw

	// prepare params for pro withdraw
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
		logger.Debug("ProWithdraw Err:", err)
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

	logger.Debug("begin WithdrawFromFs in Role contract...")

	auth, err := makeAuth(cm.chainID, cm.hexSK, nil, nil)
	if err != nil {
		return err
	}
	tx, err := roleIns.WithdrawFromFs(auth, rIndex, tIndex, amount, sign)
	if err != nil {
		return err
	}

	return checkTx(cm.endPoint, tx, "WithdrawFromFs")
}
