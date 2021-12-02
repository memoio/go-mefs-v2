package txPool

import (
	"golang.org/x/xerrors"

	logging "github.com/memoio/go-mefs-v2/lib/log"
	"github.com/memoio/go-mefs-v2/lib/tx"
	"github.com/memoio/go-mefs-v2/lib/types"
)

var logger = logging.Logger("txPool")

var (
	ErrNotReady    = xerrors.New("service not ready")
	ErrInvalidSign = xerrors.New("invalid sign")
	ErrLowHeight   = xerrors.New("height is low")
	ErrLowNonce    = xerrors.New("nonce is low")
)

type HandlerMessageFunc func(*tx.Message, *tx.Receipt) (types.MsgID, error)
type HandlerBlockFunc func(*tx.Block) (types.MsgID, error)

type ValidateMessageFunc func(*tx.Message) (types.MsgID, error)
type ValidateBlockFunc func(*tx.Block) (types.MsgID, error)
