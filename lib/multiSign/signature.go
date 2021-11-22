package msign

import (
	"errors"

	"github.com/fxamacker/cbor/v2"

	bls "github.com/memoio/go-mefs-v2/lib/crypto/bls12_381"
	"github.com/memoio/go-mefs-v2/lib/types"
)

// aggregate sign
type MultiSignature struct {
	Type   types.SigType
	Signer []uint64
	Data   []byte
}

func NewMultiSignature(typ types.SigType) MultiSignature {
	return MultiSignature{
		Type: typ,
	}
}

func (ms *MultiSignature) Add(id uint64, sig types.Signature) error {
	if sig.Type != ms.Type {
		return errors.New("type not equal")
	}

	switch ms.Type {
	case types.SigBLS:
		if len(ms.Data) == 0 {
			ms.Data = sig.Data
			ms.Signer = append(ms.Signer, id)
		} else {
			asig, err := bls.AggregateSignature(ms.Data, sig.Data)
			if err != nil {
				return err
			}
			ms.Data = asig
			ms.Signer = append(ms.Signer, id)
		}
	default:
		ms.Data = append(ms.Data, sig.Data...)
		ms.Signer = append(ms.Signer, id)
	}

	return nil
}

func (ms *MultiSignature) Serialize() ([]byte, error) {
	return cbor.Marshal(ms)
}

func (ms *MultiSignature) Deserialize(b []byte) error {
	return cbor.Unmarshal(b, ms)
}
