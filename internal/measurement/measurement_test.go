package measurement_test

import (
	"encoding/binary"
	"net"
	"testing"
	"time"

	"codeberg.org/davidgroves/tcp_handshake_timer/internal/measurement"
)

func TestDecodeEventIPv4(t *testing.T) {
	raw := make([]byte, 56)
	raw[0] = 2
	copy(raw[4:8], net.ParseIP("203.0.113.5").To4())
	copy(raw[20:24], net.ParseIP("198.51.100.1").To4())
	binary.NativeEndian.PutUint16(raw[36:38], 12345)
	binary.NativeEndian.PutUint16(raw[38:40], 53)
	binary.NativeEndian.PutUint64(raw[40:48], uint64(50*time.Millisecond))

	before := time.Now().Add(-time.Second)
	m, err := measurement.DecodeEvent(raw)
	if err != nil {
		t.Fatal(err)
	}
	after := time.Now().Add(time.Second)
	if m.ClientIP.String() != "203.0.113.5" {
		t.Fatalf("client ip: %s", m.ClientIP)
	}
	if m.ServerPort != 53 {
		t.Fatalf("server port: %d", m.ServerPort)
	}
	if m.RTT != 50*time.Millisecond {
		t.Fatalf("rtt: %v", m.RTT)
	}
	if m.Time.Before(before) || m.Time.After(after) {
		t.Fatalf("timestamp out of range: %v", m.Time)
	}
}

func TestDecodeEventTooShort(t *testing.T) {
	_, err := measurement.DecodeEvent([]byte{1, 2, 3})
	if err == nil {
		t.Fatal("expected error")
	}
}
