package settle

import (
	"math/big"

	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/lib/types"
	inter "github.com/memoio/go-mefs-v2/submodule/connect/settle/interface"
)

func (cm *ContractMgr) Recharge(roleID uint64, ti uint8, isLock bool, val *big.Int) error {
	// check erc20
	bal := cm.ercIns.BalanceOf(cm.eAddr)
	if val.Cmp(bal) > 0 {
		return xerrors.Errorf("balance not enough, need %d, has %d", val, bal)
	}

	fpool, err := cm.getIns.GetFsPool()
	if err != nil {
		return err
	}

	// check allowance
	al := cm.ercIns.Allowance(cm.eAddr, fpool)
	if val.Cmp(al) > 0 {
		logger.Debugf("Approve %d in pool %s", val, fpool)
		err := cm.ercIns.Approve(fpool, val)
		if err != nil {
			return err
		}
	}

	logger.Debugf("Recharge %d", val)
	err = cm.proxyIns.Recharge(roleID, ti, isLock, val)
	if err != nil {
		return err
	}

	return nil
}

func (cm *ContractMgr) AddOrder(so *types.SignedOrder) error {
	logger.Debug("begin AddOrder...")
	// check start,end,size
	if so.Size == 0 {
		return xerrors.Errorf("size is zero")
	}
	if so.End <= so.Start {
		return xerrors.Errorf("start %d should before end %d", so.Start, so.End)
	}
	if so.End%types.Day != 0 {
		return xerrors.Errorf("end %d should be aligned to 86400(one day)", so.End)
	}
	// todo: check uIndex,pIndex,gIndex,tIndex

	// check balance
	avail, lock, _, err := cm.getIns.GetBalAt(so.UserID, uint8(so.TokenIndex))
	if err != nil {
		return err
	}
	avail.Add(avail, lock)
	pay := new(big.Int).Set(so.Price)
	pay.Mul(pay, big.NewInt(so.End-so.Start))
	pay.Mul(pay, big.NewInt(12))
	pay.Div(pay, big.NewInt(10))

	if pay.Cmp(avail) > 0 {
		return xerrors.Errorf("add order insufficiecnt funds user %d has balance %s, require %s", so.UserID, types.FormatMemo(avail), types.FormatMemo(pay))
	}

	poi := inter.OrderIn{
		UIndex: so.UserID,
		PIndex: so.ProID,
		Start:  uint64(so.Start),
		End:    uint64(so.End),
		Size:   so.Size,
		Nonce:  so.Nonce,
		TIndex: uint8(so.TokenIndex),
		Sprice: so.Price,
	}

	err = cm.proxyIns.AddOrder(poi, so.Usign.Data, so.Psign.Data)
	if err != nil {
		return err
	}
	return nil
}

func (cm *ContractMgr) SubOrder(so *types.SignedOrder) error {
	logger.Debug("begin SubOrder...")
	// check start,end,size
	if so.Size == 0 {
		return xerrors.Errorf("size is zero")
	}
	if so.End <= so.Start {
		return xerrors.Errorf("start %d should before end %d", so.Start, so.End)
	}
	if so.End%types.Day != 0 {
		return xerrors.Errorf("end %d should be aligned to 86400(one day)", so.End)
	}
	// todo: check uIndex,pIndex,gIndex,tIndex

	poi := inter.OrderIn{
		UIndex: so.UserID,
		PIndex: so.ProID,
		Start:  uint64(so.Start),
		End:    uint64(so.End),
		Size:   so.Size,
		Nonce:  so.Nonce,
		TIndex: uint8(so.TokenIndex),
		Sprice: so.Price,
	}

	go cm.proxyIns.SubOrder(poi, so.Usign.Data, so.Psign.Data)

	return nil
}
