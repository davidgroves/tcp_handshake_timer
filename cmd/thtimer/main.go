//go:build linux

package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dwg/tcp_handshake_timer/internal/api"
	"github.com/dwg/tcp_handshake_timer/internal/config"
	"github.com/dwg/tcp_handshake_timer/internal/ebpf"
	"github.com/dwg/tcp_handshake_timer/internal/logging"
	"github.com/dwg/tcp_handshake_timer/internal/metrics"
	"github.com/dwg/tcp_handshake_timer/internal/pipeline"
	"github.com/dwg/tcp_handshake_timer/internal/state"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	cfg, err := config.Load()
	if err != nil {
		slog.Error("load config", "err", err)
		os.Exit(1)
	}

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: cfg.SlogLevel()})))

	store, err := state.New(cfg)
	if err != nil {
		slog.Error("init state store", "err", err)
		os.Exit(1)
	}

	m := metrics.New()
	logger := logging.New(cfg.LogSamplePct, cfg.SlogLevel())

	loader, err := ebpf.NewLoader(cfg)
	if err != nil {
		slog.Error("load ebpf", "err", err)
		os.Exit(1)
	}
	defer loader.Close()

	slog.Info("ebpf attached", "mode", loader.Mode(), "interfaces", cfg.Interfaces)

	consumer := pipeline.NewConsumer(loader, m, store, logger)
	apiServer := api.New(m, store, cfg.TopN)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		if err := consumer.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			slog.Error("consumer stopped", "err", err)
			cancel()
		}
	}()

	httpServer := &http.Server{
		Addr:    cfg.ListenAddr,
		Handler: apiServer.Handler(),
	}
	go func() {
		slog.Info("http listening", "addr", cfg.ListenAddr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("http server", "err", err)
			cancel()
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	slog.Info("shutting down")

	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	_ = httpServer.Shutdown(shutdownCtx)
}
