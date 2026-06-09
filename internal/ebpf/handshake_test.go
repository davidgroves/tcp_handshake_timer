//go:build linux

package ebpf_test

import (
	"encoding/binary"
	"os"
	"testing"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/rlimit"

	thtimer "github.com/dwg/tcp_handshake_timer/internal/ebpf"
)

func TestHandshakePrograms(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root to load eBPF programs")
	}
	if err := rlimit.RemoveMemlock(); err != nil {
		t.Skipf("memlock: %v", err)
	}

	objs := thtimer.HandshakeObjects{}
	if err := thtimer.LoadHandshakeObjects(&objs, nil); err != nil {
		t.Fatalf("load objects: %v", err)
	}
	defer objs.Close()

	cfgKey := uint32(0)
	cfgVal := thtimer.HandshakeThtimerConfig{
		SampleRatePct:     100,
		PortFilterEnabled: 1,
	}
	if err := objs.ConfigMap.Put(cfgKey, cfgVal); err != nil {
		t.Fatal(err)
	}
	if err := objs.Ports.Put(uint16(8080), uint8(1)); err != nil {
		t.Fatal(err)
	}

	synAck := buildIPv4TCPPacket(0x12, 1000, 0, 8080, 54321, false)
	ack := buildIPv4TCPPacket(0x10, 2001, 1001, 54321, 8080, true)

	runTC(t, objs.ThtimerEgress, synAck)
	runTC(t, objs.ThtimerIngress, ack)
}

func runTC(t *testing.T, prog *ebpf.Program, pkt []byte) {
	t.Helper()
	ret, _, err := prog.Test(pkt)
	if err != nil {
		t.Fatalf("prog test: %v", err)
	}
	if ret != 0 {
		t.Fatalf("unexpected tc action: %d", ret)
	}
}

func buildIPv4TCPPacket(flags byte, seq, ackSeq, srcPort, dstPort uint32, clientToServer bool) []byte {
	const (
		ethLen = 14
		ipLen  = 20
		tcpLen = 20
	)
	total := ethLen + ipLen + tcpLen
	buf := make([]byte, total)

	binary.BigEndian.PutUint16(buf[12:14], 0x0800)

	ip := buf[ethLen:]
	ip[0] = 0x45
	binary.BigEndian.PutUint16(ip[2:4], uint16(ipLen+tcpLen))
	ip[8] = 64
	ip[9] = 6
	if clientToServer {
		copy(ip[12:16], []byte{203, 0, 113, 5})
		copy(ip[16:20], []byte{198, 51, 100, 1})
	} else {
		copy(ip[12:16], []byte{198, 51, 100, 1})
		copy(ip[16:20], []byte{203, 0, 113, 5})
	}

	tcp := buf[ethLen+ipLen:]
	binary.BigEndian.PutUint16(tcp[0:2], uint16(srcPort))
	binary.BigEndian.PutUint16(tcp[2:4], uint16(dstPort))
	binary.BigEndian.PutUint32(tcp[4:8], seq)
	binary.BigEndian.PutUint32(tcp[8:12], ackSeq)
	tcp[12] = 5 << 4
	tcp[13] = flags
	return buf
}
