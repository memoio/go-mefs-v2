package state

import (
	"math/big"

	"github.com/bits-and-blooms/bitset"
	"github.com/fxamacker/cbor/v2"
	"golang.org/x/xerrors"

	bls "github.com/memoio/go-mefs-v2/lib/crypto/bls12_381"
	pdpcommon "github.com/memoio/go-mefs-v2/lib/crypto/pdp/common"
	"github.com/memoio/go-mefs-v2/lib/crypto/signature"
	logging "github.com/memoio/go-mefs-v2/lib/log"
	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
)

var logger = logging.Logger("data-state")

type HanderAddRoleFunc func(roleID uint64, typ pb.RoleInfo_Type)
type HandleAddUserFunc func(userID uint64)
type HandleAddUPFunc func(userID, proID uint64)
type HandleAddPayFunc func(userID, proID, epoch uint64, pay, penaly *big.Int)
type HandleAddSeqFunc func(types.OrderSeq)
type HandleDelSegFunc func(*tx.SegRemoveParas)
type HandleCommitOrderFunc func(types.SignedOrder)

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

func newChalEpoch(seed types.MsgID) *types.ChalEpoch {
	return &types.ChalEpoch{
		Epoch: 0,
		Slot:  0,
		Seed:  seed,
	}
}

type orderKey struct {
	userID uint64
	proID  uint64
}

type orderInfo struct {
	prove    uint64 // next prove epoch
	income   *types.PostIncome
	ns       *types.NonceSeq
	accFr    bls.Fr // for base
	base     *types.SignedOrder
	preStart int64
	preEnd   int64
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

func verify(ri *pb.RoleInfo, msg []byte, sig types.Signature) error {
	switch sig.Type {
	case types.SigSecp256k1:
		ok, err := signature.Verify(ri.ChainVerifyKey, msg, sig.Data)
		if err != nil {
			return err
		}
		if !ok {
			return xerrors.Errorf("%d sign %d is wrong", ri.RoleID, sig.Type)
		}
	case types.SigBLS:
		ok, err := signature.Verify(ri.BlsVerifyKey, msg, sig.Data)
		if err != nil {
			return err
		}
		if !ok {
			return xerrors.Errorf("%d sign %d is wrong", ri.RoleID, sig.Type)
		}
	default:
		return xerrors.Errorf("sign type %d is not supported", sig.Type)
	}
	return nil
}
