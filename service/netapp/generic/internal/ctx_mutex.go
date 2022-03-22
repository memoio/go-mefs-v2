package internal

import (
	"bufio"
	"context"
	"io"
	"sync"

	"github.com/libp2p/go-msgio/protoio"
	"github.com/memoio/go-mefs-v2/lib/pb"
)

type CtxMutex chan struct{}

func NewCtxMutex() CtxMutex {
	return make(CtxMutex, 1)
}

func (m CtxMutex) Lock(ctx context.Context) error {
	select {
	case m <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m CtxMutex) Unlock() {
	select {
	case <-m:
	default:
		panic("not locked")
	}
}

// The Protobuf writer performs multiple small writes when writing a message.
// We need to buffer those writes, to make sure that we're not sending a new
// packet for every single write.
type bufferedDelimitedWriter struct {
	*bufio.Writer
	protoio.WriteCloser
}

var writerPool = sync.Pool{
	New: func() interface{} {
		w := bufio.NewWriter(nil)
		return &bufferedDelimitedWriter{
			Writer:      w,
			WriteCloser: protoio.NewDelimitedWriter(w),
		}
	},
}

func WriteMsg(w io.Writer, mes *pb.NetMessage) error {
	bw := writerPool.Get().(*bufferedDelimitedWriter)
	bw.Reset(w)
	err := bw.WriteMsg(mes)
	if err == nil {
		err = bw.Flush()
	}
	bw.Reset(nil)
	writerPool.Put(bw)
	return err
}

func (w *bufferedDelimitedWriter) Flush() error {
	return w.Writer.Flush()
}
