package node

import (
	"log"
	"net/http"

	"contrib.go.opencensus.io/exporter/prometheus"
	promclient "github.com/prometheus/client_golang/prometheus"
)

func exporter() http.Handler {
	registry, ok := promclient.DefaultRegisterer.(*promclient.Registry)
	if !ok {
		log.Printf("failed to export default prometheus registry; some metrics will be unavailable; unexpected type: %T", promclient.DefaultRegisterer)
	}
	exporter, err := prometheus.NewExporter(prometheus.Options{
		Registry:  registry,
		Namespace: "memo",
	})
	if err != nil {
		log.Printf("could not create the prometheus stats exporter: %v", err)
	}

	return exporter
}
