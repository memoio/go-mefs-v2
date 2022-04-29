package types

import (
	"github.com/fxamacker/cbor/v2"
)

type Record struct {
	Key   []byte
	Value []byte
}

func (r *Record) Hash() MsgID {
	buf, err := cbor.Marshal(r)
	if err != nil {
		return MsgIDUndef
	}

	return NewMsgID(buf)
}

func (r *Record) Serialize() ([]byte, error) {
	return cbor.Marshal(r)
}

func (r *Record) Deserialize(b []byte) error {
	return cbor.Unmarshal(b, r)
}

type SignedRecord struct {
	Record
	Sign Signature
}

func (sr *SignedRecord) Serialize() ([]byte, error) {
	return cbor.Marshal(sr)
}

func (sr *SignedRecord) Deserialize(b []byte) error {
	return cbor.Unmarshal(b, sr)
}
