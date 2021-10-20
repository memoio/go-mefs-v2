package account

import (
	"os"
	"testing"

	"github.com/memoio/go-mefs-v2/lib/backend/keystore"
	"github.com/memoio/go-mefs-v2/lib/crypto/signature"
	sig_common "github.com/memoio/go-mefs-v2/lib/crypto/signature/common"
	"github.com/mitchellh/go-homedir"
	"github.com/zeebo/blake3"
)

func TestAccount(t *testing.T) {
	p, _ := homedir.Expand("~/test/wallet")
	err := os.MkdirAll(p, os.ModePerm)
	if err != nil {
		t.Fatal(err)
	}

	ks, err := keystore.NewKeyRepo(p)
	if err != nil {
		t.Fatal(err)
	}

	lw := NewWallet("123456", ks)

	addr, err := lw.WalletNew(sig_common.Secp256k1)
	if err != nil {
		t.Fatal(err)
	}

	msg := blake3.Sum256([]byte("aa"))
	sig, err := lw.WalletSign(addr, msg[:])
	if err != nil {
		t.Fatal(err)
	}

	ok, err := signature.Verify(addr, msg[:], sig)
	if err != nil {
		t.Fatal(err)
	}

	if !ok {
		t.Fatal("wrong")
	}
}
