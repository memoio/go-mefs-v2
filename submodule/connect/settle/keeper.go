package settle

import (
	"encoding/hex"
	"log"
	"math/big"
	"time"

	callconts "memoc/callcontracts"
	"memoc/contracts/role"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	bls "github.com/memoio/go-mefs-v2/lib/crypto/bls12_381"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/zeebo/blake3"
	"golang.org/x/xerrors"
)

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

func (cm *ContractMgr) registerKeeper(pledgePoolAddr common.Address, index uint64, blskey []byte, sign []byte) error {
	client := getClient(cm.endPoint)
	defer client.Close()
	roleIns, err := role.NewRole(cm.rAddr, client)
	if err != nil {
		return err
	}

	// check

	pledge, err := cm.getBalanceInPPool(index, 0) // tindex:0 表示主代币
	if err != nil {
		return err
	}
	pledgek, err := cm.getPledgeK()
	if err != nil {
		return err
	}
	if pledge.Cmp(pledgek) < 0 {
		return xerrors.Errorf("%d is not enough, shouldn't less than %d", pledge, pledgek)
	}

	log.Println("begin RegisterKeeper in Role contract...")

	auth, errMA := makeAuth(cm.hexSK, nil, nil)
	if errMA != nil {
		return errMA
	}

	tx, err := roleIns.RegisterKeeper(auth, index, blskey, sign)
	if err != nil {
		return err
	}

	return checkTx(cm.endPoint, tx, "RegisterKeeper")
}

func (cm *ContractMgr) RegisterKeeper() error {
	logger.Debug("register keeper")

	pledgek, err := cm.getPledgeK() // 申请Provider最少需质押的金额
	if err != nil {
		log.Fatal(err)
	}

	ple, err := cm.getBalanceInPPool(cm.roleID, cm.tIndex)
	if err != nil {
		return err
	}

	if ple.Cmp(pledgek) < 0 {
		err = cm.pledge(pledgek)
		if err != nil {
			return err
		}
	}

	logger.Debugf("keeper register %d", pledgek)

	skByte, err := hex.DecodeString(cm.hexSK)
	if err != nil {
		return err
	}
	blsSeed := make([]byte, len(skByte)+1)
	copy(blsSeed[:len(skByte)], skByte)
	blsSeed[len(skByte)] = byte(types.BLS)
	blsByte := blake3.Sum256(blsSeed)
	pk, err := bls.PublicKey(blsByte[:])
	if err != nil {
		return err
	}

	err = cm.registerKeeper(cm.ppAddr, cm.roleID, pk, nil)
	if err != nil {
		return err
	}
	return err
}

func AddKeeperToGroup(endPoint, roleAddr string, roleID, gIndex uint64) error {
	txopts := &callconts.TxOpts{
		Nonce:    nil,
		GasPrice: big.NewInt(callconts.DefaultGasPrice),
		GasLimit: callconts.DefaultGasLimit,
	}

	status := make(chan error)
	ar := callconts.NewR(common.HexToAddress(roleAddr), callconts.AdminAddr, callconts.AdminSk, txopts, endPoint, status)

	err := ar.AddKeeperToGroup(roleID, gIndex)
	if err != nil {
		return err
	}

	if err = <-status; err != nil {
		logger.Fatal("add keeper to group fail: ", roleID, gIndex, err)
		return err
	}

	return nil
}
