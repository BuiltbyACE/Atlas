package notifications

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/safaristack/atlas/apps/api/internal/authz"
	"github.com/safaristack/atlas/apps/api/internal/platform/httpx"
)

func withUser(r *http.Request, personID uuid.UUID) *http.Request {
	principal := &authz.Principal{
		Kind:     authz.KindUser,
		Subject:  "user:" + personID.String(),
		UserID:   uuid.Must(uuid.NewV7()),
		PersonID: personID,
	}
	return r.WithContext(authz.WithPrincipal(r.Context(), principal))
}

func TestListNotifications(t *testing.T) {
	personID := uuid.Must(uuid.NewV7())
	store := &fakeStore{items: []Notification{{
		ID:        uuid.Must(uuid.NewV7()),
		Channel:   ChannelInApp,
		Subject:   "A task was assigned to you",
		Status:    StatusDelivered,
		CreatedAt: time.Date(2026, time.October, 13, 10, 0, 0, 0, time.UTC),
	}}}
	h := NewHandler(NewService(store, discardLogger()), discardLogger())

	rec := httptest.NewRecorder()
	h.List(rec, withUser(httptest.NewRequest(http.MethodGet, "/api/v1/notifications", nil), personID))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	var page httpx.Page[notificationView]
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatalf("decode page: %v", err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(page.Items))
	}
	if page.Items[0].Subject != "A task was assigned to you" {
		t.Errorf("subject = %q", page.Items[0].Subject)
	}
}

func TestNotificationRoutesRequireAuthentication(t *testing.T) {
	h := NewHandler(NewService(&fakeStore{}, discardLogger()), discardLogger())

	cases := []struct {
		name   string
		method string
		target string
		handle func(http.ResponseWriter, *http.Request)
	}{
		{"list", http.MethodGet, "/api/v1/notifications", h.List},
		{"unread", http.MethodGet, "/api/v1/notifications/unread-count", h.UnreadCount},
		{"read-all", http.MethodPost, "/api/v1/notifications/read-all", h.MarkAllRead},
		{"preferences", http.MethodGet, "/api/v1/notifications/preferences", h.Preferences},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			tt.handle(rec, httptest.NewRequest(tt.method, tt.target, nil))
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", rec.Code)
			}
		})
	}
}

func TestMarkReadRoute(t *testing.T) {
	personID := uuid.Must(uuid.NewV7())
	id := uuid.Must(uuid.NewV7())
	store := &fakeStore{markRead: true}
	h := NewHandler(NewService(store, discardLogger()), discardLogger())

	r := chi.NewRouter()
	r.Post("/api/v1/notifications/{id}/read", h.MarkRead)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/"+id.String()+"/read", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, withUser(req, personID))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
}

func TestMarkReadMissingIs404(t *testing.T) {
	personID := uuid.Must(uuid.NewV7())
	id := uuid.Must(uuid.NewV7())
	h := NewHandler(NewService(&fakeStore{markRead: false}, discardLogger()), discardLogger())

	r := chi.NewRouter()
	r.Post("/api/v1/notifications/{id}/read", h.MarkRead)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/notifications/"+id.String()+"/read", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, withUser(req, personID))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestUnreadCount(t *testing.T) {
	personID := uuid.Must(uuid.NewV7())
	h := NewHandler(NewService(&fakeStore{unread: 3}, discardLogger()), discardLogger())

	req := withUser(httptest.NewRequest(http.MethodGet, "/api/v1/notifications/unread-count", nil), personID)
	rec := httptest.NewRecorder()
	h.UnreadCount(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body map[string]int
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["count"] != 3 {
		t.Fatalf("count = %d, want 3", body["count"])
	}
}

func TestSavePreferencesRequiresJSON(t *testing.T) {
	personID := uuid.Must(uuid.NewV7())
	h := NewHandler(NewService(&fakeStore{}, discardLogger()), discardLogger())

	req := httptest.NewRequest(http.MethodPut, "/api/v1/notifications/preferences",
		strings.NewReader(`{"in_app":true,"email":false,"timezone":"Africa/Nairobi"}`))
	req = withUser(req, personID)
	rec := httptest.NewRecorder()
	h.SavePreferences(rec, req)

	// httpx.DecodeJSON enforces application/json (the CSRF posture, §11).
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d, want 415 without a JSON content type", rec.Code)
	}
}

func TestSavePreferences(t *testing.T) {
	personID := uuid.Must(uuid.NewV7())
	store := &fakeStore{}
	h := NewHandler(NewService(store, discardLogger()), discardLogger())

	req := httptest.NewRequest(http.MethodPut, "/api/v1/notifications/preferences",
		strings.NewReader(`{"in_app":true,"email":false,"timezone":"Africa/Nairobi"}`))
	req.Header.Set("Content-Type", "application/json")
	req = withUser(req, personID)
	rec := httptest.NewRecorder()
	h.SavePreferences(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var view preferenceView
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if view.Email {
		t.Fatal("email should be false after saving")
	}
	if store.saved == nil || store.saved.PersonID != personID {
		t.Fatalf("preferences not saved for %s", personID)
	}
}

func TestSendCommandUsesIdempotencyHeader(t *testing.T) {
	personID := uuid.Must(uuid.NewV7())
	store := &fakeStore{recipient: &Recipient{PersonID: personID, Email: "elijah@example.com", Status: "active"}}
	h := NewHandler(NewService(store, discardLogger()), discardLogger())

	body := `{"recipient_person_id":"` + personID.String() + `","subject":"Hi","body":"there"}`
	req := httptest.NewRequest(http.MethodPost, "/internal/v1/commands/notifications.send", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "req-123")
	rec := httptest.NewRecorder()
	h.Send(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202: %s", rec.Code, rec.Body.String())
	}
	if len(store.created) == 0 {
		t.Fatal("no notification created")
	}
	for _, n := range store.created {
		if n.IdempotencyKey != "123" && n.IdempotencyKey != "req-123" {
			t.Fatalf("idempotency key = %q, want the Idempotency-Key header value", n.IdempotencyKey)
		}
	}
}
