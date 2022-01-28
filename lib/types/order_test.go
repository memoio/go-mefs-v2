package types

import (
	"encoding/hex"
	"math/big"
	"testing"

	"github.com/fxamacker/cbor/v2"
	"github.com/zeebo/blake3"
)

func TestOrder(t *testing.T) {
	ha := blake3.Sum256([]byte("test"))

	sob := &SignedOrder{
		OrderBase: OrderBase{
			SegPrice: big.NewInt(10),
		},
		Usign: Signature{
			Type: SigSecp256k1,
			Data: ha[:],
		},
	}

	sbyte, _ := cbor.Marshal(sob)

	nsob := new(SignedOrder)

	cbor.Unmarshal(sbyte, nsob)
	t.Fatal(nsob.SegPrice, nsob.Usign.Type, hex.EncodeToString(ha[:]), hex.EncodeToString(nsob.Usign.Data))
}
