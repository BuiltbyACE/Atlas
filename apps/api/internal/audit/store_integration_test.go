package audit

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// testDatabaseURL returns the CI-provided database or skips the package's
// integration tests when there is none, keeping `go test ./...` hermetic.
func testDatabaseURL(t *testing.T) string {
	t.Helper()
	url := os.Getenv("ATLAS_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("ATLAS_TEST_DATABASE_URL is not set; skipping audit integration tests")
	}
	return url
}

// newTestStore connects, returns a writer and reader, and schedules cleanup.
// audit.events is append-only (a trigger rejects DELETE), so cleanup briefly
// disables that trigger — the connecting role is a superuser in tests.
func newTestStore(t *testing.T) (*Writer, *PGStore, *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, testDatabaseURL(t))
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("ping: %v", err)
	}
	t.Cleanup(pool.Close)
	return NewWriter(pool), NewPGStore(pool), pool
}

// cleanupEntity removes the rows a test wrote for one entity. It is called
// before and after, so a previous failed run cannot poison the assertions.
func cleanupEntity(t *testing.T, pool *pgxpool.Pool, entityID uuid.UUID) func() {
	t.Helper()
	ctx := context.Background()
	remove := func() {
		_, _ = pool.Exec(ctx, `ALTER TABLE audit.events DISABLE TRIGGER audit_no_update`)
		_, _ = pool.Exec(ctx, `DELETE FROM audit.events WHERE entity_id = $1`, entityID)
		_, _ = pool.Exec(ctx, `ALTER TABLE audit.events ENABLE TRIGGER audit_no_update`)
	}
	remove()
	return remove
}

// TestStoreRedactsAndOrdersNewestFirst writes two events carrying sensitive and
// ordinary fields and proves the sensitive ones never reach the table or the
// reader, while the page stays newest-first (§6.2, §6.3).
func TestStoreRedactsAndOrdersNewestFirst(t *testing.T) {
	ctx := context.Background()
	w, store, pool := newTestStore(t)
	entityID := uuid.New()
	t.Cleanup(cleanupEntity(t, pool, entityID))
	actor := uuid.New().String()
	base := time.Now().UTC().Add(-time.Hour)

	if err := w.Write(ctx, nil, Event{
		OccurredAt: base,
		ActorType:  "user",
		ActorID:    actor,
		Action:     "people.person.updated",
		EntityType: "person",
		EntityID:   &entityID,
		Before:     map[string]any{"name": "A", "salary_minor": 100000},
		After:      map[string]any{"name": "A", "salary_minor": 120000},
		Reason:     "promotion",
		RequestID:  "req-older",
	}); err != nil {
		t.Fatalf("write older: %v", err)
	}
	if err := w.Write(ctx, nil, Event{
		OccurredAt: base.Add(time.Minute),
		ActorType:  "user",
		ActorID:    actor,
		Action:     "people.person.updated",
		EntityType: "person",
		EntityID:   &entityID,
		After:      map[string]any{"name": "A", "phone_e164": "+254700000000"},
		RequestID:  "req-newer",
	}); err != nil {
		t.Fatalf("write newer: %v", err)
	}

	entries, err := store.List(ctx, Query{EntityType: "person", EntityID: &entityID}, 10, nil)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(entries))
	}
	if !entries[0].OccurredAt.After(entries[1].OccurredAt) {
		t.Fatalf("order = %v then %v, want newest first", entries[0].OccurredAt, entries[1].OccurredAt)
	}
	if entries[0].RequestID != "req-newer" || entries[1].RequestID != "req-older" {
		t.Fatalf("request ids = %q, %q", entries[0].RequestID, entries[1].RequestID)
	}

	// The reader returns what the Writer stored: sensitive fields already gone.
	if !strings.Contains(string(entries[1].Before), RedactedMarker) {
		t.Fatalf("before = %s, want the salary redacted", entries[1].Before)
	}
	if strings.Contains(string(entries[1].Before), "100000") {
		t.Fatalf("salary leaked to the reader: %s", entries[1].Before)
	}
	if !strings.Contains(string(entries[1].Before), `"name"`) {
		t.Fatalf("ordinary field was dropped: %s", entries[1].Before)
	}
	if strings.Contains(string(entries[0].After), "+254700000000") {
		t.Fatalf("phone leaked to the reader: %s", entries[0].After)
	}
	if entries[1].Reason != "promotion" {
		t.Fatalf("reason = %q, want promotion", entries[1].Reason)
	}

	// Defence in depth: the raw column must not hold the secret either.
	var raw string
	if err := pool.QueryRow(ctx,
		`SELECT before::text FROM audit.events WHERE entity_id = $1 AND request_id = 'req-older'`, entityID,
	).Scan(&raw); err != nil {
		t.Fatalf("raw query: %v", err)
	}
	if strings.Contains(raw, "100000") {
		t.Fatalf("salary leaked into audit.events: %s", raw)
	}
	if !strings.Contains(raw, RedactedMarker) {
		t.Fatalf("raw before = %s, want the marker", raw)
	}
}

// TestStoreKeysetPagination walks two pages with the cursor the first page ends
// on and asserts the pages do not overlap.
func TestStoreKeysetPagination(t *testing.T) {
	ctx := context.Background()
	w, store, pool := newTestStore(t)
	entityID := uuid.New()
	t.Cleanup(cleanupEntity(t, pool, entityID))
	base := time.Now().UTC().Add(-time.Hour)

	for i := 0; i < 3; i++ {
		if err := w.Write(ctx, nil, Event{
			OccurredAt: base.Add(time.Duration(i) * time.Minute),
			ActorType:  "user",
			ActorID:    "pagination-test",
			Action:     "contract.created",
			EntityType: "contract",
			EntityID:   &entityID,
		}); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
	}

	filter := Query{EntityType: "contract", EntityID: &entityID}
	page1, err := store.List(ctx, filter, 2, nil)
	if err != nil {
		t.Fatalf("page1: %v", err)
	}
	if len(page1) != 2 {
		t.Fatalf("page1 = %d entries, want 2", len(page1))
	}
	last := page1[len(page1)-1]
	page2, err := store.List(ctx, filter, 2, &Cursor{At: last.OccurredAt, ID: last.ID})
	if err != nil {
		t.Fatalf("page2: %v", err)
	}
	if len(page2) != 1 {
		t.Fatalf("page2 = %d entries, want 1", len(page2))
	}
	if !page2[0].OccurredAt.Before(page1[0].OccurredAt) {
		t.Fatalf("page2 is not older than page1")
	}
	for _, a := range page1 {
		for _, b := range page2 {
			if a.ID == b.ID {
				t.Fatalf("pages overlap on %s", a.ID)
			}
		}
	}
}

// TestStoreFilters exercises the actor, action and time predicates.
func TestStoreFilters(t *testing.T) {
	ctx := context.Background()
	w, store, pool := newTestStore(t)
	entityID := uuid.New()
	t.Cleanup(cleanupEntity(t, pool, entityID))
	base := time.Now().UTC().Add(-time.Hour).Truncate(time.Millisecond)

	events := []Event{
		{OccurredAt: base, ActorType: "user", ActorID: "filter-a", Action: "x.done", EntityType: "widget", EntityID: &entityID},
		{OccurredAt: base.Add(time.Minute), ActorType: "user", ActorID: "filter-b", Action: "y.done", EntityType: "widget", EntityID: &entityID},
		{OccurredAt: base.Add(2 * time.Minute), ActorType: "user", ActorID: "filter-a", Action: "x.done", EntityType: "widget", EntityID: &entityID},
	}
	for i, e := range events {
		if err := w.Write(ctx, nil, e); err != nil {
			t.Fatalf("write %d: %v", i, err)
		}
	}

	byActor, err := store.List(ctx, Query{EntityType: "widget", EntityID: &entityID, ActorID: "filter-a"}, 10, nil)
	if err != nil {
		t.Fatalf("actor filter: %v", err)
	}
	if len(byActor) != 2 {
		t.Fatalf("actor filter = %d entries, want 2", len(byActor))
	}
	for _, e := range byActor {
		if e.ActorID != "filter-a" {
			t.Fatalf("actor filter leaked %q", e.ActorID)
		}
	}

	byAction, err := store.List(ctx, Query{EntityType: "widget", EntityID: &entityID, Action: "y.done"}, 10, nil)
	if err != nil {
		t.Fatalf("action filter: %v", err)
	}
	if len(byAction) != 1 || byAction[0].Action != "y.done" {
		t.Fatalf("action filter = %+v", byAction)
	}

	since := base.Add(30 * time.Second)
	byTime, err := store.List(ctx, Query{EntityType: "widget", EntityID: &entityID, Since: &since}, 10, nil)
	if err != nil {
		t.Fatalf("since filter: %v", err)
	}
	if len(byTime) != 2 {
		t.Fatalf("since filter = %d entries, want 2", len(byTime))
	}
}
