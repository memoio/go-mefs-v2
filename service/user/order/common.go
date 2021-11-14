package order

import (
	"errors"

	"github.com/bits-and-blooms/bitset"
	logging "github.com/memoio/go-mefs-v2/lib/log"
	"github.com/memoio/go-mefs-v2/lib/types"
)

var logger = logging.Logger("user")

var (
	ErrDataAdded = errors.New("add data fails")
	ErrState     = errors.New("state is wrong")
	ErrNotFound  = errors.New("not found")
	ErrPrice     = errors.New("price is not right")
)

const (
	DefaultOrderDuration = 8640000 // 10 days
	DefaultAckWaiting    = 300
	DefaultOrderLast     = 86400 // 1 day
	DefaultOrderSeqLast  = 3600  // 1 hour
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
	dispatch *bitset.BitSet
	done     *bitset.BitSet
}

type segJobState struct {
	Sj       types.SegJob
	Dispatch []uint64
	Done     []uint64
}
