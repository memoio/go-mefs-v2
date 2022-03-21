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
	Slot  uint64
	Seed  MsgID
}

func (ce *ChalEpoch) Serialize() ([]byte, error) {
	return cbor.Marshal(ce)
}

func (ce *ChalEpoch) Deserialize(b []byte) error {
	return cbor.Unmarshal(b, ce)
}

type PostIncome struct {
	UserID     uint64
	ProID      uint64
	TokenIndex uint32
	Value      *big.Int // duo to income
	Penalty    *big.Int // due to delete
}

func (pi *PostIncome) Serialize() ([]byte, error) {
	return cbor.Marshal(pi)
}

func (pi *PostIncome) Deserialize(b []byte) error {
	return cbor.Unmarshal(b, pi)
}

type AccPostIncome struct {
	ProID      uint64
	TokenIndex uint32
	Value      *big.Int // duo to income
	Penalty    *big.Int // due to delete
}

func (api *AccPostIncome) Hash() []byte {
	var buf = make([]byte, 8)
	d := sha3.NewLegacyKeccak256()
	binary.BigEndian.PutUint64(buf, api.ProID)
	d.Write(buf)
	binary.BigEndian.PutUint32(buf[:4], api.TokenIndex)
	d.Write(buf[:4])
	d.Write(utils.LeftPadBytes(api.Value.Bytes(), 32))
	d.Write(utils.LeftPadBytes(api.Penalty.Bytes(), 32))
	return d.Sum(nil)
}

func (api *AccPostIncome) Serialize() ([]byte, error) {
	return cbor.Marshal(api)
}

func (api *AccPostIncome) Deserialize(b []byte) error {
	return cbor.Unmarshal(b, api)
}

type SignedAccPostIncome struct {
	AccPostIncome
	Sig MultiSignature
}

func (sapi *SignedAccPostIncome) Serialize() ([]byte, error) {
	return cbor.Marshal(sapi)
}

func (sapi *SignedAccPostIncome) Deserialize(b []byte) error {
	return cbor.Unmarshal(b, sapi)
}
