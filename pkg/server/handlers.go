package server

import (
	"net/http"

	"github.com/felipeweb/meli/pkg/metrics"
	"github.com/sirupsen/logrus"
	"go.opencensus.io/stats"
	"go.opencensus.io/tag"

	"github.com/caarlos0/httperr"
	"github.com/felipeweb/meli/pkg/redis"
	"github.com/gorilla/mux"
)

type handlers struct {
	cli *redis.Client
}

func (h handlers) redirect(w http.ResponseWriter, r *http.Request) error {
	short := mux.Vars(r)["short"]
	url, err := h.cli.Find(short)
	if err == redis.ErrKeyNotFound {
		return httperr.Wrap(err, http.StatusNotFound)
	}
	if err != nil {
		return httperr.Wrap(err, http.StatusInternalServerError)
	}
	ctx, err := tag.New(r.Context(),
		tag.Insert(metrics.ShortKey, short),
		tag.Insert(metrics.FullKey, url),
	)
	if err != nil {
		logrus.Warnf("unable to get metric: %v", err)
	}
	stats.Record(ctx, metrics.Redirect.M(1))
	http.Redirect(w, r, url, http.StatusMovedPermanently)
	return nil
}
