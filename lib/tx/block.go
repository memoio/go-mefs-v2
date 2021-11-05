package tx

import (
	"encoding/binary"
	"time"

	"github.com/fxamacker/cbor/v2"
	msign "github.com/memoio/go-mefs-v2/lib/multiSign"
	"github.com/memoio/go-mefs-v2/lib/types"
)

type BlockHeader struct {
	Height  uint64
	MinerID uint64
	PrevID  types.MsgID // previous block id
	Time    time.Time   // block time

	Txs      []types.MsgID
	Receipts []Receipt
}

func (bh *BlockHeader) Serialize() ([]byte, error) {
	return cbor.Marshal(bh)
}

func (bh *BlockHeader) Hash() (types.MsgID, error) {
	res, err := bh.Serialize()
	if err != nil {
		return types.Undef, err
	}

	return types.NewMsgID(res), nil
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

	ID types.MsgID
}

func (b *Block) Serialize() ([]byte, error) {
	bh, err := b.BlockHeader.Serialize()
	if err != nil {
		return nil, err
	}

	s, err := b.MultiSignature.Serialize()
	if err != nil {
		return nil, err
	}

	rLen := len(bh)

	buf := make([]byte, 2+rLen+len(s))
	binary.BigEndian.PutUint16(buf[:2], uint16(rLen))

	copy(buf[2:2+rLen], bh)
	copy(buf[2+rLen:], s)

	return buf, nil
}

func (b *Block) Deserialize(d []byte) error {
	if len(d) < 2 {
		return ErrMsgLenShort
	}

	rLen := binary.BigEndian.Uint16(d[:2])
	if len(d) < 2+int(rLen) {
		return ErrMsgLenShort
	}

	bh := new(BlockHeader)
	mid, err := bh.Deserialize(d[2 : 2+rLen])
	if err != nil {
		return err
	}

	s := new(msign.MultiSignature)
	err = s.Deserialize(d[2+rLen:])
	if err != nil {
		return err
	}

	b.BlockHeader = *bh
	b.ID = mid
	b.MultiSignature = *s

	return nil
}
