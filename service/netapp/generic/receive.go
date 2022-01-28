package generic

import (
	"bufio"
	"io"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-msgio"
	"github.com/libp2p/go-msgio/protoio"
	"go.opencensus.io/stats"
	"go.opencensus.io/tag"
	"golang.org/x/xerrors"

	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/submodule/metrics"
)

var dhtStreamIdleTimeout = 1 * time.Minute

// ErrReadTimeout is an error that occurs when no message is read within the timeout period.
var ErrReadTimeout = xerrors.New("timed out reading response")

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

func writeMsg(w io.Writer, mes *pb.NetMessage) error {
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

// handleNewStream implements the network.StreamHandler
func (service *GenericService) handleNewStream(s network.Stream) {
	if service.handleNewMessage(s) {
		// If we exited without error, close gracefully.
		_ = s.Close()
	} else {
		// otherwise, send an error.
		_ = s.Reset()
	}
}

// Returns true on orderly completion of writes (so we can Close the stream).
func (service *GenericService) handleNewMessage(s network.Stream) bool {
	ctx := service.ctx
	r := msgio.NewVarintReaderSize(s, network.MessageSizeMax)

	mPeer := s.Conn().RemotePeer()

	timer := time.AfterFunc(dhtStreamIdleTimeout, func() { _ = s.Reset() })
	defer timer.Stop()

	for {
		var req pb.NetMessage
		msgbytes, err := r.ReadMsg()
		msgLen := len(msgbytes)
		if err != nil {
			r.ReleaseMsg(msgbytes)
			if err == io.EOF {
				return true
			}

			if msgLen > 0 {
				_ = stats.RecordWithTags(ctx,
					[]tag.Mutator{tag.Upsert(metrics.NetMessageType, "UNKNOWN")},
					metrics.NetReceivedMessages.M(1),
					metrics.NetReceivedMessageErrors.M(1),
					metrics.NetReceivedBytes.M(int64(msgLen)),
				)
			}

			return false
		}
		err = req.Unmarshal(msgbytes)
		r.ReleaseMsg(msgbytes)
		if err != nil {
			_ = stats.RecordWithTags(ctx,
				[]tag.Mutator{tag.Upsert(metrics.NetMessageType, "UNKNOWN")},
				metrics.NetReceivedMessages.M(1),
				metrics.NetReceivedMessageErrors.M(1),
				metrics.NetReceivedBytes.M(int64(msgLen)),
			)

			return false
		}

		timer.Reset(dhtStreamIdleTimeout)

		startTime := time.Now()
		ctx, _ := tag.New(ctx,
			tag.Upsert(metrics.NetMessageType, req.GetHeader().GetType().String()),
		)

		stats.Record(ctx,
			metrics.NetReceivedMessages.M(1),
			metrics.NetReceivedBytes.M(int64(msgLen)),
		)

		resp, err := service.MsgHandle.Handle(ctx, mPeer, &req)
		if err != nil {
			stats.Record(ctx, metrics.NetReceivedMessageErrors.M(1))
			return false
		}

		if resp == nil {
			continue
		}

		// send out response msg
		err = writeMsg(s, resp)
		if err != nil {
			stats.Record(ctx, metrics.NetReceivedMessageErrors.M(1))
			return false
		}

		elapsedTime := time.Since(startTime).Milliseconds()
		latencyMillis := float64(elapsedTime) / float64(time.Millisecond)
		stats.Record(ctx, metrics.NetInboundRequestLatency.M(latencyMillis))
	}
}
