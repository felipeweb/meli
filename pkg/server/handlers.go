package server

import (
	"encoding/json"
	"net/http"

	"github.com/caarlos0/httperr"
	"github.com/felipeweb/meli/pkg/metrics"
	"github.com/felipeweb/meli/pkg/redis"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
	"go.opencensus.io/stats"
	"go.opencensus.io/tag"
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
	r = r.WithContext(ctx)
	stats.Record(r.Context(), metrics.Redirect.M(1))
	http.Redirect(w, r, url, http.StatusMovedPermanently)
	return nil
}

type URL struct {
	URL string `json:"url,omitempty"`
}
type Short struct {
	Short string `json:"short,omitempty"`
}

func (h handlers) create(w http.ResponseWriter, r *http.Request) error {
	b := URL{}
	err := json.NewDecoder(r.Body).Decode(&b)
	if err != nil {
		return httperr.Wrap(err, http.StatusBadRequest)
	}
	short, err := h.cli.Set(b.URL)
	if err == redis.ErrInvalidURL {
		return httperr.Wrap(err, http.StatusBadRequest)
	}
	if err != nil {
		return httperr.Wrap(err, http.StatusInternalServerError)
	}
	ctx, err := tag.New(r.Context(),
		tag.Insert(metrics.ShortKey, short),
		tag.Insert(metrics.FullKey, b.URL),
	)
	if err != nil {
		logrus.Warnf("unable to get metric: %v", err)
	}
	r = r.WithContext(ctx)
	stats.Record(r.Context(), metrics.Creation.M(1))
	w.WriteHeader(http.StatusCreated)
	err = json.NewEncoder(w).Encode(Short{
		Short: short,
	})
	if err != nil {
		return httperr.Wrap(err, http.StatusInternalServerError)
	}
	return nil
}
