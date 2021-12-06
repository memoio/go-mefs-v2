package state

import (
	"math/big"

	"github.com/bits-and-blooms/bitset"
	"github.com/fxamacker/cbor/v2"

	bls "github.com/memoio/go-mefs-v2/lib/crypto/bls12_381"
	pdpcommon "github.com/memoio/go-mefs-v2/lib/crypto/pdp/common"
	logging "github.com/memoio/go-mefs-v2/lib/log"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/types"
)

var logger = logging.Logger("state")

var (
	beginRoot = types.NewMsgID([]byte("state"))
)

type HanderAddRoleFunc func(roleID uint64, typ pb.RoleInfo_Type)
type HandleAddUserFunc func(userID uint64)
type HandleAddUPFunc func(userID, proID uint64)
type HandleAddPayFunc func(userID, proID, epoch uint64, pay, penaly *big.Int)

// todo: add msg fee here
type roleInfo struct {
	base *pb.RoleInfo
	val  *roleValue
}

type roleValue struct {
	Nonce uint64 // msg nonce
	Value *big.Int
}

func (rv *roleValue) Serialize() ([]byte, error) {
	return cbor.Marshal(rv)
}

func (rv *roleValue) Deserialize(b []byte) error {
	return cbor.Unmarshal(b, rv)
}

type chalEpochInfo struct {
	epoch    uint64
	current  *types.ChalEpoch
	previous *types.ChalEpoch
}

func newChalEpoch() *types.ChalEpoch {
	return &types.ChalEpoch{
		Epoch: 0,
		Slot:  0,
		Seed:  types.NewMsgID([]byte("chalepoch")),
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

type orderInfo struct {
	prove  uint64 // next prove epoch
	income *types.PostIncome
	ns     *types.NonceSeq
	accFr  bls.Fr
	base   *types.SignedOrder
	od     *types.OrderDuration
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
