package keystore

import (
	"os"
	"testing"

	"github.com/memoio/go-mefs-v2/lib/address"
	"github.com/memoio/go-mefs-v2/lib/crypto/signature"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/mitchellh/go-homedir"
)

func TestWallet(t *testing.T) {
	p, _ := homedir.Expand("~/test/wallet")
	err := os.MkdirAll(p, os.ModePerm)
	if err != nil {
		t.Fatal(err)
	}

	pw := "12345678"

	kp, err := NewKeyRepo(p)
	if err != nil {
		t.Fatal(err)
	}

	privkey, err := signature.GenerateKey(types.Secp256k1)
	if err != nil {
		t.Fatal(err)
	}

	pbyte, err := privkey.Raw()
	if err != nil {
		t.Fatal(err)
	}

	ki := types.KeyInfo{
		SecretKey: pbyte,
		Type:      privkey.Type(),
	}

	pubByte, _ := privkey.GetPublic().Raw()

	addr, err := address.NewAddress(pubByte)
	if err != nil {
		t.Fatal(err)
	}

	name := addr.String()

	err = kp.Put(name, pw, ki)
	if err != nil {
		t.Fatal(err)
	}

	nki, err := kp.Get(name, pw)
	if err != nil {
		t.Fatal(err)
	}

	npriv, err := signature.ParsePrivateKey(nki.SecretKey, nki.Type)
	if err != nil {
		t.Fatal(err)
	}

	ok := privkey.Equals(npriv)
	if !ok {
		t.Fatal("not equal")
	}

	as, _ := kp.List()
	t.Log(as)
	t.Fatal(as)
}
