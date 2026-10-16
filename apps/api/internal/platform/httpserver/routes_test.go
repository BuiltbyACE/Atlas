package httpserver

import (
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/safaristack/atlas/apps/api/internal/auth"
	"github.com/safaristack/atlas/apps/api/internal/platform/config"
	"github.com/safaristack/atlas/apps/api/internal/platform/ed25519jwt"
	"github.com/safaristack/atlas/apps/api/internal/platform/routing"
	"github.com/safaristack/atlas/apps/api/internal/seed"
	"github.com/safaristack/atlas/apps/api/internal/work"
)

// emptyStore satisfies auth.Store without a database; these tests never call
// its methods.
type emptyStore struct{ auth.Store }

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func testService(t *testing.T) *auth.Service {
	t.Helper()

	key, err := ed25519jwt.Generate()
	if err != nil {
		t.Fatalf("generate signing key: %v", err)
	}
	encoded, err := auth.GenerateEncryptionKey()
	if err != nil {
		t.Fatalf("generate encryption key: %v", err)
	}
	sealer, err := auth.NewSealer(encoded)
	if err != nil {
		t.Fatalf("new sealer: %v", err)
	}

	return auth.NewService(&emptyStore{}, sealer, key, auth.ServiceConfig{
		SessionCookieName:  "atlas_session",
		SessionIdleTimeout: 12 * time.Hour,
		SessionAbsoluteTTL: 7 * 24 * time.Hour,
	}, discardLogger())
}

func testDeps(t *testing.T) Deps {
	t.Helper()
	logger := discardLogger()
	return Deps{
		Cfg: &config.Config{
			Env:               config.EnvLocal,
			ServiceName:       "atlas-api",
			LogLevel:          "error",
			LogFormat:         "text",
			DatabaseURL:       "postgres://test",
			SessionCookieName: "atlas_session",
		},
		Logger:  logger,
		Version: "test",
		Auth:    testService(t),
		// Mounting work ensures its routes are covered by the registry and
		// permission-catalogue checks above. The store is never called here.
		Work: work.NewHandler(work.NewService(work.NewPGStore(nil), nil, nil, logger), logger),
	}
}

// TestEveryMountedRouteIsDeclared walks the mounted router and compares it with
// the registry: a route served without a declaration (or declared without being
// mounted) fails the build (§6.2).
func TestEveryMountedRouteIsDeclared(t *testing.T) {
	handler, reg := build(testDeps(t))

	mux, ok := handler.(*chi.Mux)
	if !ok {
		t.Fatalf("handler is %T, want *chi.Mux", handler)
	}

	mounted := map[string]bool{}
	err := chi.Walk(mux, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		mounted[method+" "+route] = true
		return nil
	})
	if err != nil {
		t.Fatalf("walk router: %v", err)
	}

	declared := map[string]bool{}
	for _, rt := range reg.Routes() {
		key := rt.Method + " " + rt.Pattern
		declared[key] = true
		if !mounted[key] {
			t.Errorf("declared route %s is not mounted", key)
		}
	}
	for key := range mounted {
		if !declared[key] {
			t.Errorf("mounted route %s is not in the registry", key)
		}
	}
}

// TestGatedPermissionsExist enforces §6.2: a gated route must name a permission
// that exists in the catalogue.
func TestGatedPermissionsExist(t *testing.T) {
	catalogue := map[string]bool{}
	for _, p := range seed.Permissions {
		catalogue[p.Key] = true
	}

	_, reg := build(testDeps(t))
	for _, rt := range reg.Routes() {
		if rt.Access != routing.Permission {
			continue
		}
		if rt.Permission == "" {
			t.Errorf("route %s %s is gated but names no permission", rt.Method, rt.Pattern)
			continue
		}
		if !catalogue[rt.Permission] {
			t.Errorf("route %s %s requires %q, which is not in the catalogue", rt.Method, rt.Pattern, rt.Permission)
		}
	}
}
