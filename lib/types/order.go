package types

import (
	"encoding/binary"
	"math/big"

	"github.com/fxamacker/cbor/v2"
	"golang.org/x/crypto/sha3"
	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/lib/utils"
)

type OrderPayInfo struct {
	ID          uint64
	Size        uint64
	ConfirmSize uint64 // order submit to data
	OnChainSize uint64 // order add to settle chain
	ExpireSize  uint64 // order expired
	NeedPay     *big.Int
	Paid        *big.Int // order is added to settle chain
	Balance     *big.Int
}

func (pi *OrderPayInfo) Serialize() ([]byte, error) {
	return cbor.Marshal(pi)
}

func (pi *OrderPayInfo) Deserialize(b []byte) error {
	return cbor.Unmarshal(b, pi)
}

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
	Nonce    uint64
	SeqNum   uint32
	SubNonce uint64
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
func (b *OrderBase) Hash() MsgID {
	buf, err := cbor.Marshal(b)
	if err != nil {
		return MsgIDUndef
	}

	return NewMsgID(buf)
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
	binary.BigEndian.PutUint64(buf, so.UserID)
	d.Write(buf)
	binary.BigEndian.PutUint64(buf, so.ProID)
	d.Write(buf)
	binary.BigEndian.PutUint64(buf, so.Nonce)
	d.Write(buf)
	binary.BigEndian.PutUint64(buf, uint64(so.Start))
	d.Write(buf)
	binary.BigEndian.PutUint64(buf, uint64(so.End))
	d.Write(buf)
	binary.BigEndian.PutUint64(buf, so.Size)
	d.Write(buf)
	binary.BigEndian.PutUint32(buf[:4], so.TokenIndex)
	d.Write(buf[:4])
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
func (os *SignedOrderSeq) Hash() MsgID {
	buf, err := cbor.Marshal(os.OrderSeq)
	if err != nil {
		return MsgIDUndef
	}

	return NewMsgID(buf)
}

func (os *SignedOrderSeq) Serialize() ([]byte, error) {
	return cbor.Marshal(os)
}

func (os *SignedOrderSeq) Deserialize(b []byte) error {
	return cbor.Unmarshal(b, os)
}

type OrderDelPart struct {
	AccFr []byte
	Size  uint64
	Price *big.Int
}

type OrderFull struct {
	SignedOrder
	SeqNum  uint32
	AccFr   []byte
	DelPart OrderDelPart
}

func (of *OrderFull) Serialize() ([]byte, error) {
	return cbor.Marshal(of)
}

func (of *OrderFull) Deserialize(b []byte) error {
	return cbor.Unmarshal(b, of)
}

type SeqFull struct {
	OrderSeq
	AccFr   []byte
	DelPart OrderDelPart
	DelSegs AggSegsQueue
}

func (sf *SeqFull) Serialize() ([]byte, error) {
	return cbor.Marshal(sf)
}

func (sf *SeqFull) Deserialize(b []byte) error {
	return cbor.Unmarshal(b, sf)
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
