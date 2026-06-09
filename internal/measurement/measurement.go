package measurement

import (
	"encoding/binary"
	"net"
	"time"
)

type Measurement struct {
	ClientIP   net.IP
	ClientPort uint16
	ServerIP   net.IP
	ServerPort uint16
	RTT        time.Duration
	Time       time.Time
}

const (
	StatSynAckObserved = iota
	StatSynAckSampled
	StatAckMatched
	StatIncomplete
	StatRingbufDrop
	StatParseError
	StatPortSkip
	StatMax // number of counters; valid indices are 0..StatMax-1
)

var ErrShortSample = errShortSample{}

type errShortSample struct{}

func (errShortSample) Error() string { return "measurement sample too short" }

func IPFromBytes(family uint8, b [16]byte) net.IP {
	if family == 2 {
		return net.IP(b[:4]).To4()
	}
	ip := make(net.IP, 16)
	copy(ip, b[:])
	return ip
}

func DecodeEvent(raw []byte) (Measurement, error) {
	const eventSize = 56
	if len(raw) < eventSize {
		return Measurement{}, ErrShortSample
	}

	var m Measurement
	family := raw[0]
	m.ClientIP = IPFromBytes(family, [16]byte(raw[4:20]))
	m.ServerIP = IPFromBytes(family, [16]byte(raw[20:36]))
	m.ClientPort = binary.NativeEndian.Uint16(raw[36:38])
	m.ServerPort = binary.NativeEndian.Uint16(raw[38:40])
	rttNS := binary.NativeEndian.Uint64(raw[40:48])
	m.RTT = time.Duration(rttNS)
	// ts_ns from the kernel is bpf_ktime_get_ns() (monotonic since boot), not wall clock.
	m.Time = time.Now().UTC()
	return m, nil
}
