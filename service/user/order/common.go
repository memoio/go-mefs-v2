package order

import (
	"errors"

	"github.com/bits-and-blooms/bitset"
	logging "github.com/memoio/go-mefs-v2/lib/log"
	"github.com/memoio/go-mefs-v2/lib/types"
)

var logger = logging.Logger("uorder")

var (
	ErrDataAdded = errors.New("add data fails")
	ErrState     = errors.New("state is wrong")
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
	os    *types.OrderSeq
}

type jobKey struct {
	bucketID uint64
	jobID    uint64
}

type segJob struct {
	types.SegJob
	dispatchBits *bitset.BitSet
	doneBits     *bitset.BitSet
}

type segJobState struct {
	Sj       types.SegJob
	Dispatch []uint64
	Done     []uint64
}
