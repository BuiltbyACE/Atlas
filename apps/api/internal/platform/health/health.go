// Package health implements Atlas liveness and readiness checks
// (docs/ATLAS.md §11, §14).
//
// Liveness answers "is this process alive?". Readiness answers "can it serve
// traffic?" — it checks dependencies such as PostgreSQL and reports the
// consumer-independent outbox depth so operators can see backlog at a glance.
package health

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/safaristack/atlas/apps/api/internal/platform/httpx"
)

// Status values for a single check and for the overall readiness document.
const (
	StatusUp      = "up"
	StatusDown    = "down"
	StatusSkipped = "skipped"
)

// Check is the result of one dependency check.
type Check struct {
	Name      string `json:"name"`
	Status    string `json:"status"`
	Detail    string `json:"detail,omitempty"`
	LatencyMs int64  `json:"latency_ms"`
}

// Report is the JSON body returned by the readiness endpoint.
type Report struct {
	Status string  `json:"status"`
	Checks []Check `json:"checks"`
}

// Checker performs dependency checks against the running system.
type Checker struct {
	DB *pgxpool.Pool
}

// NewChecker returns a Checker bound to the given pool.
func NewChecker(db *pgxpool.Pool) *Checker { return &Checker{DB: db} }

// Liveness never touches dependencies; if the process can answer, it is alive.
func (c *Checker) Liveness() Report {
	return Report{
		Status: StatusUp,
		Checks: []Check{{Name: "process", Status: StatusUp}},
	}
}

// Readiness checks every required dependency. The overall status is "down" if
// any required check fails.
func (c *Checker) Readiness(ctx context.Context) Report {
	report := Report{Status: StatusUp}

	report.Checks = append(report.Checks, c.checkDatabase(ctx))
	report.Checks = append(report.Checks, c.checkOutbox(ctx))

	for _, chk := range report.Checks {
		if chk.Status == StatusDown {
			report.Status = StatusDown
			break
		}
	}
	return report
}

func (c *Checker) checkDatabase(ctx context.Context) Check {
	start := time.Now()
	chk := Check{Name: "database", Status: StatusUp}

	if c.DB == nil {
		chk.Status = StatusDown
		chk.Detail = "no database pool configured"
		return chk
	}

	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	if err := c.DB.Ping(pingCtx); err != nil {
		chk.Status = StatusDown
		chk.Detail = err.Error()
	}
	chk.LatencyMs = time.Since(start).Milliseconds()
	return chk
}

func (c *Checker) checkOutbox(ctx context.Context) Check {
	start := time.Now()
	chk := Check{Name: "outbox", Status: StatusUp}

	if c.DB == nil {
		chk.Status = StatusSkipped
		chk.Detail = "no database pool configured"
		return chk
	}

	queryCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	// The outbox is the only interface between Go and Elixir (ADR-005); being
	// able to read it is part of being ready to serve.
	var newest int64
	if err := c.DB.QueryRow(queryCtx, `SELECT COALESCE(MAX(seq), 0) FROM outbox.events`).Scan(&newest); err != nil {
		chk.Status = StatusDown
		chk.Detail = err.Error()
	} else {
		chk.Detail = "ok"
	}
	chk.LatencyMs = time.Since(start).Milliseconds()
	return chk
}

// LiveHandler serves GET /api/v1/health.
func LiveHandler(c *Checker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, c.Liveness())
	}
}

// ReadyHandler serves GET /api/v1/health/ready.
func ReadyHandler(c *Checker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		report := c.Readiness(r.Context())
		status := http.StatusOK
		if report.Status != StatusUp {
			status = http.StatusServiceUnavailable
		}
		writeJSON(w, status, report)
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// ensure httpx stays a dependency of this package for problem-style 503s if needed.
var _ = httpx.CodeUnavailable
