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
