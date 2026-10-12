// Package telemetry wires OpenTelemetry tracing for Atlas (docs/ATLAS.md §14).
//
// Tracing is opt-in: if OTEL_EXPORTER_OTLP_ENDPOINT is unset the SDK is not
// installed and tracing becomes a no-op, so the service runs fine with no
// collector present (as it must in local development and CI).
package telemetry

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/safaristack/atlas/apps/api/internal/platform/config"
)

// Init installs a global TracerProvider when an OTLP endpoint is configured.
// The returned function flushes and shuts the provider down; it is always safe
// to call, even when tracing is disabled.
func Init(ctx context.Context, cfg *config.Config, logger *slog.Logger) (func(context.Context) error, error) {
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" {
		logger.Info("telemetry disabled: OTEL_EXPORTER_OTLP_ENDPOINT is not set")
		return func(context.Context) error { return nil }, nil
	}

	exp, err := otlptracehttp.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("create OTLP trace exporter: %w", err)
	}

	res, err := resource.New(ctx,
		resource.WithFromEnv(),
		resource.WithTelemetrySDK(),
		resource.WithAttributes(
			attribute.String("service.name", cfg.ServiceName),
			attribute.String("deployment.environment", cfg.Env),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("build OpenTelemetry resource: %w", err)
	}

	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp, sdktrace.WithBatchTimeout(5*time.Second)),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(sampleRatio(cfg)))),
	)
	otel.SetTracerProvider(provider)

	logger.Info("telemetry enabled", slog.String("otlp_endpoint", endpoint))

	return provider.Shutdown, nil
}

func sampleRatio(cfg *config.Config) float64 {
	if cfg.IsProduction() {
		return 0.1
	}
	return 1.0
}
