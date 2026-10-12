package httpx

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWriteProblem(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/things/123", nil)
	req = req.WithContext(contextWithRequestID(req, "req-abc"))

	WriteProblem(rec, req, New(http.StatusNotFound, CodeNotFound, "No such thing."))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	if ct := rec.Header().Get("Content-Type"); ct != ProblemContentType {
		t.Fatalf("content-type = %q, want %q", ct, ProblemContentType)
	}

	var p Problem
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if p.Code != CodeNotFound {
		t.Errorf("code = %q, want %q", p.Code, CodeNotFound)
	}
	if p.Status != http.StatusNotFound {
		t.Errorf("status = %d, want %d", p.Status, http.StatusNotFound)
	}
	if want := "https://atlas.safaristack.co.ke/problems/not_found"; p.Type != want {
		t.Errorf("type = %q, want %q", p.Type, want)
	}
	if p.Instance != "/api/v1/things/123" {
		t.Errorf("instance = %q, want request path", p.Instance)
	}
	if p.RequestID != "req-abc" {
		t.Errorf("request_id = %q, want %q", p.RequestID, "req-abc")
	}
	if p.Title == "" {
		t.Error("title should be derived from the status code")
	}
}

func TestProblemImplementsError(t *testing.T) {
	p := Forbidden("nope")
	if !strings.Contains(p.Error(), CodeForbidden) {
		t.Errorf("Error() = %q, want it to mention %q", p.Error(), CodeForbidden)
	}
}

func TestRequestIDGeneratesWhenAbsent(t *testing.T) {
	var got string
	h := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = RequestIDFromContext(r.Context())
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if got == "" {
		t.Fatal("expected a generated request id in the context")
	}
	if rec.Header().Get(HeaderRequestID) != got {
		t.Errorf("response header %q does not match context id %q", rec.Header().Get(HeaderRequestID), got)
	}
}

func TestRequestIDReusesInboundHeader(t *testing.T) {
	var got string
	h := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = RequestIDFromContext(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(HeaderRequestID, "supplied-id")
	h.ServeHTTP(httptest.NewRecorder(), req)

	if got != "supplied-id" {
		t.Errorf("request id = %q, want the inbound header value", got)
	}
}

// contextWithRequestID lets the test place a request id in the context the same
// way the middleware would, without depending on middleware ordering.
func contextWithRequestID(r *http.Request, id string) context.Context {
	return context.WithValue(r.Context(), requestIDKey, id)
}
