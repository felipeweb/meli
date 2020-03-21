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
	"github.com/felipeweb/meli/pkg/redis"
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

func routes(h *handlers) *mux.Router {
	r := mux.NewRouter()
	r.Handle("/", httperr.NewF(h.redirect)).Methods(http.MethodGet)
	return r
}

// Start the HTTP server
func Start(ctx context.Context, cfg *Config) error {
	opts, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		return fmt.Errorf("unable to parse redis URL: %w", err)
	}
	if opts.MaxRetries < 2 {
		opts.MaxRetries = 3
	}
	redisClient, err := redis.NewClient(&redis.Config{
		Addr:       opts.Addr,
		Password:   opts.Password,
		DB:         opts.DB,
		MaxRetries: opts.MaxRetries,
	})
	if err != nil {
		return fmt.Errorf("unable to open redis connection: %v", err)
	}
	r := routes(&handlers{redisClient})
	logrus.Info("initialize metrics\n")
	err = metrics.Initialize(ctx, r, cfg.Metrics)
	if err != nil {
		return fmt.Errorf("unable to initialize metrics: %w", err)
	}
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
