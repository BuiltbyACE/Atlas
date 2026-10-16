package audit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// fakeStore records what the handler asked for and returns a fixed page.
type fakeStore struct {
	entries    []Entry
	err        error
	lastQuery  Query
	lastLimit  int
	lastBefore *Cursor
}

func (s *fakeStore) List(_ context.Context, q Query, limit int, before *Cursor) ([]Entry, error) {
	s.lastQuery = q
	s.lastLimit = limit
	s.lastBefore = before
	if s.err != nil {
		return nil, s.err
	}
	out := s.entries
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func entryAt(i int, entityType string, entityID uuid.UUID) Entry {
	return Entry{
		ID:         uuid.New(),
		OccurredAt: time.Now().UTC().Add(-time.Duration(i) * time.Minute),
		ActorType:  "user",
		ActorID:    uuid.New().String(),
		Action:     "person.updated",
		EntityType: entityType,
		EntityID:   &entityID,
		Before:     json.RawMessage(`{"name":"A","salary_minor":"[redacted]"}`),
		After:      json.RawMessage(`{"name":"B","salary_minor":"[redacted]"}`),
	}
}

func newTestRouter(store Store) http.Handler {
	h := NewHandler(NewService(store, nil), nil)
	r := chi.NewRouter()
	r.Get("/api/v1/audit", h.List)
	r.Get("/api/v1/audit/{entity_type}/{entity_id}", h.History)
	return r
}

type pageBody struct {
	Items      []entryView `json:"items"`
	NextCursor string      `json:"next_cursor"`
	HasMore    bool        `json:"has_more"`
	Limit      int         `json:"limit"`
}

func decodePage(t *testing.T, rec *httptest.ResponseRecorder) pageBody {
	t.Helper()
	var body pageBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode page: %v (body %q)", err, rec.Body.String())
	}
	return body
}

func TestListReturnsItemsAndEnvelope(t *testing.T) {
	entityID := uuid.New()
	store := &fakeStore{entries: []Entry{
		entryAt(0, "person", entityID),
		entryAt(1, "person", entityID),
	}}
	rec := httptest.NewRecorder()
	newTestRouter(store).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/audit", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := decodePage(t, rec)
	if len(body.Items) != 2 {
		t.Fatalf("items = %d, want 2", len(body.Items))
	}
	if body.HasMore || body.NextCursor != "" {
		t.Fatalf("has_more=%v next_cursor=%q, want none", body.HasMore, body.NextCursor)
	}
	if body.Limit != 50 {
		t.Fatalf("limit = %d, want default 50", body.Limit)
	}
	if body.Items[0].EntityType != "person" || body.Items[0].EntityID == "" {
		t.Fatalf("view = %+v, want the entity fields populated", body.Items[0])
	}
	if len(body.Items[0].After) == 0 {
		t.Fatalf("after was dropped from the view")
	}
}

func TestListPaginatesWithCursor(t *testing.T) {
	entityID := uuid.New()
	store := &fakeStore{entries: []Entry{
		entryAt(0, "person", entityID),
		entryAt(1, "person", entityID),
		entryAt(2, "person", entityID),
	}}
	rec := httptest.NewRecorder()
	newTestRouter(store).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/audit?limit=2", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := decodePage(t, rec)
	if len(body.Items) != 2 {
		t.Fatalf("items = %d, want 2", len(body.Items))
	}
	if !body.HasMore || body.NextCursor == "" {
		t.Fatalf("has_more=%v next_cursor=%q, want a continuation", body.HasMore, body.NextCursor)
	}
	if store.lastLimit != 3 {
		t.Fatalf("store limit = %d, want limit+1 (3) to detect the next page", store.lastLimit)
	}
	// The cursor must point at the last item of the page.
	cur, err := decodeCursor(body.NextCursor)
	if err != nil {
		t.Fatalf("decodeCursor: %v", err)
	}
	last := body.Items[len(body.Items)-1]
	if cur.ID.String() != last.ID || !cur.At.Equal(last.OccurredAt.UTC()) {
		t.Fatalf("cursor = %+v, want the last item %s", cur, last.ID)
	}
}

func TestListRejectsBadCursor(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestRouter(&fakeStore{}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/audit?cursor=@@@", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestListRejectsBadSince(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestRouter(&fakeStore{}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/audit?since=notatime", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestListRejectsBadEntityID(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestRouter(&fakeStore{}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/audit?entity_id=nope", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestListPassesFilters(t *testing.T) {
	entityID := uuid.New()
	store := &fakeStore{}
	url := "/api/v1/audit?entity_type=person&entity_id=" + entityID.String() +
		"&actor_type=user&actor_id=abc&action=person.updated" +
		"&since=2020-01-01T00:00:00Z&until=2030-01-01T00:00:00Z"
	rec := httptest.NewRecorder()
	newTestRouter(store).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, url, nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	q := store.lastQuery
	if q.EntityType != "person" || q.EntityID == nil || *q.EntityID != entityID {
		t.Fatalf("entity filter = %+v", q)
	}
	if q.ActorType != "user" || q.ActorID != "abc" || q.Action != "person.updated" {
		t.Fatalf("actor/action filter = %+v", q)
	}
	if q.Since == nil || q.Until == nil {
		t.Fatalf("time filter not applied: %+v", q)
	}
}

func TestHistoryFiltersByEntity(t *testing.T) {
	entityID := uuid.New()
	store := &fakeStore{entries: []Entry{entryAt(0, "person", entityID)}}
	rec := httptest.NewRecorder()
	newTestRouter(store).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/audit/person/"+entityID.String(), nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := decodePage(t, rec)
	if len(body.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(body.Items))
	}
	q := store.lastQuery
	if q.EntityType != "person" || q.EntityID == nil || *q.EntityID != entityID {
		t.Fatalf("history filter = %+v", q)
	}
}

func TestHistoryRejectsBadEntityID(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestRouter(&fakeStore{}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/audit/person/not-a-uuid", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}
