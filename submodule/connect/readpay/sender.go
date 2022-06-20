package readpay

import (
	"context"
	"encoding/binary"
	"math/big"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/memoio/go-mefs-v2/api"
	"github.com/memoio/go-mefs-v2/lib/address"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/types/store"
	"github.com/memoio/go-mefs-v2/lib/utils"
	"golang.org/x/xerrors"
)

var _ ISender = &SendPay{}

type SendPay struct {
	lw sync.Mutex
	api.IWallet
	localAddr address.Address

	contractAddr common.Address
	fromAddr     common.Address

	ds   store.KVStore
	pool map[common.Address]*Paycheck
}

type ISender interface {
	Pay(to address.Address, val *big.Int) ([]byte, error)
}

func NewSender(localAddr address.Address, iw api.IWallet, ds store.KVStore) *SendPay {
	fromAddr := common.BytesToAddress(utils.ToEthAddress(localAddr.Bytes()))

	sp := &SendPay{
		IWallet:      iw,
		localAddr:    localAddr,
		contractAddr: contractAddr,
		fromAddr:     fromAddr,
		ds:           ds,
		pool:         make(map[common.Address]*Paycheck),
	}

	return sp
}

func (s *SendPay) Pay(to address.Address, val *big.Int) ([]byte, error) {
	if val.Sign() <= 0 {
		return nil, xerrors.Errorf("pay value should be larger than zero")
	}

	toAddr := common.BytesToAddress(utils.ToEthAddress(to.Bytes()))

	s.lw.Lock()
	defer s.lw.Unlock()

	p, ok := s.pool[toAddr]
	if !ok {
		nonce := uint64(0)
		key := store.NewKey(pb.MetaType_ReadPay_NonceKey, s.contractAddr.String(), s.fromAddr.String(), toAddr.String())
		data, err := s.ds.Get(key)
		if err == nil && len(data) >= 8 {
			nonce = binary.BigEndian.Uint64(data)
			key := store.NewKey(pb.MetaType_ReadPay_ChannelKey, s.contractAddr.String(), s.fromAddr.String(), toAddr.String(), nonce)
			data, err := s.ds.Get(key)
			if err != nil {
				return nil, err
			}

			pchk := new(Paycheck)
			err = pchk.Deserialize(data)
			if err != nil {
				return nil, err
			}
			p = pchk
			s.pool[toAddr] = p
		} else {
			pchk, err := s.create(toAddr, nonce)
			if err != nil {
				return nil, err
			}
			p = pchk
			s.pool[toAddr] = p
		}
	}

	paidValue := new(big.Int).Set(p.PayValue)
	paidValue.Add(paidValue, val)
	if paidValue.Cmp(p.Value) > 0 {
		// create new one
		nonce := p.Nonce + 1
		pchk, err := s.create(toAddr, nonce)
		if err != nil {
			return nil, err
		}

		p = pchk

		// replace old one
		s.pool[toAddr] = p

		// reset to val
		paidValue.Set(val)
	}

	// confirm again
	if paidValue.Cmp(p.Value) > 0 {
		return nil, xerrors.Errorf("money is not enough")
	}

	p.PayValue.Add(p.PayValue, val)
	sig, err := s.WalletSign(context.TODO(), s.localAddr, p.Hash())
	if err != nil {
		return nil, err
	}

	p.PaySig = sig

	ok, err = p.Verify()
	if err != nil || !ok {
		return nil, xerrors.Errorf("invalid pay check %s", err)
	}

	data, err := p.Serialize()
	if err != nil {
		return nil, err
	}

	// save to local
	key := store.NewKey(pb.MetaType_ReadPay_ChannelKey, p.ContractAddr.String(), p.FromAddr.String(), p.ToAddr.String(), p.Nonce)
	s.ds.Put(key, data)

	return data, nil
}

// create/buy new check
func (s *SendPay) create(toAddr common.Address, nonce uint64) (*Paycheck, error) {
	chk, err := generateCheck(s.fromAddr, toAddr, nonce)
	if err != nil {
		return nil, err
	}

	p := &Paycheck{
		Check:    *chk,
		PayValue: big.NewInt(0),
	}

	return p, p.Save(s.ds)
}
