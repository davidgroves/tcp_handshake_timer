package pipeline

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/cilium/ebpf/ringbuf"

	"codeberg.org/davidgroves/tcp_handshake_timer/internal/ebpf"
	"codeberg.org/davidgroves/tcp_handshake_timer/internal/logging"
	"codeberg.org/davidgroves/tcp_handshake_timer/internal/measurement"
	"codeberg.org/davidgroves/tcp_handshake_timer/internal/metrics"
	"codeberg.org/davidgroves/tcp_handshake_timer/internal/state"
)

type Consumer struct {
	loader  *ebpf.Loader
	metrics *metrics.Metrics
	store   *state.Store
	logger  *logging.SampledLogger
	log     *slog.Logger
}

func NewConsumer(loader *ebpf.Loader, m *metrics.Metrics, store *state.Store, logger *logging.SampledLogger) *Consumer {
	return &Consumer{
		loader:  loader,
		metrics: m,
		store:   store,
		logger:  logger,
		log:     slog.Default(),
	}
}

func (c *Consumer) Run(ctx context.Context) error {
	reader, err := ringbuf.NewReader(c.loader.Events)
	if err != nil {
		return err
	}
	defer reader.Close()

	statsTicker := time.NewTicker(5 * time.Second)
	defer statsTicker.Stop()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-statsTicker.C:
				c.scrapeStats()
				samples, mem := c.store.SnapshotCounts()
				c.metrics.UpdateStateStats(samples, mem)
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		record, err := reader.Read()
		if err != nil {
			if errors.Is(err, ringbuf.ErrClosed) {
				return nil
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			c.log.Warn("ringbuf read error", "err", err)
			continue
		}

		m, err := measurement.DecodeEvent(record.RawSample)
		if err != nil {
			c.log.Warn("decode measurement", "err", err)
			continue
		}
		c.handleMeasurement(m)
	}
}

func (c *Consumer) handleMeasurement(m measurement.Measurement) {
	c.metrics.ObserveMeasurement(float64(m.RTT) / float64(time.Second))
	c.store.Add(m)
	if c.logger.ShouldLog() {
		c.logger.LogMeasurement(m)
		c.metrics.ObserveLogged()
	}
}

func (c *Consumer) scrapeStats() {
	stats, err := c.loader.ReadStats()
	if err != nil {
		c.log.Warn("read kernel stats", "err", err)
		return
	}
	c.metrics.ApplyKernelStats(
		stats[measurement.StatSynAckObserved],
		stats[measurement.StatSynAckSampled],
		stats[measurement.StatAckMatched],
		stats[measurement.StatIncomplete],
		stats[measurement.StatRingbufDrop],
		stats[measurement.StatParseError],
	)
}
