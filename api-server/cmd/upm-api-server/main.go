package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/weiqinzhou3/upmio-clickhouse-hackathon/api-server/internal/api"
	"github.com/weiqinzhou3/upmio-clickhouse-hackathon/api-server/internal/config"
	"github.com/weiqinzhou3/upmio-clickhouse-hackathon/api-server/internal/kube"
	"github.com/weiqinzhou3/upmio-clickhouse-hackathon/api-server/internal/prometheus"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg := config.FromEnv()

	prometheusClient := prometheus.NewClient(cfg.PrometheusBaseURL, cfg.PrometheusTimeout)
	store, err := kube.NewInClusterStore(prometheusClient, cfg.DiagnosticsThresholds)
	if err != nil {
		logger.Error("initialize Kubernetes clients", "error", err)
		os.Exit(1)
	}

	server := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           api.NewServer(store, logger, cfg.RequestTimeout),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		logger.Info("upm-api-server starting", "listenAddr", cfg.ListenAddr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("upm-api-server stopped unexpectedly", "error", err)
			os.Exit(1)
		}
	}()

	signalContext, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	<-signalContext.Done()

	shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownContext); err != nil {
		logger.Error("shutdown upm-api-server", "error", err)
		os.Exit(1)
	}
}
