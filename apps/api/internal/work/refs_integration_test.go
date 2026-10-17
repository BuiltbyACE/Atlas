package work

import (
	"context"
	"regexp"
	"strings"
	"testing"
)

var projectRefPattern = regexp.MustCompile(`^PRJ-[0-9]{4}-[0-9]{3,}$`)

// TestProjectAndTaskRefs covers D6's human refs: allocation on create, the
// project-prefixed task ref, and resolution by ref for the GitHub link.
func TestProjectAndTaskRefs(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	svc := NewService(NewPGStore(pool), nil, nil, nil, nil)
	actor := mustPerson(t, pool)

	project, err := svc.CreateProject(ctx, actor, ProjectInput{Name: "Refs project"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM work.projects WHERE id = $1`, project.ID)
	})

	if !projectRefPattern.MatchString(project.Ref) {
		t.Fatalf("project ref %q does not match PRJ-YYYY-NNN", project.Ref)
	}

	task, err := svc.CreateTask(ctx, actor, project.ID, TaskInput{Title: "First"})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if !strings.HasPrefix(task.Ref, project.Ref+"/T-") {
		t.Fatalf("task ref %q does not start with %q", task.Ref, project.Ref+"/T-")
	}

	got, found, err := svc.TaskIDByRef(ctx, task.Ref)
	if err != nil {
		t.Fatalf("TaskIDByRef: %v", err)
	}
	if !found || got != task.ID {
		t.Fatalf("TaskIDByRef = %s/%v, want %s/true", got, found, task.ID)
	}

	if _, found, err := svc.TaskIDByRef(ctx, "PRJ-0000-000/T-0000"); err != nil || found {
		t.Fatalf("unknown ref = %v/%v, want false/nil", found, err)
	}
}

// TestAttachIntegrationEvidence covers the §6.5 polymorphic evidence shape the
// GitHub link uses: a reference-only row with no file and no submitter, which a
// person may still verify.
func TestAttachIntegrationEvidence(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	svc := NewService(NewPGStore(pool), nil, nil, nil, nil)
	actor := mustPerson(t, pool)

	project, err := svc.CreateProject(ctx, actor, ProjectInput{Name: "PR evidence"})
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM work.projects WHERE id = $1`, project.ID)
	})
	task, err := svc.CreateTask(ctx, actor, project.ID, TaskInput{Title: "PR task"})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	ev, err := svc.AttachIntegrationEvidence(ctx, EvidenceInput{
		TaskID:       &task.ID,
		EvidenceType: EvidenceTypePullRequest,
		Reference:    "https://github.com/safaristack/atlas/pull/7",
		Kind:         "pull request",
	})
	if err != nil {
		t.Fatalf("AttachIntegrationEvidence: %v", err)
	}
	if ev.EvidenceType != EvidenceTypePullRequest {
		t.Fatalf("evidence type = %q", ev.EvidenceType)
	}
	if ev.FileID != nil {
		t.Fatal("pull_request evidence must not carry a file")
	}
	if ev.UploaderID != nil {
		t.Fatal("integration evidence must not name a person")
	}
	if ev.ProjectID == nil || *ev.ProjectID != project.ID {
		t.Fatalf("project = %v, want %s", ev.ProjectID, project.ID)
	}

	verifier := mustPerson(t, pool)
	verified, err := svc.VerifyEvidence(ctx, verifier, ev.ID)
	if err != nil {
		t.Fatalf("VerifyEvidence: %v", err)
	}
	if verified.VerifiedBy == nil || *verified.VerifiedBy != verifier {
		t.Fatalf("verified_by = %v, want %s", verified.VerifiedBy, verifier)
	}

	// `file` still needs a file; every other type still needs a reference.
	if _, err := svc.AttachIntegrationEvidence(ctx, EvidenceInput{TaskID: &task.ID}); err == nil {
		t.Fatal("expected file_id to be required for file evidence")
	}
	if _, err := svc.AttachIntegrationEvidence(ctx, EvidenceInput{TaskID: &task.ID, EvidenceType: EvidenceTypeURL}); err == nil {
		t.Fatal("expected reference to be required for url evidence")
	}

	// The same reference attaches once per task: a redelivered webhook finds
	// the row it recorded, it does not create a second.
	again, err := svc.AttachIntegrationEvidence(ctx, EvidenceInput{
		TaskID:       &task.ID,
		EvidenceType: EvidenceTypePullRequest,
		Reference:    "https://github.com/safaristack/atlas/pull/7",
		Kind:         "pull request",
	})
	if err != nil {
		t.Fatalf("re-attach: %v", err)
	}
	if again.ID != ev.ID {
		t.Fatalf("re-attach = %s, want the existing %s", again.ID, ev.ID)
	}
	rows, err := svc.ListEvidence(ctx, ListEvidenceQuery{TaskID: &task.ID})
	if err != nil {
		t.Fatalf("ListEvidence: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("evidence rows = %d, want 1", len(rows))
	}
}
