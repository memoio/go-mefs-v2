package types

import (
	"encoding/hex"
	"errors"
	"hash"

	"github.com/mr-tron/base58/base58"
	mh "github.com/multiformats/go-multihash"
	"github.com/zeebo/blake3"
)

const (
	MsgIDHashCode = 0x4c
	MsgIDHashLen  = 20

	// todo add type for content
)

var (
	ErrMsgCode = errors.New("illegal msg code")
)

func init() {
	mh.Register(MsgIDHashCode, func() hash.Hash { return blake3.New() })
}

type MsgID struct{ str string }

var Undef = MsgID{}

func NewMsgID(data []byte) MsgID {
	res, err := mh.Sum(data, MsgIDHashCode, MsgIDHashLen)
	if err != nil {
		return Undef
	}

	return MsgID{string(res)}
}

func (m MsgID) Bytes() []byte {
	return []byte(m.str)
}

func (m MsgID) String() string {
	return base58.Encode(m.Bytes())
}

func (m MsgID) Hex() string {
	return hex.EncodeToString(m.Bytes())
}

func FromString(s string) (MsgID, error) {
	b, err := base58.Decode(s)
	if err != nil {
		return Undef, err
	}

	return FromBytes(b)
}

func FromHexString(s string) (MsgID, error) {
	b, err := hex.DecodeString(s)
	if err != nil {
		return Undef, err
	}

	return FromBytes(b)
}

func FromBytes(b []byte) (MsgID, error) {
	dh, err := mh.Decode(b)
	if err != nil {
		return Undef, err
	}

	if dh.Code != MsgIDHashCode {
		return Undef, ErrMsgCode
	}

	return MsgID{string(b)}, nil
}

type Msg interface {
	ID() MsgID
	Content() []byte
}

var _ Msg = (*BasicMsg)(nil)

type BasicMsg struct {
	id   MsgID
	data []byte
}

func NewMsg(data, sign []byte) *BasicMsg {
	return &BasicMsg{data: data, id: NewMsgID(data)}
}

func (bm *BasicMsg) ID() MsgID {
	return bm.id
}

func (bm *BasicMsg) Content() []byte {
	return bm.data
}
