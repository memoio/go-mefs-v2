package wallet

import (
	"context"
	"os"
	"testing"

	"github.com/memoio/go-mefs-v2/lib/backend/keystore"
	"github.com/memoio/go-mefs-v2/lib/crypto/signature"
	"github.com/memoio/go-mefs-v2/lib/types"

	"github.com/mitchellh/go-homedir"
	"github.com/zeebo/blake3"
)

func TestAccount(t *testing.T) {
	p, _ := homedir.Expand("~/test/wallet")
	err := os.MkdirAll(p, os.ModePerm)
	if err != nil {
		t.Fatal(err)
	}

	tmap := make(map[uint64]struct{}, 5)
	t.Log(len(tmap))
	tmap[1] = struct{}{}
	t.Log(len(tmap))

	ks, err := keystore.NewKeyRepo(p)
	if err != nil {
		t.Fatal(err)
	}

	lw := New("123456", ks)

	addr, err := lw.WalletNew(context.Background(), types.Secp256k1)
	if err != nil {
		t.Fatal(err)
	}

	msg := blake3.Sum256([]byte("aa"))
	sig, err := lw.WalletSign(context.Background(), addr, msg[:])
	if err != nil {
		t.Fatal(err)
	}

	ok, err := signature.Verify(addr.Bytes(), msg[:], sig)
	if err != nil {
		t.Fatal(err)
	}

	if !ok {
		t.Fatal("wrong")
	}

	res, err := lw.WalletList(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	t.Fatal(res)
}
