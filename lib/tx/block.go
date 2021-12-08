package tx

import (
	"time"

	"github.com/fxamacker/cbor/v2"
	"github.com/memoio/go-mefs-v2/lib/types"
)

type MessageDigest struct {
	ID    types.MsgID
	From  uint64
	Nonce uint64
}

type RawHeader struct {
	Version uint32
	Height  uint64
	Slot    uint64 // consensus epoch; logic time
	MinerID uint64
	PrevID  types.MsgID // previous block id
	Time    time.Time   // block time, need?
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

type BlockHeader struct {
	RawHeader

	// tx
	Txs      []MessageDigest
	Receipts []Receipt

	// todo: add agg signs of all tx

	// state root
	ParentRoot types.MsgID
	Root       types.MsgID
}

func (bh *BlockHeader) Hash() types.MsgID {
	res, err := bh.Serialize()
	if err != nil {
		return types.MsgIDUndef
	}

	return types.NewMsgID(res)
}

func (bh *BlockHeader) Serialize() ([]byte, error) {
	return cbor.Marshal(bh)
}

func (bh *BlockHeader) Deserialize(b []byte) error {
	return cbor.Unmarshal(b, bh)
}

type Block struct {
	BlockHeader
	// sign
	types.MultiSignature
}

func (b *Block) Serialize() ([]byte, error) {
	return cbor.Marshal(b)
}

func (b *Block) Deserialize(d []byte) error {
	err := cbor.Unmarshal(d, b)
	if err != nil {
		return err
	}

	return nil
}
