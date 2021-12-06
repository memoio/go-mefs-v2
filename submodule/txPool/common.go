package txPool

import (
	"golang.org/x/xerrors"

	logging "github.com/memoio/go-mefs-v2/lib/log"
)

var logger = logging.Logger("txPool")

var (
	ErrNotReady    = xerrors.New("service not ready")
	ErrInvalidSign = xerrors.New("invalid sign")
	ErrLowHeight   = xerrors.New("height is low")
	ErrLowNonce    = xerrors.New("nonce is low")
)
