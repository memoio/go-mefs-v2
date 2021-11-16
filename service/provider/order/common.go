package order

import (
	"errors"

	logging "github.com/memoio/go-mefs-v2/lib/log"
	"github.com/memoio/go-mefs-v2/lib/segment"
)

var logger = logging.Logger("porder")

var (
	ErrSign    = errors.New("wrong sign")
	ErrState   = errors.New("wrong state")
	ErrService = errors.New("not ready")
)

type SegReceived struct {
	seg segment.Segment // for verify
	uid uint64
}
