package settle

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"math/big"
	"math/rand"
	"time"

	callconts "memoContract/callcontracts"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"golang.org/x/xerrors"

	logging "github.com/memoio/go-mefs-v2/lib/log"
)

var logger = logging.Logger("settle")

var (
	endpoint = "http://119.147.213.220:8193"
	RoleAddr = callconts.RoleAddr
)

// TransferTo trans money
func TransferTo(toAddress common.Address, value *big.Int) error {
	client, err := ethclient.Dial(endpoint)
	if err != nil {
		return err
	}
	defer client.Close()

	privateKey, err := crypto.HexToECDSA(callconts.AdminSk)
	if err != nil {
		return err
	}

	publicKey := privateKey.Public()
	publicKeyECDSA, ok := publicKey.(*ecdsa.PublicKey)
	if !ok {
		return xerrors.Errorf("error casting public key to ECDSA")
	}

	fromAddress := crypto.PubkeyToAddress(*publicKeyECDSA)

	gasLimit := uint64(23000)           // in units
	gasPrice := big.NewInt(30000000000) // in wei (30 gwei)

	chainID, err := client.NetworkID(context.Background())
	if err != nil {
		logger.Debug("client.NetworkID error,use the default chainID")
		chainID = big.NewInt(666)
	}

	retry := 0
	for {
		if retry > 10 {
			logger.Debug("fail transfer ", value.String(), "to", toAddress)
			return errors.New("fail to transfer")
		}

		nonce, err := client.PendingNonceAt(context.Background(), fromAddress)
		if err != nil {
			continue
		}

		gasPrice, err = client.SuggestGasPrice(context.Background())
		if err != nil {
			continue
		}

		tx := types.NewTransaction(nonce, toAddress, value, gasLimit, gasPrice, nil)

		signedTx, err := types.SignTx(tx, types.NewEIP155Signer(chainID), privateKey)
		if err != nil {
			continue
		}

		err = client.SendTransaction(context.Background(), signedTx)
		if err != nil {
			logger.Error("trans transcation fail:", err)
			continue
		}

		qCount := 0
		for qCount < 5 {
			balance := QueryBalance(toAddress)
			if balance.Cmp(value) >= 0 {
				logger.Debug("transfer ", value, " to ", toAddress)
				return nil
			}
			logger.Debug(toAddress, "'s Balance now:", balance.String(), ", waiting for transfer success")

			rand.NewSource(time.Now().UnixNano())
			t := rand.Intn(20 * (qCount + 1))
			time.Sleep(time.Duration(t) * time.Second)
			qCount++
		}

		retry++
	}
}

func QueryBalance(addr common.Address) *big.Int {
	client, err := rpc.Dial(endpoint)
	if err != nil {
		logger.Error("rpc.dial err:", err)
		return big.NewInt(0)
	}
	defer client.Close()

	var result string
	err = client.Call(&result, "eth_getBalance", addr.String(), "latest")
	if err != nil {
		logger.Error("client.call err:", err)
		return big.NewInt(0)
	}

	val, _ := new(big.Int).SetString(result[2:], 16)
	return val
}

func erc20Transfer(addr common.Address, val *big.Int) error {
	txopts := &callconts.TxOpts{
		Nonce:    nil,
		GasPrice: big.NewInt(callconts.DefaultGasPrice),
		GasLimit: callconts.DefaultGasLimit,
	}

	status := make(chan error)
	erc20 := callconts.NewERC20(callconts.ERC20Addr, callconts.AdminAddr, callconts.AdminSk, txopts, endpoint, status)

	adminVal, err := erc20.BalanceOf(callconts.AdminAddr)
	if err != nil {
		return err
	}

	logger.Debug("admin has erc20 balance: ", adminVal)

	if adminVal.Cmp(val) < 0 {
		err = erc20.MintToken(callconts.AdminAddr, val)
		if err != nil {
			logger.Debug("erc20 mintToken fail: ", callconts.ERC20Addr, callconts.AdminAddr, val)
		} else {
			if err = <-status; err != nil {
				logger.Fatal("erc20 mintToken fail: ", err)
			}
		}
	}

	oldVal, err := erc20.BalanceOf(addr)
	if err != nil {
		return err
	}

	retry := 0
	for retry < 10 {
		err = erc20.Transfer(addr, val)
		if err != nil {
			logger.Debug("erc20 transfer fail: ", callconts.ERC20Addr, addr, val, err)
		}

		if err = <-status; err != nil {
			logger.Fatal("erc20 transfer fail: ", err)
		}

		newVal, err := erc20.BalanceOf(addr)
		if err != nil {
			return err
		}

		newVal.Sub(newVal, oldVal)
		if newVal.Cmp(val) >= 0 {
			return nil
		}

		retry++

		rand.NewSource(time.Now().UnixNano())
		t := rand.Intn(20 * retry)
		time.Sleep(time.Duration(t) * time.Second)
	}

	return xerrors.Errorf("fail to transfer erc20 %d to %s", val, addr)
}
