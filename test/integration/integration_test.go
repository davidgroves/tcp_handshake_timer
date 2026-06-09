//go:build linux && integration

package integration_test

import (
	"os"
	"testing"

	"codeberg.org/davidgroves/tcp_handshake_timer/internal/config"
	thtimer "codeberg.org/davidgroves/tcp_handshake_timer/internal/ebpf"
)

func TestAttachLoopback(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root")
	}

	cfg := config.Default()
	cfg.Interfaces = []string{"lo"}
	cfg.Ports = []uint16{18080}
	cfg.SampleRatePct = 100

	loader, err := thtimer.NewLoader(cfg)
	if err != nil {
		t.Fatalf("attach loader: %v", err)
	}
	defer loader.Close()

	if loader.Mode() == "" {
		t.Fatal("expected attach mode")
	}
}
