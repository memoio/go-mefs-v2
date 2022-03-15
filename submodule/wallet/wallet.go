package wallet

import (
	"context"
	"sort"
	"strings"
	"sync"

	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/lib/address"
	"github.com/memoio/go-mefs-v2/lib/crypto/signature"
	sig_common "github.com/memoio/go-mefs-v2/lib/crypto/signature/common"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/memoio/go-mefs-v2/lib/utils"
)

type LocalWallet struct {
	lw       sync.Mutex
	password string // used for decrypt; todo plaintext is not good
	accounts map[address.Address]sig_common.PrivKey
	keystore types.KeyStore // store
}

func New(pw string, ks types.KeyStore) *LocalWallet {
	lw := &LocalWallet{
		password: pw,
		keystore: ks,
		accounts: make(map[address.Address]sig_common.PrivKey),
	}

	return lw
}

func (w *LocalWallet) API() *walletAPI {
	return &walletAPI{w}
}

func (w *LocalWallet) WalletSign(ctx context.Context, addr address.Address, msg []byte) ([]byte, error) {
	pi, err := w.find(addr)
	if err != nil {
		return nil, err
	}

	return pi.Sign(msg)
}

func (w *LocalWallet) find(addr address.Address) (sig_common.PrivKey, error) {
	w.lw.Lock()
	defer w.lw.Unlock()

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

func (w *LocalWallet) WalletNew(ctx context.Context, kt types.KeyType) (address.Address, error) {
	privkey, err := signature.GenerateKey(kt)
	if err != nil {
		return address.Undef, err
	}

	pubKey := privkey.GetPublic()
	priByte, err := privkey.Raw()
	if err != nil {
		return address.Undef, err
	}

	cbyte, err := pubKey.Raw()
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

	// for eth short addr
	if kt == types.Secp256k1 {
		addrByte := utils.ToEthAddress(cbyte)

		eaddr, err := address.NewAddress(addrByte)
		if err != nil {
			return address.Undef, err
		}
		err = w.keystore.Put(eaddr.String(), w.password, ki)
		if err != nil {
			return address.Undef, err
		}
	}

	w.lw.Lock()
	w.accounts[addr] = privkey
	w.lw.Unlock()

	return addr, nil
}

func (w *LocalWallet) WalletList(ctx context.Context) ([]address.Address, error) {
	as, err := w.keystore.List()
	if err != nil {
		return nil, err
	}

	out := make([]address.Address, 0, len(as))

	for _, s := range as {
		if strings.HasPrefix(s, address.AddrPrefix) {
			addr, err := address.NewFromString(s)
			if err != nil {
				continue
			}

			out = append(out, addr)
		}
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].String() < out[j].String()
	})

	return out, nil
}

func (w *LocalWallet) WalletHas(ctx context.Context, addr address.Address) (bool, error) {
	_, err := w.keystore.Get(addr.String(), w.password)
	if err != nil {
		return false, err
	}
	return true, nil
}

func (w *LocalWallet) WalletDelete(ctx context.Context, addr address.Address) error {
	err := w.keystore.Delete(addr.String(), w.password)
	if err != nil {
		return err
	}

	w.lw.Lock()
	delete(w.accounts, addr)
	w.lw.Unlock()

	return nil
}

func (w *LocalWallet) WalletExport(ctx context.Context, addr address.Address, pw string) (*types.KeyInfo, error) {
	ki, err := w.keystore.Get(addr.String(), pw)
	if err != nil {
		return nil, err
	}
	return &ki, nil
}

func (w *LocalWallet) WalletImport(ctx context.Context, ki *types.KeyInfo) (address.Address, error) {
	switch ki.Type {
	case types.Secp256k1, types.BLS:
		privkey, err := signature.ParsePrivateKey(ki.SecretKey, ki.Type)
		if err != nil {
			return address.Undef, err
		}

		pubKey := privkey.GetPublic()

		cbyte, err := pubKey.Raw()
		if err != nil {
			return address.Undef, err
		}

		addr, err := address.NewAddress(cbyte)
		if err != nil {
			return address.Undef, err
		}

		err = w.keystore.Put(addr.String(), w.password, *ki)
		if err != nil {
			return address.Undef, err
		}

		w.lw.Lock()
		w.accounts[addr] = privkey
		w.lw.Unlock()

		// for eth short addr
		if ki.Type == types.Secp256k1 {
			addrByte := utils.ToEthAddress(cbyte)

			eaddr, err := address.NewAddress(addrByte)
			if err != nil {
				return address.Undef, err
			}
			err = w.keystore.Put(eaddr.String(), w.password, *ki)
			if err != nil {
				return address.Undef, err
			}

			w.lw.Lock()
			w.accounts[eaddr] = privkey
			w.lw.Unlock()
		}

		return addr, nil
	default:
		return address.Undef, xerrors.New("unsupported key type")
	}

}
