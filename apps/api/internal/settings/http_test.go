package settings

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/safaristack/atlas/apps/api/internal/authz"
)

// These tests exercise the HTTP contract around the service: the ETag a client
// reads, the If-Match it must echo, and the problem codes for the failure
// modes. The service is backed by the in-memory fakeStore, so no database is
// involved.

func authedRequest(method, target, body string) *http.Request {
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, target, nil)
	} else {
		req = httptest.NewRequest(method, target, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	}
	return req.WithContext(authz.WithPrincipal(req.Context(), &authz.Principal{
		Kind: authz.KindUser, Subject: "user:test", PersonID: uuid.New(),
	}))
}

func settingsRouter(h *Handler) http.Handler {
	r := chi.NewRouter()
	r.Get("/api/v1/settings/{key}", h.Get)
	r.Put("/api/v1/settings/{key}", h.Set)
	return r
}

func TestGetSetsETag(t *testing.T) {
	store := &fakeStore{get: &Setting{Key: "company.currency", Value: "KES", Version: 3}}
	h := NewHandler(NewService(store, nil, nil), nil)

	rec := httptest.NewRecorder()
	settingsRouter(h).ServeHTTP(rec, authedRequest(http.MethodGet, "/api/v1/settings/company.currency", ""))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("ETag"); got != `"3"` {
		t.Fatalf("ETag = %q, want %q", got, `"3"`)
	}
	var view settingView
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if view.Value != "KES" || view.Type != string(KindEnum) || view.Version != 3 {
		t.Fatalf("view = %+v", view)
	}
}

func TestGetUnknownKeyIsNotFound(t *testing.T) {
	h := NewHandler(NewService(&fakeStore{}, nil, nil), nil)

	rec := httptest.NewRecorder()
	settingsRouter(h).ServeHTTP(rec, authedRequest(http.MethodGet, "/api/v1/settings/does.not.exist", ""))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (%s)", rec.Code, rec.Body.String())
	}
}

func TestPutRequiresIfMatch(t *testing.T) {
	store := &fakeStore{get: &Setting{Key: "company.currency", Value: "KES", Version: 3}}
	h := NewHandler(NewService(store, nil, nil), nil)

	rec := httptest.NewRecorder()
	settingsRouter(h).ServeHTTP(rec, authedRequest(http.MethodPut, "/api/v1/settings/company.currency", `{"value":"USD"}`))

	if rec.Code != http.StatusPreconditionRequired {
		t.Fatalf("status = %d, want 428 (%s)", rec.Code, rec.Body.String())
	}
	if len(store.setCalls) != 0 {
		t.Fatalf("store written %d times without If-Match", len(store.setCalls))
	}
}

func TestPutRejectsStaleVersion(t *testing.T) {
	store := &fakeStore{get: &Setting{Key: "company.currency", Value: "KES", Version: 3}}
	h := NewHandler(NewService(store, nil, nil), nil)

	req := authedRequest(http.MethodPut, "/api/v1/settings/company.currency", `{"value":"USD"}`)
	req.Header.Set("If-Match", `"2"`)
	rec := httptest.NewRecorder()
	settingsRouter(h).ServeHTTP(rec, req)

	if rec.Code != http.StatusPreconditionFailed {
		t.Fatalf("status = %d, want 412 (%s)", rec.Code, rec.Body.String())
	}
	if len(store.setCalls) != 0 {
		t.Fatalf("stale write reached the store (%d calls)", len(store.setCalls))
	}
}

func TestPutWritesWithMatchingVersion(t *testing.T) {
	store := &fakeStore{
		get:       &Setting{Key: "company.currency", Value: "KES", Version: 3},
		setResult: &Setting{Key: "company.currency", Value: "USD", Version: 4},
	}
	h := NewHandler(NewService(store, nil, nil), nil)

	req := authedRequest(http.MethodPut, "/api/v1/settings/company.currency", `{"value":"USD","reason":"switch"}`)
	req.Header.Set("If-Match", `"3"`)
	rec := httptest.NewRecorder()
	settingsRouter(h).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("ETag"); got != `"4"` {
		t.Fatalf("ETag = %q, want %q", got, `"4"`)
	}
	if len(store.setCalls) != 1 {
		t.Fatalf("store.Set calls = %d, want 1", len(store.setCalls))
	}
	if in := store.setCalls[0]; in.ExpectedVersion != 3 || in.Value != "USD" || in.Reason != "switch" {
		t.Fatalf("SetInput = %+v", in)
	}
}

func TestPutMissingValueIsBadRequest(t *testing.T) {
	store := &fakeStore{get: &Setting{Key: "company.currency", Value: "KES", Version: 3}}
	h := NewHandler(NewService(store, nil, nil), nil)

	req := authedRequest(http.MethodPut, "/api/v1/settings/company.currency", `{"reason":"no value"}`)
	req.Header.Set("If-Match", `"3"`)
	rec := httptest.NewRecorder()
	settingsRouter(h).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (%s)", rec.Code, rec.Body.String())
	}
	if len(store.setCalls) != 0 {
		t.Fatalf("store.Set called for a body with no value")
	}
}

func TestUnauthenticatedWriteIsRejected(t *testing.T) {
	store := &fakeStore{get: &Setting{Key: "company.currency", Value: "KES", Version: 3}}
	h := NewHandler(NewService(store, nil, nil), nil)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings/company.currency", strings.NewReader(`{"value":"USD"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("If-Match", `"3"`)
	rec := httptest.NewRecorder()
	settingsRouter(h).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 (%s)", rec.Code, rec.Body.String())
	}
	if len(store.setCalls) != 0 {
		t.Fatal("store written for an unauthenticated request")
	}
}
