package types

import (
	"github.com/fxamacker/cbor/v2"
	"golang.org/x/xerrors"

	bls "github.com/memoio/go-mefs-v2/lib/crypto/bls12_381"
)

type SigType byte

const (
	SigUnkown = SigType(iota)
	SigSecp256k1
	SigBLS
)

type Signature struct {
	Type SigType
	Data []byte
}

func (s *Signature) Serialize() ([]byte, error) {
	return cbor.Marshal(s)
}

func (s *Signature) Deserialize(b []byte) error {
	return cbor.Unmarshal(b, s)
}

type MultiSignature struct {
	Type   SigType
	Signer []uint64
	Data   []byte
}

func NewMultiSignature(typ SigType) MultiSignature {
	return MultiSignature{
		Type: typ,
	}
}

func (ms *MultiSignature) Add(id uint64, sig Signature) error {
	if sig.Type != ms.Type {
		return xerrors.Errorf("type not equal expected %d, got %d", ms.Type, sig.Type)
	}

	for _, sid := range ms.Signer {
		if sid == id {
			return xerrors.Errorf("signer exist")
		}
	}

	switch ms.Type {
	case SigBLS:
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

func (ms *MultiSignature) SanityCheck() error {
	tmpMap := make(map[uint64]struct{}, len(ms.Signer))
	for _, sid := range ms.Signer {
		_, ok := tmpMap[sid]
		if ok {
			return xerrors.Errorf("duplicate signers")
		} else {
			tmpMap[sid] = struct{}{}
		}
	}
	return nil
}

func (ms *MultiSignature) Len() int {
	return len(ms.Signer)
}

func (ms *MultiSignature) Serialize() ([]byte, error) {
	return cbor.Marshal(ms)
}

func (ms *MultiSignature) Deserialize(b []byte) error {
	return cbor.Unmarshal(b, ms)
}
