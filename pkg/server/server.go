package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/caarlos0/httperr"
	"github.com/felipeweb/meli/pkg/metrics"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
	"gocloud.dev/server"
	"gocloud.dev/server/requestlog"
)

// Config server
type Config struct {
	Metrics  *metrics.Config
	Addr     string
	RedisURL string
}

func routes(r *mux.Router, h *handlers) {
	r.Handle("/", httperr.NewF(h.create)).Methods(http.MethodPost)
	r.Handle("/{short}", httperr.NewF(h.redirect)).Methods(http.MethodGet)
	r.Handle("/{short}", httperr.NewF(h.delete)).Methods(http.MethodDelete)
	r.Handle("/search/{short}", httperr.NewF(h.search)).Methods(http.MethodGet)
}

// Start the HTTP server
func Start(ctx context.Context, cfg *Config) error {
	r := mux.NewRouter()
	logrus.Info("initialize metrics\n")
	err := metrics.Initialize(ctx, r, cfg.Metrics)
	if err != nil {
		return fmt.Errorf("unable to initialize metrics: %w", err)
	}
	routes(r, &handlers{cfg})
	s := server.New(r, &server.Options{
		RequestLogger: requestlog.NewStackdriverLogger(os.Stdout, func(e error) { fmt.Println(e) }),
	})
	cerr := make(chan error, 1)
	go func(ctx context.Context) {
		sigint := make(chan os.Signal, 1)
		signal.Notify(sigint, os.Interrupt, syscall.SIGTERM)
		<-sigint
		logrus.Info("server shutting down\n")
		if err := s.Shutdown(ctx); err != nil {
			cerr <- err
		}
	}(ctx)
	go func(ctx context.Context) {
		logrus.Infof("server listening at %v\n", cfg.Addr)
		err := s.ListenAndServe(cfg.Addr)
		if err != nil {
			cerr <- err
		}
	}(ctx)
	err = <-cerr
	if err != http.ErrServerClosed {
		return fmt.Errorf("server error: %w", err)
	}
	return nil
}
