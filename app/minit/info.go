package minit

import (
	"context"
	"fmt"
	"log"
	"runtime"

	"go.opencensus.io/plugin/runmetrics"
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"

	"github.com/memoio/go-mefs-v2/build"
	"github.com/memoio/go-mefs-v2/submodule/metrics"
)

type Node interface {
	Start(perm bool) error
	Shutdown(context.Context) error
	RunDaemon() error
	Online() bool
}

func PrintVersion() {
	v := build.UserVersion()
	log.Printf("Memoriae version: %s", v)
	log.Printf("System version: %s", runtime.GOARCH+"/"+runtime.GOOS)
	log.Printf("Golang version: %s", runtime.Version())
}

func StartMetrics() {
	err := runmetrics.Enable(runmetrics.RunMetricOptions{
		EnableCPU:    true,
		EnableMemory: true,
	})
	if err != nil {
		fmt.Printf("enabling runtime metrics: %s", err)
	}

	view.Register(metrics.DefaultViews...)

	// record version
	ctx, _ := tag.New(context.Background(),
		tag.Insert(metrics.Version, build.BuildVersion),
		tag.Insert(metrics.Commit, build.CurrentCommit),
	)
	stats.Record(ctx, metrics.MemoInfo.M(1))
}
