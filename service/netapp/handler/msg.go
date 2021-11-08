package handler

import (
	"context"
	"errors"
	"log"
	"sync"

	peer "github.com/libp2p/go-libp2p-core/peer"
	"github.com/memoio/go-mefs-v2/lib/pb"
)

var (
	// ErrMetaHandlerNotAssign 节点没有挂载接口时调用，报这个错
	ErrHandlerNotAssign = errors.New("MetaMessageHandler not assign")
	//ErrMetaHandlerFailed 进行回调函数出错，没有特定错误的时候，报这个错
	ErrHandlerFailed = errors.New("meta Handler err")
)

const (
	// MetaHandlerComplete returns
	MetaHandlerComplete = "complete"
)

type MsgHandlerFunc func(context.Context, peer.ID, *pb.NetMessage) (*pb.NetMessage, error)

// MsgHandler is used fo callback on receiving msg from net
type MsgHandle interface {
	Handle(context.Context, peer.ID, *pb.NetMessage) (*pb.NetMessage, error)
	Register(pb.NetMessage_MsgType, MsgHandlerFunc)
	UnRegister(pb.NetMessage_MsgType)
	Close()
}

var _ MsgHandle = (*MsgImpl)(nil)

type MsgImpl struct {
	sync.RWMutex
	close bool
	hmap  map[pb.NetMessage_MsgType]MsgHandlerFunc
}

func NewMsgHandle() *MsgImpl {
	i := &MsgImpl{
		hmap: make(map[pb.NetMessage_MsgType]MsgHandlerFunc),
	}

	i.Register(pb.NetMessage_SayHello, defaultMsgHandler)
	i.Register(pb.NetMessage_Get, defaultMsgHandler)
	return i
}

func (i *MsgImpl) Handle(ctx context.Context, pid peer.ID, mes *pb.NetMessage) (*pb.NetMessage, error) {
	i.RLock()
	defer i.RUnlock()

	if i.close {
		return nil, nil
	}

	h, ok := i.hmap[mes.GetHeader().GetType()]
	if ok {
		log.Println("handle message")
		return h(ctx, pid, mes)
	}
	return nil, nil
}

func (i *MsgImpl) Register(mt pb.NetMessage_MsgType, h MsgHandlerFunc) {
	i.Lock()
	defer i.Unlock()
	i.hmap[mt] = h
}

func (i *MsgImpl) UnRegister(mt pb.NetMessage_MsgType) {
	i.Lock()
	defer i.Unlock()
	delete(i.hmap, mt)
}

func (i *MsgImpl) Close() {
	i.Lock()
	defer i.Unlock()
	i.close = true
}

func defaultMsgHandler(ctx context.Context, p peer.ID, mes *pb.NetMessage) (*pb.NetMessage, error) {
	log.Println("handle type::", mes.Header.Type)
	mes.Data.MsgInfo = []byte("hello")
	return mes, nil
}
