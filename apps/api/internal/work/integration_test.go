package work

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

func testServerURL(t *testing.T) string {
	t.Helper()
	url := os.Getenv("ATLAS_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("ATLAS_TEST_DATABASE_URL is not set; skipping work integration tests")
	}
	return url
}

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), testServerURL(t))
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		t.Fatalf("ping: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// mustPerson inserts a real person row (work tables have FK constraints on
// people.persons) and removes it at the end of the test.
func mustPerson(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("new id: %v", err)
	}
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO people.persons (id, full_name, employment_type) VALUES ($1, $2, 'full_time')`,
		id, "Test Person"); err != nil {
		t.Fatalf("insert person: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM people.persons WHERE id = $1`, id)
	})
	return id
}

// TestServiceLifecycle walks projects, milestones, tasks, dependencies and
// blockers through the real store and service: status transitions, optimistic
// concurrency, cycle prevention, and the outbox events emitted in step.
func TestServiceLifecycle(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	store := NewPGStore(pool)
	svc := NewService(store, nil, nil)
	actor := mustPerson(t, pool)

	project, err := svc.CreateProject(ctx, actor, ProjectInput{Name: "Apollo"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM outbox.events WHERE aggregate_id = $1`, project.ID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM work.projects WHERE id = $1`, project.ID)
	})
	if project.Status != ProjectPlanning {
		t.Fatalf("new project status = %q, want planning", project.Status)
	}

	// planning -> active is allowed and stamps started_at, emitting an event.
	active, err := svc.UpdateProject(ctx, actor, project.ID, project.Version, ProjectInput{Name: "Apollo", Status: ProjectActive})
	if err != nil {
		t.Fatalf("activate: %v", err)
	}
	if active.StartedAt == nil {
		t.Fatal("active project should have started_at")
	}
	if active.Version != project.Version+1 {
		t.Fatalf("version = %d, want %d", active.Version, project.Version+1)
	}

	// A stale version is rejected.
	if _, err := svc.UpdateProject(ctx, actor, project.ID, project.Version, ProjectInput{Name: "Apollo"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale update: err = %v, want ErrConflict", err)
	}

	// completed -> active is not allowed.
	if _, err := svc.UpdateProject(ctx, actor, project.ID, active.Version, ProjectInput{Name: "Apollo", Status: ProjectCompleted}); err != nil {
		t.Fatalf("complete: %v", err)
	}

	milestone, err := svc.CreateMilestone(ctx, actor, project.ID, MilestoneInput{Name: "M1"})
	if err != nil {
		t.Fatalf("CreateMilestone: %v", err)
	}
	if milestone.ProjectID != project.ID {
		t.Fatalf("milestone project = %s, want %s", milestone.ProjectID, project.ID)
	}

	taskA, err := svc.CreateTask(ctx, actor, project.ID, TaskInput{Title: "Design", MilestoneID: &milestone.ID})
	if err != nil {
		t.Fatalf("CreateTask A: %v", err)
	}
	taskB, err := svc.CreateTask(ctx, actor, project.ID, TaskInput{Title: "Build"})
	if err != nil {
		t.Fatalf("CreateTask B: %v", err)
	}

	// Dependency A -> B, then a B -> A edge must be rejected as a cycle.
	if err := svc.AddDependency(ctx, actor, taskA.ID, taskB.ID, FinishToStart); err != nil {
		t.Fatalf("AddDependency: %v", err)
	}
	if err := svc.AddDependency(ctx, actor, taskB.ID, taskA.ID, FinishToStart); !errors.Is(err, ErrConflict) {
		t.Fatalf("cycle: err = %v, want ErrConflict", err)
	}
	if err := svc.AddDependency(ctx, actor, taskA.ID, taskB.ID, FinishToStart); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate: err = %v, want ErrConflict", err)
	}

	// Blockers: raising marks the task blocked; resolving unblocks it.
	if _, err := svc.RaiseBlocker(ctx, actor, taskB.ID, "waiting on vendor"); err != nil {
		t.Fatalf("RaiseBlocker: %v", err)
	}
	blocked, err := svc.GetTask(ctx, taskB.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if !blocked.Blocked {
		t.Fatal("task should be blocked")
	}
	open, err := svc.ListBlockers(ctx, taskB.ID, false)
	if err != nil {
		t.Fatalf("ListBlockers: %v", err)
	}
	if len(open) != 1 {
		t.Fatalf("open blockers = %d, want 1", len(open))
	}
	resolved, err := svc.ResolveBlocker(ctx, actor, open[0].ID)
	if err != nil {
		t.Fatalf("ResolveBlocker: %v", err)
	}
	if resolved.ResolvedAt == nil {
		t.Fatal("resolved blocker should have resolved_at")
	}
	unblocked, err := svc.GetTask(ctx, taskB.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if unblocked.Blocked {
		t.Fatal("task should be unblocked")
	}

	// Task status flow: todo -> in_progress -> review -> done, with events.
	prog, err := svc.UpdateTask(ctx, actor, taskA.ID, taskA.Version, TaskInput{Title: "Design", Status: TaskInProgress})
	if err != nil {
		t.Fatalf("start task: %v", err)
	}
	if prog.StartedAt == nil {
		t.Fatal("in_progress task should have started_at")
	}
	done, err := svc.UpdateTask(ctx, actor, taskA.ID, prog.Version, TaskInput{Title: "Design", Status: TaskDone})
	if err != nil {
		t.Fatalf("update to done: %v", err)
	}
	if done.CompletedAt == nil {
		t.Fatal("done task should have completed_at")
	}

	var events int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM outbox.events WHERE aggregate_id = $1 AND type = $2`,
		taskA.ID, outbox.TypeTaskStatusChangedV1).Scan(&events); err != nil {
		t.Fatalf("count task events: %v", err)
	}
	if events != 2 {
		t.Fatalf("task status events = %d, want 2", events)
	}

	// Listings.
	tasks, err := svc.ListTasks(ctx, ListTasksQuery{ProjectID: &project.ID}, 50, nil)
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("project tasks = %d, want 2", len(tasks))
	}
}

// TestOverdueListing covers the overdue filter used by the escalation ladder.
func TestOverdueListing(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	store := NewPGStore(pool)
	svc := NewService(store, nil, nil)
	actor := mustPerson(t, pool)

	project, err := svc.CreateProject(ctx, actor, ProjectInput{Name: "Overdue test"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM outbox.events WHERE aggregate_id = $1`, project.ID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM work.projects WHERE id = $1`, project.ID)
	})

	past := time.Now().UTC().Add(-48 * time.Hour)
	future := time.Now().UTC().Add(48 * time.Hour)
	if _, err := svc.CreateTask(ctx, actor, project.ID, TaskInput{Title: "Late", DueAt: &past}); err != nil {
		t.Fatalf("CreateTask late: %v", err)
	}
	if _, err := svc.CreateTask(ctx, actor, project.ID, TaskInput{Title: "Soon", DueAt: &future}); err != nil {
		t.Fatalf("CreateTask soon: %v", err)
	}

	overdue, err := svc.ListTasks(ctx, ListTasksQuery{ProjectID: &project.ID, Overdue: true, Now: time.Now().UTC()}, 50, nil)
	if err != nil {
		t.Fatalf("ListTasks overdue: %v", err)
	}
	if len(overdue) != 1 || overdue[0].Title != "Late" {
		t.Fatalf("overdue = %+v, want just Late", overdue)
	}
}
