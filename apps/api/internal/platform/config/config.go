// Package config loads Atlas runtime configuration from the environment.
//
// Configuration is environment-driven and never hard-coded (docs/ATLAS.md §2.10).
// Values that the business tunes at runtime (approval thresholds, report times,
// escalation windows) do NOT live here — they live in the core.settings and
// core.*_policies tables (§6.7). This package is only for process bootstrap.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Environment names. Anything other than "production" is treated as non-production.
const (
	EnvLocal      = "local"
	EnvStaging    = "staging"
	EnvProduction = "production"
)

// Config is the fully-resolved runtime configuration for a Go Atlas binary.
type Config struct {
	Env         string
	ServiceName string

	// HTTPAddr is the listen address for atlas-api, e.g. ":8080".
	HTTPAddr string

	LogLevel  string // debug | info | warn | error
	LogFormat string // text | json

	// DatabaseURL is used by atlas-api and atlas-worker (business schemas).
	DatabaseURL string
	// MigrateURL is used by atlas-cli; it must be a role that may create schemas.
	MigrateURL string

	// PublicBaseURL is the externally visible base URL, used to build links
	// in notifications and webhooks.
	PublicBaseURL string

	ReadTimeout   time.Duration
	WriteTimeout  time.Duration
	IdleTimeout   time.Duration
	ShutdownGrace time.Duration

	// ── Authentication and sessions (§6.1) ───────────────────────
	// SessionCookieName is the name of the opaque session cookie.
	SessionCookieName string
	// SessionIdleTimeout and SessionAbsoluteTTL bound a session's life: it ends
	// after SessionIdleTimeout without use, and always after SessionAbsoluteTTL.
	SessionIdleTimeout time.Duration
	SessionAbsoluteTTL time.Duration
	// MFAIssuer is the issuer shown in authenticator apps when enrolling TOTP.
	MFAIssuer string
	// EncryptionKey is a base64-encoded 32-byte AES key used to encrypt
	// short-lived secrets at rest (the TOTP secret). Staging and production
	// must supply it from the secret manager; local development generates one
	// at boot.
	EncryptionKey string

	// ── Realtime tokens (§6.1, ADR-008) ──────────────────────────
	// RealtimePrivateKey is a base64-encoded PKCS#8 Ed25519 private key. Go
	// signs short-lived realtime tokens with it and publishes the public half as
	// JWKS for Elixir to verify. When empty in local development a key is
	// generated at boot; staging and production must supply it (secret manager).
	RealtimePrivateKey string
	RealtimeIssuer     string
	RealtimeAudience   string
	RealtimeTokenTTL   time.Duration
	// ServiceAudience is the `aud` expected on service-to-service tokens (§6.1).
	ServiceAudience string

	// ── Notifications (§6.4, §8.10) ──────────────────────────────
	// SMTP is where the email adapter sends: Mailpit locally, a transactional
	// provider in staging and production [DECIDE].
	SMTPHost     string
	SMTPPort     int
	SMTPFrom     string
	SMTPUsername string
	SMTPPassword string
	SMTPTLS      bool
	// Notification dispatch tuning (atlas-worker). Values are configuration,
	// not code (§2.10).
	NotifyBatchSize   int
	NotifyWorkers     int
	NotifySendTimeout time.Duration

	// ── Approvals (§6.3) ─────────────────────────────────────────
	// Overdue approvals are expired by atlas-worker on a poll. Both values are
	// configuration (§2.10), not code.
	ApprovalSweepInterval time.Duration
	ApprovalSweepBatch    int
}

// Load reads configuration from the process environment, applying defaults.
// It returns an error for values that are present but invalid or missing.
func Load() (*Config, error) {
	cfg := &Config{
		Env:           get("ATLAS_ENV", EnvLocal),
		ServiceName:   get("ATLAS_SERVICE_NAME", "atlas-api"),
		HTTPAddr:      get("ATLAS_HTTP_ADDR", ":8080"),
		LogLevel:      get("ATLAS_LOG_LEVEL", "info"),
		LogFormat:     get("ATLAS_LOG_FORMAT", "json"),
		DatabaseURL:   get("ATLAS_DATABASE_URL", ""),
		MigrateURL:    get("ATLAS_MIGRATE_DATABASE_URL", ""),
		PublicBaseURL: get("ATLAS_PUBLIC_BASE_URL", "http://localhost:8080"),
		ReadTimeout:   duration("ATLAS_HTTP_READ_TIMEOUT", 15*time.Second),
		WriteTimeout:  duration("ATLAS_HTTP_WRITE_TIMEOUT", 30*time.Second),
		IdleTimeout:   duration("ATLAS_HTTP_IDLE_TIMEOUT", 60*time.Second),
		ShutdownGrace: duration("ATLAS_SHUTDOWN_GRACE", 20*time.Second),

		SessionCookieName:  get("ATLAS_SESSION_COOKIE", "atlas_session"),
		SessionIdleTimeout: duration("ATLAS_SESSION_IDLE_TIMEOUT", 12*time.Hour),
		SessionAbsoluteTTL: duration("ATLAS_SESSION_ABSOLUTE_TTL", 7*24*time.Hour),
		MFAIssuer:          get("ATLAS_MFA_ISSUER", "SafariStack Atlas"),
		EncryptionKey:      get("ATLAS_ENCRYPTION_KEY", ""),

		RealtimePrivateKey: get("ATLAS_REALTIME_PRIVATE_KEY", ""),
		RealtimeIssuer:     get("ATLAS_REALTIME_ISSUER", "atlas"),
		RealtimeAudience:   get("ATLAS_REALTIME_AUDIENCE", "atlas-realtime"),
		RealtimeTokenTTL:   duration("ATLAS_REALTIME_TOKEN_TTL", 5*time.Minute),
		ServiceAudience:    get("ATLAS_SERVICE_AUDIENCE", "atlas-internal"),

		SMTPHost:     get("SMTP_HOST", "localhost"),
		SMTPPort:     integer("SMTP_PORT", 1025),
		SMTPFrom:     get("SMTP_FROM", "atlas@localhost"),
		SMTPUsername: get("SMTP_USERNAME", ""),
		SMTPPassword: get("SMTP_PASSWORD", ""),
		SMTPTLS:      boolean("SMTP_TLS", false),

		NotifyBatchSize:   integer("ATLAS_NOTIFY_BATCH_SIZE", 100),
		NotifyWorkers:     integer("ATLAS_NOTIFY_WORKERS", 8),
		NotifySendTimeout: duration("ATLAS_NOTIFY_SEND_TIMEOUT", 10*time.Second),

		ApprovalSweepInterval: duration("ATLAS_APPROVAL_SWEEP_INTERVAL", time.Minute),
		ApprovalSweepBatch:    integer("ATLAS_APPROVAL_SWEEP_BATCH", 100),
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// MustLoad is Load but panics on error. For use in main() only.
func MustLoad() *Config {
	cfg, err := Load()
	if err != nil {
		panic(fmt.Sprintf("invalid configuration: %v", err))
	}
	return cfg
}

// Validate checks that the configuration is internally consistent.
func (c *Config) Validate() error {
	switch c.Env {
	case EnvLocal, EnvStaging, EnvProduction:
	default:
		return fmt.Errorf("ATLAS_ENV must be one of local|staging|production, got %q", c.Env)
	}

	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("ATLAS_LOG_LEVEL must be one of debug|info|warn|error, got %q", c.LogLevel)
	}

	switch c.LogFormat {
	case "text", "json":
	default:
		return fmt.Errorf("ATLAS_LOG_FORMAT must be one of text|json, got %q", c.LogFormat)
	}

	if c.DatabaseURL == "" {
		return fmt.Errorf("ATLAS_DATABASE_URL is required (copy .env.example to .env and run via `task`)")
	}

	if c.Env == EnvProduction && c.LogFormat != "json" {
		return fmt.Errorf("ATLAS_LOG_FORMAT must be json in production")
	}

	if c.SessionAbsoluteTTL <= 0 {
		return fmt.Errorf("ATLAS_SESSION_ABSOLUTE_TTL must be positive")
	}
	if c.SessionIdleTimeout <= 0 || c.SessionIdleTimeout > c.SessionAbsoluteTTL {
		return fmt.Errorf("ATLAS_SESSION_IDLE_TIMEOUT must be positive and no greater than the absolute lifetime")
	}

	if c.Env == EnvProduction && c.RealtimePrivateKey == "" {
		return fmt.Errorf("ATLAS_REALTIME_PRIVATE_KEY is required outside local development (Ed25519, base64 PKCS#8)")
	}

	if c.Env == EnvProduction && c.EncryptionKey == "" {
		return fmt.Errorf("ATLAS_ENCRYPTION_KEY is required outside local development (base64, 32 bytes)")
	}

	if c.ApprovalSweepInterval <= 0 {
		return fmt.Errorf("ATLAS_APPROVAL_SWEEP_INTERVAL must be positive")
	}
	if c.ApprovalSweepBatch <= 0 {
		return fmt.Errorf("ATLAS_APPROVAL_SWEEP_BATCH must be positive")
	}

	return nil
}

// IsLocal reports whether this process is running in the local development environment.
func (c *Config) IsLocal() bool { return c.Env == EnvLocal }

// IsProduction reports whether this process is running in production.
func (c *Config) IsProduction() bool { return c.Env == EnvProduction }

// ── helpers ──────────────────────────────────────────────────

func get(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && strings.TrimSpace(v) != "" {
		return v
	}
	return def
}

func duration(key string, def time.Duration) time.Duration {
	raw := get(key, "")
	if raw == "" {
		return def
	}
	// Accept both a Go duration ("15s") and bare seconds ("15").
	if d, err := time.ParseDuration(raw); err == nil {
		return d
	}
	if n, err := strconv.Atoi(raw); err == nil {
		return time.Duration(n) * time.Second
	}
	return def
}

func integer(key string, def int) int {
	raw := get(key, "")
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return def
	}
	return n
}

func boolean(key string, def bool) bool {
	switch strings.ToLower(get(key, "")) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return def
	}
}
