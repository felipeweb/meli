package main

import (
	"context"
	"flag"

	"github.com/felipeweb/meli/pkg/server"
	"github.com/sirupsen/logrus"
)

func main() {
	addr := flag.String("addr", "0.0.0.0:8080", "HTTP address (default 0.0.0.0:8080)")
	flag.Parse()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := server.Start(ctx, *addr); err != nil {
		logrus.Fatal(err)
	}
}
