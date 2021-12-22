package settle

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"log"
	"math/big"
	"time"

	callconts "memoContract/callcontracts"
	"memoContract/test"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"

	logging "github.com/memoio/go-mefs-v2/lib/log"
)

var logger = logging.Logger("settle")

const RetryGetInfoSleepTime = 5 * time.Second

// TransferTo trans money
func TransferTo(toAddress common.Address, value *big.Int) error {
	eth := callconts.EndPoint
	client, err := ethclient.Dial(eth)
	if err != nil {
		fmt.Println("rpc.Dial err", err)
		log.Fatal(err)
	}

	privateKey, err := crypto.HexToECDSA(callconts.AdminSk)
	if err != nil {
		log.Fatal(err)
	}

	publicKey := privateKey.Public()
	publicKeyECDSA, ok := publicKey.(*ecdsa.PublicKey)
	if !ok {
		log.Fatal("error casting public key to ECDSA")
	}

	fromAddress := crypto.PubkeyToAddress(*publicKeyECDSA)

	gasLimit := uint64(23000)           // in units
	gasPrice := big.NewInt(30000000000) // in wei (30 gwei)

	chainID, err := client.NetworkID(context.Background())
	if err != nil {
		fmt.Println("client.NetworkID error,use the default chainID")
		chainID = big.NewInt(666)
	}

	retry := 0
	for {
		if retry > 10 {
			return errors.New("fail to transfer")
		}
		retry++
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
			log.Println("trans transcation fail:", err)
			continue
		}

		qCount := 0
		for qCount < 10 {
			balance := QueryBalance(toAddress)
			if balance.Cmp(value) >= 0 {
				break
			}
			fmt.Println(toAddress, "'s Balance now:", balance.String(), ", waiting for transfer success")
			t := 20 * (qCount + 1)
			time.Sleep(time.Duration(t) * time.Second)
		}

		if qCount < 10 {
			break
		}
	}

	fmt.Println("transfer ", value.String(), "to", toAddress)
	return nil
}

func QueryBalance(addr common.Address) *big.Int {
	ethEndPoint := callconts.EndPoint

	var result string
	client, err := rpc.Dial(ethEndPoint)
	if err != nil {
		log.Fatal("rpc.dial err:", err)
	}
	err = client.Call(&result, "eth_getBalance", addr.String(), "latest")
	if err != nil {
		log.Fatal("client.call err:", err)
	}

	val, _ := new(big.Int).SetString(result[2:], 16)
	return val
}

func erc20Transfer(addr common.Address, val *big.Int) {
	txopts := &callconts.TxOpts{
		Nonce:    nil,
		GasPrice: big.NewInt(callconts.DefaultGasPrice),
		GasLimit: callconts.DefaultGasLimit,
	}
	erc20 := callconts.NewERC20(callconts.ERC20Addr, callconts.AdminAddr, test.AdminSk, txopts)
	erc20.Transfer(addr, val)
}
