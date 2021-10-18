package wallet

import (
	"encoding/hex"
	"io/ioutil"
	"os"
	"path"

	"github.com/memoio/go-mefs-v2/lib/crypto/signature"
	sig_common "github.com/memoio/go-mefs-v2/lib/crypto/signature/common"
)

type Wallet struct {
	p        string
	accounts map[string]sig_common.PrivKey
}

func New(p string) (*Wallet, error) {
	walletPath := path.Join(p, "wallet")
	err := os.MkdirAll(walletPath, os.ModePerm)
	if err != nil {
		return nil, err
	}
	return &Wallet{
		walletPath,
		make(map[string]sig_common.PrivKey),
	}, nil
}

func (w *Wallet) List() ([]string, error) {
	var addrs []string
	files, err := ioutil.ReadDir(w.p)
	if err != nil {
		return nil, err
	}
	for _, fi := range files {
		if !fi.IsDir() {
			_, err = hex.DecodeString(fi.Name())
			if err != nil {
				continue
			}
			addrs = append(addrs, fi.Name())
		}
	}
	return addrs, nil
}

func (w *Wallet) LoadPrivateKey(addr, password string) (sig_common.PrivKey, error) {
	if priv, ok := w.accounts[addr]; ok {
		return priv, nil
	}
	walletPath := path.Join(w.p, addr)
	priv, err := LoadPrivateKey(addr, password, walletPath)
	if err != nil {
		return nil, err
	}
	w.accounts[addr] = priv
	return priv, nil
}

func (w *Wallet) StorePrivateKey(privKey sig_common.PrivKey, password string) error {
	err := StorePrivateKey(w.p, privKey, password)
	if err != nil {
		return err
	}
	addr, err := signature.GetAdressFromPubkey(privKey.GetPublic())
	if err != nil {
		return err
	}
	w.accounts[hex.EncodeToString(addr)] = privKey
	return nil
}

func (w *Wallet) API() *WalletAPI {
	return &WalletAPI{w}
}
