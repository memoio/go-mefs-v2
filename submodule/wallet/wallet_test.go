package wallet

import (
	"encoding/hex"
	"os"
	"path"
	"testing"

	"github.com/memoio/go-mefs-v2/lib/crypto/signature"
	"github.com/memoio/go-mefs-v2/lib/crypto/signature/secp256k1"
	"github.com/mitchellh/go-homedir"
)

func TestWallet(t *testing.T) {
	privkey, pubkey, err := secp256k1.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	p, _ := homedir.Expand("~/wallet")
	err = os.MkdirAll(p, os.ModePerm)
	if err != nil {
		t.Fatal(err)
	}
	addr, err := signature.GetAdressFromPubkey(pubkey)
	if err != nil {
		t.Fatal(err)
	}
	err = StorePrivateKey(p, privkey, "12345")
	if err != nil {
		t.Fatal(err)
	}

	prvKeyNew, err := LoadPrivateKey(hex.EncodeToString(addr),
		"12345",
		path.Join(hex.EncodeToString(addr)))

	if err != nil {
		t.Fatal(err)
	}
	if !prvKeyNew.Equals(privkey) {
		t.Fatal("rolling")
	}
}
