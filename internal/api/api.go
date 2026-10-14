package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/sbOogway/openagentindex/internal/index"
)

const defaultLimit = 20

type Index interface {
	Search(ctx context.Context, q index.Query) (index.Results, error)
	Agent(ctx context.Context, host string) (index.Agent, error)
}

type Config struct {
	Index Index
	// CORSOrigin is the website origin. Empty means no CORS header.
	CORSOrigin string
	Log        *slog.Logger
}

type server struct {
	Config
}

func Handler(cfg Config) http.Handler {
	s := &server{cfg}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/search", s.search)
	mux.HandleFunc("GET /v1/agents/{host}", s.agent)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "not found")
	})
	return s.cors(mux)
}

type summary struct {
	Host            string `json:"host"`
	Name            string `json:"name"`
	Description     string `json:"description"`
	ProtocolVersion string `json:"protocol_version"`
	Status          string `json:"status"`
	HTTPStatus      int    `json:"http_status"`
	Error           string `json:"error,omitempty"`
	LastChecked     int64  `json:"last_checked"`
}

type observation struct {
	ID         string `json:"id"`
	Kind       string `json:"kind"`
	At         int64  `json:"at"`
	OK         bool   `json:"ok"`
	HTTPStatus int    `json:"http_status"`
	LatencyMS  int64  `json:"latency_ms"`
	Error      string `json:"error,omitempty"`
}

type agent struct {
	summary
	Card         json.RawMessage `json:"card"`
	Observations []observation   `json:"observations"`
}

func (s *server) search(w http.ResponseWriter, r *http.Request) {
	q, err := parseQuery(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	res, err := s.Index.Search(r.Context(), q)
	if err != nil {
		s.fail(w, err)
		return
	}
	out := struct {
		Total   int       `json:"total"`
		Results []summary `json:"results"`
	}{Total: res.Total, Results: []summary{}}
	for _, a := range res.Agents {
		out.Results = append(out.Results, toSummary(a))
	}
	w.Header().Set("Cache-Control", "public, max-age=60")
	writeJSON(w, http.StatusOK, out)
}

func (s *server) agent(w http.ResponseWriter, r *http.Request) {
	a, err := s.Index.Agent(r.Context(), r.PathValue("host"))
	if err != nil {
		s.fail(w, err)
		return
	}
	out := agent{summary: toSummary(a.Summary), Card: a.Card, Observations: []observation{}}
	if out.Card == nil {
		out.Card = json.RawMessage("null")
	}
	for _, o := range a.Observations {
		out.Observations = append(out.Observations, observation{
			ID: o.ID, Kind: o.Kind, At: o.At.Unix(), OK: o.OK,
			HTTPStatus: o.HTTPStatus, LatencyMS: o.Latency.Milliseconds(), Error: o.Error,
		})
	}
	w.Header().Set("Cache-Control", "public, max-age=60")
	writeJSON(w, http.StatusOK, out)
}

func parseQuery(r *http.Request) (index.Query, error) {
	v := r.URL.Query()
	q := index.Query{Text: v.Get("q"), Status: v.Get("status"), Limit: defaultLimit}
	if q.Status == "" {
		q.Status = index.StatusOK
	}
	var err error
	if s := v.Get("limit"); s != "" {
		if q.Limit, err = strconv.Atoi(s); err != nil {
			return q, errors.New("limit is not a number")
		}
	}
	if s := v.Get("offset"); s != "" {
		if q.Offset, err = strconv.Atoi(s); err != nil {
			return q, errors.New("offset is not a number")
		}
	}
	return q, nil
}

func toSummary(a index.Summary) summary {
	status := index.StatusFailed
	if a.OK {
		status = index.StatusOK
	}
	return summary{
		Host: a.Host, Name: a.Name, Description: a.Description, ProtocolVersion: a.ProtocolVersion,
		Status: status, HTTPStatus: a.HTTPStatus, Error: a.Error, LastChecked: a.LastChecked.Unix(),
	}
}

func (s *server) fail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, index.ErrInvalidQuery):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, index.ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	default:
		s.Log.Error("request failed", "err", err)
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}

func (s *server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.CORSOrigin != "" {
			w.Header().Set("Access-Control-Allow-Origin", s.CORSOrigin)
			w.Header().Add("Vary", "Origin")
		}
		next.ServeHTTP(w, r)
	})
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
