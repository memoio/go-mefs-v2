package settle

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"math/big"
	"math/rand"
	"time"

	callconts "memoc/callcontracts"

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
	endpoint = callconts.EndPoint
	RoleAddr = callconts.RoleAddr
)

// as net prefix
func GetRolePrefix() string {
	return RoleAddr.String()[2:10]
}

// TransferTo trans money
func TransferTo(toAddress common.Address, value *big.Int, sk string) error {
	fmt.Println("transfer ", value, " to ", toAddress)
	ctx, cancle := context.WithTimeout(context.TODO(), 10*time.Second)
	defer cancle()
	client, err := ethclient.DialContext(ctx, endpoint)
	if err != nil {
		return xerrors.Errorf("dail %s fail %s", endpoint, err)
	}
	defer client.Close()

	privateKey, err := crypto.HexToECDSA(sk)
	if err != nil {
		return err
	}

	publicKey := privateKey.Public()
	publicKeyECDSA, ok := publicKey.(*ecdsa.PublicKey)
	if !ok {
		return xerrors.Errorf("error casting public key to ECDSA")
	}

	fromAddress := crypto.PubkeyToAddress(*publicKeyECDSA)

	fmt.Println("transfer ", value, " from ", fromAddress, " to ", toAddress)

	gasLimit := uint64(23000)           // in units
	gasPrice := big.NewInt(30000000000) // in wei (30 gwei)

	chainID, err := client.NetworkID(context.Background())
	if err != nil {
		chainID = big.NewInt(666)
	}

	bbal := QueryBalance(toAddress)

	retry := 0
	for {
		retry++
		if retry > 10 {
			fmt.Println("fail transfer ", value.String(), "to", toAddress)
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
			fmt.Println("trans transcation fail:", err)
			continue
		}

		qCount := 0
		for qCount < 5 {
			balance := QueryBalance(toAddress)
			if balance.Cmp(bbal) > 0 {
				fmt.Println("transfer ", value, " to ", toAddress, " has balance: ", balance)
				return nil
			}
			fmt.Println(toAddress, "'s Balance now:", balance.String(), ", waiting for transfer success")

			rand.NewSource(time.Now().UnixNano())
			t := rand.Intn(20 * (qCount + 1))
			time.Sleep(time.Duration(t) * time.Second)
			qCount++
		}
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

func Erc20Transfer(addr common.Address, val *big.Int) error {
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
			retry++
			continue
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
