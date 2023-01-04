package impl

import (
	"context"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"golang.org/x/xerrors"

	"github.com/memoio/contractsv2/go_contracts/erc"
	scom "github.com/memoio/go-mefs-v2/submodule/connect/settle/common"
	inter "github.com/memoio/go-mefs-v2/submodule/connect/settle/interface"
)

type ercImpl struct {
	endPoint string
	chainID  *big.Int

	sk string

	eAddr common.Address
	erc20 common.Address
}

func NewErc20(endPoint, hexSk string, erc20 common.Address) (inter.IERC20, error) {
	client, err := ethclient.DialContext(context.TODO(), endPoint)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	chainID, err := client.NetworkID(context.Background())
	if err != nil {
		chainID = big.NewInt(666)
	}

	eAddr, err := scom.SkToAddr(hexSk)
	if err != nil {
		return nil, err
	}

	// check erc20 is contract
	erc20Ins, err := erc.NewERC20(erc20, client)
	if err != nil {
		return nil, err
	}

	_, err = erc20Ins.Name(&bind.CallOpts{
		From: eAddr,
	})
	if err != nil {
		return nil, err
	}

	e := &ercImpl{
		endPoint: endPoint,
		chainID:  chainID,
		sk:       hexSk,
		eAddr:    eAddr,
		erc20:    erc20,
	}

	return e, nil
}

func (e *ercImpl) Transfer(recipient common.Address, amount *big.Int) error {
	val := e.BalanceOf(e.eAddr)
	if val.Cmp(amount) < 0 {
		return xerrors.Errorf("%s balance not enough, need %d, has %d", e.eAddr, amount, val)
	}

	client, err := ethclient.DialContext(context.TODO(), e.endPoint)
	if err != nil {
		return err
	}
	defer client.Close()

	erc20Ins, err := erc.NewERC20(e.erc20, client)
	if err != nil {
		return err
	}

	auth, err := scom.MakeAuth(e.chainID, e.sk)
	if err != nil {
		return err
	}

	tx, err := erc20Ins.Transfer(auth, recipient, amount)
	if err != nil {
		return err
	}

	return scom.CheckTx(e.endPoint, tx, "transfer")
}

func (e *ercImpl) Approve(spender common.Address, amount *big.Int) error {
	client, err := ethclient.DialContext(context.TODO(), e.endPoint)
	if err != nil {
		return err
	}
	defer client.Close()

	erc20Ins, err := erc.NewERC20(e.erc20, client)
	if err != nil {
		return err
	}

	auth, err := scom.MakeAuth(e.chainID, e.sk)
	if err != nil {
		return err
	}

	tx, err := erc20Ins.Approve(auth, spender, amount)
	if err != nil {
		return err
	}

	return scom.CheckTx(e.endPoint, tx, "approve")
}

func (e *ercImpl) BalanceOf(account common.Address) *big.Int {
	res := new(big.Int)
	client, err := ethclient.DialContext(context.TODO(), e.endPoint)
	if err != nil {
		return res
	}
	defer client.Close()

	erc20Ins, err := erc.NewERC20(e.erc20, client)
	if err != nil {
		return res
	}

	retryCount := 0
	for {
		retryCount++
		bal, err := erc20Ins.BalanceOf(&bind.CallOpts{
			From: e.eAddr,
		}, account)
		if err != nil {
			if retryCount > 3 {
				return res
			}
			time.Sleep(5 * time.Second)
			continue
		}

		return res.Set(bal)
	}
}

func (e *ercImpl) Allowance(owner common.Address, spender common.Address) *big.Int {
	res := new(big.Int)
	client, err := ethclient.DialContext(context.TODO(), e.endPoint)
	if err != nil {
		return res
	}
	defer client.Close()

	erc20Ins, err := erc.NewERC20(e.erc20, client)
	if err != nil {
		return res
	}

	retryCount := 0
	for {
		retryCount++
		bal, err := erc20Ins.Allowance(&bind.CallOpts{
			From: e.eAddr,
		}, owner, spender)
		if err != nil {
			if retryCount > 3 {
				return res
			}
			time.Sleep(5 * time.Second)
			continue
		}

		return res.Set(bal)
	}
}
