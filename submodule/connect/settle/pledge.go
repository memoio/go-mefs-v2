package settle

import (
	"math/big"
	"time"

	"memoc/contracts/erc20"
	"memoc/contracts/pledgepool"
	"memoc/contracts/role"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"golang.org/x/xerrors"
)

// get minimum pledge value for keeper
func (cm *ContractMgr) getPledgeK() (*big.Int, error) {
	pk := big.NewInt(0)
	client := getClient(cm.endPoint)
	defer client.Close()
	roleIns, err := role.NewRole(cm.rAddr, client)
	if err != nil {
		return pk, err
	}

	retryCount := 0
	for {
		retryCount++
		pk, err = roleIns.PledgeK(&bind.CallOpts{
			From: cm.eAddr,
		})
		if err != nil {
			if retryCount > sendTransactionRetryCount {
				return pk, err
			}
			time.Sleep(retryGetInfoSleepTime)
			continue
		}

		return pk, nil
	}
}

// get minimum pledge value for provider
func (cm *ContractMgr) getPledgeP() (*big.Int, error) {
	pk := big.NewInt(0)
	client := getClient(cm.endPoint)
	defer client.Close()
	roleIns, err := role.NewRole(cm.rAddr, client)
	if err != nil {
		return pk, err
	}

	retryCount := 0
	for {
		retryCount++
		pk, err = roleIns.PledgeP(&bind.CallOpts{
			From: cm.eAddr,
		})
		if err != nil {
			if retryCount > sendTransactionRetryCount {
				return pk, err
			}
			time.Sleep(retryGetInfoSleepTime)
			continue
		}

		return pk, nil
	}
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

// getPledge Get all pledge amount in specified token.
// pledge + reward
func (cm *ContractMgr) getPledgeAmount(tindex uint32) (*big.Int, error) {
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

// getBalanceInPPool Get balance of the account related rindex in specified token.
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

func (cm *ContractMgr) pledge(val *big.Int) error {
	// check balance
	bal := cm.getBalanceInErc(cm.eAddr)
	if bal.Cmp(val) < 0 {
		return xerrors.Errorf("addr %d balance %d is lower than pledge %d", cm.roleID, bal, val)
	}

	ri, err := cm.getRoleInfo(cm.eAddr)
	if err != nil {
		return err
	}

	if ri.isBanned {
		return xerrors.Errorf("%d is banned", cm.roleID)
	}

	// check whether the allowance[addr][pledgePoolAddr] is not less than value, if not, will approve automatically by code.
	client := getClient(cm.endPoint)
	defer client.Close()
	erc20Ins, err := erc20.NewERC20(cm.tAddr, client)
	if err != nil {
		return err
	}

	allo, err := erc20Ins.Allowance(&bind.CallOpts{
		From: cm.eAddr,
	}, cm.eAddr, cm.ppAddr)
	if err != nil {
		return err
	}
	if allo.Cmp(val) < 0 {
		tmp := new(big.Int).Sub(val, allo)
		logger.Debugf("The allowance of %s to %s is not enough,need to add allowance %d", cm.eAddr, cm.ppAddr, tmp)
		// if called by the account itself， then call IncreaseAllowance directly.
		err = cm.increaseAllowance(cm.ppAddr, tmp)
		if err != nil {
			return err
		}
	}

	logger.Debugf("%d begin Pledge %d in PledgePool contract", ri.pri.RoleID, val)

	ppIns, err := pledgepool.NewPledgePool(cm.ppAddr, client)
	if err != nil {
		return err
	}
	auth, err := makeAuth(cm.chainID, cm.hexSK, nil, nil)
	if err != nil {
		return err
	}
	tx, err := ppIns.Pledge(auth, ri.pri.RoleID, val, nil)
	if err != nil {
		return err
	}

	return checkTx(cm.endPoint, tx, "Pledge")
}

func (cm *ContractMgr) canclePledge(roleAddr, rTokenAddr common.Address, rindex uint64, tindex uint32, value *big.Int, sign []byte) error {
	client := getClient(cm.endPoint)
	defer client.Close()

	// check if rindex is banned
	ri, err := cm.getRoleInfo(cm.eAddr)
	if err != nil {
		return err
	}

	if ri.isBanned {
		return xerrors.Errorf("%d is banned", cm.roleID)
	}

	// check if tindex is valid

	logger.Debugf("%d begin Withdraw %d in PledgePool contract", rindex, value)

	ppIns, err := pledgepool.NewPledgePool(cm.ppAddr, client)
	if err != nil {
		return err
	}
	auth, err := makeAuth(cm.chainID, cm.hexSK, nil, nil)
	if err != nil {
		return err
	}
	tx, err := ppIns.Withdraw(auth, rindex, tindex, value, sign)
	if err != nil {
		return err
	}

	return checkTx(cm.endPoint, tx, "Withdraw")
}
