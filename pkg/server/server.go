package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/gorilla/mux"
	"gocloud.dev/server"
	"gocloud.dev/server/requestlog"
)

func routes() http.Handler {
	r := mux.NewRouter()
	return r
}

func setup() *server.Options {
	return &server.Options{
		RequestLogger: requestlog.NewStackdriverLogger(os.Stdout, func(e error) { fmt.Println(e) }),
	}
}

// Start the HTTP server
func Start(ctx context.Context, addr string) error {
	s := server.New(routes(), setup())
	cerr := make(chan error, 1)
	go func(ctx context.Context) {
		sigint := make(chan os.Signal, 1)
		signal.Notify(sigint, os.Interrupt, syscall.SIGTERM)
		<-sigint
		if err := s.Shutdown(ctx); err != nil {
			cerr <- err
		}
	}(ctx)
	go func(ctx context.Context) {
		err := s.ListenAndServe(addr)
		if err != nil {
			cerr <- err
		}
	}(ctx)
	err := <-cerr
	if err != http.ErrServerClosed {
		return err
	}
	return nil
}
