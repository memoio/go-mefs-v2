package tx

import (
	"time"

	"github.com/fxamacker/cbor/v2"
	msign "github.com/memoio/go-mefs-v2/lib/multiSign"
	"github.com/memoio/go-mefs-v2/lib/types"
)

type MessageDigest struct {
	ID    types.MsgID
	From  uint64
	Nonce uint64
}

type BlockHeader struct {
	Version uint32
	Height  uint64
	MinerID uint64
	PrevID  types.MsgID // previous block id
	Time    time.Time   // block time

	Txs      []MessageDigest
	Receipts []Receipt
}

func (bh *BlockHeader) Hash() (types.MsgID, error) {
	res, err := bh.Serialize()
	if err != nil {
		return types.Undef, err
	}

	return types.NewMsgID(res), nil
}

func (bh *BlockHeader) Serialize() ([]byte, error) {
	return cbor.Marshal(bh)
}

func (bh *BlockHeader) Deserialize(b []byte) (types.MsgID, error) {
	err := cbor.Unmarshal(b, bh)
	if err != nil {
		return types.Undef, err
	}

	return types.NewMsgID(b), nil
}

type Block struct {
	BlockHeader
	msign.MultiSignature

	id types.MsgID
}

func (b *Block) Serialize() ([]byte, error) {
	return cbor.Marshal(b)
}

func (b *Block) Deserialize(d []byte) error {
	err := cbor.Unmarshal(d, b)
	if err != nil {
		return err
	}

	id, err := b.Hash()
	if err != nil {
		return err
	}

	b.id = id
	return nil
}
