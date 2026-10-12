// Package logging builds the structured logger used across Atlas.
//
// Structured JSON logs only in staged/production; human-readable text locally
// (docs/ATLAS.md §14). Never log secrets or full personal data.
package logging

import (
	"log/slog"
	"os"
	"strings"

	"github.com/safaristack/atlas/apps/api/internal/platform/config"
)

// New returns a *slog.Logger configured from cfg.
func New(cfg *config.Config) *slog.Logger {
	level := parseLevel(cfg.LogLevel)

	opts := &slog.HandlerOptions{
		Level:     level,
		AddSource: cfg.Env != config.EnvProduction,
	}

	var handler slog.Handler
	switch cfg.LogFormat {
	case "text":
		handler = slog.NewTextHandler(os.Stdout, opts)
	default:
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}

	logger := slog.New(handler).With(
		slog.String("service", cfg.ServiceName),
		slog.String("env", cfg.Env),
	)
	return logger
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
