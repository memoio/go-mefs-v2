package order

import (
	"golang.org/x/xerrors"

	logging "github.com/memoio/go-mefs-v2/lib/log"
)

var logger = logging.Logger("porder")

var (
	ErrDataSign = xerrors.New("wrong data content")
	ErrState    = xerrors.New("wrong state")
	ErrService  = xerrors.New("order service not ready")
)
