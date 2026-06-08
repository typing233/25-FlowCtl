package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowctl/flowctl/internal/config"
	"github.com/flowctl/flowctl/internal/engine"
	"github.com/flowctl/flowctl/internal/runner"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		logger.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	poolCfg, err := pgxpool.ParseConfig(cfg.Database.URL)
	if err != nil {
		logger.Error("invalid database URL", "error", err)
		os.Exit(1)
	}
	poolCfg.MaxConns = int32(cfg.Database.MaxConns)
	poolCfg.MinConns = int32(cfg.Database.MinConns)

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		logger.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		logger.Error("failed to ping database", "error", err)
		os.Exit(1)
	}

	logger.Info("worker starting", "node_id", cfg.Scheduler.NodeID)

	runnerFactory := runner.DefaultFactory()
	executor := engine.NewExecutor(pool, runnerFactory, logger)
	scheduler := engine.NewScheduler(pool, cfg.Scheduler, executor, logger)

	if err := scheduler.Start(ctx); err != nil {
		logger.Error("failed to start scheduler", "error", err)
		os.Exit(1)
	}
	defer scheduler.Stop()

	logger.Info("worker started, waiting for work...")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("worker shutting down...")
	cancel()
	logger.Info("worker stopped")
}
