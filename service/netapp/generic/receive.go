package generic

import (
	"io"
	"time"

	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-msgio"
	"go.opencensus.io/stats"
	"go.opencensus.io/tag"

	"github.com/memoio/go-mefs-v2/lib/pb"
	"github.com/memoio/go-mefs-v2/service/netapp/generic/internal"
	"github.com/memoio/go-mefs-v2/submodule/metrics"
)

var dhtStreamIdleTimeout = 1 * time.Minute

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
		err = internal.WriteMsg(s, resp)
		if err != nil {
			stats.Record(ctx, metrics.NetReceivedMessageErrors.M(1))
			return false
		}

		elapsedTime := time.Since(startTime).Milliseconds()
		latencyMillis := float64(elapsedTime) / float64(time.Millisecond)
		stats.Record(ctx, metrics.NetInboundRequestLatency.M(latencyMillis))
	}
}
