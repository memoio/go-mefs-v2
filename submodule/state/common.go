package state

import (
	"errors"

	"github.com/bits-and-blooms/bitset"
	"github.com/fxamacker/cbor/v2"

	bls "github.com/memoio/go-mefs-v2/lib/crypto/bls12_381"
	pdpcommon "github.com/memoio/go-mefs-v2/lib/crypto/pdp/common"
	logging "github.com/memoio/go-mefs-v2/lib/log"
	"github.com/memoio/go-mefs-v2/lib/types"
)

var logger = logging.Logger("txPool")

const (
	StatePrefix = "state"
)

var (
	ErrRes       = errors.New("erros result")
	ErrNonce     = errors.New("nonce is wrong")
	ErrSeq       = errors.New("seq is wrong")
	ErrBucket    = errors.New("bucket is wrong")
	ErrChunk     = errors.New("chunk is wrong")
	ErrDuplicate = errors.New("chunk is duplicate")
)

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

type segPerUser struct {
	fsID      []byte
	root      []byte // merkel root
	verifyKey pdpcommon.VerifyKey

	nextBucket uint64                   // next bucket number
	buckets    map[uint64]*bucketManage //

	lastChallenge uint64
}

// A=24+16+100*B
// BucketManage manage each bucket
type bucketManage struct {
	chunks []*chunkManage // each chunkID

	accHw map[uint64]*chalManage // key: proID;

	unavail *bitset.BitSet // banned stripes or expire
}

type chalManage struct {
	size  uint64
	avail *bitset.BitSet
	accFr bls.Fr //aggreated hashToFr
}

type chalManageStored struct {
	Size  uint64
	Avail []uint64
	AccFr []byte
}

func (cm *chalManage) Serialize() ([]byte, error) {
	cms := &chalManageStored{
		Size:  cm.size,
		Avail: cm.avail.Bytes(),
		AccFr: bls.FrToBytes(&cm.accFr),
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
