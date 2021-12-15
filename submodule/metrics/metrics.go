package metrics

import (
	"context"
	"time"

	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
)

var defaultMillisecondsDistribution = view.Distribution(0.01, 0.05, 0.1, 0.3, 0.6, 0.8, 1, 2, 3, 4, 5, 6, 8, 10, 13, 16, 20, 25, 30, 40, 50, 65, 80, 100, 130, 160, 200, 250, 300, 400, 500, 650, 800, 1000, 2000, 3000, 4000, 5000, 7500, 10000, 20000, 50000, 100000)

var (
	Version, _   = tag.NewKey("version")
	Commit, _    = tag.NewKey("commit")
	APIMethod, _ = tag.NewKey("api_method")
)

var (
	// common
	MemoInfo           = stats.Int64("info", "Memo info", stats.UnitDimensionless)
	PeerCount          = stats.Int64("peer/count", "Peer Count", stats.UnitDimensionless)
	APIRequestDuration = stats.Float64("api/request_duration_ms", "Duration of API requests", stats.UnitMilliseconds)

	// message
	TxMessagePublished = stats.Int64("message/published", "Counter for total locally published messages", stats.UnitDimensionless)
	TxMessageReceived  = stats.Int64("message/received", "Counter for total received messages", stats.UnitDimensionless)
	TxMessageSuccess   = stats.Int64("message/success", "Counter for message validation successes", stats.UnitDimensionless)
	TxMessageFailure   = stats.Int64("message/failure", "Counter for message validation failures", stats.UnitDimensionless)
	TxMessageApply     = stats.Float64("message/apply_total_ms", "Time spent applying block messages", stats.UnitMilliseconds)

	// block
	TxBlockSyncdHeight  = stats.Int64("block/synced_height", "Syned height of block", stats.UnitDimensionless)
	TxBlockRemoteHeight = stats.Int64("block/remote_height", "Remote height of block", stats.UnitDimensionless)
	TxBlockPublished    = stats.Int64("block/published", "Counter for total locally published blocks", stats.UnitDimensionless)
	TxBlockReceived     = stats.Int64("block/received", "Counter for total received blocks", stats.UnitDimensionless)
	TxBlockSuccess      = stats.Int64("block/success", "Counter for block validation successes", stats.UnitDimensionless)
	TxBlockFailure      = stats.Int64("block/failure", "Counter for block validation failures", stats.UnitDimensionless)
	TxBlockApply        = stats.Float64("block/apply_total_ms", "Time spent applying block", stats.UnitMilliseconds)
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

		TxBlockSyncedHeightView,
		TxBlockRemoteHeightView,
		TxBlockPublishedView,
		TxBlockReceivedView,
		TxBlockSuccessView,
		TxBlockApplyView,
	}
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
