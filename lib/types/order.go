package types

import (
	"math/big"

	"github.com/fxamacker/cbor/v2"
	"github.com/zeebo/blake3"
)

type OrderHash [8]byte

// 报价单
type Quotation struct {
	ProID      uint64
	TokenIndex uint32
	SegPrice   *big.Int
	PiecePrice *big.Int
}

// key: 'OrderNonce'/user/pro; value: nonce
// key: 'OrderNonceDone'/user/pro; value: nonce
// key: 'OrderBase'/user/pro/nonce; value: content
type OrderBase struct {
	UserID     uint64
	ProID      uint64
	Nonce      uint64
	Start      uint64
	End        uint64
	TokenIndex uint32
	SegPrice   *big.Int
	PiecePrice *big.Int
}

func (b *OrderBase) GetHash() []byte {
	buf, err := cbor.Marshal(b)
	if err != nil {
		return nil
	}

	h := blake3.Sum256(buf)

	return h[:]
}

func (b *OrderBase) GetShortHash() OrderHash {
	h := b.GetHash()

	var o OrderHash
	copy(o[:], h[:8])
	return o
}

type SignedOrderBase struct {
	OrderBase
	Usign []byte
	Psign []byte
}

// key: 'OrderSeq'/user/pro/nonce/seqnum; value: OrderSeq
type OrderSeq struct {
	ID          OrderHash // fast lookup
	SeqNum      uint32    // strict incremental from 0
	Size        uint64    // accumulated
	Price       *big.Int  //
	Segments    []Segs    // dataType/name/size; 多个dataName;
	Pieces      [][]byte  // piece
	UserDataSig []byte    // for data chain; hash(hash(OrderBase)+seqnum+size+price+name); signed by fs and pro
	ProDataSig  []byte
	UserSig     []byte // for settlement chain; hash(fsID+proID+nonce+start+end+size+price)
	ProSig      []byte
}

type OrderData struct {
	ID       OrderHash
	DataName []byte // dataType + name
	Start    uint64 // 获取指定位置
	Length   uint64 // 获取指定长度
}
