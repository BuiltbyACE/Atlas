package work

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TestTimeEntryLifecycle exercises D7's time-entry surface end to end against the
// real schema: record, read, filter, edit under optimistic concurrency, delete,
// and the work.time_entry.* events written in the same transaction (ADR-019).
func TestTimeEntryLifecycle(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	svc := NewService(NewPGStore(pool), nil, nil, nil, nil)
	actor := mustPerson(t, pool)

	project, err := svc.CreateProject(ctx, actor, ProjectInput{Name: "Timesheets", ProjectType: "client_delivery"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	task, err := svc.CreateTask(ctx, actor, project.ID, TaskInput{Title: "Deliver"})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	entryIDs := make([]any, 0, 2)
	t.Cleanup(func() {
		for _, id := range entryIDs {
			_, _ = pool.Exec(context.Background(), `DELETE FROM outbox.events WHERE aggregate_id = $1`, id)
		}
		_, _ = pool.Exec(context.Background(), `DELETE FROM outbox.events WHERE aggregate_id = $1`, project.ID)
		// Deleting the project cascades tasks and their time entries.
		_, _ = pool.Exec(context.Background(), `DELETE FROM work.projects WHERE id = $1`, project.ID)
	})

	// The caller logs their own time: a nil person defaults to the actor.
	entry, err := svc.RecordTimeEntry(ctx, actor, TimeEntryInput{
		TaskID:    task.ID,
		EntryDate: time.Date(2026, 10, 14, 15, 30, 0, 0, time.UTC),
		Minutes:   90,
		Note:      "  wired the webhook  ",
	})
	if err != nil {
		t.Fatalf("RecordTimeEntry: %v", err)
	}
	entryIDs = append(entryIDs, entry.ID)
	if entry.PersonID != actor {
		t.Fatalf("person = %s, want the actor %s", entry.PersonID, actor)
	}
	if entry.ProjectID != project.ID {
		t.Fatalf("project = %s, want the task's project %s", entry.ProjectID, project.ID)
	}
	if want := time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC); !entry.EntryDate.Equal(want) {
		t.Fatalf("entry_date = %s, want %s", entry.EntryDate, want)
	}
	if entry.Note != "wired the webhook" {
		t.Fatalf("note = %q, want trimmed", entry.Note)
	}
	if entry.Version != 1 {
		t.Fatalf("version = %d, want 1", entry.Version)
	}

	// Read it back and confirm the logged event landed in the outbox.
	got, err := svc.GetTimeEntry(ctx, entry.ID)
	if err != nil {
		t.Fatalf("GetTimeEntry: %v", err)
	}
	if got.Minutes != 90 {
		t.Fatalf("minutes = %d, want 90", got.Minutes)
	}
	assertEventCount(t, pool, "work.time_entry.logged.v1", entry.ID, 1)

	// A second entry by another person, for the project+range filters.
	other := mustPerson(t, pool)
	second, err := svc.RecordTimeEntry(ctx, actor, TimeEntryInput{
		PersonID:  &other,
		TaskID:    task.ID,
		EntryDate: time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC),
		Minutes:   30,
	})
	if err != nil {
		t.Fatalf("RecordTimeEntry (other): %v", err)
	}
	entryIDs = append(entryIDs, second.ID)

	// Filter by task, project and person, and by an inclusive date range.
	byTask, err := svc.ListTimeEntries(ctx, ListTimeEntriesQuery{TaskID: &task.ID}, 10, nil)
	if err != nil {
		t.Fatalf("ListTimeEntries(task): %v", err)
	}
	if len(byTask) != 2 {
		t.Fatalf("entries for task = %d, want 2", len(byTask))
	}
	byProject, err := svc.ListTimeEntries(ctx, ListTimeEntriesQuery{ProjectID: &project.ID}, 10, nil)
	if err != nil {
		t.Fatalf("ListTimeEntries(project): %v", err)
	}
	if len(byProject) != 2 {
		t.Fatalf("entries for project = %d, want 2", len(byProject))
	}
	byPerson, err := svc.ListTimeEntries(ctx, ListTimeEntriesQuery{PersonID: &other}, 10, nil)
	if err != nil {
		t.Fatalf("ListTimeEntries(person): %v", err)
	}
	if len(byPerson) != 1 || byPerson[0].ID != second.ID {
		t.Fatalf("entries for other person = %d, want just the second", len(byPerson))
	}
	from := time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)
	inRange, err := svc.ListTimeEntries(ctx, ListTimeEntriesQuery{From: &from, To: &to}, 10, nil)
	if err != nil {
		t.Fatalf("ListTimeEntries(range): %v", err)
	}
	if len(inRange) != 1 || inRange[0].ID != entry.ID {
		t.Fatalf("entries in range = %d, want just the first", len(inRange))
	}

	// Edit under optimistic concurrency: the stale version conflicts, then the
	// fresh one applies and the omitted person is preserved.
	if _, err := svc.UpdateTimeEntry(ctx, actor, entry.ID, entry.Version+5, TimeEntryInput{
		TaskID: task.ID, EntryDate: entry.EntryDate, Minutes: 120,
	}); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale update: err = %v, want ErrConflict", err)
	}
	updated, err := svc.UpdateTimeEntry(ctx, actor, entry.ID, entry.Version, TimeEntryInput{
		TaskID: task.ID, EntryDate: entry.EntryDate, Minutes: 120, Note: "revised",
	})
	if err != nil {
		t.Fatalf("UpdateTimeEntry: %v", err)
	}
	if updated.Minutes != 120 || updated.Version != entry.Version+1 {
		t.Fatalf("updated = %d minutes at v%d, want 120 at v%d", updated.Minutes, updated.Version, entry.Version+1)
	}
	if updated.PersonID != actor {
		t.Fatalf("update lost the owner: %s, want %s", updated.PersonID, actor)
	}
	assertEventCount(t, pool, "work.time_entry.updated.v1", entry.ID, 1)

	// Validation: zero minutes and an unknown task are rejected.
	if _, err := svc.RecordTimeEntry(ctx, actor, TimeEntryInput{
		TaskID: task.ID, EntryDate: entry.EntryDate, Minutes: 0,
	}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("zero minutes: err = %v, want ErrInvalid", err)
	}
	if _, err := svc.RecordTimeEntry(ctx, actor, TimeEntryInput{
		TaskID: uuid.Nil, EntryDate: entry.EntryDate, Minutes: 30,
	}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("missing task: err = %v, want ErrInvalid", err)
	}

	// Delete writes a removal event and the entry is gone.
	if err := svc.DeleteTimeEntry(ctx, actor, entry.ID); err != nil {
		t.Fatalf("DeleteTimeEntry: %v", err)
	}
	if _, err := svc.GetTimeEntry(ctx, entry.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get deleted: err = %v, want ErrNotFound", err)
	}
	assertEventCount(t, pool, "work.time_entry.removed.v1", entry.ID, 1)
}

func assertEventCount(t *testing.T, pool *pgxpool.Pool, eventType string, aggregateID any, want int) {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM outbox.events WHERE type = $1 AND aggregate_id = $2`,
		eventType, aggregateID).Scan(&n); err != nil {
		t.Fatalf("count %s events: %v", eventType, err)
	}
	if n != want {
		t.Fatalf("%s events for %s = %d, want %d", eventType, aggregateID, n, want)
	}
}
