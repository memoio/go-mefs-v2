package settle

import (
	"context"
	"math/big"
	"math/rand"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"golang.org/x/xerrors"

	logging "github.com/memoio/go-mefs-v2/lib/log"
	scom "github.com/memoio/go-mefs-v2/submodule/connect/settle/common"
	"github.com/memoio/go-mefs-v2/submodule/connect/settle/impl"
)

var logger = logging.Logger("settle")

const (
	EndPoint     = "http://119.147.213.220:8191"
	RoleContract = "0x3A014045154403aFF1C07C19553Bc985C123CB6E"

	InvalidAddr = "0x0000000000000000000000000000000000000000"
)

func GetTxBalance(endPoint string, addr common.Address) *big.Int {
	client, err := rpc.Dial(endPoint)
	if err != nil {
		return big.NewInt(0)
	}
	defer client.Close()

	var result string
	err = client.Call(&result, "eth_getBalance", addr.String(), "latest")
	if err != nil {
		return big.NewInt(0)
	}

	val, _ := new(big.Int).SetString(result[2:], 16)
	return val
}

// TransferTo trans money
func TransferTo(endPoint string, toAddress common.Address, value *big.Int, sk string) error {
	client, err := ethclient.DialContext(context.TODO(), endPoint)
	if err != nil {
		return xerrors.Errorf("dail %s fail %s", endPoint, err)
	}
	defer client.Close()

	privateKey, err := crypto.HexToECDSA(sk)
	if err != nil {
		return err
	}

	fromAddress, err := scom.SkToAddr(sk)
	if err != nil {
		return err
	}

	logger.Debugf("%s transfer %d from %s to %s", endPoint, value, fromAddress, toAddress)

	gasLimit := uint64(23000) // in units

	chainID, err := client.NetworkID(context.Background())
	if err != nil {
		chainID = big.NewInt(666)
	}

	bbal := GetTxBalance(endPoint, toAddress)

	retry := 0
	for {
		retry++
		if retry > 10 {
			return xerrors.New("fail to transfer")
		}

		nonce, err := client.PendingNonceAt(context.Background(), fromAddress)
		if err != nil {
			continue
		}

		gasPrice, err := client.SuggestGasPrice(context.Background())
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
			continue
		}

		qCount := 0
		for qCount < 5 {
			balance := GetTxBalance(endPoint, toAddress)
			if balance.Cmp(bbal) > 0 {
				logger.Infof("transfer txfee ok, %s has %d now", toAddress, balance)
				return nil
			}

			rand.NewSource(time.Now().UnixNano())
			t := rand.Intn(20 * (qCount + 1))
			time.Sleep(time.Duration(t) * time.Second)
			qCount++
		}
	}
}

func TransferMemoTo(endPoint, sk string, tAddr, addr common.Address, val *big.Int) error {
	logger.Debugf("Memo %s %s transfer %d to %s", endPoint, tAddr, val, addr)

	ercIns, err := impl.NewErc20(endPoint, sk, tAddr)
	if err != nil {
		return err
	}

	oldVal := ercIns.BalanceOf(addr)

	retry := 0
	for retry < 10 {
		err = ercIns.Transfer(addr, val)
		if err != nil {
			retry++
			continue
		}

		newVal := ercIns.BalanceOf(addr)
		deltaVal := big.NewInt(0)
		deltaVal.Sub(newVal, oldVal)
		if deltaVal.Cmp(big.NewInt(0)) > 0 {
			logger.Infof("transfer memo %d ok, %s has memo %d now", deltaVal, addr, newVal)
			return nil
		}

		retry++

		rand.NewSource(time.Now().UnixNano())
		t := rand.Intn(20 * retry)
		time.Sleep(time.Duration(t) * time.Second)
	}

	return xerrors.Errorf("Memo %s transfer %d to %s fail", tAddr, val, addr)
}
