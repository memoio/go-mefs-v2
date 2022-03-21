package handler

import (
	"context"
	"sync"

	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/lib/tx"
)

type HandlerBlockFunc func(context.Context, *tx.SignedBlock) error

// BlockHandle is used for handle received tx block from pubsub
type BlockHandle interface {
	Handle(context.Context, *tx.SignedBlock) error
	Register(h HandlerBlockFunc)
	Close()
}

var _ BlockHandle = (*BlockImpl)(nil)

type BlockImpl struct {
	sync.RWMutex
	handler HandlerBlockFunc
	close   bool
}

func NewBlockHandle() *BlockImpl {
	i := &BlockImpl{
		handler: defaultBlockHandler,
		close:   false,
	}
	return i
}

func (i *BlockImpl) Handle(ctx context.Context, mes *tx.SignedBlock) error {
	i.RLock()
	defer i.RUnlock()

	if i.close {
		return nil
	}

	if i.handler == nil {
		return xerrors.New("no block handle")
	}
	return i.handler(ctx, mes)
}

func (i *BlockImpl) Register(h HandlerBlockFunc) {
	i.Lock()
	defer i.Unlock()
	i.handler = h
}

func (i *BlockImpl) Close() {
	i.Lock()
	defer i.Unlock()
	i.close = true
}

func defaultBlockHandler(ctx context.Context, msg *tx.SignedBlock) error {
	return nil
}
