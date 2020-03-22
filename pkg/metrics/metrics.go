package metrics

import (
	"context"
	"fmt"

	"contrib.go.opencensus.io/exporter/jaeger"
	"contrib.go.opencensus.io/exporter/prometheus"
	"github.com/google/gops/agent"
	"github.com/gorilla/mux"
	"go.opencensus.io/stats"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
	"go.opencensus.io/trace"
)

var (
	ShortKey      = tag.MustNewKey("meli/short")
	FullKey       = tag.MustNewKey("meli/full")
	Redirect      = stats.Int64("redirects", "redirects", stats.UnitDimensionless)
	redirectsView = &view.View{
		Name:        "redirects",
		Description: "redirects",
		TagKeys:     []tag.Key{ShortKey, FullKey},
		Measure:     Redirect,
		Aggregation: view.Count(),
	}
	Creation    = stats.Int64("creation", "creation", stats.UnitDimensionless)
	cretionView = &view.View{
		Name:        "creation",
		Description: "creation",
		TagKeys:     []tag.Key{ShortKey, FullKey},
		Measure:     Creation,
		Aggregation: view.Count(),
	}
	Deletion     = stats.Int64("deletion", "deletion", stats.UnitDimensionless)
	deletionView = &view.View{
		Name:        "deletion",
		Description: "deletion",
		TagKeys:     []tag.Key{ShortKey},
		Measure:     Deletion,
		Aggregation: view.Count(),
	}
	Search     = stats.Int64("search", "search", stats.UnitDimensionless)
	searchView = &view.View{
		Name:        "search",
		Description: "search",
		TagKeys:     []tag.Key{ShortKey},
		Measure:     Search,
		Aggregation: view.Count(),
	}
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
	err = view.Register(redirectsView, cretionView, deletionView, searchView)
	if err != nil {
		return err
	}
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
