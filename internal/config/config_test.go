package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dwg/tcp_handshake_timer/internal/config"
)

func TestDefaultValidateMissingInterfaces(t *testing.T) {
	cfg := config.Default()
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected error for missing interfaces")
	}
}

func TestLoadFromYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cfg.yaml")
	content := `
interfaces:
  - lo
ports:
  - 53
  - 853
sample_rate_pct: 25
listen_addr: ":9090"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"thtimer", "-config", path}

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(cfg.Interfaces) != 1 || cfg.Interfaces[0] != "lo" {
		t.Fatalf("interfaces: %#v", cfg.Interfaces)
	}
	if len(cfg.Ports) != 2 || cfg.Ports[0] != 53 {
		t.Fatalf("ports: %#v", cfg.Ports)
	}
	if cfg.SampleRatePct != 25 {
		t.Fatalf("sample rate: %d", cfg.SampleRatePct)
	}
	if cfg.ListenAddr != ":9090" {
		t.Fatalf("listen addr: %s", cfg.ListenAddr)
	}
}

func TestPortFilterEnabled(t *testing.T) {
	cfg := config.Default()
	cfg.Interfaces = []string{"lo"}
	if !cfg.PortFilterEnabled() {
		t.Fatal("expected port filter enabled with default port list")
	}
	cfg.Ports = nil
	if cfg.PortFilterEnabled() {
		t.Fatal("expected port filter disabled with empty ports")
	}
}
