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

type URL struct {
	URL string `json:"url,omitempty"`
}
type Short struct {
	Short string `json:"short,omitempty"`
}

type handlers struct {
	cfg *Config
}

func (h handlers) delete(w http.ResponseWriter, r *http.Request) error {
	short := mux.Vars(r)["short"]
	rc, err := redis.NewClient(r.Context(), h.cfg.RedisURL)
	if err != nil {
		return httperr.Wrap(err, http.StatusInternalServerError)
	}
	defer rc.Close()
	err = rc.Del(short)
	if err == redis.ErrKeyNotFound {
		return httperr.Wrap(err, http.StatusNotFound)
	}
	if err != nil {
		return httperr.Wrap(err, http.StatusInternalServerError)
	}
	ctx, err := tag.New(rc.Context(),
		tag.Insert(metrics.ShortKey, short),
	)
	if err != nil {
		logrus.Warnf("unable to get metric: %v", err)
	}
	r = r.WithContext(ctx)
	stats.Record(r.Context(), metrics.Deletion.M(1))
	return nil
}

func (h handlers) redirect(w http.ResponseWriter, r *http.Request) error {
	short := mux.Vars(r)["short"]
	rc, err := redis.NewClient(r.Context(), h.cfg.RedisURL)
	if err != nil {
		return httperr.Wrap(err, http.StatusInternalServerError)
	}
	defer rc.Close()
	url, err := rc.Find(short)
	if err == redis.ErrKeyNotFound {
		return httperr.Wrap(err, http.StatusNotFound)
	}
	if err != nil {
		return httperr.Wrap(err, http.StatusInternalServerError)
	}
	ctx, err := tag.New(rc.Context(),
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

func (h handlers) search(w http.ResponseWriter, r *http.Request) error {
	short := mux.Vars(r)["short"]
	rc, err := redis.NewClient(r.Context(), h.cfg.RedisURL)
	if err != nil {
		return httperr.Wrap(err, http.StatusInternalServerError)
	}
	defer rc.Close()
	url, err := rc.Find(short)
	if err == redis.ErrKeyNotFound {
		return httperr.Wrap(err, http.StatusNotFound)
	}
	if err != nil {
		return httperr.Wrap(err, http.StatusInternalServerError)
	}
	ctx, err := tag.New(rc.Context(),
		tag.Insert(metrics.ShortKey, short),
		tag.Insert(metrics.FullKey, url),
	)
	if err != nil {
		logrus.Warnf("unable to get metric: %v", err)
	}
	r = r.WithContext(ctx)
	stats.Record(r.Context(), metrics.Search.M(1))
	err = json.NewEncoder(w).Encode(URL{
		URL: url,
	})
	if err != nil {
		return httperr.Wrap(err, http.StatusInternalServerError)
	}
	return nil
}

func (h handlers) create(w http.ResponseWriter, r *http.Request) error {
	b := URL{}
	err := json.NewDecoder(r.Body).Decode(&b)
	if err != nil {
		return httperr.Wrap(err, http.StatusBadRequest)
	}
	defer r.Body.Close()
	rc, err := redis.NewClient(r.Context(), h.cfg.RedisURL)
	if err != nil {
		return httperr.Wrap(err, http.StatusInternalServerError)
	}
	defer rc.Close()
	short, err := rc.Set(b.URL)
	if err == redis.ErrInvalidURL {
		return httperr.Wrap(err, http.StatusBadRequest)
	}
	if err != nil {
		return httperr.Wrap(err, http.StatusInternalServerError)
	}
	ctx, err := tag.New(rc.Context(),
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
