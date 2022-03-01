package tx

import (
	"github.com/fxamacker/cbor/v2"
	"github.com/memoio/go-mefs-v2/lib/types"
)

type RawHeader struct {
	Version uint32 // for upgrade
	GroupID uint64
	Height  uint64
	Slot    uint64 // consensus epoch; logic time
	MinerID uint64
	PrevID  types.MsgID // previous block id
}

func (rh *RawHeader) Hash() types.MsgID {
	res, err := rh.Serialize()
	if err != nil {
		return types.MsgIDUndef
	}

	return types.NewMsgID(res)
}

func (rh *RawHeader) Serialize() ([]byte, error) {
	return cbor.Marshal(rh)
}

func (rh *RawHeader) Deserialize(b []byte) error {
	return cbor.Unmarshal(b, rh)
}

type MessageDigest struct {
	ID    types.MsgID
	From  uint64
	Nonce uint64
}

type MsgSet struct {
	// tx
	Msgs     []SignedMessage
	Receipts []Receipt

	// todo: add agg signs of all tx

	// state root
	ParentRoot types.MsgID
	Root       types.MsgID
}

type RawBlock struct {
	RawHeader
	MsgSet
}

func (bh *RawBlock) Hash() types.MsgID {
	res, err := bh.Serialize()
	if err != nil {
		return types.MsgIDUndef
	}

	return types.NewMsgID(res)
}

func (bh *RawBlock) Serialize() ([]byte, error) {
	return cbor.Marshal(bh)
}

func (bh *RawBlock) Deserialize(b []byte) error {
	return cbor.Unmarshal(b, bh)
}

type SignedBlock struct {
	RawBlock
	// sign
	types.MultiSignature
}

func (sb *SignedBlock) Serialize() ([]byte, error) {
	return cbor.Marshal(sb)
}

func (sb *SignedBlock) Deserialize(d []byte) error {
	err := cbor.Unmarshal(d, sb)
	if err != nil {
		return err
	}

	return nil
}
