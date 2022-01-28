package readpay

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/memoio/go-mefs-v2/lib/address"
	"github.com/memoio/go-mefs-v2/lib/types/store"
	"golang.org/x/xerrors"
)

type ReceivePay struct {
	localAddr address.Address
	ds        store.KVStore
	pool      map[common.Address]*Paycheck // key: from addr
}

type IReceiver interface {
	Verify(p *Paycheck, val *big.Int) error
}

func NewReceivePay(localAddr address.Address, ds store.KVStore) *ReceivePay {
	rp := &ReceivePay{
		localAddr: localAddr,
		ds:        ds,
		pool:      make(map[common.Address]*Paycheck),
	}

	return rp
}

func (rp *ReceivePay) Verify(p *Paycheck, val *big.Int) error {
	ok, err := p.Verify()
	if !ok || err != nil {
		return xerrors.Errorf("pay check is invalid: %w", err)
	}

	paidValue := big.NewInt(0)

	pchk, ok := rp.pool[p.FromAddr]
	if ok {
		ok, err := pchk.Check.Equal(&p.Check)
		if err == nil && ok {
			paidValue.Set(pchk.PayValue)
		}
	}

	paidValue.Add(paidValue, val)
	if paidValue.Cmp(p.Value) > 0 {
		return xerrors.Errorf("paycheck value is not enough")
	}

	return nil
}
