package tx

import (
	"math/big"
	"testing"

	"github.com/memoio/go-mefs-v2/lib/crypto/signature"
	"github.com/memoio/go-mefs-v2/lib/types"
)

func TestMessage(t *testing.T) {
	sm := new(SignedMessage)
	sm.GasLimit = 100

	sm.GasPrice = big.NewInt(10)

	sm.To = "hello"

	id, err := sm.Hash()
	if err != nil {
		t.Fatal(err)
	}

	priv, _ := signature.GenerateKey(types.Secp256k1)
	sign, _ := priv.Sign(id.Bytes())

	sig := types.Signature{
		Data: sign,
		Type: types.SigSecp256k1,
	}

	sm.Signature = sig

	sms, err := sm.Serialize()
	if err != nil {
		t.Fatal(err)
	}

	nsm := new(SignedMessage)
	err = nsm.Deserilize(sms)
	if err != nil {
		t.Fatal(err)
	}

	ok, err := priv.GetPublic().Verify(nsm.ID.Bytes(), nsm.Signature.Data)
	if err != nil {
		t.Fatal(err)
	}

	if !ok {
		t.Fatal("signature wrong")
	}

	t.Fatal(id.Hex(), nsm.ID.Hex(), nsm.GasLimit, nsm.GasPrice, nsm.Message.To)
}

func TestBlock(t *testing.T) {
	b := new(Block)
	b.BlockHeader.MinerID = 100
	b.MultiSignature.Type = types.SigBLS

	id, err := b.Hash()
	if err != nil {
		t.Fatal(err)
	}

	priv, _ := signature.GenerateKey(types.BLS)
	sign, _ := priv.Sign(id.Bytes())

	sig := types.Signature{
		Data: sign,
		Type: types.SigBLS,
	}

	err = b.MultiSignature.Add(0, sig)
	if err != nil {
		t.Fatal("add fail")
	}

	bbyte, err := b.Serialize()
	if err != nil {
		t.Fatal(err)
	}

	bid, err := b.BlockHeader.Hash()
	if err != nil {
		t.Fatal(err)
	}

	nb := new(Block)
	err = nb.Deserilize(bbyte)
	if err != nil {
		t.Fatal(err)
	}

	ok, err := priv.GetPublic().Verify(nb.ID.Bytes(), nb.MultiSignature.Data)
	if err != nil {
		t.Fatal(err)
	}

	if !ok {
		t.Fatal("signature wrong")
	}

	t.Fatal(bid.String(), nb.ID.String())
}
