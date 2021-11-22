package handler

import (
	"context"
	"log"
	"sync"

	"github.com/memoio/go-mefs-v2/lib/tx"
)

type HandlerFunc func(context.Context, *tx.SignedMessage) error

// TxMsgHandle is used for handle received msg from pubsub
type TxMsgHandle interface {
	Handle(context.Context, *tx.SignedMessage) error
	Register(HandlerFunc)
	Close()
}

var _ TxMsgHandle = (*Impl)(nil)

type Impl struct {
	sync.RWMutex
	close   bool
	handler HandlerFunc
}

func NewTxMsgHandle() *Impl {
	i := &Impl{
		handler: defaultHandler,
	}

	return i
}

func (i *Impl) Handle(ctx context.Context, mes *tx.SignedMessage) error {
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

func (i *Impl) Register(h HandlerFunc) {
	i.Lock()
	defer i.Unlock()
	i.handler = h
}

func (i *Impl) Close() {
	i.Lock()
	defer i.Unlock()
	i.close = true
}

func defaultHandler(ctx context.Context, msg *tx.SignedMessage) error {
	log.Println("received tx msg:", msg.From)
	return nil
}
