package handler

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/memoio/go-mefs-v2/lib/tx"
)

type HandlerFunc func(context.Context, *tx.SignedMessage) error

// TxMsgHandle is used for handle received msg from pubsub
type TxMsgHandle interface {
	Handle(context.Context, *tx.SignedMessage) error
	Register(tx.MsgType, HandlerFunc)
	UnRegister(tx.MsgType)
	Close()
}

var _ TxMsgHandle = (*Impl)(nil)

type Impl struct {
	sync.RWMutex
	close bool
	hmap  map[tx.MsgType]HandlerFunc
}

func NewTxMsgHandle() *Impl {
	i := &Impl{
		hmap: make(map[tx.MsgType]HandlerFunc),
	}

	i.Register(tx.DataTxErr, defaultHandler)
	return i
}

func (i *Impl) Handle(ctx context.Context, mes *tx.SignedMessage) error {
	i.RLock()
	defer i.RUnlock()

	if i.close {
		return nil
	}

	h, ok := i.hmap[mes.Method]
	if ok {
		log.Println("handle tx mes")
		return h(ctx, mes)
	}
	return nil
}

func (i *Impl) Register(mt tx.MsgType, h HandlerFunc) {
	i.Lock()
	defer i.Unlock()
	i.hmap[mt] = h
}

func (i *Impl) UnRegister(mt tx.MsgType) {
	i.Lock()
	defer i.Unlock()
	delete(i.hmap, mt)
}

func (i *Impl) Close() {
	i.Lock()
	defer i.Unlock()
	i.close = true
}

func defaultHandler(ctx context.Context, msg *tx.SignedMessage) error {
	fmt.Println("received msg:", msg.From)
	return nil
}
