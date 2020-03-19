package metrics

import (
	"context"
	"fmt"

	"contrib.go.opencensus.io/exporter/jaeger"
	"contrib.go.opencensus.io/exporter/prometheus"
	"github.com/google/gops/agent"
	"github.com/gorilla/mux"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/trace"
)

// Config metrics
type Config struct {
	TraceExporterAddr string
}

// Initialize metrics
func Initialize(ctx context.Context, r *mux.Router, cfg *Config) error {
	serviceName := "meli"
	if err := agent.Listen(agent.Options{
		ShutdownCleanup: true,
	}); err != nil {
		return err
	}
	prom, err := prometheus.NewExporter(prometheus.Options{Namespace: serviceName})
	if err != nil {
		return err
	}
	view.RegisterExporter(prom)
	r.Handle("/metrics", prom)
	if cfg.TraceExporterAddr != "" {
		te, err := jaeger.NewExporter(jaeger.Options{
			Endpoint: fmt.Sprintf("http://%s", cfg.TraceExporterAddr),
			Process: jaeger.Process{
				ServiceName: serviceName,
			},
		})
		if err != nil {
			return err
		}
		trace.ApplyConfig(trace.Config{DefaultSampler: trace.AlwaysSample()})
		trace.RegisterExporter(te)
	}
	return nil
}
