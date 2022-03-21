package types

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"hash"

	"github.com/mr-tron/base58/base58"
	mh "github.com/multiformats/go-multihash"
	"github.com/zeebo/blake3"
	"golang.org/x/xerrors"
)

const (
	MsgIDHashCode = 0x4c
	MsgIDHashLen  = 20

	// todo add type for content
)

func init() {
	mh.Register(MsgIDHashCode, func() hash.Hash { return blake3.New() })
}

type MsgID struct{ str string }

var MsgIDUndef = MsgID{}

func NewMsgID(data []byte) MsgID {
	res, err := mh.Sum(data, MsgIDHashCode, MsgIDHashLen)
	if err != nil {
		return MsgIDUndef
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

func (m MsgID) Equal(old MsgID) bool {
	return bytes.Equal(m.Bytes(), old.Bytes())
}

// json encode and decode
func (m MsgID) MarshalJSON() ([]byte, error) {
	return []byte(`"` + m.String() + `"`), nil
}

func (m *MsgID) UnmarshalJSON(b []byte) error {
	var s string
	err := json.Unmarshal(b, &s)
	if err != nil {
		return err
	}

	id, err := FromString(s)
	if err != nil {
		return err
	}
	*m = id
	return nil
}

// for cbor
func (m *MsgID) UnmarshalBinary(data []byte) error {
	id, err := FromBytes(data)
	if err == nil {
		*m = id
	}

	return nil
}

func (m MsgID) MarshalBinary() ([]byte, error) {
	return m.Bytes(), nil
}

func FromString(s string) (MsgID, error) {
	b, err := base58.Decode(s)
	if err != nil {
		return MsgIDUndef, err
	}

	return FromBytes(b)
}

func FromHexString(s string) (MsgID, error) {
	b, err := hex.DecodeString(s)
	if err != nil {
		return MsgIDUndef, err
	}

	return FromBytes(b)
}

func FromBytes(b []byte) (MsgID, error) {
	dh, err := mh.Decode(b)
	if err != nil {
		return MsgIDUndef, err
	}

	if dh.Code != MsgIDHashCode {
		return MsgIDUndef, xerrors.Errorf("illegal message code %d", dh.Code)
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
