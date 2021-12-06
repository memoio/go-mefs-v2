package types

import (
	"encoding/binary"
	"math/big"

	"github.com/fxamacker/cbor/v2"
	"github.com/zeebo/blake3"
	"golang.org/x/crypto/sha3"
	"golang.org/x/xerrors"

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

type NonceSeq struct {
	Nonce  uint64
	SeqNum uint32
}

func (ns *NonceSeq) Serialize() ([]byte, error) {
	return cbor.Marshal(ns)
}

func (ns *NonceSeq) Deserialize(b []byte) error {
	return cbor.Unmarshal(b, ns)
}

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

// for sign on data chain
func (b *OrderBase) Hash() ([]byte, error) {
	buf, err := cbor.Marshal(b)
	if err != nil {
		return nil, err
	}

	h := blake3.Sum256(buf)

	return h[:], nil
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
	Psign Signature // sign hash
}

// for sign on settle chain
func (so *SignedOrder) Hash() []byte {
	var buf = make([]byte, 8)
	d := sha3.NewLegacyKeccak256()
	d.Write(utils.LeftPadBytes(big.NewInt(int64(so.UserID)).Bytes(), 32))
	d.Write(utils.LeftPadBytes(big.NewInt(int64(so.ProID)).Bytes(), 32))
	d.Write(utils.LeftPadBytes(big.NewInt(int64(so.Nonce)).Bytes(), 32))
	binary.BigEndian.PutUint64(buf, uint64(so.Start))
	d.Write(buf)
	binary.BigEndian.PutUint64(buf, uint64(so.End))
	d.Write(buf)
	binary.BigEndian.PutUint64(buf, so.Size)
	d.Write(buf)
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
}

type SignedOrderSeq struct {
	OrderSeq
	UserDataSig Signature // for data chain; sign serialize); signed by fs and pro
	ProDataSig  Signature
	UserSig     Signature // for settlement chain; sign hash
	ProSig      Signature
}

// for sign on data chain
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

// for quick filter expired
type OrderDuration struct {
	Start []int64
	End   []int64
}

func (od *OrderDuration) Add(start, end int64) error {
	if start >= end {
		return xerrors.Errorf("start %d is later than end %d", start, end)
	}

	if len(od.Start) > 0 {
		olen := len(od.Start)
		if od.Start[olen-1] > start {
			return xerrors.Errorf("start %d is later than previous %d", start, od.Start[olen-1])
		}

		if od.End[olen-1] > end {
			return xerrors.Errorf("end %d is early than previous %d", end, od.End[olen-1])
		}
	}

	od.Start = append(od.Start, start)
	od.End = append(od.End, end)
	return nil
}

func (od *OrderDuration) Serialize() ([]byte, error) {
	return cbor.Marshal(od)
}

func (od *OrderDuration) Deserialize(b []byte) error {
	return cbor.Unmarshal(b, od)
}
