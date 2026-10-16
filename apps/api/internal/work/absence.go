package work

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/safaristack/atlas/apps/api/internal/outbox"
)

// RequestAbsence records a leave request. A person cannot hold two active
// (requested or approved) absences covering the same day, which keeps the
// workload view honest. Emits work.absence.requested.v1.
func (s *Service) RequestAbsence(ctx context.Context, actor uuid.UUID, in AbsenceInput) (Absence, error) {
	a, err := validateAbsence(Absence{
		PersonID: actor,
		Type:     in.Type,
		FromDate: in.FromDate,
		ToDate:   in.ToDate,
		Reason:   in.Reason,
		Status:   AbsenceRequested,
		Version:  1,
	})
	if err != nil {
		return Absence{}, err
	}
	overlap, err := s.store.OverlappingAbsence(ctx, actor, a.FromDate, a.ToDate)
	if err != nil {
		return Absence{}, err
	}
	if overlap {
		return Absence{}, fmt.Errorf("%w: an absence already covers part of that period", ErrConflict)
	}
	id, err := uuid.NewV7()
	if err != nil {
		return Absence{}, fmt.Errorf("work: new id: %w", err)
	}
	now := s.now()
	a.ID = id
	a.CreatedAt, a.UpdatedAt = now, now

	ev, err := outbox.NewAbsenceRequested(&actor, a.ID, actor, string(a.Type), dateString(a.FromDate), dateString(a.ToDate))
	if err != nil {
		return Absence{}, err
	}
	created, err := s.store.CreateAbsence(ctx, a, &ev)
	if err != nil {
		return Absence{}, err
	}
	s.writeAudit(ctx, actor, "work.absence.requested", EntityAbsence, created.ID, nil, absenceSnapshot(created))
	return created, nil
}

// GetAbsence returns an absence or ErrNotFound.
func (s *Service) GetAbsence(ctx context.Context, id uuid.UUID) (Absence, error) {
	a, err := s.store.GetAbsence(ctx, id)
	if err != nil {
		return Absence{}, err
	}
	if a == nil {
		return Absence{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	return *a, nil
}

// ListAbsences returns a page of absences, newest first.
func (s *Service) ListAbsences(ctx context.Context, q ListAbsencesQuery, limit int, before *Cursor) ([]Absence, error) {
	if limit <= 0 {
		limit = 50
	}
	return s.store.ListAbsences(ctx, q, limit, before)
}

// DecideAbsence approves or declines a pending request. A person cannot decide
// their own request (separation of duties, §3.3). Emits
// work.absence.approved.v1 / work.absence.declined.v1.
func (s *Service) DecideAbsence(ctx context.Context, actor, absenceID uuid.UUID, approve bool, reason string) (Absence, error) {
	current, err := s.GetAbsence(ctx, absenceID)
	if err != nil {
		return Absence{}, err
	}
	if current.PersonID == actor {
		return Absence{}, fmt.Errorf("%w: you cannot decide your own absence", ErrForbidden)
	}
	if current.Status != AbsenceRequested {
		return Absence{}, fmt.Errorf("%w: only a requested absence can be decided", ErrConflict)
	}
	now := s.now()
	next := current
	next.ApproverID = &actor
	if approve {
		next.Status = AbsenceApproved
		next.ApprovedAt = &now
	} else {
		next.Status = AbsenceDeclined
		next.DeclinedAt = &now
		next.CancelReason = trim(reason)
	}

	var ev outbox.Event
	if approve {
		ev, err = outbox.NewAbsenceApproved(&actor, current.ID, current.PersonID, actor)
	} else {
		ev, err = outbox.NewAbsenceDeclined(&actor, current.ID, current.PersonID, actor, trim(reason))
	}
	if err != nil {
		return Absence{}, err
	}
	updated, err := s.store.UpdateAbsence(ctx, next, current.Version, &ev)
	if err != nil {
		return Absence{}, err
	}
	action := "work.absence.declined"
	if approve {
		action = "work.absence.approved"
	}
	s.writeAudit(ctx, actor, action, EntityAbsence, updated.ID, absenceSnapshot(current), absenceSnapshot(updated))
	return updated, nil
}

// CancelAbsence withdraws the caller's own pending or approved absence. Emits
// work.absence.cancelled.v1.
func (s *Service) CancelAbsence(ctx context.Context, actor, absenceID uuid.UUID, reason string) (Absence, error) {
	current, err := s.GetAbsence(ctx, absenceID)
	if err != nil {
		return Absence{}, err
	}
	if current.PersonID != actor {
		return Absence{}, fmt.Errorf("%w: you can only cancel your own absence", ErrForbidden)
	}
	if current.Status != AbsenceRequested && current.Status != AbsenceApproved {
		return Absence{}, fmt.Errorf("%w: this absence cannot be cancelled", ErrConflict)
	}
	next := current
	next.Status = AbsenceCancelled
	next.CancelReason = trim(reason)

	ev, err := outbox.NewAbsenceCancelled(&actor, current.ID, current.PersonID, actor, trim(reason))
	if err != nil {
		return Absence{}, err
	}
	updated, err := s.store.UpdateAbsence(ctx, next, current.Version, &ev)
	if err != nil {
		return Absence{}, err
	}
	s.writeAudit(ctx, actor, "work.absence.cancelled", EntityAbsence, updated.ID, absenceSnapshot(current), absenceSnapshot(updated))
	return updated, nil
}

func absenceSnapshot(a Absence) map[string]any {
	return map[string]any{
		"person_id":    a.PersonID.String(),
		"absence_type": string(a.Type),
		"from_date":    dateString(a.FromDate),
		"to_date":      dateString(a.ToDate),
		"status":       string(a.Status),
	}
}

func dateString(t time.Time) string { return t.Format("2006-01-02") }
