package types

import (
	"math/big"

	"github.com/fxamacker/cbor/v2"
	"github.com/zeebo/blake3"
	"golang.org/x/crypto/sha3"

	"github.com/memoio/go-mefs-v2/lib/utils"
)

type OrderHash [32]byte

// 报价单
type Quotation struct {
	ProID      uint64
	TokenIndex uint32
	SegPrice   *big.Int
	PiecePrice *big.Int
}

func (q *Quotation) Serialize() ([]byte, error) {
	return cbor.Marshal(q)
}

func (q *Quotation) Deserialize(b []byte) error {
	return cbor.Unmarshal(b, q)
}

// key: 'OrderNonce'/user/pro; value: nonce
// key: 'OrderNonceDone'/user/pro; value: nonce
// key: 'OrderBase'/user/pro/nonce; value: content
type OrderBase struct {
	UserID     uint64
	ProID      uint64
	Nonce      uint64
	Start      int64
	End        int64
	TokenIndex uint32
	SegPrice   *big.Int
	PiecePrice *big.Int
}

func (b *OrderBase) Hash() OrderHash {
	var oh OrderHash

	buf, err := cbor.Marshal(b)
	if err != nil {
		return oh
	}

	h := blake3.Sum256(buf)

	copy(oh[:], h[:])

	return oh
}

func (b *OrderBase) Serialize() ([]byte, error) {
	return cbor.Marshal(b)
}

func (b *OrderBase) Deserialize(buf []byte) error {
	return cbor.Unmarshal(buf, b)
}

type SignedOrder struct {
	OrderBase
	Size  uint64
	Price *big.Int
	Usign Signature
	Psign Signature
}

// for sign on chain
func (so *SignedOrder) Hash() []byte {
	d := sha3.NewLegacyKeccak256()
	d.Write(utils.LeftPadBytes(big.NewInt(int64(so.UserID)).Bytes(), 32))
	d.Write(utils.LeftPadBytes(big.NewInt(int64(so.ProID)).Bytes(), 32))
	d.Write(utils.LeftPadBytes(big.NewInt(int64(so.Nonce)).Bytes(), 32))
	d.Write(utils.LeftPadBytes(big.NewInt(so.Start).Bytes(), 32))
	d.Write(utils.LeftPadBytes(big.NewInt(so.End).Bytes(), 32))
	d.Write(utils.LeftPadBytes(so.SegPrice.Bytes(), 32))
	d.Write(utils.LeftPadBytes(so.PiecePrice.Bytes(), 32))
	d.Write(utils.LeftPadBytes(big.NewInt(int64(so.Size)).Bytes(), 32))
	d.Write(utils.LeftPadBytes(so.Price.Bytes(), 32))
	return d.Sum(nil)
}

func (so *SignedOrder) Serialize() ([]byte, error) {
	return cbor.Marshal(so)
}

func (so *SignedOrder) Deserialize(b []byte) error {
	return cbor.Unmarshal(b, so)
}

type OrderSeq struct {
	UserID   uint64
	ProID    uint64
	Nonce    uint64
	SeqNum   uint32   // strict incremental from 0
	Size     uint64   // accumulated
	Price    *big.Int //
	Segments AggSegsQueue
	//Pieces    [][]byte // dataType/name/size; 多个dataName;
}

// key: 'SignedOrderSeq'/user/pro/nonce/seqnum; value: SignedOrderSeq
type SignedOrderSeq struct {
	OrderSeq
	UserDataSig Signature // for data chain; hash(hash(OrderBase)+seqnum+size+price+name); signed by fs and pro
	ProDataSig  Signature
	UserSig     Signature // for settlement chain; hash(fsID+proID+nonce+start+end+size+price)
	ProSig      Signature
}

func (os *SignedOrderSeq) Hash() ([]byte, error) {
	data, err := cbor.Marshal(os.OrderSeq)
	if err != nil {
		return nil, err
	}

	h := blake3.Sum256(data)
	return h[:], nil
}

func (os *SignedOrderSeq) Serialize() ([]byte, error) {
	return cbor.Marshal(os)
}

func (os *SignedOrderSeq) Deserialize(b []byte) error {
	return cbor.Unmarshal(b, os)
}
