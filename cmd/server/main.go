package main

import (
	"context"
	"errors"
	"flag"
	platform "github.com/enterprise-labs/quota-decision-plane/internal/platform/application"
	runtimepkg "github.com/enterprise-labs/quota-decision-plane/internal/platform/infrastructure"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	configPath := flag.String("config", "configs/local.yaml", "path to YAML configuration")
	flag.Parse()
	cfg, err := platform.LoadConfig(*configPath)
	if err != nil {
		slog.Error("configuration failed", "error", err)
		os.Exit(1)
	}
	level := slog.LevelInfo
	if cfg.LogLevel == "debug" {
		level = slog.LevelDebug
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	runtime, err := runtimepkg.NewRuntime(ctx, cfg, logger)
	if err != nil {
		logger.Error("runtime initialization failed", "error", err)
		os.Exit(1)
	}
	runtime.RunBackground(ctx)
	errorsCh := make(chan error, 1)
	go func() {
		logger.Info("quota decision plane started", "address", cfg.Address, "node_id", cfg.NodeID)
		errorsCh <- runtime.Server.ListenAndServe()
	}()
	select {
	case err = <-errorsCh:
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Error("HTTP server failed", "error", err)
			os.Exit(1)
		}
	case <-ctx.Done():
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		if err = runtime.Shutdown(shutdownCtx); err != nil {
			logger.Error("graceful shutdown failed", "error", err)
			os.Exit(1)
		}
		err = <-errorsCh
		if !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server termination failed", "error", err)
			os.Exit(1)
		}
	}
	logger.Info("quota decision plane stopped")
}
