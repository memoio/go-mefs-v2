package types

import (
	"encoding/binary"
	"math/big"

	"github.com/fxamacker/cbor/v2"
	"github.com/memoio/go-mefs-v2/lib/utils"
	"golang.org/x/crypto/sha3"
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

func (pi *PostIncome) Hash() []byte {
	var buf = make([]byte, 8)
	d := sha3.NewLegacyKeccak256()
	binary.BigEndian.PutUint64(buf, pi.UserID)
	d.Write(buf)
	binary.BigEndian.PutUint64(buf, pi.ProID)
	d.Write(buf)
	d.Write(utils.LeftPadBytes(pi.Value.Bytes(), 32))
	d.Write(utils.LeftPadBytes(pi.Penalty.Bytes(), 32))
	return d.Sum(nil)
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

func (spi *SignedPostIncome) Serialize() ([]byte, error) {
	return cbor.Marshal(spi)
}

func (spi *SignedPostIncome) Deserialize(b []byte) error {
	return cbor.Unmarshal(b, spi)
}
