package work

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestProjectStatusValid(t *testing.T) {
	for _, s := range []ProjectStatus{ProjectPlanning, ProjectActive, ProjectPaused, ProjectCompleted, ProjectCancelled} {
		if !s.Valid() {
			t.Errorf("%q should be valid", s)
		}
	}
	if ProjectStatus("bogus").Valid() {
		t.Error("bogus project status should be invalid")
	}
}

func TestTaskStatusTerminal(t *testing.T) {
	if !TaskDone.Terminal() || !TaskCancelled.Terminal() {
		t.Error("done and cancelled should be terminal")
	}
	if TaskInProgress.Terminal() {
		t.Error("in_progress should not be terminal")
	}
}

func TestValidateProject(t *testing.T) {
	if _, err := validateProject(Project{Name: "  "}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("blank name: err = %v, want ErrInvalid", err)
	}
	p, err := validateProject(Project{Name: "  Apollo  "})
	if err != nil {
		t.Fatalf("valid project: %v", err)
	}
	if p.Name != "Apollo" {
		t.Fatalf("name = %q, want trimmed", p.Name)
	}
	if p.Status != ProjectPlanning {
		t.Fatalf("default status = %q, want planning", p.Status)
	}

	start := time.Now().UTC()
	due := start.Add(-time.Hour)
	if _, err := validateProject(Project{Name: "x", StartedAt: &start, DueAt: &due}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("due before start: err = %v, want ErrInvalid", err)
	}
}

func TestValidateTask(t *testing.T) {
	if _, err := validateTask(Task{Title: ""}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("blank title: err = %v, want ErrInvalid", err)
	}
	tk, err := validateTask(Task{Title: "  Write spec  "})
	if err != nil {
		t.Fatalf("valid task: %v", err)
	}
	if tk.Status != TaskTodo || tk.Priority != PriorityMedium {
		t.Fatalf("defaults = %s/%s, want todo/medium", tk.Status, tk.Priority)
	}
	neg := -1.0
	if _, err := validateTask(Task{Title: "x", EstimateHours: &neg}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("negative estimate: err = %v, want ErrInvalid", err)
	}
}

func TestAllowedProjectTransition(t *testing.T) {
	cases := []struct {
		from, to ProjectStatus
		want     bool
	}{
		{ProjectPlanning, ProjectActive, true},
		{ProjectPlanning, ProjectCompleted, false},
		{ProjectActive, ProjectPaused, true},
		{ProjectActive, ProjectCompleted, true},
		{ProjectPaused, ProjectActive, true},
		{ProjectCompleted, ProjectActive, false},
		{ProjectCancelled, ProjectActive, false},
	}
	for _, c := range cases {
		if got := allowedProjectTransition(c.from, c.to); got != c.want {
			t.Errorf("%s -> %s = %v, want %v", c.from, c.to, got, c.want)
		}
	}
}

func TestAllowedTaskTransition(t *testing.T) {
	cases := []struct {
		from, to TaskStatus
		want     bool
	}{
		{TaskTodo, TaskInProgress, true},
		{TaskTodo, TaskDone, false},
		{TaskInProgress, TaskReview, true},
		{TaskInProgress, TaskDone, true},
		{TaskBlocked, TaskInProgress, true},
		{TaskReview, TaskDone, true},
		{TaskDone, TaskInProgress, false},
		{TaskCancelled, TaskTodo, false},
	}
	for _, c := range cases {
		if got := allowedTaskTransition(c.from, c.to); got != c.want {
			t.Errorf("%s -> %s = %v, want %v", c.from, c.to, got, c.want)
		}
	}
}

func TestSameUUID(t *testing.T) {
	a := uuid.New()
	b := uuid.New()
	if !sameUUID(nil, nil) {
		t.Error("nil,nil should be equal")
	}
	if sameUUID(&a, nil) || sameUUID(nil, &a) {
		t.Error("nil vs non-nil should differ")
	}
	if !sameUUID(&a, &a) {
		t.Error("same value should be equal")
	}
	if sameUUID(&a, &b) {
		t.Error("different ids should differ")
	}
}
