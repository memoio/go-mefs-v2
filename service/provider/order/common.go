package order

import (
	"errors"

	logging "github.com/memoio/go-mefs-v2/lib/log"
)

var logger = logging.Logger("porder")

var (
	ErrDataSign = errors.New("wrong data content")
	ErrState    = errors.New("wrong state")
	ErrService  = errors.New("not ready")
)
