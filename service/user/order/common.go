package order

import (
	"github.com/bits-and-blooms/bitset"
	"github.com/fxamacker/cbor/v2"
	logging "github.com/memoio/go-mefs-v2/lib/log"
	"github.com/memoio/go-mefs-v2/lib/types"
)

var logger = logging.Logger("user-order")

const (
	defaultAckWaiting = 35

	// parallel number of net send
	defaultWeighted = 50
	minFailCnt      = 100
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
	confirmBits  *bitset.BitSet
}

type segJobStateStored struct {
	types.SegJob
	Dispatch []uint64
	Done     []uint64
	Confirm  []uint64
}

func (sj *segJob) Serialize() ([]byte, error) {
	sjss := &segJobStateStored{
		SegJob:   sj.SegJob,
		Dispatch: sj.dispatchBits.Bytes(),
		Done:     sj.doneBits.Bytes(),
		Confirm:  sj.confirmBits.Bytes(),
	}

	return cbor.Marshal(sjss)
}

func (sj *segJob) Deserialize(b []byte) error {
	sjss := new(segJobStateStored)

	err := cbor.Unmarshal(b, sjss)
	if err != nil {
		return err
	}

	sj.SegJob = sjss.SegJob
	sj.dispatchBits = bitset.From(sjss.Dispatch)
	sj.doneBits = bitset.From(sjss.Done)
	sj.confirmBits = bitset.From(sjss.Confirm)

	return nil
}
