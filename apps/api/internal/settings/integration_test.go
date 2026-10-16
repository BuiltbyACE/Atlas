package settings

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const testKey = "company.name"

// testDatabaseURL returns the integration database, or skips the test when the
// environment has not provided one (CI exports ATLAS_TEST_DATABASE_URL).
func testDatabaseURL(t *testing.T) string {
	t.Helper()
	url := os.Getenv("ATLAS_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("ATLAS_TEST_DATABASE_URL is not set; skipping settings integration tests")
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

func newTestService(t *testing.T) (*Service, *pgxpool.Pool) {
	t.Helper()
	pool := testPool(t)
	clean := func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM core.settings WHERE key = $1`, testKey)
	}
	clean()
	t.Cleanup(clean)
	return NewService(NewPGStore(pool), nil, nil), pool
}

func find(settings []Setting, key string) *Setting {
	for i := range settings {
		if settings[i].Key == key {
			return &settings[i]
		}
	}
	return nil
}

// TestSettingLifecycleVersioningAndHistory walks the real write path: the
// default before any row, the first write, a stale write rejected, the version
// advancing, and the history holding both changes with their values.
func TestSettingLifecycleVersioningAndHistory(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestService(t)
	actor := uuid.New()

	cur, err := svc.Get(ctx, testKey)
	if err != nil {
		t.Fatalf("Get default: %v", err)
	}
	if cur.Version != 0 || cur.Value != "Atlas" {
		t.Fatalf("default = %+v, want version 0 value Atlas", cur)
	}

	set, err := svc.Set(ctx, SetInput{Key: testKey, Value: "Safari", ExpectedVersion: 0, ActorID: actor, Reason: "initial"})
	if err != nil {
		t.Fatalf("Set v1: %v", err)
	}
	if set.Version != 1 || set.Value != "Safari" {
		t.Fatalf("after v1 = %+v", set)
	}

	if _, err := svc.Set(ctx, SetInput{Key: testKey, Value: "Stale", ExpectedVersion: 0, ActorID: actor}); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale write: err = %v, want ErrConflict", err)
	}

	if _, err := svc.Set(ctx, SetInput{Key: testKey, Value: 5, ExpectedVersion: 1, ActorID: actor}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("non-string value: err = %v, want ErrInvalid", err)
	}

	set2, err := svc.Set(ctx, SetInput{Key: testKey, Value: "Safari Atlas", ExpectedVersion: 1, ActorID: actor})
	if err != nil {
		t.Fatalf("Set v2: %v", err)
	}
	if set2.Version != 2 {
		t.Fatalf("version after v2 = %d, want 2", set2.Version)
	}

	list, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if got := find(list, testKey); got == nil || got.Version != 2 || got.Value != "Safari Atlas" {
		t.Fatalf("List did not show the stored value: %+v", got)
	}

	hist, err := svc.History(ctx, testKey, 10, nil)
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(hist) != 2 {
		t.Fatalf("history len = %d, want 2", len(hist))
	}
	if hist[0].Version != 2 || hist[1].Version != 1 {
		t.Fatalf("history not newest-first: %+v", hist)
	}
	if hist[0].OldValue != "Safari" || hist[0].NewValue != "Safari Atlas" {
		t.Fatalf("history v2 = %+v", hist[0])
	}
	if hist[1].OldValue != nil {
		t.Fatalf("first change old value = %v, want nil", hist[1].OldValue)
	}
	if hist[0].ChangedBy == nil || *hist[0].ChangedBy != actor {
		t.Fatalf("changed_by not recorded: %+v", hist[0])
	}
}

func TestSettingUnknownKeyIsNotFound(t *testing.T) {
	ctx := context.Background()
	svc, _ := newTestService(t)

	if _, err := svc.Get(ctx, "not.a.setting"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get unknown: err = %v, want ErrNotFound", err)
	}
	if _, err := svc.Set(ctx, SetInput{Key: "not.a.setting", Value: true, ActorID: uuid.New()}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Set unknown: err = %v, want ErrNotFound", err)
	}
}
