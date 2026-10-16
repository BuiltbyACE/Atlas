package files

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

func user() *authz.Principal {
	return &authz.Principal{Kind: authz.KindUser, Subject: "user:test", UserID: uuid.New(), PersonID: uuid.New()}
}

func authRequest(method, target, body string, as *authz.Principal) *http.Request {
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, target, nil)
	} else {
		req = httptest.NewRequest(method, target, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	}
	if as != nil {
		req = req.WithContext(authz.WithPrincipal(req.Context(), as))
	}
	return req
}

func filesRouter(h *Handler) http.Handler {
	r := chi.NewRouter()
	r.Get("/api/v1/files", h.List)
	r.Post("/api/v1/files", h.RequestUpload)
	r.Get("/api/v1/files/{id}", h.Get)
	r.Get("/api/v1/files/{id}/download", h.Download)
	r.Post("/api/v1/files/{id}/complete", h.Complete)
	r.Delete("/api/v1/files/{id}", h.Archive)
	return r
}

func TestRequestUploadReturnsTicket(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store, &fakeObjects{putURL: "https://r2.example/put"}, NoopScanner{}, nil, Config{Bucket: "atlas-files"}, nil)
	h := NewHandler(svc, nil)

	rec := httptest.NewRecorder()
	filesRouter(h).ServeHTTP(rec, authRequest(http.MethodPost, "/api/v1/files",
		`{"filename":"a.pdf","content_type":"application/pdf","size_bytes":10}`, user()))

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (%s)", rec.Code, rec.Body.String())
	}
	var body struct {
		File   fileView         `json:"file"`
		Upload uploadTicketView `json:"upload"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Upload.UploadURL != "https://r2.example/put" || body.Upload.Method != http.MethodPut {
		t.Fatalf("upload ticket = %+v", body.Upload)
	}
	if body.File.Status != string(StatusPending) {
		t.Fatalf("file status = %q, want pending", body.File.Status)
	}
}

func TestRequestUploadInvalidIsBadRequest(t *testing.T) {
	svc := NewService(newFakeStore(), &fakeObjects{}, NoopScanner{}, nil, Config{Bucket: "atlas-files"}, nil)
	h := NewHandler(svc, nil)

	rec := httptest.NewRecorder()
	filesRouter(h).ServeHTTP(rec, authRequest(http.MethodPost, "/api/v1/files",
		`{"filename":"a.pdf","size_bytes":0}`, user()))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (%s)", rec.Code, rec.Body.String())
	}
}

func TestRequestUploadUnauthenticatedIsRejected(t *testing.T) {
	svc := NewService(newFakeStore(), &fakeObjects{}, NoopScanner{}, nil, Config{Bucket: "atlas-files"}, nil)
	h := NewHandler(svc, nil)

	rec := httptest.NewRecorder()
	filesRouter(h).ServeHTTP(rec, authRequest(http.MethodPost, "/api/v1/files",
		`{"filename":"a.pdf","size_bytes":10}`, nil))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 (%s)", rec.Code, rec.Body.String())
	}
}

func TestGetBadIDIsBadRequest(t *testing.T) {
	svc := NewService(newFakeStore(), &fakeObjects{}, NoopScanner{}, nil, Config{Bucket: "atlas-files"}, nil)
	h := NewHandler(svc, nil)

	rec := httptest.NewRecorder()
	filesRouter(h).ServeHTTP(rec, authRequest(http.MethodGet, "/api/v1/files/not-a-uuid", "", user()))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (%s)", rec.Code, rec.Body.String())
	}
}

func TestGetUnknownFileIsNotFound(t *testing.T) {
	svc := NewService(newFakeStore(), &fakeObjects{}, NoopScanner{}, nil, Config{Bucket: "atlas-files"}, nil)
	h := NewHandler(svc, nil)

	rec := httptest.NewRecorder()
	filesRouter(h).ServeHTTP(rec, authRequest(http.MethodGet, "/api/v1/files/"+uuid.New().String(), "", user()))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (%s)", rec.Code, rec.Body.String())
	}
}

func TestDownloadPendingIsConflict(t *testing.T) {
	actor := user()
	pending := pendingFile(actor.PersonID)
	store := newFakeStore(pending)
	svc := NewService(store, &fakeObjects{getURL: "https://r2.example/get"}, NoopScanner{}, nil, Config{Bucket: "atlas-files"}, nil)
	h := NewHandler(svc, nil)

	rec := httptest.NewRecorder()
	filesRouter(h).ServeHTTP(rec, authRequest(http.MethodGet, "/api/v1/files/"+pending.ID.String()+"/download", "", user()))

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409 (%s)", rec.Code, rec.Body.String())
	}
}

func TestCompleteMarksAvailable(t *testing.T) {
	actor := user()
	pending := pendingFile(actor.PersonID)
	store := newFakeStore(pending)
	objects := &fakeObjects{info: ObjectInfo{SizeBytes: 42, ContentType: "application/pdf"}}
	svc := NewService(store, objects, NoopScanner{}, nil, Config{Bucket: "atlas-files"}, nil)
	h := NewHandler(svc, nil)

	rec := httptest.NewRecorder()
	filesRouter(h).ServeHTTP(rec, authRequest(http.MethodPost, "/api/v1/files/"+pending.ID.String()+"/complete", "", actor))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	var view fileView
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if view.Status != string(StatusAvailable) || view.SizeBytes != 42 {
		t.Fatalf("view = %+v", view)
	}
}
