// Package identity bridges the two identities Atlas keeps apart: the person
// (people.persons — who someone is) and the user account (core.user_accounts —
// whether, and how, they can be reached).
//
// Domain events name the person they are about; realtime topics are keyed by
// the account (§7.5, §10.1). Producers resolve person→account the same way, so
// the bridge lives here rather than being reinvented per module.
package identity

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Querier is the pgx subset shared by *pgxpool.Pool, pgx.Tx and pgx.Conn, so a
// caller can resolve through whichever handle it already holds.
type Querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// AccountForPerson returns the user account id linked to a person, or nil when
// the person has no account. A person exists independently of a login, so
// "no account" is a normal answer, not an error: the caller carries it as an
// optional identity and simply cannot nudge the person (§10.1).
func AccountForPerson(ctx context.Context, q Querier, personID uuid.UUID) (*uuid.UUID, error) {
	if personID == uuid.Nil {
		return nil, nil
	}
	var id uuid.UUID
	err := q.QueryRow(ctx, `SELECT id FROM core.user_accounts WHERE person_id = $1`, personID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("identity: account for person %s: %w", personID, err)
	}
	return &id, nil
}
