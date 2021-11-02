package instance

import (
	"context"
	"errors"
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

type HandlerFunc func(context.Context, peer.ID, *pb.NetMessage) (*pb.NetMessage, error)

// Subscriber is used fo callback on receiving msg from net
type Subscriber interface {
	HandlerForMsgType(pb.NetMessage_MsgType) HandlerFunc
	Register(pb.NetMessage_MsgType, HandlerFunc)
	UnRegister(pb.NetMessage_MsgType)
	Close()
}

var _ Subscriber = (*Impl)(nil)

type Impl struct {
	sync.RWMutex
	close bool
	hmap  map[pb.NetMessage_MsgType]HandlerFunc
}

func New() *Impl {
	i := &Impl{
		hmap: make(map[pb.NetMessage_MsgType]HandlerFunc),
	}

	i.Register(pb.NetMessage_SayHello, defaultHandler)
	i.Register(pb.NetMessage_Get, defaultGetHandler)
	return i
}

func (i *Impl) HandlerForMsgType(mt pb.NetMessage_MsgType) HandlerFunc {
	i.RLock()
	defer i.RUnlock()

	if i.close {
		return nil
	}

	h, ok := i.hmap[mt]
	if ok {
		return h
	}
	return nil
}

func (i *Impl) Register(mt pb.NetMessage_MsgType, h HandlerFunc) {
	i.Lock()
	defer i.Unlock()
	i.hmap[mt] = h
}

func (i *Impl) UnRegister(mt pb.NetMessage_MsgType) {
	i.Lock()
	defer i.Unlock()
	delete(i.hmap, mt)
}

func (i *Impl) Close() {
	i.Lock()
	defer i.Unlock()
	i.close = true
}

func defaultHandler(ctx context.Context, p peer.ID, mes *pb.NetMessage) (*pb.NetMessage, error) {
	mes.Data.MsgInfo = []byte("hello")
	return mes, nil
}

func defaultGetHandler(ctx context.Context, p peer.ID, mes *pb.NetMessage) (*pb.NetMessage, error) {
	mes.Data.MsgInfo = []byte("get")
	return mes, nil
}
