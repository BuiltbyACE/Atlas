package dashboard

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/safaristack/atlas/apps/api/internal/audit"
)

type fakeStore struct {
	people       map[string]int
	approvals    ApprovalsSummary
	work         WorkSummary
	peopleErr    error
	approvalsErr error
	workErr      error
	gotNow       time.Time
}

func (s *fakeStore) PeopleByStatus(context.Context) (map[string]int, error) {
	if s.peopleErr != nil {
		return nil, s.peopleErr
	}
	return s.people, nil
}

func (s *fakeStore) Approvals(_ context.Context, now time.Time) (ApprovalsSummary, error) {
	s.gotNow = now
	if s.approvalsErr != nil {
		return ApprovalsSummary{}, s.approvalsErr
	}
	return s.approvals, nil
}

func (s *fakeStore) Work(_ context.Context, now time.Time) (WorkSummary, error) {
	s.gotNow = now
	if s.workErr != nil {
		return WorkSummary{}, s.workErr
	}
	return s.work, nil
}

type fakeAuditStore struct {
	entries []audit.Entry
	err     error
}

func (s *fakeAuditStore) List(context.Context, audit.Query, int, *audit.Cursor) ([]audit.Entry, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.entries, nil
}

func TestSummaryBuildsTiles(t *testing.T) {
	entityID := uuid.New()
	store := &fakeStore{
		people:    map[string]int{"active": 3, "on_leave": 1},
		approvals: ApprovalsSummary{Pending: 2, Escalated: 1, Overdue: 1, PendingByRole: map[string]int{"ceo_finance": 2}},
		work:      WorkSummary{Overdue: 4, OpenEscalations: 2},
	}
	auditStore := &fakeAuditStore{entries: []audit.Entry{{
		ID:         uuid.New(),
		OccurredAt: time.Now().UTC(),
		ActorType:  "user",
		ActorID:    uuid.New().String(),
		Action:     "person.updated",
		EntityType: "person",
		EntityID:   &entityID,
	}}}

	svc := NewService(store, auditStore, nil)
	sum, err := svc.Summary(context.Background())
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if sum.People.Total != 4 {
		t.Fatalf("total = %d, want 4", sum.People.Total)
	}
	if sum.People.ByStatus["active"] != 3 || sum.People.ByStatus["on_leave"] != 1 {
		t.Fatalf("by_status = %+v", sum.People.ByStatus)
	}
	if sum.Approvals.Pending != 2 || sum.Approvals.Escalated != 1 || sum.Approvals.Overdue != 1 {
		t.Fatalf("approvals = %+v", sum.Approvals)
	}
	if sum.Approvals.PendingByRole["ceo_finance"] != 2 {
		t.Fatalf("pending_by_role = %+v", sum.Approvals.PendingByRole)
	}
	if sum.Work.Overdue != 4 || sum.Work.OpenEscalations != 2 {
		t.Fatalf("work = %+v", sum.Work)
	}
	if len(sum.RecentAudit) != 1 {
		t.Fatalf("recent_audit = %d, want 1", len(sum.RecentAudit))
	}
	if sum.RecentAudit[0].EntityID != entityID.String() || sum.RecentAudit[0].EntityType != "person" {
		t.Fatalf("highlight = %+v", sum.RecentAudit[0])
	}
	if sum.GeneratedAt.IsZero() {
		t.Fatalf("generated_at was not set")
	}
}

func TestSummaryFillsMissingStatuses(t *testing.T) {
	svc := NewService(&fakeStore{people: map[string]int{"active": 1}}, &fakeAuditStore{}, nil)
	sum, err := svc.Summary(context.Background())
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	for _, status := range PersonStatuses {
		if _, ok := sum.People.ByStatus[status]; !ok {
			t.Fatalf("by_status is missing %q: %+v", status, sum.People.ByStatus)
		}
	}
	if sum.People.ByStatus["exited"] != 0 {
		t.Fatalf("exited = %d, want 0", sum.People.ByStatus["exited"])
	}
}

func TestSummaryNilPendingByRoleBecomesEmptyMap(t *testing.T) {
	svc := NewService(&fakeStore{people: map[string]int{}}, &fakeAuditStore{}, nil)
	sum, err := svc.Summary(context.Background())
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if sum.Approvals.PendingByRole == nil {
		t.Fatalf("pending_by_role is nil, want an empty map so it serialises as {}")
	}
}

func TestSummaryUsesNow(t *testing.T) {
	fixed := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
	store := &fakeStore{people: map[string]int{}}
	svc := NewService(store, &fakeAuditStore{}, nil)
	svc.now = func() time.Time { return fixed }

	sum, err := svc.Summary(context.Background())
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if !sum.GeneratedAt.Equal(fixed) {
		t.Fatalf("generated_at = %v, want %v", sum.GeneratedAt, fixed)
	}
	if !store.gotNow.Equal(fixed) {
		t.Fatalf("store now = %v, want %v", store.gotNow, fixed)
	}
}

func TestSummaryPropagatesErrors(t *testing.T) {
	cases := map[string]*Service{
		"people": NewService(&fakeStore{peopleErr: errors.New("boom")}, &fakeAuditStore{}, nil),
		"approvals": NewService(&fakeStore{approvalsErr: errors.New("boom")},
			&fakeAuditStore{}, nil),
		"audit": NewService(&fakeStore{people: map[string]int{}},
			&fakeAuditStore{err: errors.New("boom")}, nil),
	}
	for name, svc := range cases {
		if _, err := svc.Summary(context.Background()); err == nil {
			t.Fatalf("%s: Summary returned no error", name)
		}
	}
}
