package order

import (
	"errors"

	"github.com/bits-and-blooms/bitset"
	"github.com/fxamacker/cbor/v2"
	logging "github.com/memoio/go-mefs-v2/lib/log"
	"github.com/memoio/go-mefs-v2/lib/types"
)

var logger = logging.Logger("uorder")

var (
	ErrDataAdded = errors.New("add data fails")
	ErrState     = errors.New("state is wrong")
	ErrDataSign  = errors.New("sign is wrong")
	ErrEmpty     = errors.New("data is empty")
	ErrNotFound  = errors.New("not found")
	ErrPrice     = errors.New("price is not right")
)

const (
	DefaultOrderDuration = 8640000 // 100 days
	DefaultAckWaiting    = 30
	DefaultOrderLast     = 600 // 1 day
	DefaultOrderSeqLast  = 300 // 1 hour
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

func (sj *segJobState) Serialize() ([]byte, error) {
	sjs := &segJobStateStored{
		Dispatch: sj.dispatchBits.Bytes(),
		Done:     sj.doneBits.Bytes(),
	}

	return cbor.Marshal(sjs)
}

func (sj *segJobState) Deserialize(b []byte) error {
	sjs := new(segJobStateStored)

	err := cbor.Unmarshal(b, sjs)
	if err != nil {
		return err
	}

	sj.dispatchBits = bitset.From(sjs.Dispatch)
	sj.doneBits = bitset.From(sjs.Done)

	return nil
}

type segJobStateStored struct {
	Dispatch []uint64
	Done     []uint64
}
