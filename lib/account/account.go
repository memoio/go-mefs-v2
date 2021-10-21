package account

import (
	"sync"

	"github.com/memoio/go-mefs-v2/lib/address"
	"github.com/memoio/go-mefs-v2/lib/crypto/signature"
	"github.com/memoio/go-mefs-v2/lib/crypto/signature/bls"
	sig_common "github.com/memoio/go-mefs-v2/lib/crypto/signature/common"
	"github.com/memoio/go-mefs-v2/lib/crypto/signature/secp256k1"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/types/wallet"
)

type LocalWallet struct {
	sync.Mutex
	password string // used for decrypt
	accounts map[address.Address]sig_common.PrivKey
	keystore types.KeyStore // store
}

func NewWallet(pw string, ks types.KeyStore) wallet.Wallet {
	lw := &LocalWallet{
		password: pw,
		keystore: ks,
		accounts: make(map[address.Address]sig_common.PrivKey),
	}

	return lw
}

func (w *LocalWallet) WalletSign(addr address.Address, msg []byte) ([]byte, error) {
	pi, err := w.find(addr)
	if err != nil {
		return nil, err
	}

	return pi.Sign(msg)
}

func (w *LocalWallet) find(addr address.Address) (sig_common.PrivKey, error) {
	w.Lock()
	defer w.Unlock()

	pi, ok := w.accounts[addr]
	if ok {
		return pi, nil
	}

	ki, err := w.keystore.Get(addr.String(), w.password)
	if err != nil {
		return nil, err
	}

	pi, err = signature.ParsePrivateKey(ki.SecretKey, ki.Type)
	if err != nil {
		return nil, err
	}

	w.accounts[addr] = pi

	return pi, nil
}

func (w *LocalWallet) WalletNew(kt types.KeyType) (address.Address, error) {
	var privkey sig_common.PrivKey
	var err error
	switch kt {
	case types.Secp256k1:
		privkey, err = secp256k1.GenerateKey()
		if err != nil {
			return address.Undef, err
		}

	case types.BLS:
		privkey, err = bls.GenerateKey()
		if err != nil {
			return address.Undef, err
		}

	default:
		return address.Undef, types.ErrKeyFromat
	}

	pubKey := privkey.GetPublic()
	priByte, err := privkey.Raw()
	if err != nil {
		return address.Undef, err
	}

	cbyte, err := pubKey.CompressedByte()
	if err != nil {
		return address.Undef, err
	}

	addr, err := address.NewAddress(cbyte)
	if err != nil {
		return address.Undef, err
	}

	ki := types.KeyInfo{
		Type:      kt,
		SecretKey: priByte,
	}

	err = w.keystore.Put(addr.String(), w.password, ki)
	if err != nil {
		return address.Undef, err
	}

	w.Lock()
	defer w.Unlock()

	w.accounts[addr] = privkey

	return addr, nil
}

func (w *LocalWallet) WalletList() ([]address.Address, error) {
	as, err := w.keystore.List()
	if err != nil {
		return nil, err
	}

	res := make([]address.Address, 0, len(as))

	for _, s := range as {
		addr, err := address.NewAddressFromString(s)
		if err != nil {
			continue
		}
		res = append(res, addr)
	}

	return res, nil
}

func (w *LocalWallet) WalletHas(addr address.Address) bool {
	_, err := w.keystore.Get(addr.String(), w.password)
	if err != nil {
		return false
	}
	return true
}

func (w *LocalWallet) WalletDelete(addr address.Address) error {
	err := w.keystore.Delete(addr.String(), w.password)
	if err != nil {
		return err
	}

	w.Lock()
	defer w.Unlock()
	delete(w.accounts, addr)

	return nil
}

func (w *LocalWallet) WalletExport(addr address.Address) (*types.KeyInfo, error) {
	return nil, nil
}

func (w *LocalWallet) WalletImport(ki *types.KeyInfo) (address.Address, error) {
	return address.Address{}, nil
}
