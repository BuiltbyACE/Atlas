package people

import (
	"context"

	"github.com/google/uuid"

	"github.com/safaristack/atlas/apps/api/internal/outbox"
)

// OffboardPerson ends a person's time at Atlas and closes their access (§8.5):
// in one transaction the person is marked exited with a last working day, their
// account (if any) is deactivated, and the account's sessions and role grants
// are revoked. Emits people.person.offboarded.v1 and writes an audit row.
//
// The blueprint's offboarding checklist, asset return and work handover are not
// modelled yet (there is no checklist table); this endpoint is the access
// lifecycle, which the account lifecycle requires to be immediate (§8.5, §7.1).
func (s *Service) OffboardPerson(ctx context.Context, actor, personID uuid.UUID, in OffboardInput) (OffboardResult, error) {
	end := dateOnly(s.now())
	if in.LastWorkingDay != nil {
		end = dateOnly(*in.LastWorkingDay)
	}

	// Resolve the account before the transaction so the event can carry the
	// realtime address of the account being closed (nil when they never had one).
	accountID, err := s.store.AccountForPerson(ctx, personID)
	if err != nil {
		return OffboardResult{}, err
	}
	ev, err := outbox.NewPersonOffboarded(&actor, personID, accountID, dateString(end), trim(in.Reason))
	if err != nil {
		return OffboardResult{}, err
	}

	res, err := s.store.OffboardPerson(ctx, personID, end, &ev)
	if err != nil {
		return OffboardResult{}, err
	}
	s.writeAudit(ctx, actor, "people.person.offboarded", EntityPerson, personID, nil, offboardSnapshot(res))
	return res, nil
}

func offboardSnapshot(res OffboardResult) map[string]any {
	snap := map[string]any{
		"person_id": res.Person.ID.String(),
		"status":    string(res.Person.Status),
		"version":   res.Person.Version,
	}
	if res.AccountID != nil {
		snap["account_id"] = res.AccountID.String()
		snap["account_status"] = res.AccountStatus
		snap["sessions_revoked"] = res.SessionsRevoked
		snap["roles_revoked"] = res.RolesRevoked
	}
	return snap
}
