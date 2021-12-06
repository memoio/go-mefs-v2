package types

import (
	"math/big"

	"github.com/fxamacker/cbor/v2"
)

type ChalEpoch struct {
	Epoch uint64
	Slot  uint64 // todo: need change to epoch
	Seed  MsgID
}

func (ce *ChalEpoch) Serialize() ([]byte, error) {
	return cbor.Marshal(ce)
}

func (ce *ChalEpoch) Deserialize(b []byte) error {
	return cbor.Unmarshal(b, ce)
}

type PostIncome struct {
	UserID  uint64
	ProID   uint64
	Value   *big.Int // duo to income
	Penalty *big.Int // due to delete
}

func (pi *PostIncome) Serialize() ([]byte, error) {
	return cbor.Marshal(pi)
}

func (pi *PostIncome) Deserialize(b []byte) error {
	return cbor.Unmarshal(b, pi)
}

type SignedPostIncome struct {
	PostIncome
	Sign MultiSignature // signed by keepers
}
