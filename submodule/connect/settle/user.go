package settle

import (
	"encoding/hex"
	"math/big"

	"github.com/memoio/go-mefs-v2/lib/crypto/pdp"
	pdpcommon "github.com/memoio/go-mefs-v2/lib/crypto/pdp/common"
	"github.com/memoio/go-mefs-v2/lib/types"
)

func (cm *ContractMgr) RegisterUser(gIndex uint64) error {
	logger.Debug("resgister user")

	skByte, err := hex.DecodeString(cm.hexSK)
	if err != nil {
		return err
	}
	blsSeed := make([]byte, len(skByte)+1)
	copy(blsSeed[:len(skByte)], skByte)
	blsSeed[len(skByte)] = byte(types.PDP)

	pdpKeySet, err := pdp.GenerateKeyWithSeed(pdpcommon.PDPV2, blsSeed)
	if err != nil {
		return err
	}

	return cm.iRole.RegisterUser(cm.rtAddr, cm.roleID, gIndex, cm.tIndex, pdpKeySet.VerifyKey().Serialize(), nil)
}

func (cm *ContractMgr) Recharge(val *big.Int) error {
	logger.Debug("recharge user")

	avail, _, err := cm.iFS.GetBalance(cm.roleID, cm.tIndex)
	if err != nil {
		return err
	}

	if avail.Cmp(val) < 0 {
		bal, err := cm.iErc.BalanceOf(cm.eAddr)
		if err != nil {
			logger.Debug(err)
			return err
		}

		logger.Debugf("erc20 balance is %d", bal)

		if bal.Cmp(val) < 0 {
			erc20Transfer(cm.eAddr, val)
		}

		logger.Debug("recharge user approve: ", val)

		err = cm.iErc.Approve(cm.fsAddr, val)
		if err != nil {
			return err
		}

		logger.Debug("recharge user charge: ", val)

		err = cm.iRole.Recharge(cm.rtAddr, cm.roleID, 0, val, nil)
		if err != nil {
			return err
		}

		logger.Debug("recharge user charged: ", val)
	}

	return nil
}
