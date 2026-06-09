package logging

import (
	"log/slog"
	"math/rand/v2"
	"os"

	"codeberg.org/davidgroves/tcp_handshake_timer/internal/measurement"
)

type SampledLogger struct {
	logger   *slog.Logger
	samplePct uint32
}

func New(samplePct uint32, level slog.Level) *SampledLogger {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	return &SampledLogger{
		logger:    slog.New(handler),
		samplePct: samplePct,
	}
}

func (l *SampledLogger) ShouldLog() bool {
	if l.samplePct >= 100 {
		return true
	}
	if l.samplePct == 0 {
		return false
	}
	return uint32(rand.IntN(100)) < l.samplePct
}

func (l *SampledLogger) LogMeasurement(m measurement.Measurement) {
	if !l.ShouldLog() {
		return
	}
	l.logger.Info("handshake_rtt",
		"client_ip", m.ClientIP.String(),
		"client_port", m.ClientPort,
		"server_ip", m.ServerIP.String(),
		"server_port", m.ServerPort,
		"rtt_ns", m.RTT.Nanoseconds(),
		"timestamp", m.Time,
	)
}
