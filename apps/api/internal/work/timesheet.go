package work

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/safaristack/atlas/apps/api/internal/outbox"
	"github.com/safaristack/atlas/apps/api/internal/settings"
)

// D7 time entries (docs/ATLAS.md §8.2, docs/adr/ADR-019).
//
// A time entry is minutes one person logged against a task on a date. The
// project is derived from the task and stored alongside it so a person's week,
// a task's logged effort and a project's timesheet coverage are each a straight
// index scan. Whether timesheets are *required* for a project's type is policy,
// not schema: it is the typed setting work.timesheet.required_project_types,
// read through the audited settings API.

// TimeEntry is a row in work.time_entries.
type TimeEntry struct {
	ID        uuid.UUID
	PersonID  uuid.UUID
	TaskID    uuid.UUID
	ProjectID uuid.UUID
	// EntryDate is the calendar date the work belongs to (UTC midnight; the
	// column is a Postgres date, so there is no time-of-day).
	EntryDate time.Time
	Minutes   int
	Note      string
	Version   int
	CreatedAt time.Time
	UpdatedAt time.Time
}

// TimeEntryInput is the payload for recording or replacing a time entry. When
// PersonID is nil the caller logs their own time; a project lead may pass the
// person the hours belong to.
type TimeEntryInput struct {
	PersonID  *uuid.UUID
	TaskID    uuid.UUID
	EntryDate time.Time
	Minutes   int
	Note      string
}

// ListTimeEntriesQuery filters a time-entry listing. At most one of PersonID,
// TaskID or ProjectID narrows it; From and To bound entry_date inclusively.
type ListTimeEntriesQuery struct {
	PersonID  *uuid.UUID
	TaskID    *uuid.UUID
	ProjectID *uuid.UUID
	From      *time.Time
	To        *time.Time
}

// ── policy (D7) ─────────────────────────────────────────────────────────────

// TimesheetPolicy is decision D7's decided configuration: the project types
// whose work requires timesheets. Project types are free text (§8.2), so the
// comparison is case-insensitive.
type TimesheetPolicy struct {
	RequiredProjectTypes []string
}

// DefaultTimesheetPolicy is D7's decided starting configuration (ADR-019):
// client-delivery projects require timesheets; every other type is optional.
// The settings catalogue serves the same list, so this only applies where no
// settings store is wired (tests).
func DefaultTimesheetPolicy() TimesheetPolicy {
	return TimesheetPolicy{RequiredProjectTypes: []string{"client_delivery"}}
}

// Required reports whether a project of the given type requires timesheets. An
// unknown or empty type requires none.
func (p TimesheetPolicy) Required(projectType string) bool {
	want := strings.ToLower(strings.TrimSpace(projectType))
	if want == "" {
		return false
	}
	for _, t := range p.RequiredProjectTypes {
		if strings.ToLower(strings.TrimSpace(t)) == want {
			return true
		}
	}
	return false
}

func (p TimesheetPolicy) normalize() TimesheetPolicy {
	out := TimesheetPolicy{}
	seen := make(map[string]struct{})
	for _, t := range p.RequiredProjectTypes {
		key := strings.ToLower(strings.TrimSpace(t))
		if key == "" {
			continue
		}
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out.RequiredProjectTypes = append(out.RequiredProjectTypes, key)
	}
	return out
}

// TimesheetReader supplies the D7 policy when the work module decorates a
// project with a timesheet-required flag.
type TimesheetReader interface {
	TimesheetPolicy(ctx context.Context) (TimesheetPolicy, error)
}

// StaticTimesheetPolicy serves one fixed policy; tests and processes without a
// settings store use it.
type StaticTimesheetPolicy struct{ Value TimesheetPolicy }

// TimesheetPolicy implements TimesheetReader.
func (s StaticTimesheetPolicy) TimesheetPolicy(context.Context) (TimesheetPolicy, error) {
	return s.Value.normalize(), nil
}

// SettingsTimesheetPolicy reads the policy from the typed settings store (§6.7).
// A settings failure is returned rather than papered over: the caller decides
// whether the flag is advisory for that read.
type SettingsTimesheetPolicy struct{ settings *settings.Service }

// NewSettingsTimesheetPolicy builds a reader over the settings service.
func NewSettingsTimesheetPolicy(s *settings.Service) *SettingsTimesheetPolicy {
	return &SettingsTimesheetPolicy{settings: s}
}

// TimesheetPolicy implements TimesheetReader.
func (r *SettingsTimesheetPolicy) TimesheetPolicy(ctx context.Context) (TimesheetPolicy, error) {
	value, err := stringSetting(ctx, r.settings, "work.timesheet.required_project_types")
	if err != nil {
		return TimesheetPolicy{}, err
	}
	return TimesheetPolicy{RequiredProjectTypes: parseProjectTypes(value)}.normalize(), nil
}

// SetTimesheetPolicy wires the D7 policy. A nil reader means the flag falls back
// to DefaultTimesheetPolicy, so a process without a settings store still behaves
// as decided.
func (s *Service) SetTimesheetPolicy(r TimesheetReader) { s.timesheet = r }

// TimesheetPolicy returns the current D7 policy, falling back to the decided
// default when no settings store is wired or the read fails. The flag is
// advisory: a policy read must never fail a project listing.
func (s *Service) TimesheetPolicy(ctx context.Context) TimesheetPolicy {
	if s.timesheet == nil {
		return DefaultTimesheetPolicy().normalize()
	}
	p, err := s.timesheet.TimesheetPolicy(ctx)
	if err != nil {
		s.logger.ErrorContext(ctx, "work: timesheet policy read failed", slog.String("error", err.Error()))
		return DefaultTimesheetPolicy().normalize()
	}
	return p.normalize()
}

// parseProjectTypes splits the comma-separated setting value.
func parseProjectTypes(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// ── service ─────────────────────────────────────────────────────────────────

// RecordTimeEntry validates and inserts a time entry, writing an audit row and a
// work.time_entry.logged.v1 event in the same transaction.
func (s *Service) RecordTimeEntry(ctx context.Context, actor uuid.UUID, in TimeEntryInput) (TimeEntry, error) {
	e, err := s.validateTimeEntry(ctx, in)
	if err != nil {
		return TimeEntry{}, err
	}
	if e.PersonID == uuid.Nil {
		e.PersonID = actor
	}
	if e.PersonID == uuid.Nil {
		e.PersonID = actor
	}

	id, err := uuid.NewV7()
	if err != nil {
		return TimeEntry{}, fmt.Errorf("work: new id: %w", err)
	}
	e.ID = id
	now := s.now()
	e.CreatedAt, e.UpdatedAt = now, now
	e.Version = 1

	ev, err := outbox.NewTimeEntryLogged(&actor, timeEntryRef(e), actor)
	if err != nil {
		return TimeEntry{}, err
	}
	created, err := s.store.CreateTimeEntry(ctx, e, &ev)
	if err != nil {
		return TimeEntry{}, err
	}
	s.writeAudit(ctx, actor, "work.time_entry.logged", EntityTimeEntry, created.ID, nil, timeEntrySnapshot(created))
	return created, nil
}

// GetTimeEntry returns a time entry or ErrNotFound.
func (s *Service) GetTimeEntry(ctx context.Context, id uuid.UUID) (TimeEntry, error) {
	e, err := s.store.GetTimeEntry(ctx, id)
	if err != nil {
		return TimeEntry{}, err
	}
	if e == nil {
		return TimeEntry{}, fmt.Errorf("%w: time entry %s", ErrNotFound, id)
	}
	return *e, nil
}

// ListTimeEntries returns a page of time entries, newest first.
func (s *Service) ListTimeEntries(ctx context.Context, q ListTimeEntriesQuery, limit int, before *Cursor) ([]TimeEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	return s.store.ListTimeEntries(ctx, q, limit, before)
}

// UpdateTimeEntry replaces an existing entry. The task may change, in which case
// the project is re-derived. A stale expectedVersion is an ErrConflict.
func (s *Service) UpdateTimeEntry(ctx context.Context, actor, id uuid.UUID, expectedVersion int, in TimeEntryInput) (TimeEntry, error) {
	cur, err := s.GetTimeEntry(ctx, id)
	if err != nil {
		return TimeEntry{}, err
	}
	// An omitted person keeps the entry's owner; the person is not what an
	// update is usually changing.
	if in.PersonID == nil {
		person := cur.PersonID
		in.PersonID = &person
	}
	next, err := s.validateTimeEntry(ctx, in)
	if err != nil {
		return TimeEntry{}, err
	}
	next.ID = id
	next.CreatedAt = cur.CreatedAt
	next.UpdatedAt = s.now()

	ev, err := outbox.NewTimeEntryUpdated(&actor, timeEntryRef(next), actor)
	if err != nil {
		return TimeEntry{}, err
	}
	saved, err := s.store.UpdateTimeEntry(ctx, next, expectedVersion, &ev)
	if err != nil {
		return TimeEntry{}, err
	}
	s.writeAudit(ctx, actor, "work.time_entry.updated", EntityTimeEntry, saved.ID,
		timeEntrySnapshot(cur), timeEntrySnapshot(saved))
	return saved, nil
}

// DeleteTimeEntry removes an entry, writing an audit row and a
// work.time_entry.removed.v1 event in the same transaction.
func (s *Service) DeleteTimeEntry(ctx context.Context, actor, id uuid.UUID) error {
	cur, err := s.GetTimeEntry(ctx, id)
	if err != nil {
		return err
	}
	ev, err := outbox.NewTimeEntryRemoved(&actor, timeEntryRef(cur), actor)
	if err != nil {
		return err
	}
	if err := s.store.DeleteTimeEntry(ctx, id, &ev); err != nil {
		return err
	}
	s.writeAudit(ctx, actor, "work.time_entry.removed", EntityTimeEntry, id, timeEntrySnapshot(cur), nil)
	return nil
}

// validateTimeEntry checks the entry and resolves the task, filling PersonID
// (defaulting to the actor) and ProjectID from it. The task must exist and
// active: a task that was cancelled still accepts hours already worked, but an
// unknown one does not.
func (s *Service) validateTimeEntry(ctx context.Context, in TimeEntryInput) (TimeEntry, error) {
	if in.TaskID == uuid.Nil {
		return TimeEntry{}, fmt.Errorf("%w: task_id is required", ErrInvalid)
	}
	if in.Minutes <= 0 || in.Minutes > 1440 {
		return TimeEntry{}, fmt.Errorf("%w: minutes must be between 1 and 1440", ErrInvalid)
	}
	date := dateOnly(in.EntryDate)
	if date.IsZero() {
		return TimeEntry{}, fmt.Errorf("%w: entry_date is required", ErrInvalid)
	}

	task, err := s.GetTask(ctx, in.TaskID)
	if err != nil {
		return TimeEntry{}, err
	}

	person := uuid.Nil
	if in.PersonID != nil {
		person = *in.PersonID
	}

	return TimeEntry{
		PersonID:  person,
		TaskID:    in.TaskID,
		ProjectID: task.ProjectID,
		EntryDate: date,
		Minutes:   in.Minutes,
		Note:      trim(in.Note),
	}, nil
}

func timeEntrySnapshot(e TimeEntry) map[string]any {
	return map[string]any{
		"person_id":  e.PersonID.String(),
		"task_id":    e.TaskID.String(),
		"project_id": e.ProjectID.String(),
		"entry_date": dateString(e.EntryDate),
		"minutes":    e.Minutes,
		"note":       e.Note,
	}
}

// timeEntryRef is the minimal view the outbox builders need.
func timeEntryRef(e TimeEntry) outbox.TimeEntryRef {
	return outbox.TimeEntryRef{
		ID: e.ID, ProjectID: e.ProjectID, TaskID: e.TaskID,
		PersonID: e.PersonID, EntryDate: dateString(e.EntryDate), Minutes: e.Minutes,
	}
}

// dateOnly normalises a timestamp to the UTC midnight of its calendar date,
// which is how Postgres stores a date.
func dateOnly(t time.Time) time.Time {
	if t.IsZero() {
		return time.Time{}
	}
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
}

func dateString(t time.Time) string { return dateOnly(t).Format("2006-01-02") }
