package common

import (
	"context"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/memoio/go-mefs-v2/api"
	"github.com/memoio/go-mefs-v2/lib/address"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"golang.org/x/xerrors"
)

const (
	sendTransactionRetryCount = 5
	checkTxRetryCount         = 8
	retryTxSleepTime          = time.Minute
	retryGetInfoSleepTime     = 30 * time.Second
	checkTxSleepTime          = 6 // 先等待6s（出块时间加1）
	nextBlockTime             = 5 // 出块时间5s

	DefaultGasLimit = uint64(5000000) // as small as possible
	DefaultGasPrice = 200
)

type SettleMgr interface {
	api.ISettle
	Start(pb.RoleInfo_Type, uint64) error
	SettleGetRoleInfo(address.Address) (*api.RoleInfo, error)
}

func MakeAuth(chainID *big.Int, hexSk string) (*bind.TransactOpts, error) {
	auth := &bind.TransactOpts{}
	sk, err := crypto.HexToECDSA(hexSk)
	if err != nil {
		return auth, err
	}

	auth, err = bind.NewKeyedTransactorWithChainID(sk, chainID)
	if err != nil {
		return nil, xerrors.Errorf("new keyed transaction failed %s", err)
	}

	auth.Value = big.NewInt(0)
	auth.GasPrice = big.NewInt(200)
	return auth, nil
}

//GetTransactionReceipt 通过交易hash获得交易详情
func GetTransactionReceipt(endPoint string, hash common.Hash) *types.Receipt {
	client, err := ethclient.Dial(endPoint)
	if err != nil {
		return nil
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
	defer cancel()

	receipt, _ := client.TransactionReceipt(ctx, hash)
	return receipt
}

//CheckTx check whether transaction is successful through receipt
func CheckTx(endPoint string, tx *types.Transaction, name string) error {
	var receipt *types.Receipt
	th := tx.Hash()

	t := checkTxSleepTime
	for i := 0; i < 10; i++ {
		if i != 0 {
			t = nextBlockTime * i
		}
		time.Sleep(time.Duration(t) * time.Second)
		receipt = GetTransactionReceipt(endPoint, th)
		if receipt != nil {
			break
		}
	}

	if receipt == nil {
		return xerrors.Errorf("%s %s cann't get tx receipt, not packaged", name, tx.Hash())
	}

	// 0 means fail
	if receipt.Status == 0 {
		if receipt.GasUsed != receipt.CumulativeGasUsed {
			return xerrors.Errorf("%s %s transaction exceed gas limit", name, tx.Hash())
		}
		return xerrors.Errorf("%s %s transaction mined but execution failed, please check your tx input", name, tx.Hash())
	}
	return nil
}

func SkToAddr(hexSk string) (common.Address, error) {
	privateKey, err := crypto.HexToECDSA(hexSk)
	if err != nil {
		return common.Address{}, err
	}

	eAddr := crypto.PubkeyToAddress(privateKey.PublicKey)
	return eAddr, nil
}
