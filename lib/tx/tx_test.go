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

	sm.To = 10

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
	err = nsm.Deserialize(sms)
	if err != nil {
		t.Fatal(err)
	}

	nid, _ := nsm.Hash()

	ok, err := priv.GetPublic().Verify(nid.Bytes(), nsm.Signature.Data)
	if err != nil {
		t.Fatal(err)
	}

	if !ok {
		t.Fatal("signature wrong")
	}

	t.Fatal(id.Hex(), nid.Hex(), nsm.GasLimit, nsm.GasPrice, nsm.Message.To)
}

func TestBlock(t *testing.T) {
	b := new(Block)
	b.BlockHeader.MinerID = 100
	b.BlockHeader.PrevID = types.NewMsgID([]byte("test"))
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
	err = nb.Deserialize(bbyte)
	if err != nil {
		t.Fatal(err)
	}

	nid, _ := nb.Hash()

	ok, err := priv.GetPublic().Verify(nid.Bytes(), nb.MultiSignature.Data)
	if err != nil {
		t.Fatal(err)
	}

	if !ok {
		t.Fatal("signature wrong")
	}

	t.Fatal(bid.String(), nid.String(), b.PrevID.String(), nb.PrevID.String())
}
