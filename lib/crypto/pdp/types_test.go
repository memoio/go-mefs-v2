package pdp

import (
	"bytes"
	"testing"

	pdpv2 "github.com/memoio/go-mefs-v2/lib/crypto/pdp/version2"
)

func TestPkSerialize(t *testing.T) {
	keyset, err := pdpv2.GenKeySet()
	if err != nil {
		t.Fatal(err)
	}

	pkv := NewPublicKeyWithVersion(keyset.PublicKey())
	pkvBytes := pkv.Serialize()

	pkv1 := new(PublicKeyWithVersion)
	err = pkv1.Deserialize(pkvBytes)
	if err != nil {
		t.Fatal(err)
	}
	pks := pkv1.Pk.Serialize()
	pksi := keyset.Pk.Serialize()
	pks0 := pkv.Pk.Serialize()

	if !bytes.Equal(pks, pksi) || !bytes.Equal(pks0, pksi) {
		t.Error("a")
	}
}
