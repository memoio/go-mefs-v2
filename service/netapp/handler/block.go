package handler

import (
	"context"
	"errors"
	"log"
	"sync"

	"github.com/memoio/go-mefs-v2/lib/tx"
)

var (
	ErrNoHandle = errors.New("no handle")
)

type HandlerBlockFunc func(context.Context, *tx.Block) error

// TxMsgHandle is used for handle received msg from pubsub
type BlockHandle interface {
	Handle(context.Context, *tx.Block) error
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

func (i *BlockImpl) Handle(ctx context.Context, mes *tx.Block) error {
	i.RLock()
	defer i.RUnlock()

	if i.close {
		return nil
	}

	if i.handler == nil {
		return ErrNoHandle
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

func defaultBlockHandler(ctx context.Context, msg *tx.Block) error {
	log.Println("received block:", msg.Height)
	return nil
}
