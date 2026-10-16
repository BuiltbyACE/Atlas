// Command atlas-worker runs Atlas's asynchronous and time-driven Go work
// (docs/ATLAS.md §4.1, §5.1).
//
// It runs two jobs. The notification dispatcher (§6.4) leases due rows that
// producers (atlas-api) recorded with their outbox event in one transaction and
// performs the provider calls with a timeout, retries, backoff and a circuit
// breaker (§8.10). The approvals expiry sweeper (§6.3) expires approvals whose
// deadline passed and fans out the core.approval.expired.v1 event. Both are
// time-driven, which is why they live here and not in the request path: it is
// what makes the write path fast and the SLOs in §11 hold.
//
// The heartbeat loop stays: a worker that cannot see the database is idle in
// the worst way, so it is logged rather than assumed.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/safaristack/atlas/apps/api/internal/adapters/email"
	"github.com/safaristack/atlas/apps/api/internal/approvals"
	"github.com/safaristack/atlas/apps/api/internal/audit"
	"github.com/safaristack/atlas/apps/api/internal/notifications"
	"github.com/safaristack/atlas/apps/api/internal/platform/config"
	"github.com/safaristack/atlas/apps/api/internal/platform/database"
	"github.com/safaristack/atlas/apps/api/internal/platform/logging"
	"github.com/safaristack/atlas/apps/api/internal/platform/telemetry"
	"github.com/safaristack/atlas/apps/api/internal/work"
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

	dispatcher := notifications.NewDispatcher(
		notifications.NewPGStore(pool),
		[]notifications.Sender{
			email.New(email.Config{
				Host:     cfg.SMTPHost,
				Port:     cfg.SMTPPort,
				From:     cfg.SMTPFrom,
				Username: cfg.SMTPUsername,
				Password: cfg.SMTPPassword,
				TLS:      cfg.SMTPTLS,
			}),
		},
		dispatcherConfig(cfg),
		logger,
	)

	notificationService := notifications.NewService(notifications.NewPGStore(pool), logger)
	approvalService := approvals.NewService(approvals.NewPGStore(pool), audit.NewWriter(pool),
		approvals.NewNotificationsAdapter(notificationService), logger)
	sweeper := approvals.NewSweeper(approvalService, cfg.ApprovalSweepInterval, cfg.ApprovalSweepBatch, logger)

	// The overdue-work escalation ladder is a second time-driven job (§3.2,
	// §5.8): it raises escalations for overdue tasks and resolves the ones whose
	// task is no longer late.
	workService := work.NewService(work.NewPGStore(pool), nil, audit.NewWriter(pool), logger)
	workSweeper := work.NewSweeper(workService, cfg.WorkSweepInterval, cfg.WorkSweepBatch, logger)

	// Run the dispatcher, the two sweepers and the heartbeat side by side;
	// whichever ends first (a fatal engine error, or a shutdown signal) ends the
	// process.
	dispatchErr := make(chan error, 1)
	go func() { dispatchErr <- dispatcher.Run(ctx) }()

	sweepErr := make(chan error, 1)
	go func() { sweepErr <- sweeper.Run(ctx) }()

	workSweepErr := make(chan error, 1)
	go func() { workSweepErr <- workSweeper.Run(ctx) }()

	heartbeatErr := make(chan error, 1)
	go func() { heartbeatErr <- loop(ctx, logger, pool) }()

	select {
	case err := <-dispatchErr:
		return err
	case err := <-sweepErr:
		return err
	case err := <-workSweepErr:
		return err
	case err := <-heartbeatErr:
		return err
	case <-ctx.Done():
		return nil
	}
}

// dispatcherConfig maps process configuration onto the delivery engine's tuning.
func dispatcherConfig(cfg *config.Config) notifications.DispatcherConfig {
	d := notifications.DefaultDispatcherConfig()
	if cfg.NotifyBatchSize > 0 {
		d.BatchSize = cfg.NotifyBatchSize
	}
	if cfg.NotifyWorkers > 0 {
		d.PerProvider = cfg.NotifyWorkers
	}
	if cfg.NotifySendTimeout > 0 {
		d.SendTimeout = cfg.NotifySendTimeout
	}
	return d
}

// loop is the worker's heartbeat. It keeps the process shape honest and
// observable alongside the dispatcher.
func loop(ctx context.Context, logger *slog.Logger, pool *pgxpool.Pool) error {
	const interval = 30 * time.Second

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	logger.Info("worker heartbeat ready", slog.Duration("heartbeat", interval))

	for {
		select {
		case <-ctx.Done():
			logger.Info("worker heartbeat stopping")
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
			logger.Debug("heartbeat: database ok")
		}
	}
}
