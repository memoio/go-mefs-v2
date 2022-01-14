package metrics

import (
	"context"
	"time"

	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"

	"github.com/libp2p/go-libp2p-kad-dht/metrics"
)

var (
	txBytesDistribution             = view.Distribution(64, 128, 256, 512, 1024, 2048, 4096, 16384, 65536, 262144, 1048576, 4194304)
	defaultBytesDistribution        = view.Distribution(1024, 2048, 4096, 16384, 65536, 262144, 1048576, 4194304, 16777216, 67108864, 268435456, 1073741824, 4294967296)
	defaultMillisecondsDistribution = view.Distribution(0.01, 0.05, 0.1, 0.3, 0.6, 0.8, 1, 2, 3, 4, 5, 6, 8, 10, 13, 16, 20, 25, 30, 40, 50, 65, 80, 100, 130, 160, 200, 250, 300, 400, 500, 650, 800, 1000, 2000, 3000, 4000, 5000, 7500, 10000, 20000, 50000, 100000)
)

var (
	Version, _   = tag.NewKey("version")
	Commit, _    = tag.NewKey("commit")
	APIMethod, _ = tag.NewKey("api_method")

	NetMessageType, _ = tag.NewKey("message_type")
	NetPeerID, _      = tag.NewKey("peer_id")
)

var (
	// common
	MemoInfo           = stats.Int64("info", "Memo info", stats.UnitDimensionless)
	PeerCount          = stats.Int64("peer/count", "Peer Count", stats.UnitDimensionless)
	APIRequestDuration = stats.Float64("api/request_duration_ms", "Duration of API requests", stats.UnitMilliseconds)

	// message
	TxMessagePublished    = stats.Int64("message/published", "Counter for total locally published messages", stats.UnitDimensionless)
	TxMessageReceived     = stats.Int64("message/received", "Counter for total received messages", stats.UnitDimensionless)
	TxMessageSuccess      = stats.Int64("message/success", "Counter for message received validation successes", stats.UnitDimensionless)
	TxMessageFailure      = stats.Int64("message/failure", "Counter for message received validation failures", stats.UnitDimensionless)
	TxMessageApply        = stats.Float64("message/apply_total_ms", "Time spent applying block messages", stats.UnitMilliseconds)
	TxMessageApplySuccess = stats.Int64("message/apply_success", "Counter for message applied validation successes", stats.UnitDimensionless)
	TxMessageApplyFailure = stats.Int64("message/apply_failure", "Counter for message applied validation failures", stats.UnitDimensionless)
	TxMessageBytes        = stats.Int64("message/bytes", "Bytes per message", stats.UnitBytes)

	// block
	TxBlockSyncdHeight    = stats.Int64("block/synced_height", "Syned height of block", stats.UnitDimensionless)
	TxBlockRemoteHeight   = stats.Int64("block/remote_height", "Remote height of block", stats.UnitDimensionless)
	TxBlockPublished      = stats.Int64("block/published", "Counter for total locally published blocks", stats.UnitDimensionless)
	TxBlockReceived       = stats.Int64("block/received", "Counter for total received blocks", stats.UnitDimensionless)
	TxBlockSuccess        = stats.Int64("block/success", "Counter for block validation successes", stats.UnitDimensionless)
	TxBlockFailure        = stats.Int64("block/failure", "Counter for block validation failures", stats.UnitDimensionless)
	TxBlockApply          = stats.Float64("block/apply_total_ms", "Time spent applying block", stats.UnitMilliseconds)
	TxBlockCreateExpected = stats.Int64("block/create_expected", "Counter for block create expected", stats.UnitDimensionless)
	TxBlockCreateSuccess  = stats.Int64("block/create_success", "Counter for block create success", stats.UnitDimensionless)
	TxBlockBytes          = stats.Int64("block/bytes", "Bytes per block", stats.UnitBytes)

	NetReceivedMessages       = stats.Int64("net/received_messages", "Total number of messages received per RPC", stats.UnitDimensionless)
	NetReceivedMessageErrors  = stats.Int64("net/received_message_errors", "Total number of errors for messages received per RPC", stats.UnitDimensionless)
	NetReceivedBytes          = stats.Int64("net/received_bytes", "Total received bytes per RPC", stats.UnitBytes)
	NetInboundRequestLatency  = stats.Float64("net/inbound_request_latency", "Latency per RPC", stats.UnitMilliseconds)
	NetOutboundRequestLatency = stats.Float64("net/outbound_request_latency", "Latency per RPC", stats.UnitMilliseconds)
	NetSentRequests           = stats.Int64("net/sent_requests", "Total number of requests sent per RPC", stats.UnitDimensionless)
	NetSentRequestErrors      = stats.Int64("net/sent_request_errors", "Total number of errors for requests sent per RPC", stats.UnitDimensionless)
	NetSentBytes              = stats.Int64("net/sent_bytes", "Total sent bytes per RPC", stats.UnitBytes)
)

var (
	InfoView = &view.View{
		Name:        "info",
		Description: "mefs information",
		Measure:     MemoInfo,
		Aggregation: view.LastValue(),
		TagKeys:     []tag.Key{Version, Commit},
	}
	APIRequestDurationView = &view.View{
		Measure:     APIRequestDuration,
		Aggregation: defaultMillisecondsDistribution,
		TagKeys:     []tag.Key{APIMethod},
	}
	PeerCountView = &view.View{
		Measure:     PeerCount,
		Aggregation: view.LastValue(),
	}

	TxMessagePublishedView = &view.View{
		Measure:     TxMessagePublished,
		Aggregation: view.Count(),
	}
	TxMessageReceivedView = &view.View{
		Measure:     TxMessageReceived,
		Aggregation: view.Count(),
	}
	TxMessageSuccessView = &view.View{
		Measure:     TxMessageSuccess,
		Aggregation: view.Count(),
	}
	TxMessageApplyView = &view.View{
		Measure:     TxMessageApply,
		Aggregation: defaultMillisecondsDistribution,
	}
	TxMessageApplySuccessView = &view.View{
		Measure:     TxMessageApplySuccess,
		Aggregation: view.Count(),
	}
	TxMessageApplyFailureView = &view.View{
		Measure:     TxMessageApplyFailure,
		Aggregation: view.Count(),
	}
	TxMessageBytesView = &view.View{
		Measure:     TxMessageBytes,
		Aggregation: txBytesDistribution,
	}

	TxBlockSyncedHeightView = &view.View{
		Measure:     TxBlockSyncdHeight,
		Aggregation: view.LastValue(),
	}
	TxBlockRemoteHeightView = &view.View{
		Measure:     TxBlockRemoteHeight,
		Aggregation: view.LastValue(),
	}
	TxBlockReceivedView = &view.View{
		Measure:     TxBlockReceived,
		Aggregation: view.Count(),
	}
	TxBlockPublishedView = &view.View{
		Measure:     TxBlockPublished,
		Aggregation: view.Count(),
	}
	TxBlockSuccessView = &view.View{
		Measure:     TxBlockSuccess,
		Aggregation: view.Count(),
	}
	TxBlockApplyView = &view.View{
		Measure:     TxBlockApply,
		Aggregation: defaultMillisecondsDistribution,
	}
	TxBlockCreateExpectedView = &view.View{
		Measure:     TxBlockCreateExpected,
		Aggregation: view.Count(),
	}
	TxBlockCreateSuccessView = &view.View{
		Measure:     TxBlockCreateSuccess,
		Aggregation: view.Count(),
	}
	TxBlockBytesView = &view.View{
		Measure:     TxBlockBytes,
		Aggregation: txBytesDistribution,
	}

	NetReceivedMessagesView = &view.View{
		Measure:     NetReceivedMessages,
		TagKeys:     []tag.Key{NetMessageType, NetPeerID},
		Aggregation: view.Count(),
	}
	NetReceivedMessageErrorsView = &view.View{
		Measure:     NetReceivedMessageErrors,
		TagKeys:     []tag.Key{NetMessageType, NetPeerID},
		Aggregation: view.Count(),
	}
	NetReceivedBytesView = &view.View{
		Measure:     NetReceivedBytes,
		TagKeys:     []tag.Key{NetMessageType, NetPeerID},
		Aggregation: defaultBytesDistribution,
	}
	NetInboundRequestLatencyView = &view.View{
		Measure:     NetInboundRequestLatency,
		TagKeys:     []tag.Key{NetMessageType, NetPeerID},
		Aggregation: defaultMillisecondsDistribution,
	}
	NetOutboundRequestLatencyView = &view.View{
		Measure:     NetOutboundRequestLatency,
		TagKeys:     []tag.Key{NetMessageType, NetPeerID},
		Aggregation: defaultMillisecondsDistribution,
	}
	NetSentRequestsView = &view.View{
		Measure:     NetSentRequests,
		TagKeys:     []tag.Key{NetMessageType, NetPeerID},
		Aggregation: view.Count(),
	}
	NetSentRequestErrorsView = &view.View{
		Measure:     NetSentRequestErrors,
		TagKeys:     []tag.Key{NetMessageType, NetPeerID},
		Aggregation: view.Count(),
	}
	NetSentBytesView = &view.View{
		Measure:     NetSentBytes,
		TagKeys:     []tag.Key{NetMessageType, NetPeerID},
		Aggregation: defaultBytesDistribution,
	}
)

var DefaultViews = func() []*view.View {
	views := []*view.View{
		InfoView,
		PeerCountView,
		APIRequestDurationView,

		TxMessagePublishedView,
		TxMessageReceivedView,
		TxMessageSuccessView,
		TxMessageApplyView,
		TxMessageApplySuccessView,
		TxMessageApplyFailureView,
		TxMessageBytesView,

		TxBlockSyncedHeightView,
		TxBlockRemoteHeightView,
		TxBlockPublishedView,
		TxBlockReceivedView,
		TxBlockSuccessView,
		TxBlockApplyView,
		TxBlockCreateExpectedView,
		TxBlockCreateSuccessView,
		TxBlockBytesView,

		NetReceivedMessagesView,
		NetReceivedMessageErrorsView,
		NetReceivedBytesView,
		NetInboundRequestLatencyView,
		NetOutboundRequestLatencyView,
		NetSentRequestsView,
		NetSentRequestErrorsView,
		NetSentBytesView,
	}

	views = append(views, metrics.DefaultViews...)
	return views
}()

func SinceInMilliseconds(startTime time.Time) float64 {
	return float64(time.Since(startTime).Nanoseconds()) / 1e6
}

func Timer(ctx context.Context, m *stats.Float64Measure) func() {
	start := time.Now()
	return func() {
		stats.Record(ctx, m.M(SinceInMilliseconds(start)))
	}
}
