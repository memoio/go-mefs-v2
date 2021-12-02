package types

import "github.com/fxamacker/cbor/v2"

type ChalEpoch struct {
	Epoch  uint64
	Height uint64
	Seed   MsgID
}

func (ce *ChalEpoch) Serialize() ([]byte, error) {
	return cbor.Marshal(ce)
}

func (ce *ChalEpoch) Deserialize(b []byte) error {
	return cbor.Unmarshal(b, ce)
}
