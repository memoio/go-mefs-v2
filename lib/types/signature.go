package types

import (
	"github.com/fxamacker/cbor/v2"
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

func (s *Signature) Deserilize(b []byte) error {
	return cbor.Unmarshal(b, s)
}
