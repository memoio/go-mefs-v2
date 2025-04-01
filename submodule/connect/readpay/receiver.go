package readpay

import (
	"bytes"
	"math/big"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/memoio/go-mefs-v2/lib/address"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/types/store"
	"github.com/memoio/go-mefs-v2/lib/utils"
	"golang.org/x/xerrors"
)

type ReceivePay struct {
	sync.RWMutex
	localAddr address.Address

	toAddr common.Address
	ds     store.KVStore
	pool   map[string]*Paycheck // key: from addr
}

type IReceiver interface {
	Verify(p *Paycheck, val *big.Int) error
}

func NewReceivePay(localAddr address.Address, ds store.KVStore) *ReceivePay {
	toAddr := common.BytesToAddress(utils.ToEthAddress(localAddr.Bytes()))

	rp := &ReceivePay{
		localAddr: localAddr,
		toAddr:    toAddr,
		ds:        ds,
		pool:      make(map[string]*Paycheck),
	}

	return rp
}

func (rp *ReceivePay) Verify(p *Paycheck, val *big.Int) error {
	if !bytes.Equal(p.ToAddr.Bytes(), rp.toAddr.Bytes()) {
		return xerrors.Errorf("pay check is not mine")
	}

	ok, err := p.Verify()
	if err != nil || !ok {
		return xerrors.Errorf("pay check is invalid: %s", err)
	}

	pStr := p.String()

	key := store.NewKey(pb.MetaType_ReadPay_ChannelKey, p.ContractAddr.String(), p.FromAddr.String(), p.ToAddr.String(), p.Nonce)

	rp.Lock()
	defer rp.Unlock()

	lchk, ok := rp.pool[pStr]
	if !ok {
		data, err := rp.ds.Get(key)
		if err == nil {
			pchk := new(Paycheck)
			err = pchk.Deserialize(data)
			if err == nil {
				lchk = pchk
				rp.pool[pStr] = lchk
			}
		}
	}

	paidValue := big.NewInt(0)
	if lchk != nil {
		ok, err := lchk.Check.Equal(&p.Check)
		if err != nil || !ok {
			return xerrors.Errorf("may duplicate check")
		}
		paidValue.Set(lchk.PayValue)
	}

	paidValue.Add(paidValue, val)
	if paidValue.Cmp(p.Value) > 0 {
		return xerrors.Errorf("paycheck value is not enough")
	}

	rp.pool[pStr] = p

	data, err := p.Serialize()
	if err != nil {
		return err
	}

	return rp.ds.Put(key, data)
}
