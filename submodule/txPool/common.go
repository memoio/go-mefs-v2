package txPool

import (
	"errors"

	logging "github.com/memoio/go-mefs-v2/lib/log"
	"github.com/memoio/go-mefs-v2/lib/tx"
)

var logger = logging.Logger("txPool")

var (
	ErrNotReady    = errors.New("service not ready")
	ErrInvalidSign = errors.New("invalid sign")
	ErrLowHeight   = errors.New("height is low")
	ErrLowNonce    = errors.New("nonce is low")
)

type HandlerMessageFunc func(*tx.Message) error
type ValidateMessageFunc func(*tx.Message) error
