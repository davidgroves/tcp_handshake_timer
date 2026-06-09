package metrics

import (
	"sync/atomic"

	"github.com/prometheus/client_golang/prometheus"
)

type Metrics struct {
	MeasurementsMade   prometheus.Counter
	MeasurementsLogged prometheus.Counter
	SynAcksObserved    prometheus.Counter
	Incomplete         prometheus.Counter
	RingbufDropped     prometheus.Counter
	ParseErrors        prometheus.Counter
	RTT                prometheus.Histogram
	FlowsInFlight      prometheus.Gauge
	StateSamples       prometheus.Gauge
	StateMemoryBytes   prometheus.Gauge

	registry *prometheus.Registry
	flows    atomic.Int64
}

func New() *Metrics {
	reg := prometheus.NewRegistry()
	m := &Metrics{registry: reg}

	m.MeasurementsMade = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "thtimer_measurements_made_total",
		Help: "Total completed SYN-ACK to ACK RTT measurements.",
	})
	m.MeasurementsLogged = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "thtimer_measurements_logged_total",
		Help: "Total measurements written to sampled logs.",
	})
	m.SynAcksObserved = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "thtimer_synacks_observed_total",
		Help: "Total SYN-ACK packets observed on egress.",
	})
	m.Incomplete = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "thtimer_incomplete_handshakes_total",
		Help: "ACK packets with no matching tracked SYN-ACK flow.",
	})
	m.RingbufDropped = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "thtimer_ringbuf_events_dropped_total",
		Help: "Measurement events dropped because the ring buffer was full.",
	})
	m.ParseErrors = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "thtimer_parse_errors_total",
		Help: "Packet parse errors in eBPF programs.",
	})
	m.RTT = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "thtimer_handshake_rtt_seconds",
		Help:    "Client RTT measured from SYN-ACK egress to completing ACK ingress.",
		Buckets: prometheus.ExponentialBuckets(0.0001, 2, 18),
	})
	m.FlowsInFlight = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "thtimer_flows_in_flight",
		Help: "Approximate number of SYN-ACK flows awaiting client ACK.",
	})
	m.StateSamples = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "thtimer_state_samples",
		Help: "Number of raw samples retained in userspace state.",
	})
	m.StateMemoryBytes = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "thtimer_state_memory_bytes",
		Help: "Approximate bytes used by userspace state store.",
	})

	reg.MustRegister(
		m.MeasurementsMade,
		m.MeasurementsLogged,
		m.SynAcksObserved,
		m.Incomplete,
		m.RingbufDropped,
		m.ParseErrors,
		m.RTT,
		m.FlowsInFlight,
		m.StateSamples,
		m.StateMemoryBytes,
	)

	return m
}

func (m *Metrics) Registry() *prometheus.Registry {
	return m.registry
}

func (m *Metrics) ObserveMeasurement(rttSeconds float64) {
	m.MeasurementsMade.Inc()
	m.RTT.Observe(rttSeconds)
}

func (m *Metrics) ObserveLogged() {
	m.MeasurementsLogged.Inc()
}

func (m *Metrics) SetFlowsInFlight(n int64) {
	m.flows.Store(n)
	m.FlowsInFlight.Set(float64(n))
}

func (m *Metrics) UpdateStateStats(samples int, memoryBytes uint64) {
	m.StateSamples.Set(float64(samples))
	m.StateMemoryBytes.Set(float64(memoryBytes))
}

func (m *Metrics) ApplyKernelStats(synAckObserved, synAckSampled, ackMatched, incomplete, ringbufDrop, parseErrors uint64) {
	if synAckObserved > 0 {
		m.SynAcksObserved.Add(float64(synAckObserved))
	}
	_ = synAckSampled
	if incomplete > 0 {
		m.Incomplete.Add(float64(incomplete))
	}
	if ringbufDrop > 0 {
		m.RingbufDropped.Add(float64(ringbufDrop))
	}
	if parseErrors > 0 {
		m.ParseErrors.Add(float64(parseErrors))
	}
	m.SetFlowsInFlight(int64(synAckSampled) - int64(ackMatched))
	if m.flows.Load() < 0 {
		m.SetFlowsInFlight(0)
	}
}
