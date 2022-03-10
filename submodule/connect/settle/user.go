package settle

import (
	"encoding/hex"
	"log"
	"math/big"
	"memoc/contracts/role"

	"github.com/ethereum/go-ethereum/common"
	"github.com/memoio/go-mefs-v2/lib/crypto/pdp"
	pdpcommon "github.com/memoio/go-mefs-v2/lib/crypto/pdp/common"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/types"
	"golang.org/x/xerrors"
)

func (cm *ContractMgr) registerUser(rTokenAddr common.Address, index uint64, gindex uint64, blskey []byte, sign []byte) error {
	client := getClient(cm.endPoint)
	defer client.Close()
	roleIns, err := role.NewRole(cm.rAddr, client)
	if err != nil {
		return err
	}

	// check index
	addr, err := cm.getAddrAt(index)
	if err != nil {
		return err
	}

	ri, err := cm.getRoleInfo(addr)
	if err != nil {
		return err
	}
	if ri.pri.Type != pb.RoleInfo_Unknown { // role already registered
		return nil
	}

	// check gindex
	isActive, isBanned, _, _, _, _, _, err := cm.getGroupInfo(gindex)
	if err != nil {
		return err
	}
	if !isActive || isBanned {
		return xerrors.Errorf("group %d is not active or banned", gindex)
	}

	// don't need to check fs

	logger.Debug("begin RegisterUser in Role contract...")

	auth, err := makeAuth(cm.hexSK, nil, nil)
	if err != nil {
		return err
	}
	tx, err := roleIns.RegisterUser(auth, index, gindex, blskey, sign)
	if err != nil {
		return err
	}

	return checkTx(cm.endPoint, tx, "RegisterUser")
}

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

	err = cm.registerUser(cm.rtAddr, cm.roleID, gIndex, pdpKeySet.VerifyKey().Serialize(), nil)
	if err != nil {
		return err
	}
	return nil
}

// Recharge called by user or called by others.
func (cm *ContractMgr) recharge(rTokenAddr common.Address, uIndex uint64, tIndex uint32, money *big.Int, sign []byte) error {
	client := getClient(cm.endPoint)
	defer client.Close()
	roleIns, err := role.NewRole(cm.rAddr, client)
	if err != nil {
		return err
	}

	// uIndex need to be user
	addr, err := cm.getAddrAt(uIndex)
	if err != nil {
		return err
	}
	ri, err := cm.getRoleInfo(addr)
	if err != nil {
		return err
	}
	if ri.isBanned || ri.pri.Type != pb.RoleInfo_User {
		return xerrors.Errorf("invalid user")
	}

	// todo: check tindex

	// check allowance
	allo, err := cm.getAllowance(addr, cm.fsAddr)
	if err != nil {
		return err
	}
	if allo.Cmp(money) < 0 {
		return xerrors.Errorf("allowance is not enough")
	}

	log.Println("begin Recharge in Role contract...")

	auth, errMA := makeAuth(cm.hexSK, nil, nil)
	if errMA != nil {
		return errMA
	}
	tx, err := roleIns.Recharge(auth, uIndex, tIndex, money, sign)
	if err != nil {
		log.Println("Recharge Err:", err)
		return err
	}

	return checkTx(cm.endPoint, tx, "Recharge")
}

func (cm *ContractMgr) Recharge(val *big.Int) error {
	logger.Debug("recharge user: ", cm.roleID, val)

	avail, _, err := cm.getBalanceInFs(cm.roleID, cm.tIndex)
	if err != nil {
		return err
	}

	if avail.Cmp(val) < 0 {
		bal := cm.getBalanceInErc(cm.eAddr)

		logger.Debugf("recharge user approve: %d, has: %d", val, bal)

		if bal.Cmp(val) < 0 {
			val.Set(bal)
		}

		err = cm.approve(cm.fsAddr, val)
		if err != nil {
			return err
		}

		logger.Debug("recharge user charge: ", val)

		err = cm.recharge(cm.rtAddr, cm.roleID, 0, val, nil)
		if err != nil {
			return err
		}

		logger.Debug("recharge user charged: ", val)
	}

	avail, _, err = cm.getBalanceInFs(cm.roleID, cm.tIndex)
	if err != nil {
		return err
	}

	logger.Debug("recharge user has balance: ", avail)

	return nil
}

func (cm *ContractMgr) AddOrder(so *types.SignedOrder) error {
	so.TokenIndex = cm.tIndex

	avil, _, err := cm.getBalanceInFs(so.UserID, so.TokenIndex)
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

	err = cm.addOrder(cm.rAddr, cm.rtAddr, so.UserID, so.ProID, uint64(so.Start), uint64(so.End), so.Size, so.Nonce, so.TokenIndex, so.Price, so.Usign.Data, so.Psign.Data)
	//if err != nil {
	//	return xerrors.Errorf("add order user %d pro %d nonce %d size %d start %d end %d, price %d balance %s fail %w", so.UserID, so.ProID, so.Nonce, so.Size, so.Start, so.End, so.Price, types.FormatWei(avil), err)
	//}

	if err != nil {
		logger.Errorf("add order user %d pro %d nonce %d size %d start %d end %d, price %d balance %s tx fail %w", so.UserID, so.ProID, so.Nonce, so.Size, so.Start, so.End, so.Price, types.FormatWei(avil), err)
	} else {
		logger.Debugf("add order user %d pro %d nonce %d size %d", so.UserID, so.ProID, so.Nonce, so.Size)
	}

	return err
}

func (cm *ContractMgr) SubOrder(so *types.SignedOrder) error {
	err := cm.subOrder(cm.rAddr, cm.rtAddr, so.UserID, so.ProID, uint64(so.Start), uint64(so.End), so.Size, so.Nonce, so.TokenIndex, so.Price, so.Usign.Data, so.Psign.Data)
	//if err != nil {
	//	return xerrors.Errorf("sub order user %d pro %d nonce %d size %d start %d end %d, price %d fail %w", so.UserID, so.ProID, so.Nonce, so.Size, so.Start, so.End, so.Price, err)
	//}

	if err != nil {
		logger.Errorf("sub order user %d pro %d nonce %d size %d start %d end %d, price %d tx fail %w", so.UserID, so.ProID, so.Nonce, so.Size, so.Start, so.End, so.Price, err)
	} else {
		logger.Debugf("sub order user %d pro %d nonce %d size %d", so.UserID, so.ProID, so.Nonce, so.Size)
	}

	return nil
}
