package secp256k1

import (
	"bytes"
	"crypto/sha256"
	"testing"
)

func TestSign(t *testing.T) {
	sk, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}

	skb, err := sk.Raw()
	if err != nil {
		t.Fatal(err)
	}

	h := sha256.Sum256(skb)

	sig, err := Sign(skb, h[:])
	if err != nil {
		t.Fatal(err)
	}
	t.Log(len(sig))

	pkb, err := sk.GetPublic().Raw()
	if err != nil {
		t.Fatal(err)
	}

	ok := Verify(pkb, h[:], sig)
	if !ok {
		t.Fatal("verify fail")
	}

	t.Log(len(sig))

	pkbc, err := EcRecover(h[:], sig)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(pkb, pkbc) {
		t.Fatal("ecrecover fail")
	}
}
