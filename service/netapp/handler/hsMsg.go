package handler

import (
	"context"
	"sync"

	hs "github.com/memoio/go-mefs-v2/lib/hotstuff"
	"golang.org/x/xerrors"
)

type HandlerHsMsgFunc func(context.Context, *hs.HotstuffMessage) error

// HsMsgHandle is used for handle received msg from pubsub
type HsMsgHandle interface {
	Handle(context.Context, *hs.HotstuffMessage) error
	Register(h HandlerHsMsgFunc)
	Close()
}

var _ HsMsgHandle = (*HsMsgImpl)(nil)

type HsMsgImpl struct {
	sync.RWMutex
	handler HandlerHsMsgFunc
	close   bool
}

func NewHsMsgHandle() *HsMsgImpl {
	i := &HsMsgImpl{
		handler: defaultHsMsgHandler,
		close:   false,
	}
	return i
}

func (hmi *HsMsgImpl) Handle(ctx context.Context, mes *hs.HotstuffMessage) error {
	hmi.RLock()
	defer hmi.RUnlock()

	if hmi.close {
		return nil
	}

	if hmi.handler == nil {
		return xerrors.New("no hotstuff message handle")
	}
	return hmi.handler(ctx, mes)
}

func (hmi *HsMsgImpl) Register(h HandlerHsMsgFunc) {
	hmi.Lock()
	defer hmi.Unlock()
	hmi.handler = h
}

func (hmi *HsMsgImpl) Close() {
	hmi.Lock()
	defer hmi.Unlock()
	hmi.close = true
}

func defaultHsMsgHandler(ctx context.Context, msg *hs.HotstuffMessage) error {
	return nil
}
