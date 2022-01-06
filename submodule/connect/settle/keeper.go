package settle

import (
	"encoding/hex"
	"log"
	"math/big"

	callconts "memoContract/callcontracts"

	bls "github.com/memoio/go-mefs-v2/lib/crypto/bls12_381"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/zeebo/blake3"
	"golang.org/x/xerrors"
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
	ar := callconts.NewR(callconts.RoleAddr, callconts.AdminAddr, callconts.AdminSk, txopts, endpoint)

	return ar.AddKeeperToGroup(cm.roleID, gIndex)
}

func (cm *ContractMgr) AddOrder(so *types.SignedOrder, ksigns [][]byte) error {
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

	err = cm.iRFS.AddOrder(cm.rAddr, cm.rtAddr, so.UserID, so.ProID, uint64(so.Start), uint64(so.End), so.Size, so.Nonce, so.TokenIndex, so.Price, so.Usign.Data, so.Psign.Data, ksigns)
	if err != nil {
		return xerrors.Errorf("add order user %d pro %d nonce %d size %d start %d end %d, price %d balance %s fail %w", so.UserID, so.ProID, so.Nonce, so.Size, so.Start, so.End, so.Price, types.FormatWei(avil), err)
	}

	logger.Debugf("add order user %d pro %d nonce %d size %d", so.UserID, so.ProID, so.Nonce, so.Size)

	return nil
}

func (cm *ContractMgr) SubOrder(so *types.SignedOrder, ksigns [][]byte) error {
	so.TokenIndex = cm.tIndex
	err := cm.iRFS.SubOrder(cm.rAddr, cm.rtAddr, so.UserID, so.ProID, uint64(so.Start), uint64(so.End), so.Size, so.Nonce, so.TokenIndex, so.Price, so.Usign.Data, so.Psign.Data, ksigns)
	if err != nil {
		return xerrors.Errorf("sub order user %d pro %d nonce %d size %d start %d end %d, price %d fail %w", so.UserID, so.ProID, so.Nonce, so.Size, so.Start, so.End, so.Price, err)
	}

	logger.Debugf("sub order user %d pro %d nonce %d size %d", so.UserID, so.ProID, so.Nonce, so.Size)

	return nil
}
