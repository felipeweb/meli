package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/felipeweb/meli/pkg/redis"
	"github.com/gorilla/mux"
)

func TestCreate(t *testing.T) {
	s, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	redisClient, err := redis.NewClient(&redis.Config{
		Addr:       s.Addr(),
		DB:         0,
		MaxRetries: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	r := mux.NewRouter()
	routes(r, &handlers{redisClient})
	st := httptest.NewServer(r)
	defer st.Close()
	type args struct {
		body string
	}
	tests := []struct {
		name       string
		args       args
		statusCode int
	}{
		{
			"create with success",
			args{`{"url":"https://felipeweb.dev"}`},
			http.StatusCreated,
		},
		{
			"invalid URL",
			args{`{"url":"bla"}`},
			http.StatusBadRequest,
		},
		{
			"invalid JSON",
			args{`bla`},
			http.StatusBadRequest,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := http.Post(st.URL, "application/json", strings.NewReader(tt.args.body))
			if err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != tt.statusCode {
				t.Errorf("expected %v but got %v", tt.statusCode, resp.StatusCode)
			}
		})
	}
}

func TestDel(t *testing.T) {
	s, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	redisClient, err := redis.NewClient(&redis.Config{
		Addr:       s.Addr(),
		DB:         0,
		MaxRetries: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	r := mux.NewRouter()
	routes(r, &handlers{redisClient})
	st := httptest.NewServer(r)
	defer st.Close()
	type args struct {
		Short string `json:"short"`
	}
	tests := []struct {
		name       string
		args       args
		statusCode int
		setup      func(*testing.T) string
	}{
		{
			"del with success",
			args{},
			http.StatusOK,
			func(t *testing.T) string {
				resp, err := http.Post(st.URL, "application/json", strings.NewReader(`{"url":"https://felipeweb.dev"}`))
				if err != nil {
					t.Fatal(err)
				}
				short := args{}
				err = json.NewDecoder(resp.Body).Decode(&short)
				if err != nil {
					t.Fatal(err)
				}
				defer resp.Body.Close()
				return short.Short
			},
		},
		{
			"not found",
			args{`bla`},
			http.StatusNotFound,
			nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setup != nil {
				tt.args.Short = tt.setup(t)
			}
			url := fmt.Sprintf("%s/%s", st.URL, tt.args.Short)
			req, err := http.NewRequest(http.MethodDelete, url, nil)
			if err != nil {
				t.Fatal(err)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != tt.statusCode {
				t.Errorf("expected %v but got %v", tt.statusCode, resp.StatusCode)
			}
		})
	}
}

func TestSearch(t *testing.T) {
	s, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	redisClient, err := redis.NewClient(&redis.Config{
		Addr:       s.Addr(),
		DB:         0,
		MaxRetries: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	r := mux.NewRouter()
	routes(r, &handlers{redisClient})
	st := httptest.NewServer(r)
	defer st.Close()
	type args struct {
		Short string `json:"short"`
	}
	tests := []struct {
		name       string
		args       args
		statusCode int
		setup      func(*testing.T) string
	}{
		{
			"search with success",
			args{},
			http.StatusOK,
			func(t *testing.T) string {
				resp, err := http.Post(st.URL, "application/json", strings.NewReader(`{"url":"https://felipeweb.dev"}`))
				if err != nil {
					t.Fatal(err)
				}
				short := args{}
				err = json.NewDecoder(resp.Body).Decode(&short)
				if err != nil {
					t.Fatal(err)
				}
				defer resp.Body.Close()
				return short.Short
			},
		},
		{
			"not found",
			args{`bla`},
			http.StatusNotFound,
			nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setup != nil {
				tt.args.Short = tt.setup(t)
			}
			url := fmt.Sprintf("%s/search/%s", st.URL, tt.args.Short)
			resp, err := http.Get(url)
			if err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != tt.statusCode {
				t.Errorf("expected %v but got %v", tt.statusCode, resp.StatusCode)
			}
			if resp.StatusCode == http.StatusOK {
				u := URL{}
				err = json.NewDecoder(resp.Body).Decode(&u)
				if err != nil {
					t.Fatal(err)
				}
				defer resp.Body.Close()
				if u.URL != "https://felipeweb.dev" {
					t.Error("is not a full URL")
				}
			}
		})
	}
}

func TestRedirect(t *testing.T) {
	s, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	redisClient, err := redis.NewClient(&redis.Config{
		Addr:       s.Addr(),
		DB:         0,
		MaxRetries: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	r := mux.NewRouter()
	routes(r, &handlers{redisClient})
	st := httptest.NewServer(r)
	defer st.Close()
	type args struct {
		Short string `json:"short"`
	}
	tests := []struct {
		name        string
		args        args
		statusCode  int
		contentType string
		setup       func(*testing.T) string
	}{
		{
			"redirect with success",
			args{},
			http.StatusOK,
			"text/html; charset=utf-8",
			func(t *testing.T) string {
				resp, err := http.Post(st.URL, "application/json", strings.NewReader(`{"url":"https://felipeweb.dev"}`))
				if err != nil {
					t.Fatal(err)
				}
				short := args{}
				err = json.NewDecoder(resp.Body).Decode(&short)
				if err != nil {
					t.Fatal(err)
				}
				defer resp.Body.Close()
				return short.Short
			},
		},
		{
			"not found",
			args{`bla`},
			http.StatusNotFound,
			"text/plain; charset=utf-8",
			nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setup != nil {
				tt.args.Short = tt.setup(t)
			}
			url := fmt.Sprintf("%s/%s", st.URL, tt.args.Short)
			resp, err := http.Get(url)
			if err != nil {
				t.Fatal(err)
			}
			if resp.StatusCode != tt.statusCode {
				t.Errorf("expected %v but got %v", tt.statusCode, resp.StatusCode)
			}
			if resp.Header.Get("Content-Type") != tt.contentType {
				t.Errorf("expected %v but got %v", tt.contentType, resp.Header.Get("Content-Type"))
			}
		})
	}
}
