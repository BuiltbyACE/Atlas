package people

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Availability returns the roster for a day: every non-exited person, with the
// approved leave that covers that day (if any). It backs
// GET /api/v1/people/availability?date= (§8.5). A zero date means today.
//
// "Available" here means only "no recorded leave covering the day". The
// blueprint's other absences (sick, unplanned, public holiday) are absence rows
// it does not yet model, so this is deliberately the leave-backed view.
func (s *Service) Availability(ctx context.Context, date time.Time) ([]AvailabilityEntry, error) {
	if date.IsZero() {
		date = s.now()
	}

	persons, err := s.store.ListPersons(ctx)
	if err != nil {
		return nil, err
	}
	overlaps, err := s.store.ListApprovedLeaveOn(ctx, dateOnly(date))
	if err != nil {
		return nil, err
	}

	onLeave := make(map[uuid.UUID]LeaveRequest, len(overlaps))
	for _, l := range overlaps {
		onLeave[l.PersonID] = l
	}

	entries := make([]AvailabilityEntry, 0, len(persons))
	for _, p := range persons {
		entry := AvailabilityEntry{Person: p}
		if l, ok := onLeave[p.ID]; ok {
			leave := l
			entry.Leave = &leave
		}
		entries = append(entries, entry)
	}
	return entries, nil
}
