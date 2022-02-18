package settle

import (
	"encoding/hex"
	"math/big"

	"github.com/memoio/go-mefs-v2/lib/crypto/pdp"
	pdpcommon "github.com/memoio/go-mefs-v2/lib/crypto/pdp/common"
	"github.com/memoio/go-mefs-v2/lib/types"
	"golang.org/x/xerrors"
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

	err = cm.iRole.RegisterUser(cm.rtAddr, cm.roleID, gIndex, pdpKeySet.VerifyKey().Serialize(), nil)
	if err != nil {
		return err
	}

	if err = <-cm.status; err != nil {
		logger.Fatal("register user fail: ", cm.roleID, gIndex, err)
		return err
	}
	return nil
}

func (cm *ContractMgr) Recharge(val *big.Int) error {
	logger.Debug("recharge user: ", cm.roleID, val)

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

		logger.Debugf("recharge user approve: %d, has: %d", val, bal)

		err = cm.iErc.Approve(cm.fsAddr, val)
		if err != nil {
			return err
		}

		if err = <-cm.status; err != nil {
			logger.Fatal("approve fail: ", cm.roleID, err)
			return err
		}
		logger.Debug("recharge user charge: ", val)

		err = cm.iRole.Recharge(cm.rtAddr, cm.roleID, 0, val, nil)
		if err != nil {
			return err
		}

		if err = <-cm.status; err != nil {
			logger.Fatal("recharge fail: ", cm.roleID, err)
			return err
		}

		logger.Debug("recharge user charged: ", val)
	}

	avail, _, err = cm.iFS.GetBalance(cm.roleID, cm.tIndex)
	if err != nil {
		return err
	}

	logger.Debug("recharge user has balance: ", avail)

	return nil
}

func (cm *ContractMgr) AddOrder(so *types.SignedOrder) error {
	so.TokenIndex = cm.tIndex

	avil, _, err := cm.iFS.GetBalance(so.UserID, so.TokenIndex)
	if err != nil {
		return err
	}

	pay := new(big.Int).Set(so.Price)
	pay.Mul(pay, big.NewInt(so.End-so.Start))
	pay.Mul(pay, big.NewInt(12))
	pay.Div(pay, big.NewInt(10))

	if pay.Cmp(avil) > 0 {
		return xerrors.Errorf("add order insufficiecnt funds user %d has balance %s, require %s", so.UserID, types.FormatWei(avil), types.FormatWei(pay))
	}

	err = cm.iRFS.AddOrder(cm.rAddr, cm.rtAddr, so.UserID, so.ProID, uint64(so.Start), uint64(so.End), so.Size, so.Nonce, so.TokenIndex, so.Price, so.Usign.Data, so.Psign.Data)
	if err != nil {
		return xerrors.Errorf("add order user %d pro %d nonce %d size %d start %d end %d, price %d balance %s fail %w", so.UserID, so.ProID, so.Nonce, so.Size, so.Start, so.End, so.Price, types.FormatWei(avil), err)
	}

	go func() {
		err := <-cm.status
		if err != nil {
			logger.Errorf("add order user %d pro %d nonce %d size %d start %d end %d, price %d balance %s tx fail %w", so.UserID, so.ProID, so.Nonce, so.Size, so.Start, so.End, so.Price, types.FormatWei(avil), err)
		} else {
			logger.Debugf("add order user %d pro %d nonce %d size %d", so.UserID, so.ProID, so.Nonce, so.Size)
		}
	}()

	return nil
}

func (cm *ContractMgr) SubOrder(so *types.SignedOrder) error {
	so.TokenIndex = cm.tIndex
	err := cm.iRFS.SubOrder(cm.rAddr, cm.rtAddr, so.UserID, so.ProID, uint64(so.Start), uint64(so.End), so.Size, so.Nonce, so.TokenIndex, so.Price, so.Usign.Data, so.Psign.Data)
	if err != nil {
		return xerrors.Errorf("sub order user %d pro %d nonce %d size %d start %d end %d, price %d fail %w", so.UserID, so.ProID, so.Nonce, so.Size, so.Start, so.End, so.Price, err)
	}

	go func() {
		err := <-cm.status
		if err != nil {
			logger.Errorf("sub order user %d pro %d nonce %d size %d start %d end %d, price %d tx fail %w", so.UserID, so.ProID, so.Nonce, so.Size, so.Start, so.End, so.Price, err)
		} else {
			logger.Debugf("sub order user %d pro %d nonce %d size %d", so.UserID, so.ProID, so.Nonce, so.Size)
		}
	}()

	return nil
}
