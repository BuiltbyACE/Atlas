package httpserver

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/safaristack/atlas/apps/api/internal/platform/config"
	"github.com/safaristack/atlas/apps/api/internal/platform/httpx"
)

func testRouter(env string) http.Handler {
	cfg := &config.Config{
		Env:         env,
		ServiceName: "atlas-api",
		LogLevel:    "error",
		LogFormat:   "text",
		DatabaseURL: "postgres://test",
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewRouter(Deps{Cfg: cfg, Logger: logger, DB: nil, Version: "test"})
}

func TestLivenessNeedsNoDependencies(t *testing.T) {
	rec := httptest.NewRecorder()
	testRouter(config.EnvLocal).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/health", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if rec.Header().Get(httpx.HeaderRequestID) == "" {
		t.Error("every response must carry an X-Request-Id")
	}
}

func TestReadinessFailsWithoutDatabase(t *testing.T) {
	rec := httptest.NewRecorder()
	testRouter(config.EnvLocal).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/health/ready", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 when the database pool is absent", rec.Code)
	}

	var report struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &report); err != nil {
		t.Fatalf("decode readiness body: %v", err)
	}
	if report.Status != "down" {
		t.Errorf("readiness status = %q, want down", report.Status)
	}
}

func TestUnknownRouteReturnsProblem(t *testing.T) {
	rec := httptest.NewRecorder()
	testRouter(config.EnvLocal).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/nope", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != httpx.ProblemContentType {
		t.Errorf("content-type = %q, want %q", ct, httpx.ProblemContentType)
	}
}

