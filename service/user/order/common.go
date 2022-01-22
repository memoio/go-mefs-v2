package order

import (
	"github.com/bits-and-blooms/bitset"
	"github.com/fxamacker/cbor/v2"
	logging "github.com/memoio/go-mefs-v2/lib/log"
	"github.com/memoio/go-mefs-v2/lib/types"
)

var logger = logging.Logger("user-order")

const (
	defaultAckWaiting   = 30
	defaultOrderLast    = 800 // 1 day
	defaultOrderSeqLast = 300 // 1 hour

	// parallel number of net send
	defaultWeighted = 50
)

type orderSeqPro struct {
	proID uint64
	os    *types.SignedOrderSeq
}

type jobKey struct {
	bucketID uint64
	jobID    uint64
}

type bucketJob struct {
	jobs []*types.SegJob
}

type segJob struct {
	types.SegJob
	segJobState
}

type segJobState struct {
	dispatchBits *bitset.BitSet
	doneBits     *bitset.BitSet
}

func (sj *segJob) Serialize() ([]byte, error) {
	sjs := &segJobStateStored{
		SegJob:   sj.SegJob,
		Dispatch: sj.dispatchBits.Bytes(),
		Done:     sj.doneBits.Bytes(),
	}

	return cbor.Marshal(sjs)
}

func (sj *segJob) Deserialize(b []byte) error {
	sjs := new(segJobStateStored)

	err := cbor.Unmarshal(b, sjs)
	if err != nil {
		return err
	}

	sj.SegJob = sjs.SegJob
	sj.dispatchBits = bitset.From(sjs.Dispatch)
	sj.doneBits = bitset.From(sjs.Done)

	return nil
}

type segJobStateStored struct {
	types.SegJob
	Dispatch []uint64
	Done     []uint64
}
