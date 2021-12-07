package handler

import (
	"context"
	"sync"

	"github.com/memoio/go-mefs-v2/lib/pb"
)

type EventHandlerFunc func(context.Context, *pb.EventMessage) error

// EventHandle is used for handle received event msg from pubsub
type EventHandle interface {
	Handle(context.Context, *pb.EventMessage) error
	Register(pb.EventMessage_Type, EventHandlerFunc)
	UnRegister(pb.EventMessage_Type)
	Close()
}

var _ EventHandle = (*EventImpl)(nil)

type EventImpl struct {
	sync.RWMutex
	close bool
	hmap  map[pb.EventMessage_Type]EventHandlerFunc
}

func NewEventHandle() *EventImpl {
	i := &EventImpl{
		hmap: make(map[pb.EventMessage_Type]EventHandlerFunc),
	}

	i.Register(pb.EventMessage_Unknown, defaultEventHandler)
	return i
}

func (ei *EventImpl) Handle(ctx context.Context, mes *pb.EventMessage) error {
	ei.RLock()
	defer ei.RUnlock()

	if ei.close {
		return nil
	}

	h, ok := ei.hmap[mes.GetType()]
	if ok {
		return h(ctx, mes)
	}
	return nil
}

func (ei *EventImpl) Register(mt pb.EventMessage_Type, h EventHandlerFunc) {
	ei.Lock()
	defer ei.Unlock()
	ei.hmap[mt] = h
}

func (ei *EventImpl) UnRegister(mt pb.EventMessage_Type) {
	ei.Lock()
	defer ei.Unlock()
	delete(ei.hmap, mt)
}

func (i *EventImpl) Close() {
	i.Lock()
	defer i.Unlock()
	i.close = true
}

func defaultEventHandler(ctx context.Context, msg *pb.EventMessage) error {
	return nil
}
