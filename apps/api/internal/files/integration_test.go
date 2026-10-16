package files

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/safaristack/atlas/apps/api/internal/outbox"
)

// testDatabaseURL returns the integration database, or skips the test when the
// environment has not provided one (CI exports ATLAS_TEST_DATABASE_URL).
func testDatabaseURL(t *testing.T) string {
	t.Helper()
	url := os.Getenv("ATLAS_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("ATLAS_TEST_DATABASE_URL is not set; skipping files integration tests")
	}
	return url
}

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), testDatabaseURL(t))
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		t.Fatalf("ping: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func containsFile(files []File, id uuid.UUID) bool {
	for _, f := range files {
		if f.ID == id {
			return true
		}
	}
	return false
}

// TestFileStoreLifecycle walks the real store: create pending, read it back
// with its entity link, confirm it (writing the outbox event in the same
// transaction), reject a second confirmation, then archive it and see it drop
// out of the default listing.
func TestFileStoreLifecycle(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	store := NewPGStore(pool)
	actor := uuid.New()
	entity := uuid.New()

	id, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("new id: %v", err)
	}
	now := time.Now().UTC()
	f := File{
		ID: id, Bucket: "atlas-test", ObjectKey: "test/" + id.String() + "/a.txt",
		Filename: "a.txt", ContentType: "text/plain", SizeBytes: 5,
		Classification: Internal, Status: StatusPending, UploadedBy: &actor,
		EntityType: "project", EntityID: &entity, CreatedAt: now, UpdatedAt: now,
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM core.files WHERE id = $1`, id)
		_, _ = pool.Exec(context.Background(), `DELETE FROM outbox.events WHERE aggregate_id = $1`, id)
	})

	if err := store.Create(ctx, f); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := store.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil || got.Status != StatusPending {
		t.Fatalf("Get = %+v, want a pending file", got)
	}
	if got.EntityType != "project" || got.EntityID == nil || *got.EntityID != entity {
		t.Fatalf("entity link not stored: %+v", got)
	}

	list, err := store.List(ctx, ListQuery{EntityType: "project", EntityID: &entity}, 10, nil)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if !containsFile(list, id) {
		t.Fatalf("entity listing did not include the file: %+v", list)
	}

	ev, err := outbox.NewFileUploaded(&actor, outbox.FileRef{
		ID: id, Filename: "a.txt", ContentType: "text/plain", SizeBytes: 5,
		Classification: string(Internal), EntityType: "project", EntityID: entity, UploadedBy: actor,
	})
	if err != nil {
		t.Fatalf("event: %v", err)
	}
	done, err := store.Complete(ctx, id, ObjectInfo{SizeBytes: 5, ContentType: "text/plain"}, StatusAvailable, &ev)
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if done.Status != StatusAvailable {
		t.Fatalf("status after complete = %q, want available", done.Status)
	}

	var eventCount int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM outbox.events WHERE aggregate_id = $1 AND type = $2`,
		id, outbox.TypeFileUploadedV1).Scan(&eventCount); err != nil {
		t.Fatalf("count outbox: %v", err)
	}
	if eventCount != 1 {
		t.Fatalf("outbox rows = %d, want 1", eventCount)
	}

	if _, err := store.Complete(ctx, id, ObjectInfo{}, StatusAvailable, nil); !errors.Is(err, ErrConflict) {
		t.Fatalf("second Complete: err = %v, want ErrConflict", err)
	}

	archived, err := store.Archive(ctx, id, nil)
	if err != nil {
		t.Fatalf("Archive: %v", err)
	}
	if archived.Status != StatusDeleted || archived.ArchivedAt == nil {
		t.Fatalf("archived = %+v, want deleted with archived_at", archived)
	}

	all, err := store.List(ctx, ListQuery{}, 50, nil)
	if err != nil {
		t.Fatalf("List all: %v", err)
	}
	if containsFile(all, id) {
		t.Fatalf("archived file still in the default listing: %+v", all)
	}
}

func TestFileStoreGetMissingIsNil(t *testing.T) {
	pool := testPool(t)
	store := NewPGStore(pool)

	got, err := store.Get(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != nil {
		t.Fatalf("Get unknown = %+v, want nil", got)
	}
}

func TestFileStoreCompleteUnknownIsNotFound(t *testing.T) {
	store := NewPGStore(testPool(t))
	if _, err := store.Complete(context.Background(), uuid.New(), ObjectInfo{}, StatusAvailable, nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}
