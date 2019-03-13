package metrics

import (
	"net/http"
	"time"

	ma "gx/ipfs/QmNTCey11oxhb1AxDnQBRHtdhap6Ctud872NjAYPYYXPuc/go-multiaddr"
	manet "gx/ipfs/QmZcLBXKaFe8ND5YHPkJRAwmhJGrVsi1JqDZNyJ4nRK5Mj/go-multiaddr-net"

	prom "github.com/prometheus/client_golang/prometheus"
	"go.opencensus.io/exporter/prometheus"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/zpages"

	"github.com/filecoin-project/go-filecoin/config"
)

// SetupMetrics registers and serves prometheus metrics
func SetupMetrics(cfg *config.MetricsConfig) error {
	if !cfg.Enabled {
		return nil
	}

	// validate config values and marshal to types
	interval, err := time.ParseDuration(cfg.ReportInterval)
	if err != nil {
		log.Errorf("invalid metrics interval: %s", err)
		return err
	}

	promma, err := ma.NewMultiaddr(cfg.PrometheusEndpoint)
	if err != nil {
		return err
	}

	_, promAddr, err := manet.DialArgs(promma)
	if err != nil {
		return err
	}

	// setup prometheus
	registry := prom.NewRegistry()
	pe, err := prometheus.NewExporter(prometheus.Options{
		Namespace: "filecoin",
		Registry:  registry,
	})
	if err != nil {
		return err
	}

	view.RegisterExporter(pe)
	view.SetReportingPeriod(interval)
	if err := view.Register(processBlockView); err != nil {
		return err
	}

	go func() {
		mux := http.NewServeMux()
		zpages.Handle(mux, "/debug")
		mux.Handle("/metrics", pe)
		if err := http.ListenAndServe(promAddr, mux); err != nil {
			log.Errorf("failed to serve /metrics endpoint on %v", err)
		}
	}()

	return nil
}
