package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"codeberg.org/davidgroves/tcp_handshake_timer/internal/metrics"
	"codeberg.org/davidgroves/tcp_handshake_timer/internal/state"
)

type Server struct {
	metrics *metrics.Metrics
	store   *state.Store
	topN    int
}

func New(m *metrics.Metrics, store *state.Store, topN int) *Server {
	return &Server{metrics: m, store: store, topN: topN}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(s.metrics.Registry(), promhttp.HandlerOpts{}))
	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/api/openapi.yaml", s.handleOpenAPI)
	mux.Handle("/api/docs/static/", s.swaggerUIHandler())
	mux.HandleFunc("/api/docs", s.handleDocs)
	mux.HandleFunc("/api/dump", s.handleDump)
	mux.HandleFunc("/api/stats", s.handleStats)
	mux.HandleFunc("/api/topn", s.handleTopN)
	return mux
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *Server) handleDump(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.store.Dump())
}

func (s *Server) handleStats(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, s.store.Stats())
}

func (s *Server) handleTopN(w http.ResponseWriter, r *http.Request) {
	n := s.topN
	if raw := r.URL.Query().Get("n"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			http.Error(w, "invalid n", http.StatusBadRequest)
			return
		}
		n = parsed
	}
	writeJSON(w, s.store.TopN(n))
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
