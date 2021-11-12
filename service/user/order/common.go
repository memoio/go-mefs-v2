package order

import (
	"errors"

	logging "github.com/memoio/go-mefs-v2/lib/log"
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
