package state

import (
	"github.com/bits-and-blooms/bitset"
	"github.com/fxamacker/cbor/v2"
	"golang.org/x/xerrors"

	bls "github.com/memoio/go-mefs-v2/lib/crypto/bls12_381"
	pdpcommon "github.com/memoio/go-mefs-v2/lib/crypto/pdp/common"
	logging "github.com/memoio/go-mefs-v2/lib/log"
	"github.com/memoio/go-mefs-v2/lib/types"
)

var logger = logging.Logger("state")

var (
	beginRoot = types.NewMsgID([]byte("state"))
)

type HandleAddUserFunc func(userID uint64)

// todo: add msg fee here
type roleInfo struct {
	Nonce uint64 // msg nonce
}

func newChalEpoch() *types.ChalEpoch {
	return &types.ChalEpoch{
		Epoch:  0,
		Height: 0,
		Seed:   types.NewMsgID([]byte("chalepoch")),
	}
}

type orderKey struct {
	userID uint64
	proID  uint64
}

type orderFull struct {
	types.SignedOrder
	SeqNum uint32
	AccFr  []byte
}

func (of *orderFull) Serialize() ([]byte, error) {
	return cbor.Marshal(of)
}

func (of *orderFull) Deserialize(b []byte) error {
	return cbor.Unmarshal(b, of)
}

type seqFull struct {
	types.OrderSeq
	AccFr []byte
}

func (sf *seqFull) Serialize() ([]byte, error) {
	return cbor.Marshal(sf)
}

func (sf *seqFull) Deserialize(b []byte) error {
	return cbor.Unmarshal(b, sf)
}

type OrderDuration struct {
	Start []int64
	End   []int64
}

func (od *OrderDuration) Add(start, end int64) error {
	if start < end {
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

type orderInfo struct {
	prove uint64 // next prove epoch
	ns    *types.NonceSeq
	accFr bls.Fr
	base  *types.SignedOrder
	od    *OrderDuration
}

type segPerUser struct {
	userID    uint64
	fsID      []byte
	verifyKey pdpcommon.VerifyKey

	// need?
	nextBucket uint64                   // next bucket number
	buckets    map[uint64]*bucketManage //
}

type bucketManage struct {
	chunks  []*types.AggStripe        // lastest stripe of each chunkID
	stripes map[uint64]*bitset.BitSet // key: proID; val: avali stripe
}

type bitsetStored struct {
	Val []uint64
}

func (bs *bitsetStored) Serialize() ([]byte, error) {
	return cbor.Marshal(bs)
}

func (bs *bitsetStored) Deserialize(b []byte) error {
	return cbor.Unmarshal(b, bs)
}
