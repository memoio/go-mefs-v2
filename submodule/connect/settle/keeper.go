package settle

import (
	"encoding/hex"
	"log"
	"math/big"
	callconts "memoContract/callcontracts"
	"memoContract/test"

	bls "github.com/memoio/go-mefs-v2/lib/crypto/bls12_381"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/zeebo/blake3"
)

func (cm *ContractMgr) RegisterKeeper() error {
	logger.Debug("register keeper")

	pledgek, err := cm.iRole.PledgeK() // 申请Provider最少需质押的金额
	if err != nil {
		log.Fatal(err)
	}

	ple, err := cm.iPP.GetBalanceInPPool(cm.roleID, cm.tIndex)
	if err != nil {
		return err
	}

	if ple.Cmp(pledgek) < 0 {
		bal, err := cm.iErc.BalanceOf(cm.eAddr)
		if err != nil {
			logger.Debug(err)
			return err
		}

		logger.Debugf("erc20 balance is %d", bal)

		if bal.Cmp(pledgek) < 0 {
			erc20Transfer(cm.eAddr, pledgek)
		}

		logger.Debugf("keeper pledge %d", pledgek)

		err = cm.iPP.Pledge(cm.tAddr, cm.rAddr, cm.roleID, pledgek, nil)
		if err != nil {
			log.Fatal(err)
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

	return cm.iRole.RegisterKeeper(cm.ppAddr, cm.roleID, pk, nil)
}

func (cm *ContractMgr) AddKeeperToGroup(gIndex uint64) error {
	txopts := &callconts.TxOpts{
		Nonce:    nil,
		GasPrice: big.NewInt(callconts.DefaultGasPrice),
		GasLimit: callconts.DefaultGasLimit,
	}
	ar := callconts.NewR(callconts.RoleAddr, callconts.AdminAddr, test.AdminSk, txopts)

	return ar.AddKeeperToGroup(cm.roleID, gIndex)
}
