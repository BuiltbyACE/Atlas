// Package httpx contains the shared HTTP conventions for Atlas
// (docs/ATLAS.md §11): RFC 9457 problem details, request IDs, and middleware.
package httpx

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
)

// ProblemContentType is the media type for RFC 9457 problem responses.
const ProblemContentType = "application/problem+json"

// Stable error codes. Clients switch on these values, so they are part of the
// API contract: add new ones freely, never rename an existing one silently.
const (
	CodeValidationFailed      = "VALIDATION_FAILED"
	CodeUnauthenticated       = "UNAUTHENTICATED"
	CodeForbidden             = "FORBIDDEN"
	CodeNotFound              = "NOT_FOUND"
	CodeMethodNotAllowed      = "METHOD_NOT_ALLOWED"
	CodeConflict              = "CONFLICT"
	CodePreconditionFailed    = "PRECONDITION_FAILED"
	CodeRateLimited           = "RATE_LIMITED"
	CodeInternal              = "INTERNAL_ERROR"
	CodeUnavailable           = "SERVICE_UNAVAILABLE"
	CodeApprovalLimitExceeded = "APPROVAL_LIMIT_EXCEEDED"
	CodeTaskEvidenceRequired  = "TASK_EVIDENCE_REQUIRED"
)

// FieldError describes a single validation failure.
type FieldError struct {
	Field  string `json:"field"`
	Code   string `json:"code"`
	Detail string `json:"detail,omitempty"`
}

// Problem is an RFC 9457 problem detail with Atlas extensions.
type Problem struct {
	Type      string       `json:"type"`
	Title     string       `json:"title"`
	Status    int          `json:"status"`
	Detail    string       `json:"detail,omitempty"`
	Instance  string       `json:"instance,omitempty"`
	Code      string       `json:"code,omitempty"`
	RequestID string       `json:"request_id,omitempty"`
	Errors    []FieldError `json:"errors,omitempty"`
}

// Error implements error so a Problem can be returned through normal Go error paths.
func (p Problem) Error() string {
	if p.Detail != "" {
		return p.Code + ": " + p.Detail
	}
	return p.Code + ": " + p.Title
}

// New builds a Problem with a sensible title derived from the status code.
func New(status int, code, detail string) Problem {
	return Problem{Status: status, Code: code, Title: http.StatusText(status), Detail: detail}
}

// WriteProblem writes p as an application/problem+json response, filling in the
// type URI, status and request id if they are not already set.
func WriteProblem(w http.ResponseWriter, r *http.Request, p Problem) {
	if p.Status == 0 {
		p.Status = http.StatusInternalServerError
	}
	if p.Title == "" {
		p.Title = http.StatusText(p.Status)
	}
	if p.Type == "" {
		p.Type = problemType(p.Code)
	}
	if p.RequestID == "" {
		p.RequestID = RequestIDFromContext(r.Context())
	}
	if p.Instance == "" && r != nil && r.URL != nil {
		p.Instance = r.URL.Path
	}

	w.Header().Set("Content-Type", ProblemContentType)
	w.WriteHeader(p.Status)
	_ = json.NewEncoder(w).Encode(p)
}

// WriteError writes err as a problem response. A Problem (or *Problem) is
// written as-is; anything else becomes a generic 500 that does not leak
// internal detail to the caller.
func WriteError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, err error) {
	var p Problem
	if errors.As(err, &p) {
		WriteProblem(w, r, p)
		return
	}
	var pp *Problem
	if errors.As(err, &pp) && pp != nil {
		WriteProblem(w, r, *pp)
		return
	}

	if logger != nil {
		logger.ErrorContext(r.Context(), "unhandled error", slog.String("error", err.Error()), slog.String("path", r.URL.Path))
	}
	WriteProblem(w, r, New(http.StatusInternalServerError, CodeInternal, "An unexpected error occurred."))
}

// ── common constructors ──────────────────────────────────────

func BadRequest(detail string) Problem {
	return New(http.StatusBadRequest, CodeValidationFailed, detail)
}

func Unauthenticated(detail string) Problem {
	return New(http.StatusUnauthorized, CodeUnauthenticated, detail)
}

func Forbidden(detail string) Problem {
	return New(http.StatusForbidden, CodeForbidden, detail)
}

func NotFound(detail string) Problem {
	return New(http.StatusNotFound, CodeNotFound, detail)
}

func Conflict(detail string) Problem {
	return New(http.StatusConflict, CodeConflict, detail)
}

func Unavailable(detail string) Problem {
	return New(http.StatusServiceUnavailable, CodeUnavailable, detail)
}

// NotFoundHandler is the router-level 404 responder.
func NotFoundHandler(w http.ResponseWriter, r *http.Request) {
	WriteProblem(w, r, NotFound("The requested resource does not exist."))
}

// MethodNotAllowedHandler is the router-level 405 responder.
func MethodNotAllowedHandler(w http.ResponseWriter, r *http.Request) {
	WriteProblem(w, r, New(http.StatusMethodNotAllowed, CodeMethodNotAllowed, "That method is not supported for this resource."))
}

func problemType(code string) string {
	if code == "" {
		return "about:blank"
	}
	return "https://atlas.safaristack.co.ke/problems/" + strings.ToLower(code)
}
