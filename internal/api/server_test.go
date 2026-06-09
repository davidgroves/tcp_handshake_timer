package api_test

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"codeberg.org/davidgroves/tcp_handshake_timer/internal/api"
	"codeberg.org/davidgroves/tcp_handshake_timer/internal/config"
	"codeberg.org/davidgroves/tcp_handshake_timer/internal/measurement"
	"codeberg.org/davidgroves/tcp_handshake_timer/internal/metrics"
	"codeberg.org/davidgroves/tcp_handshake_timer/internal/state"
)

func TestAPIHandlers(t *testing.T) {
	cfg := config.Default()
	cfg.Interfaces = []string{"lo"}
	store, err := state.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	store.Add(measurement.Measurement{
		ClientIP:   mustIP("203.0.113.1"),
		ClientPort: 1234,
		ServerIP:   mustIP("198.51.100.1"),
		ServerPort: 53,
		RTT:        25 * time.Millisecond,
		Time:       time.Now(),
	})

	srv := api.New(metrics.New(), store, cfg.TopN)
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	for _, path := range []string{
		"/healthz",
		"/api/dump",
		"/api/stats",
		"/api/topn?n=1",
		"/metrics",
		"/api/docs",
		"/api/openapi.yaml",
		"/api/docs/static/swagger-ui.css",
		"/api/docs/static/swagger-ui-bundle.js",
	} {
		resp, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET %s status: %d", path, resp.StatusCode)
		}
	}

	resp, err := http.Get(ts.URL + "/api/dump")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var dump struct {
		Count int `json:"count"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&dump); err != nil {
		t.Fatal(err)
	}
	if dump.Count != 1 {
		t.Fatalf("dump count: %d", dump.Count)
	}
}

func mustIP(s string) net.IP {
	ip := net.ParseIP(s)
	if ip == nil {
		panic("bad ip")
	}
	return ip
}
