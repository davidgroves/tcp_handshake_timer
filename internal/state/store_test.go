package state_test

import (
	"net"
	"testing"
	"time"

	"codeberg.org/davidgroves/tcp_handshake_timer/internal/config"
	"codeberg.org/davidgroves/tcp_handshake_timer/internal/measurement"
	"codeberg.org/davidgroves/tcp_handshake_timer/internal/state"
)

func TestStoreRetentionAndStats(t *testing.T) {
	cfg := config.Default()
	cfg.Interfaces = []string{"lo"}
	cfg.MaxSamples = 3
	cfg.TopN = 2
	cfg.MaxSubnets = 10

	store, err := state.New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	base := time.Now()
	for i := range 5 {
		store.Add(measurement.Measurement{
			ClientIP:   net.ParseIP("203.0.113.10"),
			ClientPort: uint16(1000 + i),
			ServerIP:   net.ParseIP("198.51.100.1"),
			ServerPort: 53,
			RTT:        time.Duration((i + 1) * int(time.Millisecond)),
			Time:       base.Add(time.Duration(i) * time.Second),
		})
	}

	dump := store.Dump()
	if dump.Count != 3 {
		t.Fatalf("expected 3 retained samples, got %d", dump.Count)
	}

	stats := store.Stats()
	if stats.Overall.Count != 5 {
		t.Fatalf("overall count: %d", stats.Overall.Count)
	}
	if stats.Overall.P50NS <= 0 {
		t.Fatalf("expected positive p50, got %v", stats.Overall.P50NS)
	}

	top := store.TopN(2)
	if len(top.Entries) != 2 {
		t.Fatalf("topn entries: %d", len(top.Entries))
	}
	if top.Entries[0].RTTNS <= top.Entries[1].RTTNS {
		t.Fatalf("expected descending RTT order: %#v", top.Entries)
	}
}

func TestSubnetAggregation(t *testing.T) {
	cfg := config.Default()
	cfg.Interfaces = []string{"lo"}
	cfg.MaxSamples = 100
	cfg.SubnetPrefixV4 = 24

	store, err := state.New(cfg)
	if err != nil {
		t.Fatal(err)
	}

	store.Add(measurement.Measurement{
		ClientIP: net.ParseIP("203.0.113.10"),
		RTT:    10 * time.Millisecond,
		Time:   time.Now(),
	})
	store.Add(measurement.Measurement{
		ClientIP: net.ParseIP("203.0.113.20"),
		RTT:    20 * time.Millisecond,
		Time:   time.Now(),
	})

	stats := store.Stats()
	if len(stats.Subnets) != 1 {
		t.Fatalf("expected one subnet bucket, got %d", len(stats.Subnets))
	}
	if stats.Subnets[0].Subnet != "203.0.113.0/24" {
		t.Fatalf("subnet key: %s", stats.Subnets[0].Subnet)
	}
}
