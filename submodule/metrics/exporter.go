package metrics

import (
	"net/http"
	_ "net/http/pprof"

	"contrib.go.opencensus.io/exporter/prometheus"
	promclient "github.com/prometheus/client_golang/prometheus"

	logging "github.com/memoio/go-mefs-v2/lib/log"
)

var logger = logging.Logger("metrics")

func Exporter() http.Handler {
	registry, ok := promclient.DefaultRegisterer.(*promclient.Registry)
	if !ok {
		logger.Warn("failed to export default prometheus registry; some metrics will be unavailable; unexpected type: %T", promclient.DefaultRegisterer)
	}
	exporter, err := prometheus.NewExporter(prometheus.Options{
		Registry:  registry,
		Namespace: "mefs",
	})
	if err != nil {
		logger.Warnf("could not create the prometheus stats exporter: %v", err)
	}

	return exporter
}

func NewExporter() http.Handler {
	registry := promclient.NewRegistry()
	exporter, err := prometheus.NewExporter(prometheus.Options{
		Registry:  registry,
		Namespace: "mefs",
	})
	if err != nil {
		logger.Warnf("could not create the prometheus stats exporter: %v", err)
	}

	return exporter
}
