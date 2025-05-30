package v1

import (
	"context"
	"crypto/ecdsa"
	"math/big"
	"math/rand"
	"time"

	callconts "memoc/callcontracts"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"golang.org/x/xerrors"

	logging "github.com/memoio/go-mefs-v2/lib/log"
)

var logger = logging.Logger("settle/v1")

const (
	EndPoint     = "http://119.147.213.220:8191"
	RoleContract = "0x3A014045154403aFF1C07C19553Bc985C123CB6E"

	InvalidAddr = "0x0000000000000000000000000000000000000000"
)

const (
	sendTransactionRetryCount = 5
	checkTxRetryCount         = 8
	retryTxSleepTime          = time.Minute
	retryGetInfoSleepTime     = 30 * time.Second
	checkTxSleepTime          = 6 // 先等待6s（出块时间加1）
	nextBlockTime             = 5 // 出块时间5s

	// DefaultGasLimit default gas limit in sending transaction
	DefaultGasLimit = uint64(5000000) // as small as possible
	DefaultGasPrice = 200
)

func getClient(endPoint string) *ethclient.Client {
	client, err := rpc.Dial(endPoint)
	if err != nil {
		logger.Debug("eth dail fail:", err)
	}
	return ethclient.NewClient(client)
}

func getBalance(endPoint string, addr common.Address) *big.Int {
	client, err := rpc.Dial(endPoint)
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

// makeAuth make the transactOpts to call contract
func makeAuth(chainID *big.Int, hexSk string, moneyToContract, gasPrice *big.Int) (*bind.TransactOpts, error) {
	auth := &bind.TransactOpts{}
	sk, err := crypto.HexToECDSA(hexSk)
	if err != nil {
		return auth, err
	}

	auth, err = bind.NewKeyedTransactorWithChainID(sk, chainID)
	if err != nil {
		return nil, xerrors.Errorf("new keyed transaction failed %s", err)
	}

	auth.Value = moneyToContract //放进合约里的钱
	auth.GasLimit = DefaultGasLimit
	auth.GasPrice = big.NewInt(DefaultGasPrice)
	return auth, nil
}

//CheckTx check whether transaction is successful through receipt
func checkTx(endPoint string, tx *types.Transaction, name string) error {
	logger.Debugf("check transcation:%s %s nonce %d", name, tx.Hash(), tx.Nonce())

	var receipt *types.Receipt
	t := checkTxSleepTime
	for i := 0; i < 10; i++ {
		if i != 0 {
			t = nextBlockTime * i
		}
		logger.Debugf("getting txReceipt, waiting %d sec", t)
		time.Sleep(time.Duration(t) * time.Second)
		receipt = getTransactionReceipt(endPoint, tx.Hash())
		if receipt != nil {
			break
		}
	}

	// 矿工挂掉等情况导致交易无法被打包
	if receipt == nil { //231s获取不到交易信息，判定交易失败
		return xerrors.Errorf("%s %s cann't get tx receipt, tx not packaged", name, tx.Hash())
	}

	logger.Debugf("%s %s GasUsed: %d, CumulativeGasUsed: %d", name, tx.Hash(), receipt.GasUsed, receipt.CumulativeGasUsed)

	if receipt.Status == 0 { //等于0表示交易失败，等于1表示成功
		if receipt.GasUsed != receipt.CumulativeGasUsed {
			return xerrors.Errorf("%s %s transaction exceed gas limit", name, tx.Hash())
		}
		return xerrors.Errorf("%s %s transaction mined but execution failed, please check your tx input", name, tx.Hash())
	}

	// 交易成功
	logger.Debugf("%s %s has been successful!", name, tx.Hash())
	return nil
}

//GetTransactionReceipt 通过交易hash获得交易详情
func getTransactionReceipt(endPoint string, hash common.Hash) *types.Receipt {
	client, err := ethclient.Dial(endPoint)
	if err != nil {
		return nil
	}
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
	defer cancel()
	receipt, err := client.TransactionReceipt(ctx, hash)
	if err != nil {
		logger.Debugf("get transaction %s receipt fail: %s", hash, err)
	}
	return receipt
}

func TransferMemoTo(endPoint, sk string, tAddr, addr common.Address, val *big.Int) error {
	logger.Debugf("Memo %s %s transfer %d to %s", endPoint, tAddr, val, addr)
	txopts := &callconts.TxOpts{
		Nonce:    nil,
		GasPrice: big.NewInt(DefaultGasPrice),
		GasLimit: DefaultGasLimit,
	}

	privateKey, err := crypto.HexToECDSA(sk)
	if err != nil {
		return err
	}

	publicKey := privateKey.Public()
	publicKeyECDSA, ok := publicKey.(*ecdsa.PublicKey)
	if !ok {
		return xerrors.Errorf("error casting public key to ECDSA")
	}

	adminAddress := crypto.PubkeyToAddress(*publicKeyECDSA)

	status := make(chan error)
	erc20 := callconts.NewERC20(tAddr, adminAddress, sk, txopts, endPoint, status)

	adminVal, err := erc20.BalanceOf(adminAddress)
	if err != nil {
		return err
	}

	logger.Debugf("Memo %s has balance %d", adminAddress, adminVal)

	if adminVal.Cmp(val) < 0 {
		return xerrors.Errorf("Memo transfer fail: %d is not enough", adminVal)
	}

	oldVal, err := erc20.BalanceOf(addr)
	if err != nil {
		return err
	}

	retry := 0
	for retry < 10 {
		err = erc20.Transfer(addr, val)
		if err != nil {
			logger.Debug("Memo transfer fail: ", tAddr, addr, val, err)
			retry++
			continue
		}

		if err = <-status; err != nil {
			logger.Fatal("Memo transfer fail: ", err)
		}

		newVal, err := erc20.BalanceOf(addr)
		if err != nil {
			return err
		}

		newVal.Sub(newVal, oldVal)
		if newVal.Cmp(val) >= 0 {
			logger.Debugf("Memo %s received balance %d", addr, newVal)
			return nil
		}

		retry++

		rand.NewSource(time.Now().UnixNano())
		t := rand.Intn(20 * retry)
		time.Sleep(time.Duration(t) * time.Second)
	}

	return xerrors.Errorf("Memo %s transfer %d to %s fail", tAddr, val, addr)
}
