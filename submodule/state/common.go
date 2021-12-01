package state

import (
	"math/big"

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

var (
	ErrRes         = xerrors.New("erros result")
	ErrBlockHeight = xerrors.New("block height is wrong")
	ErrEpoch       = xerrors.New("epoch is wrong")
	ErrNonce       = xerrors.New("nonce is wrong")
	ErrSeq         = xerrors.New("seq is wrong")
	ErrBucket      = xerrors.New("bucket is wrong")
	ErrChunk       = xerrors.New("chunk is wrong")
	ErrDuplicate   = xerrors.New("chunk is duplicate")
	ErrSize        = xerrors.New("size is wrong")
	ErrPrice       = xerrors.New("price is wrong")
)

type HandleAddStripeFunc func(userID, bucketID, stripeStart, stripeLength, proID, epoch uint64, chunkID uint32)

type ChalEpoch struct {
	Epoch  uint64
	Height uint64
	Seed   types.MsgID
}

func newChalEpoch() *ChalEpoch {
	return &ChalEpoch{
		Epoch:  0,
		Height: 0,
		Seed:   types.NewMsgID([]byte("chalepoch")),
	}
}

func (ce *ChalEpoch) Serialize() ([]byte, error) {
	return cbor.Marshal(ce)
}

func (ce *ChalEpoch) Deserialize(b []byte) error {
	return cbor.Unmarshal(b, ce)
}

type orderKey struct {
	userID uint64
	proID  uint64
}

// key: x/userID/proID; val:
//
type orderInfo struct {
	ns   *types.NonceSeq
	base *types.SignedOrder
}

type chalResult struct {
	Epoch uint64
}

type segPerUser struct {
	userID    uint64
	fsID      []byte
	verifyKey pdpcommon.VerifyKey

	nextBucket uint64                   // next bucket number
	buckets    map[uint64]*bucketManage //

	chalRes map[uint64]*chalResult
}

// A=24+16+100*B
// BucketManage manage each bucket
type bucketManage struct {
	chunks []*chunkManage // each chunkID

	accHw map[uint64]*chalManage // key: proID;

	unavail *bitset.BitSet // banned stripes or expire
}

type chalManage struct {
	size      uint64
	price     *big.Int
	avail     *bitset.BitSet
	accFr     bls.Fr //aggreated hashToFr
	deletedFr bls.Fr
}

type chalManageStored struct {
	Size      uint64
	Price     *big.Int
	Avail     []uint64
	AccFr     []byte
	DeletedFr []byte
}

func (cm *chalManage) Serialize() ([]byte, error) {
	cms := &chalManageStored{
		Size:      cm.size,
		Price:     cm.price,
		Avail:     cm.avail.Bytes(),
		AccFr:     bls.FrToBytes(&cm.accFr),
		DeletedFr: bls.FrToBytes(&cm.deletedFr),
	}

	return cbor.Marshal(cms)
}

func (cm *chalManage) Deserialize(b []byte) error {
	cms := new(chalManageStored)
	err := cbor.Unmarshal(b, cms)
	if err != nil {
		return err
	}

	err = bls.FrFromBytes(&cm.accFr, cms.AccFr)
	if err != nil {
		return err
	}
	cm.size = cms.Size
	cm.avail = bitset.From(cms.Avail)

	return nil
}

// key: ST_SegLocKey/userID/bucketID/chunkID/stripeID; val: stripe
// key: ST_SegLocKey/userID/bucketID/chunkID; val: lastest stripe
type chunkManage struct {
	stripe *types.AggStripe
}
