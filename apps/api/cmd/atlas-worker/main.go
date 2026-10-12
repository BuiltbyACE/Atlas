// Command atlas-worker runs Atlas's scheduled and asynchronous Go work
// (docs/ATLAS.md §4.1, §5.1): deadline sweeps, due-date checks, and other
// time-driven jobs that produce events.
//
// Phase 0 provides the process skeleton — boot, database, structured logs and
// graceful shutdown — with a heartbeat loop. The real job runner (River, on
// PostgreSQL, per ADR-001 and ADR-003) is introduced in Phase 1 when there are
// jobs to run. Until then this binary must not pretend to do work it does not
// do: the loop logs and does nothing else.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/safaristack/atlas/apps/api/internal/platform/config"
	"github.com/safaristack/atlas/apps/api/internal/platform/database"
	"github.com/safaristack/atlas/apps/api/internal/platform/logging"
	"github.com/safaristack/atlas/apps/api/internal/platform/telemetry"
)

var version = "dev"

func main() {
	if err := run(); err != nil {
		slog.Error("atlas-worker exited with error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	if os.Getenv("ATLAS_SERVICE_NAME") == "" {
		_ = os.Setenv("ATLAS_SERVICE_NAME", "atlas-worker")
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	logger := logging.New(cfg)
	logger.Info("starting", slog.String("version", version), slog.String("env", cfg.Env))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	shutdownTracing, err := telemetry.Init(ctx, cfg, logger)
	if err != nil {
		return err
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = shutdownTracing(shutdownCtx)
	}()

	pool, err := database.Connect(ctx, cfg.DatabaseURL, logger)
	if err != nil {
		return err
	}
	defer pool.Close()

	return loop(ctx, logger, pool)
}

// loop is the worker's heartbeat. It keeps the process shape honest and
// observable until the Phase 1 job runner replaces it.
func loop(ctx context.Context, logger *slog.Logger, pool *pgxpool.Pool) error {
	const interval = 30 * time.Second

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	logger.Info("worker ready", slog.Duration("heartbeat", interval))

	for {
		select {
		case <-ctx.Done():
			logger.Info("worker stopping")
			return nil
		case <-ticker.C:
			// Prove the database is still reachable; a worker that cannot see
			// the database is not working, it is idle in the worst way.
			pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			err := pool.Ping(pingCtx)
			cancel()
			if err != nil {
				logger.Warn("heartbeat: database unreachable", slog.String("error", err.Error()))
				continue
			}
			logger.Debug("heartbeat: database ok (no jobs registered yet; see Phase 1)")
		}
	}
}
