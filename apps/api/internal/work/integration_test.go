package work

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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
	svc := NewService(store, nil, nil, nil)
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
	svc := NewService(store, nil, nil, nil)
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

// mustFile inserts a core.files row with the given status and removes it at the
// end of the test.
func mustFile(t *testing.T, pool *pgxpool.Pool, status string) uuid.UUID {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("new id: %v", err)
	}
	key := "test/" + id.String()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO core.files (id, bucket, object_key, filename, content_type, size_bytes, status)
		 VALUES ($1, 'test', $2, 'proof.pdf', 'application/pdf', 10, $3)`,
		id, key, status); err != nil {
		t.Fatalf("insert file: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM core.files WHERE id = $1`, id)
	})
	return id
}

// dbFileResolver reads core.files directly so the test exercises the real
// availability check without depending on the files package.
type dbFileResolver struct{ pool *pgxpool.Pool }

func (r dbFileResolver) ResolveFile(ctx context.Context, id uuid.UUID) (*EvidenceFile, error) {
	var filename, status string
	err := r.pool.QueryRow(ctx, `SELECT filename, status FROM core.files WHERE id = $1`, id).
		Scan(&filename, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &EvidenceFile{ID: id, Filename: filename, Status: status}, nil
}

// TestEvidenceLifecycle covers attaching available files, rejecting files that
// are not available, deriving the project from a task, separation of duties on
// verification, listing and removal.
func TestEvidenceLifecycle(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	store := NewPGStore(pool)
	svc := NewService(store, dbFileResolver{pool: pool}, nil, nil)
	actor := mustPerson(t, pool)
	reviewer := mustPerson(t, pool)

	project, err := svc.CreateProject(ctx, actor, ProjectInput{Name: "Evidence"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	task, err := svc.CreateTask(ctx, actor, project.ID, TaskInput{Title: "Attach proof"})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM work.projects WHERE id = $1`, project.ID)
	})

	available := mustFile(t, pool, "available")
	pending := mustFile(t, pool, "pending")

	// Attaching an available file to a task succeeds and derives the project.
	ev, err := svc.AttachEvidence(ctx, actor, EvidenceInput{TaskID: &task.ID, FileID: available, Kind: "deliverable"})
	if err != nil {
		t.Fatalf("AttachEvidence: %v", err)
	}
	if ev.ProjectID == nil || *ev.ProjectID != project.ID {
		t.Fatalf("project_id = %v, want %s", ev.ProjectID, project.ID)
	}

	// A file that is not available may not become evidence.
	if _, err := svc.AttachEvidence(ctx, actor, EvidenceInput{TaskID: &task.ID, FileID: pending}); !errors.Is(err, ErrConflict) {
		t.Fatalf("attach pending: err = %v, want ErrConflict", err)
	}

	// Exactly one of task_id / project_id.
	if _, err := svc.AttachEvidence(ctx, actor, EvidenceInput{TaskID: &task.ID, ProjectID: &project.ID, FileID: available}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("attach both: err = %v, want ErrInvalid", err)
	}

	// Project-level evidence.
	if _, err := svc.AttachEvidence(ctx, actor, EvidenceInput{ProjectID: &project.ID, FileID: available}); err != nil {
		t.Fatalf("AttachEvidence project: %v", err)
	}

	byTask, err := svc.ListEvidence(ctx, ListEvidenceQuery{TaskID: &task.ID})
	if err != nil || len(byTask) != 1 {
		t.Fatalf("ListEvidence task = %d, err = %v, want 1", len(byTask), err)
	}
	byProject, err := svc.ListEvidence(ctx, ListEvidenceQuery{ProjectID: &project.ID})
	if err != nil || len(byProject) != 2 {
		t.Fatalf("ListEvidence project = %d, err = %v, want 2", len(byProject), err)
	}

	// A submitter cannot verify their own evidence.
	if _, err := svc.VerifyEvidence(ctx, actor, ev.ID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("self verify: err = %v, want ErrForbidden", err)
	}
	verified, err := svc.VerifyEvidence(ctx, reviewer, ev.ID)
	if err != nil {
		t.Fatalf("VerifyEvidence: %v", err)
	}
	if verified.VerifiedBy == nil || *verified.VerifiedBy != reviewer || verified.VerifiedAt == nil {
		t.Fatalf("verified = %+v, want reviewer %s", verified, reviewer)
	}
	// Verification is once.
	if _, err := svc.VerifyEvidence(ctx, reviewer, ev.ID); !errors.Is(err, ErrConflict) {
		t.Fatalf("double verify: err = %v, want ErrConflict", err)
	}

	if err := svc.DeleteEvidence(ctx, actor, ev.ID); err != nil {
		t.Fatalf("DeleteEvidence: %v", err)
	}
	if _, err := svc.GetEvidence(ctx, ev.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("GetEvidence after delete: err = %v, want ErrNotFound", err)
	}
}

// TestEscalationSweep covers the overdue ladder end to end: an overdue task is
// escalated once (assigned to the project owner), a manual resolve is honoured
// and not re-raised in the same episode, and the outbox carries both the overdue
// and the escalation events.
func TestEscalationSweep(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	svc := NewService(NewPGStore(pool), nil, nil, nil)
	owner := mustPerson(t, pool)
	assignee := mustPerson(t, pool)

	project, err := svc.CreateProject(ctx, owner, ProjectInput{Name: "Escalation test", OwnerPersonID: &owner})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM work.projects WHERE id = $1`, project.ID)
	})

	past := time.Now().UTC().Add(-72 * time.Hour)
	task, err := svc.CreateTask(ctx, owner, project.ID, TaskInput{Title: "Late", DueAt: &past, AssigneePersonID: &assignee})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	raised, resolved, err := svc.Sweep(ctx, 10)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if raised != 1 || resolved != 0 {
		t.Fatalf("sweep raised=%d resolved=%d, want 1/0", raised, resolved)
	}

	opens, err := svc.ListEscalations(ctx, ListEscalationsQuery{EntityType: "task", EntityID: &task.ID, Status: EscalationOpen}, 10, nil)
	if err != nil || len(opens) != 1 {
		t.Fatalf("open escalations = %d, err = %v, want 1", len(opens), err)
	}
	esc := opens[0]
	if esc.Level != 1 || esc.AssigneeID == nil || *esc.AssigneeID != owner {
		t.Fatalf("escalation = %+v, want level 1 assigned to owner %s", esc, owner)
	}

	// Both events were written for the escalation.
	var events int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM outbox.events
		 WHERE aggregate_id IN ($1, $2)
		   AND type IN ('work.task.overdue.v1','core.escalation.raised.v1')`,
		task.ID, esc.ID).Scan(&events); err != nil {
		t.Fatalf("count events: %v", err)
	}
	if events != 2 {
		t.Fatalf("events = %d, want work.task.overdue.v1 and core.escalation.raised.v1", events)
	}

	// A second sweep does not re-raise the same episode.
	if raised, _, err := svc.Sweep(ctx, 10); err != nil || raised != 0 {
		t.Fatalf("second sweep raised=%d err=%v, want 0", raised, err)
	}

	// A human can resolve it; a manual resolution is honoured by later sweeps.
	resolvedEsc, err := svc.ResolveEscalation(ctx, assignee, esc.ID)
	if err != nil {
		t.Fatalf("ResolveEscalation: %v", err)
	}
	if resolvedEsc.Status != EscalationResolved || resolvedEsc.ResolvedBy == nil || *resolvedEsc.ResolvedBy != assignee {
		t.Fatalf("resolved = %+v, want resolved by %s", resolvedEsc, assignee)
	}
	if _, err := svc.ResolveEscalation(ctx, assignee, esc.ID); !errors.Is(err, ErrConflict) {
		t.Fatalf("double resolve: err = %v, want ErrConflict", err)
	}
	if raised, _, err := svc.Sweep(ctx, 10); err != nil || raised != 0 {
		t.Fatalf("post-resolve sweep raised=%d err=%v, want 0", raised, err)
	}
}

// TestEscalationResolvesWhenTaskCompletes covers the automatic resolution: once
// the overdue task is finished the sweep closes the escalation and emits
// core.escalation.resolved.v1.
func TestEscalationResolvesWhenTaskCompletes(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	svc := NewService(NewPGStore(pool), nil, nil, nil)
	owner := mustPerson(t, pool)

	project, err := svc.CreateProject(ctx, owner, ProjectInput{Name: "Resolve test", OwnerPersonID: &owner})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM work.projects WHERE id = $1`, project.ID)
	})

	past := time.Now().UTC().Add(-48 * time.Hour)
	task, err := svc.CreateTask(ctx, owner, project.ID, TaskInput{Title: "Late", DueAt: &past})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if raised, _, err := svc.Sweep(ctx, 10); err != nil || raised != 1 {
		t.Fatalf("raise sweep raised=%d err=%v, want 1", raised, err)
	}

	if _, err := svc.UpdateTask(ctx, owner, task.ID, task.Version, TaskInput{Title: "Late", Status: TaskInProgress}); err != nil {
		t.Fatalf("UpdateTask in progress: %v", err)
	}
	progress, err := svc.GetTask(ctx, task.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if _, err := svc.UpdateTask(ctx, owner, task.ID, progress.Version, TaskInput{Title: "Late", Status: TaskDone}); err != nil {
		t.Fatalf("UpdateTask done: %v", err)
	}

	raised, resolved, err := svc.Sweep(ctx, 10)
	if err != nil {
		t.Fatalf("resolve sweep: %v", err)
	}
	if raised != 0 || resolved != 1 {
		t.Fatalf("sweep raised=%d resolved=%d, want 0/1", raised, resolved)
	}
	opens, err := svc.ListEscalations(ctx, ListEscalationsQuery{EntityType: "task", EntityID: &task.ID, Status: EscalationOpen}, 10, nil)
	if err != nil || len(opens) != 0 {
		t.Fatalf("open escalations = %d, err = %v, want 0", len(opens), err)
	}
}
