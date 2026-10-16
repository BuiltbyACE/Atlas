// Command atlas-api is the Go core HTTP service (docs/ATLAS.md §4.1, §5.1).
//
// It owns the business rules, the database writes, and the public/internal API.
// It is the only writer to business schemas (ADR-002).
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/safaristack/atlas/apps/api/internal/approvals"
	"github.com/safaristack/atlas/apps/api/internal/audit"
	"github.com/safaristack/atlas/apps/api/internal/auth"
	"github.com/safaristack/atlas/apps/api/internal/files"
	"github.com/safaristack/atlas/apps/api/internal/notifications"
	"github.com/safaristack/atlas/apps/api/internal/platform/config"
	"github.com/safaristack/atlas/apps/api/internal/platform/database"
	"github.com/safaristack/atlas/apps/api/internal/platform/ed25519jwt"
	"github.com/safaristack/atlas/apps/api/internal/platform/httpserver"
	"github.com/safaristack/atlas/apps/api/internal/platform/logging"
	"github.com/safaristack/atlas/apps/api/internal/platform/telemetry"
	"github.com/safaristack/atlas/apps/api/internal/settings"
)

// version is overridden at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if err := run(); err != nil {
		// Logging may not be configured yet, so use a bare logger for fatals.
		slog.Error("atlas-api exited with error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	defaultServiceName("atlas-api")

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

	authService, err := newAuthService(cfg, logger, pool)
	if err != nil {
		return err
	}

	// Notifications: atlas-api owns the write path (records + outbox event in one
	// transaction) and the notification centre. Delivery runs in atlas-worker, so
	// a request here never waits on a provider (§6.4, §8.10).
	notificationService := notifications.NewService(notifications.NewPGStore(pool), logger)
	notificationHandler := notifications.NewHandler(notificationService, logger)

	// Approvals engine. Its fan-out records notification intents through the
	// notifications engine after a state change has committed (§6.3, §6.4).
	auditWriter := audit.NewWriter(pool)
	approvalService := approvals.NewService(approvals.NewPGStore(pool), auditWriter,
		approvals.NewNotificationsAdapter(notificationService), logger)

	// Settings are read through the API and changed by administrators; every
	// write is versioned and audited (§6.7).
	settingsService := settings.NewService(settings.NewPGStore(pool), auditWriter, logger)

	// Files: bytes go straight to object storage (R2, or SeaweedFS locally)
	// through pre-signed URLs; Go keeps the metadata and emits core.file.*
	// events (§6.5). Storage is optional in local development: when no bucket
	// is configured the file routes are simply not mounted.
	var filesHandler *files.Handler
	if cfg.ObjectStorageConfigured() {
		objects, err := files.NewS3Store(ctx, files.S3Config{
			Endpoint:  cfg.S3Endpoint,
			Region:    cfg.S3Region,
			Bucket:    cfg.S3Bucket,
			AccessKey: cfg.S3AccessKey,
			SecretKey: cfg.S3SecretKey,
			PathStyle: cfg.S3PathStyle,
		})
		if err != nil {
			return err
		}
		fileService := files.NewService(files.NewPGStore(pool), objects, files.NoopScanner{}, auditWriter, files.Config{
			Bucket:      cfg.S3Bucket,
			UploadTTL:   cfg.FileUploadTTL,
			DownloadTTL: cfg.FileDownloadTTL,
		}, logger)
		filesHandler = files.NewHandler(fileService, logger)
	} else {
		logger.Warn("S3_BUCKET is not set; file routes are disabled")
	}

	auditService := audit.NewService(audit.NewPGStore(pool), logger)

	router := httpserver.NewRouter(httpserver.Deps{
		Cfg:           cfg,
		Logger:        logger,
		DB:            pool,
		Version:       version,
		Auth:          authService,
		Notifications: notificationHandler,
		Approvals:     approvals.NewHandler(approvalService, logger),
		Settings:      settings.NewHandler(settingsService, logger),
		Audit:         audit.NewHandler(auditService, logger),
		Files:         filesHandler,
	})

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           router,
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("http listening", slog.String("addr", cfg.HTTPAddr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownGrace)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	logger.Info("stopped cleanly")
	return nil
}

// defaultServiceName sets ATLAS_SERVICE_NAME only when the operator has not, so
// each binary logs under its own name without clobbering explicit config.
func defaultServiceName(name string) {
	if os.Getenv("ATLAS_SERVICE_NAME") == "" {
		_ = os.Setenv("ATLAS_SERVICE_NAME", name)
	}
}

// newAuthService assembles the sign-in, session and realtime-token machinery.
func newAuthService(cfg *config.Config, logger *slog.Logger, pool *pgxpool.Pool) (*auth.Service, error) {
	key, err := signingKey(cfg, logger)
	if err != nil {
		return nil, err
	}
	sealer, err := secretSealer(cfg, logger)
	if err != nil {
		return nil, err
	}

	return auth.NewService(auth.NewPGStore(pool), sealer, key, auth.ServiceConfig{
		SessionCookieName:    cfg.SessionCookieName,
		SessionIdleTimeout:   cfg.SessionIdleTimeout,
		SessionAbsoluteTTL:   cfg.SessionAbsoluteTTL,
		MFAIssuer:            cfg.MFAIssuer,
		MFAChallengeAudience: "atlas-mfa",
		MFAChallengeTTL:      5 * time.Minute,
		RealtimeIssuer:       cfg.RealtimeIssuer,
		RealtimeAudience:     cfg.RealtimeAudience,
		RealtimeTTL:          cfg.RealtimeTokenTTL,
		ServiceAudience:      cfg.ServiceAudience,
		LoginMaxFailures:     5,
		LoginWindow:          15 * time.Minute,
	}, logger), nil
}

// signingKey parses the configured Ed25519 key, or generates an ephemeral one
// in local development so a checkout runs without extra setup. The public half
// is published as JWKS either way.
func signingKey(cfg *config.Config, logger *slog.Logger) (*ed25519jwt.Key, error) {
	if material := strings.TrimSpace(cfg.RealtimePrivateKey); material != "" {
		priv, err := ed25519jwt.ParsePrivateKey(material)
		if err != nil {
			return nil, fmt.Errorf("ATLAS_REALTIME_PRIVATE_KEY: %w", err)
		}
		return ed25519jwt.FromPrivateKey(priv), nil
	}
	logger.Warn("ATLAS_REALTIME_PRIVATE_KEY is empty; generated an ephemeral key (local development only)")
	return ed25519jwt.Generate()
}

// secretSealer builds the AES-GCM sealer for short-lived secrets.
func secretSealer(cfg *config.Config, logger *slog.Logger) (*auth.Sealer, error) {
	if material := strings.TrimSpace(cfg.EncryptionKey); material != "" {
		sealer, err := auth.NewSealer(material)
		if err != nil {
			return nil, fmt.Errorf("ATLAS_ENCRYPTION_KEY: %w", err)
		}
		return sealer, nil
	}

	generated, err := auth.GenerateEncryptionKey()
	if err != nil {
		return nil, err
	}
	logger.Warn("ATLAS_ENCRYPTION_KEY is empty; generated an ephemeral key (local development only; MFA secrets will not survive a restart)")
	return auth.NewSealer(generated)
}
