package state

import (
	"container/heap"
	"fmt"
	"net"
	"sort"
	"sync"
	"time"

	tdigest "github.com/caio/go-tdigest/v4"

	"codeberg.org/davidgroves/tcp_handshake_timer/internal/config"
	"codeberg.org/davidgroves/tcp_handshake_timer/internal/measurement"
)

type Store struct {
	mu sync.RWMutex
	cfg config.Config

	samples []measurement.Measurement
	head    int
	count   int
	bytes   uint64

	overall *tdigest.TDigest
	subnets map[string]*subnetEntry
	subnetLRU []string

	topN    int
	slowest slowestHeap
}

type subnetEntry struct {
	subnet  string
	digest  *tdigest.TDigest
	lastSeen time.Time
}

type slowEntry struct {
	clientKey string
	clientIP  string
	clientPort uint16
	rtt       time.Duration
	seen      time.Time
	index     int
}

type slowestHeap []*slowEntry

func (h slowestHeap) Len() int           { return len(h) }
func (h slowestHeap) Less(i, j int) bool { return h[i].rtt > h[j].rtt }
func (h slowestHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}
func (h *slowestHeap) Push(x any) {
	e := x.(*slowEntry)
	e.index = len(*h)
	*h = append(*h, e)
}
func (h *slowestHeap) Pop() any {
	old := *h
	n := len(old)
 e := old[n-1]
	old[n-1] = nil
	*h = old[:n-1]
	return e
}

func New(cfg config.Config) (*Store, error) {
	overall, err := tdigest.New(tdigest.Compression(100))
	if err != nil {
		return nil, err
	}
	return &Store{
		cfg:     cfg,
		samples: make([]measurement.Measurement, cfg.MaxSamples),
		overall: overall,
		subnets: make(map[string]*subnetEntry),
		topN:    cfg.TopN,
		slowest: make(slowestHeap, 0, cfg.TopN),
	}, nil
}

func (s *Store) Add(m measurement.Measurement) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.pushSample(m)
	s.overall.Add(float64(m.RTT.Nanoseconds()))

	subnet := subnetKey(m.ClientIP, s.cfg.SubnetPrefixFor(m.ClientIP))
	s.touchSubnet(subnet, m)
	s.updateTopN(m)
	s.recalcBytes()
}

func (s *Store) pushSample(m measurement.Measurement) {
	if len(s.samples) == 0 {
		return
	}
	s.samples[s.head] = m
	s.head = (s.head + 1) % len(s.samples)
	if s.count < len(s.samples) {
		s.count++
	}
}

func (s *Store) touchSubnet(subnet string, m measurement.Measurement) {
	entry, ok := s.subnets[subnet]
	if !ok {
		if len(s.subnets) >= s.cfg.MaxSubnets && len(s.subnetLRU) > 0 {
			evict := s.subnetLRU[0]
			delete(s.subnets, evict)
			s.subnetLRU = s.subnetLRU[1:]
		}
		d, err := tdigest.New(tdigest.Compression(100))
		if err != nil {
			return
		}
		entry = &subnetEntry{subnet: subnet, digest: d}
		s.subnets[subnet] = entry
	}
	entry.lastSeen = m.Time
	entry.digest.Add(float64(m.RTT.Nanoseconds()))
	for i, existing := range s.subnetLRU {
		if existing == subnet {
			s.subnetLRU = append(s.subnetLRU[:i], s.subnetLRU[i+1:]...)
			break
		}
	}
	s.subnetLRU = append(s.subnetLRU, subnet)
}

func (s *Store) updateTopN(m measurement.Measurement) {
	key := fmt.Sprintf("%s:%d", m.ClientIP.String(), m.ClientPort)
	e := &slowEntry{
		clientKey:  key,
		clientIP:   m.ClientIP.String(),
		clientPort: m.ClientPort,
		rtt:        m.RTT,
		seen:       m.Time,
	}
	if s.slowest.Len() < s.topN {
		heap.Push(&s.slowest, e)
		return
	}
	if s.slowest.Len() == 0 {
		return
	}
	min := s.slowest[s.slowest.Len()-1]
	if e.rtt <= min.rtt {
		return
	}
	heap.Pop(&s.slowest)
	heap.Push(&s.slowest, e)
}

func (s *Store) recalcBytes() {
	var b uint64
	b += uint64(len(s.samples)) * 128
	b += uint64(len(s.subnets)) * 4096
	b += uint64(s.slowest.Len()) * 64
	s.bytes = b
	if s.cfg.MaxMemory > 0 && s.bytes > s.cfg.MaxMemory {
		for len(s.subnets) > 0 && s.bytes > s.cfg.MaxMemory {
			evict := s.subnetLRU[0]
			delete(s.subnets, evict)
			s.subnetLRU = s.subnetLRU[1:]
			s.recalcBytes()
		}
	}
}

func (s *Store) Stats() StatsResponse {
	s.mu.RLock()
	defer s.mu.RUnlock()

	resp := StatsResponse{
		GeneratedAt: time.Now().UTC(),
		Overall:     percentilesFromDigest(s.overall),
		Subnets:     make([]SubnetStats, 0, len(s.subnets)),
	}
	for _, entry := range s.subnets {
		resp.Subnets = append(resp.Subnets, SubnetStats{
			Subnet: entry.subnet,
			Stats:  percentilesFromDigest(entry.digest),
		})
	}
	sort.Slice(resp.Subnets, func(i, j int) bool {
		return resp.Subnets[i].Subnet < resp.Subnets[j].Subnet
	})
	return resp
}

func (s *Store) TopN(n int) TopNResponse {
	if n <= 0 {
		n = s.topN
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	items := make([]TopNEntry, 0, n)
	sorted := make(slowestHeap, s.slowest.Len())
	copy(sorted, s.slowest)
	sort.Sort(sorted)
	for i := 0; i < len(sorted) && i < n; i++ {
		e := sorted[i]
		items = append(items, TopNEntry{
			ClientIP:   e.clientIP,
			ClientPort: e.clientPort,
			RTTNS:      e.rtt.Nanoseconds(),
			SeenAt:     e.seen.UTC(),
		})
	}
	return TopNResponse{GeneratedAt: time.Now().UTC(), Entries: items}
}

func (s *Store) Dump() DumpResponse {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]MeasurementRecord, 0, s.count)
	if s.count == 0 {
		return DumpResponse{GeneratedAt: time.Now().UTC(), Count: 0, Measurements: out}
	}
	start := 0
	if s.count == len(s.samples) {
		start = s.head
	}
	for i := 0; i < s.count; i++ {
		idx := (start + i) % len(s.samples)
		m := s.samples[idx]
		out = append(out, MeasurementRecord{
			ClientIP:   m.ClientIP.String(),
			ClientPort: m.ClientPort,
			ServerIP:   m.ServerIP.String(),
			ServerPort: m.ServerPort,
			RTTNS:      m.RTT.Nanoseconds(),
			Timestamp:  m.Time.UTC(),
		})
	}
	return DumpResponse{
		GeneratedAt:  time.Now().UTC(),
		Count:        len(out),
		MemoryBytes:  s.bytes,
		Measurements: out,
	}
}

func (s *Store) SnapshotCounts() (samples int, memoryBytes uint64) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.count, s.bytes
}

type PercentileStats struct {
	Count int64 `json:"count"`
	P50NS int64 `json:"p50_ns"`
	P90NS int64 `json:"p90_ns"`
	P99NS int64 `json:"p99_ns"`
	MaxNS int64 `json:"max_ns"`
}

type SubnetStats struct {
	Subnet string          `json:"subnet"`
	Stats  PercentileStats `json:"stats"`
}

type StatsResponse struct {
	GeneratedAt time.Time       `json:"generated_at"`
	Overall     PercentileStats `json:"overall"`
	Subnets     []SubnetStats   `json:"subnets"`
}

type TopNEntry struct {
	ClientIP   string    `json:"client_ip"`
	ClientPort uint16    `json:"client_port"`
	RTTNS      int64     `json:"rtt_ns"`
	SeenAt     time.Time `json:"seen_at"`
}

type TopNResponse struct {
	GeneratedAt time.Time   `json:"generated_at"`
	Entries     []TopNEntry `json:"entries"`
}

type MeasurementRecord struct {
	ClientIP   string    `json:"client_ip"`
	ClientPort uint16    `json:"client_port"`
	ServerIP   string    `json:"server_ip"`
	ServerPort uint16    `json:"server_port"`
	RTTNS      int64     `json:"rtt_ns"`
	Timestamp  time.Time `json:"timestamp"`
}

type DumpResponse struct {
	GeneratedAt  time.Time           `json:"generated_at"`
	Count        int                 `json:"count"`
	MemoryBytes  uint64              `json:"memory_bytes"`
	Measurements []MeasurementRecord `json:"measurements"`
}

func percentilesFromDigest(d *tdigest.TDigest) PercentileStats {
	if d == nil || d.Count() == 0 {
		return PercentileStats{}
	}
	return PercentileStats{
		Count: int64(d.Count()),
		P50NS: int64(d.Quantile(0.50)),
		P90NS: int64(d.Quantile(0.90)),
		P99NS: int64(d.Quantile(0.99)),
		MaxNS: int64(d.Quantile(1.0)),
	}
}

func subnetKey(ip net.IP, prefix int) string {
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	bits := len(ip) * 8
	if prefix < 0 {
		prefix = 0
	}
	if prefix > bits {
		prefix = bits
	}
	masked := ip.Mask(net.CIDRMask(prefix, bits))
	return fmt.Sprintf("%s/%d", masked.String(), prefix)
}
