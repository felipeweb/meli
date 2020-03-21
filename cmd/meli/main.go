package main

import (
	"context"
	"flag"

	"github.com/felipeweb/meli/pkg/metrics"
	"github.com/felipeweb/meli/pkg/server"
	"github.com/sirupsen/logrus"
)

func main() {
	addr := flag.String("addr", "0.0.0.0:8080", "HTTP address (default :8080)")
	traceEndpoint := flag.String("trace_addr", "", "jaeger endpoint")
	redisURL := flag.String("redis_url", "redis://127.0.0.1", "redis URL")
	flag.Parse()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg := &server.Config{
		Addr:     *addr,
		RedisURL: *redisURL,
		Metrics: &metrics.Config{
			TraceExporterAddr: *traceEndpoint,
		},
	}
	if err := server.Start(ctx, cfg); err != nil {
		logrus.Fatal(err)
	}
}
