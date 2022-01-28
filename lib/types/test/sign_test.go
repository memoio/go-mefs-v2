package test

import (
	"testing"

	bls "github.com/memoio/go-mefs-v2/lib/crypto/bls12_381"
	"github.com/memoio/go-mefs-v2/lib/crypto/signature"
	"github.com/memoio/go-mefs-v2/lib/types"
	"github.com/zeebo/blake3"
)

func TestMultiSign(t *testing.T) {
	sk1, _ := signature.GenerateKey(types.BLS)
	pk1, _ := sk1.GetPublic().Raw()

	sk2, _ := signature.GenerateKey(types.BLS)
	pk2, _ := sk2.GetPublic().Raw()

	msg := []byte("hello")
	sign1, _ := sk1.Sign(msg)
	s1 := types.Signature{
		Type: types.SigBLS,
		Data: sign1,
	}

	ok, err := sk1.GetPublic().Verify(msg, sign1)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("wrong")
	}

	sign2, _ := sk2.Sign(msg)
	s2 := types.Signature{
		Type: types.SigBLS,
		Data: sign2,
	}

	ms := new(types.MultiSignature)
	ms.Type = types.SigBLS

	ms.Add(1, s1)
	ms.Add(2, s2)

	apk, _ := bls.AggregatePublicKey(pk1, pk2)

	hmsg := blake3.Sum256(msg)
	err = bls.Verify(apk, hmsg[:], ms.Data)
	if err != nil {
		t.Fatal(err)
	}

	msb, _ := ms.Serialize()

	nms := new(types.MultiSignature)

	nms.Deserialize(msb)

	t.Fatal(ms.Type, nms.Type)
}
