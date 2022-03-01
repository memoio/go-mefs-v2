package readpay

import (
	"context"
	"math/big"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/memoio/go-mefs-v2/api"
	"github.com/memoio/go-mefs-v2/lib/address"
	"github.com/memoio/go-mefs-v2/lib/types/store"
	"github.com/memoio/go-mefs-v2/lib/utils"
	"golang.org/x/xerrors"
)

var _ ISender = &SendPay{}

type SendPay struct {
	lw sync.Mutex
	api.IWallet
	localAddr address.Address
	ds        store.KVStore
	pool      map[common.Address]*Paycheck
}

type ISender interface {
	Pay(to address.Address, val *big.Int) ([]byte, error)
}

func NewSender(localAddr address.Address, iw api.IWallet, ds store.KVStore) *SendPay {
	sp := &SendPay{
		IWallet:   iw,
		localAddr: localAddr,
		ds:        ds,
		pool:      make(map[common.Address]*Paycheck),
	}

	return sp
}

func (s *SendPay) Pay(to address.Address, val *big.Int) ([]byte, error) {
	s.lw.Lock()
	defer s.lw.Unlock()

	if val.Sign() <= 0 {
		return nil, xerrors.Errorf("pay value should be larger than zero")
	}

	toAddr := common.BytesToAddress(utils.ToEthAddress(s.localAddr.Bytes()))

	pchk := new(Paycheck)

	p, ok := s.pool[toAddr]
	if ok {
		paidValue := new(big.Int).Set(p.PayValue)
		paidValue.Add(paidValue, val)
		if paidValue.Cmp(p.Value) > 0 {
			// create new one
			chk, err := s.create(toAddr)
			if err != nil {
				return nil, err
			}

			p = &Paycheck{
				Check:    *chk,
				PayValue: big.NewInt(0),
			}
			// replace old one
			s.pool[toAddr] = p
		}
	} else {
		// create new one
		chk, err := s.create(toAddr)
		if err != nil {
			return nil, err
		}

		p = &Paycheck{
			Check:    *chk,
			PayValue: big.NewInt(0),
		}

		s.pool[toAddr] = p
	}

	pbyte, err := p.Serialize()
	if err != nil {
		return nil, err
	}

	err = pchk.Deserialize(pbyte)
	if err != nil {
		return nil, err
	}

	pchk.PayValue.Add(pchk.PayValue, val)
	sig, err := s.WalletSign(context.TODO(), s.localAddr, pchk.Hash())
	if err != nil {
		return nil, err
	}

	pchk.PaySig = sig

	ok, err = pchk.Verify()
	if err != nil || !ok {
		return nil, xerrors.Errorf("invalid pay check %s", err)
	}

	data, err := pchk.Serialize()
	if err != nil {
		return nil, err
	}

	p.PayValue.Add(p.PayValue, val)
	p.PaySig = sig

	// save to local

	return data, nil
}

// create/buy new check
func (s *SendPay) create(toAddr common.Address) (*Check, error) {
	// load from ds first
	ethByte := utils.ToEthAddress(s.localAddr.Bytes())
	return generateCheck(common.BytesToAddress(ethByte), toAddr)
}
