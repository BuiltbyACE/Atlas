package approvals

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/safaristack/atlas/apps/api/internal/outbox"
)

// Cursor is keyset position for list endpoints.
type Cursor struct {
	At time.Time
	ID uuid.UUID
}

// Store is the approvals persistence interface.
type Store interface {
	// CreateWithEvent writes approval, history and outbox event in one tx.
	CreateWithEvent(ctx context.Context, a Approval, hist []History, ev outbox.Event) error
	Get(ctx context.Context, id uuid.UUID) (*Approval, error)
	GetByRef(ctx context.Context, ref string) (*Approval, error)
	ListPendingForRole(ctx context.Context, role string, limit int, before *Cursor) ([]Approval, error)
	ListRequestedBy(ctx context.Context, requestedBy uuid.UUID, limit int, before *Cursor) ([]Approval, error)
	// LookupPolicy returns the most specific active policy for a decision, or
	// nil when none matches.
	LookupPolicy(ctx context.Context, q PolicyQuery) (*Policy, error)
	// ListPolicies returns every policy, newest first (keyset by
	// effective_from then id), for the configuration screen.
	ListPolicies(ctx context.Context, limit int, before *Cursor) ([]Policy, error)
	// PeopleWithRole resolves a role key to the active people who hold it, for
	// notification fan-out.
	PeopleWithRole(ctx context.Context, role string) ([]uuid.UUID, error)
	// ApprovalProgress reports the approval's status and how many distinct
	// approvals have been recorded so far, plus the first approver's id (the
	// zero UUID when none), so the service can enforce the two-person rule.
	ApprovalProgress(ctx context.Context, id uuid.UUID) (Status, int, uuid.UUID, error)
	// DecideWithEvent records one decision. The approval becomes approved only
	// when Required distinct approvals exist; a decline is always final. The
	// event (when non-nil) is written in the same transaction as a finalising
	// decision. It returns the resulting status.
	DecideWithEvent(ctx context.Context, d Decision, ev *outbox.Event) (Status, error)
	EscalateWithEvent(ctx context.Context, id uuid.UUID, actor uuid.UUID, toRole string, toPerson *uuid.UUID, reason string, ev outbox.Event) error
	CancelWithEvent(ctx context.Context, id uuid.UUID, actor uuid.UUID, reason string, ev outbox.Event) error
	ExpireWithEvent(ctx context.Context, id uuid.UUID, ev outbox.Event) error
	ClaimExpired(ctx context.Context, limit int) ([]Approval, error)
	// ExpireDue claims up to limit overdue approvals, marks them expired and
	// writes their history and outbox events in one transaction. buildEvent
	// supplies the event for each approval, so event construction stays in the
	// service. FOR UPDATE SKIP LOCKED makes a second worker skip claimed rows.
	ExpireDue(ctx context.Context, limit int, buildEvent func(Approval) (outbox.Event, error)) ([]Approval, error)
}

// History is an approval history row.
type History struct {
	ID         uuid.UUID
	ApprovalID uuid.UUID
	ActorID    *uuid.UUID
	Action     string
	Reason     string
	FromStatus string
	ToStatus   string
	CreatedAt  time.Time
}

// PGStore implements Store over Postgres.
type PGStore struct{ db *pgxpool.Pool }

func NewPGStore(db *pgxpool.Pool) *PGStore { return &PGStore{db: db} }

const approvalCols = `id, ref, entity_type, entity_id, requested_by, amount_minor, currency,
	approver_role, approver_id, status, policy_ref, approvals_required, due_at, decided_at, decided_by,
	reason, version, created_at, updated_at, archived_at`

func (s *PGStore) CreateWithEvent(ctx context.Context, a Approval, hist []History, ev outbox.Event) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("approvals: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	_, err = tx.Exec(ctx, `
		INSERT INTO core.approvals (
			id, ref, entity_type, entity_id, requested_by, amount_minor, currency,
			approver_role, approver_id, status, policy_ref, approvals_required, due_at, reason, version, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,now(),now())
	`, a.ID, a.Ref, a.EntityType, a.EntityID, a.RequestedBy, a.AmountMinor, a.Currency,
		a.ApproverRole, a.ApproverID, string(a.Status), a.PolicyRef, a.ApprovalsRequired, timePtr(a.DueAt), a.Reason, a.Version)
	if err != nil {
		return fmt.Errorf("approvals: insert: %w", err)
	}

	for _, h := range hist {
		_, err = tx.Exec(ctx, `
			INSERT INTO core.approval_history (
				id, approval_id, actor_id, action, reason, from_status, to_status, created_at
			) VALUES ($1,$2,$3,$4,$5,$6,$7,now())
		`, h.ID, h.ApprovalID, h.ActorID, h.Action, h.Reason, h.FromStatus, h.ToStatus)
		if err != nil {
			return fmt.Errorf("approvals: history: %w", err)
		}
	}

	if _, err := outbox.Write(ctx, tx, ev); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("approvals: commit: %w", err)
	}
	return nil
}

func (s *PGStore) Get(ctx context.Context, id uuid.UUID) (*Approval, error) {
	var a Approval
	err := s.db.QueryRow(ctx, `SELECT `+approvalCols+` FROM core.approvals WHERE id=$1 AND archived_at IS NULL`, id).
		Scan(&a.ID, &a.Ref, &a.EntityType, &a.EntityID, &a.RequestedBy, &a.AmountMinor, &a.Currency,
			&a.ApproverRole, &a.ApproverID, &a.Status, &a.PolicyRef, &a.ApprovalsRequired, &a.DueAt, &a.DecidedAt, &a.DecidedBy,
			&a.Reason, &a.Version, &a.CreatedAt, &a.UpdatedAt, &a.ArchivedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("approvals: get: %w", err)
	}
	return &a, nil
}

func (s *PGStore) GetByRef(ctx context.Context, ref string) (*Approval, error) {
	var a Approval
	err := s.db.QueryRow(ctx, `SELECT `+approvalCols+` FROM core.approvals WHERE ref=$1 AND archived_at IS NULL`, ref).
		Scan(&a.ID, &a.Ref, &a.EntityType, &a.EntityID, &a.RequestedBy, &a.AmountMinor, &a.Currency,
			&a.ApproverRole, &a.ApproverID, &a.Status, &a.PolicyRef, &a.ApprovalsRequired, &a.DueAt, &a.DecidedAt, &a.DecidedBy,
			&a.Reason, &a.Version, &a.CreatedAt, &a.UpdatedAt, &a.ArchivedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("approvals: get ref: %w", err)
	}
	return &a, nil
}

func (s *PGStore) ListPendingForRole(ctx context.Context, role string, limit int, before *Cursor) ([]Approval, error) {
	var beforeTS *time.Time
	var beforeID uuid.UUID = uuid.Nil
	if before != nil {
		ts := before.At.UTC()
		beforeTS = &ts
		beforeID = before.ID
	}

	rows, err := s.db.Query(ctx, `
		SELECT `+approvalCols+`
		FROM core.approvals
		WHERE status IN ('requested','pending') AND approver_role=$1 AND archived_at IS NULL
		  AND ($2::timestamptz IS NULL OR (created_at,id) < ($2::timestamptz,$3::uuid))
		ORDER BY created_at DESC, id DESC
		LIMIT $4
	`, strings.ToLower(role), timePtr(beforeTS), beforeID, limit)
	if err != nil {
		return nil, fmt.Errorf("approvals: list pending: %w", err)
	}
	defer rows.Close()

	out := make([]Approval, 0, limit)
	for rows.Next() {
		var a Approval
		if err := rows.Scan(&a.ID, &a.Ref, &a.EntityType, &a.EntityID, &a.RequestedBy, &a.AmountMinor, &a.Currency,
			&a.ApproverRole, &a.ApproverID, &a.Status, &a.PolicyRef, &a.ApprovalsRequired, &a.DueAt, &a.DecidedAt, &a.DecidedBy,
			&a.Reason, &a.Version, &a.CreatedAt, &a.UpdatedAt, &a.ArchivedAt); err != nil {
			return nil, fmt.Errorf("approvals: scan: %w", err)
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *PGStore) ListRequestedBy(ctx context.Context, requestedBy uuid.UUID, limit int, before *Cursor) ([]Approval, error) {
	var beforeTS *time.Time
	var beforeID uuid.UUID = uuid.Nil
	if before != nil {
		ts := before.At.UTC()
		beforeTS = &ts
		beforeID = before.ID
	}

	rows, err := s.db.Query(ctx, `
		SELECT `+approvalCols+`
		FROM core.approvals
		WHERE requested_by=$1 AND archived_at IS NULL
		  AND ($2::timestamptz IS NULL OR (created_at,id) < ($2::timestamptz,$3::uuid))
		ORDER BY created_at DESC, id DESC
		LIMIT $4
	`, requestedBy, timePtr(beforeTS), beforeID, limit)
	if err != nil {
		return nil, fmt.Errorf("approvals: list requested: %w", err)
	}
	defer rows.Close()

	out := make([]Approval, 0, limit)
	for rows.Next() {
		var a Approval
		if err := rows.Scan(&a.ID, &a.Ref, &a.EntityType, &a.EntityID, &a.RequestedBy, &a.AmountMinor, &a.Currency,
			&a.ApproverRole, &a.ApproverID, &a.Status, &a.PolicyRef, &a.ApprovalsRequired, &a.DueAt, &a.DecidedAt, &a.DecidedBy,
			&a.Reason, &a.Version, &a.CreatedAt, &a.UpdatedAt, &a.ArchivedAt); err != nil {
			return nil, fmt.Errorf("approvals: scan: %w", err)
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// DecideWithEvent records one decision in core.approval_decisions and finalises
// the approval only once the required number of distinct people have approved
// (§3.3 both-CEO decisions). The whole thing — the decision row, the state
// transition, history and the outbox event — is one transaction, so a crash
// can never leave an approval approved without its event.
//
// A repeated decision from the same person is rejected by the
// (approval_id, approver_id) primary key before anything else is written; that
// is the database-level counterpart of authz.TwoPersonRule.
func (s *PGStore) DecideWithEvent(ctx context.Context, d Decision, ev *outbox.Event) (Status, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("approvals: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var (
		status   Status
		required int
	)
	err = tx.QueryRow(ctx, `
		SELECT status, approvals_required
		FROM core.approvals
		WHERE id=$1 AND archived_at IS NULL
		FOR UPDATE
	`, d.ApprovalID).Scan(&status, &required)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("approvals: lock: %w", err)
	}
	if status != StatusRequested && status != StatusPending {
		return "", ErrNotFound
	}
	if required < 1 {
		required = 1
	}
	if d.Required >= 1 {
		required = d.Required
	}

	final := true
	newStatus := StatusDeclined
	action := "decline"

	if d.Approve {
		tag, err := tx.Exec(ctx, `
			INSERT INTO core.approval_decisions (approval_id, approver_id, decision, comment, decided_at)
			VALUES ($1,$2,'approve',$3,now())
			ON CONFLICT (approval_id, approver_id) DO NOTHING
		`, d.ApprovalID, d.ActorID, d.Reason)
		if err != nil {
			return "", fmt.Errorf("approvals: decision: %w", err)
		}
		if tag.RowsAffected() == 0 {
			// The same person already approved: a second eye must be someone else.
			return "", fmt.Errorf("%w: %s", ErrForbidden, "this person has already approved")
		}

		var approvals int
		if err := tx.QueryRow(ctx, `
			SELECT count(*) FROM core.approval_decisions
			WHERE approval_id=$1 AND decision='approve'
		`, d.ApprovalID).Scan(&approvals); err != nil {
			return "", fmt.Errorf("approvals: count decisions: %w", err)
		}

		final = approvals >= required
		newStatus = StatusPending
		action = "approve"
		if final {
			newStatus = StatusApproved
		}
	}

	if final {
		_, err = tx.Exec(ctx, `
			UPDATE core.approvals
			SET status=$2, decided_at=now(), decided_by=$3, reason=$4, version=version+1, updated_at=now()
			WHERE id=$1
		`, d.ApprovalID, string(newStatus), d.ActorID, d.Reason)
	} else {
		// Keep decided_at/decided_by null until the decision is complete, but
		// record the interim approval's reason in the history row.
		_, err = tx.Exec(ctx, `
			UPDATE core.approvals
			SET status=$2, version=version+1, updated_at=now()
			WHERE id=$1
		`, d.ApprovalID, string(newStatus))
	}
	if err != nil {
		return "", fmt.Errorf("approvals: update: %w", err)
	}

	hID, _ := uuid.NewV7()
	_, err = tx.Exec(ctx, `
		INSERT INTO core.approval_history (id, approval_id, actor_id, action, reason, from_status, to_status, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,now())
	`, hID, d.ApprovalID, d.ActorID, action, d.Reason, string(status), string(newStatus))
	if err != nil {
		return "", fmt.Errorf("approvals: history: %w", err)
	}

	if final && ev != nil {
		if _, err := outbox.Write(ctx, tx, *ev); err != nil {
			return "", err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("approvals: commit: %w", err)
	}
	return newStatus, nil
}

// ApprovalProgress reports an approval's status, the number of distinct
// approvals recorded so far and the first approver's id (zero UUID when none).
// The service reads it to choose the right event and to enforce the two-person
// rule before it starts the deciding transaction.
func (s *PGStore) ApprovalProgress(ctx context.Context, id uuid.UUID) (Status, int, uuid.UUID, error) {
	var (
		status    Status
		approvals int
		first     uuid.UUID
	)
	err := s.db.QueryRow(ctx, `
		SELECT a.status,
		       (SELECT count(*) FROM core.approval_decisions d
		        WHERE d.approval_id = a.id AND d.decision = 'approve'),
		       COALESCE((SELECT d.approver_id FROM core.approval_decisions d
		                 WHERE d.approval_id = a.id AND d.decision = 'approve'
		                 ORDER BY d.decided_at, d.approver_id LIMIT 1),
		                '00000000-0000-0000-0000-000000000000'::uuid)
		FROM core.approvals a
		WHERE a.id=$1 AND a.archived_at IS NULL
	`, id).Scan(&status, &approvals, &first)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", 0, uuid.Nil, ErrNotFound
	}
	if err != nil {
		return "", 0, uuid.Nil, fmt.Errorf("approvals: progress: %w", err)
	}
	return status, approvals, first, nil
}

// LookupPolicy returns the most specific active policy for a decision. "Most
// specific" means a currency-specific policy beats a currency-agnostic one, and
// a narrower amount band beats a wider one.
func (s *PGStore) LookupPolicy(ctx context.Context, q PolicyQuery) (*Policy, error) {
	var (
		p        Policy
		currency string
	)
	err := s.db.QueryRow(ctx, `
		SELECT id, decision_type, min_amount_minor, max_amount_minor,
		       COALESCE(currency, ''), approver_rule, effective_from, effective_to
		FROM core.approval_policies
		WHERE decision_type = $1
		  AND $2 >= COALESCE(min_amount_minor, 0)
		  AND $2 <= COALESCE(max_amount_minor, 9223372036854775807)
		  AND (currency IS NULL OR currency = NULLIF($3, ''))
		  AND effective_from <= $4
		  AND (effective_to IS NULL OR effective_to > $4)
		ORDER BY (currency IS NOT NULL) DESC, min_amount_minor DESC NULLS LAST, effective_from DESC
		LIMIT 1
	`, q.DecisionType, q.AmountMinor, q.Currency, q.At).
		Scan(&p.ID, &p.DecisionType, &p.MinAmountMinor, &p.MaxAmountMinor, &currency,
			&p.ApproverRule, &p.EffectiveFrom, &p.EffectiveTo)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("approvals: lookup policy: %w", err)
	}
	p.Currency = currency
	return &p, nil
}

// ListPolicies returns approval policies newest-first, using
// (effective_from, id) as the keyset — the same shape the lookup orders by.
func (s *PGStore) ListPolicies(ctx context.Context, limit int, before *Cursor) ([]Policy, error) {
	var (
		beforeTS *time.Time
		beforeID uuid.UUID
	)
	if before != nil {
		ts := before.At.UTC()
		beforeTS = &ts
		beforeID = before.ID
	}

	rows, err := s.db.Query(ctx, `
		SELECT id, decision_type, min_amount_minor, max_amount_minor,
		       COALESCE(currency, ''), approver_rule, effective_from, effective_to
		FROM core.approval_policies
		WHERE ($1::timestamptz IS NULL OR (effective_from, id) < ($1::timestamptz, $2::uuid))
		ORDER BY effective_from DESC, id DESC
		LIMIT $3
	`, beforeTS, beforeID, limit)
	if err != nil {
		return nil, fmt.Errorf("approvals: list policies: %w", err)
	}
	defer rows.Close()

	out := make([]Policy, 0, limit)
	for rows.Next() {
		var (
			p        Policy
			currency string
		)
		if err := rows.Scan(&p.ID, &p.DecisionType, &p.MinAmountMinor, &p.MaxAmountMinor,
			&currency, &p.ApproverRule, &p.EffectiveFrom, &p.EffectiveTo); err != nil {
			return nil, fmt.Errorf("approvals: scan policy: %w", err)
		}
		p.Currency = currency
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// PeopleWithRole resolves a role key to the active people who hold it. A grant
// is scoped (§6.2) but a notification recipient is a person, so scope is not
// applied here: someone notified about an approval decides whether it concerns
// them by opening it.
func (s *PGStore) PeopleWithRole(ctx context.Context, role string) ([]uuid.UUID, error) {
	rows, err := s.db.Query(ctx, `
		SELECT DISTINCT ua.person_id
		FROM core.role_grants g
		JOIN core.user_accounts ua ON ua.id = g.user_id
		JOIN people.persons p ON p.id = ua.person_id
		WHERE g.role_key = $1
		  AND g.revoked_at IS NULL
		  AND ua.status = 'active'
		  AND p.status = 'active'
	`, role)
	if err != nil {
		return nil, fmt.Errorf("approvals: people with role: %w", err)
	}
	defer rows.Close()

	out := make([]uuid.UUID, 0, 4)
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("approvals: scan person: %w", err)
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *PGStore) EscalateWithEvent(ctx context.Context, id uuid.UUID, actor uuid.UUID, toRole string, toPerson *uuid.UUID, reason string, ev outbox.Event) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("approvals: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx, `
		UPDATE core.approvals
		SET status='escalated', approver_role=$2, approver_id=$3, version=version+1, updated_at=now()
		WHERE id=$1 AND status IN ('requested','pending') AND archived_at IS NULL
	`, id, strings.ToLower(toRole), toPerson)
	if err != nil {
		return fmt.Errorf("approvals: escalate: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}

	hID, _ := uuid.NewV7()
	_, err = tx.Exec(ctx, `
		INSERT INTO core.approval_history (id, approval_id, actor_id, action, reason, from_status, to_status, created_at)
		VALUES ($1,$2,$3,'escalate',$4,'requested','escalated',now())
	`, hID, id, actor, reason)
	if err != nil {
		return fmt.Errorf("approvals: history: %w", err)
	}

	if _, err := outbox.Write(ctx, tx, ev); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("approvals: commit: %w", err)
	}
	return nil
}

func (s *PGStore) CancelWithEvent(ctx context.Context, id uuid.UUID, actor uuid.UUID, reason string, ev outbox.Event) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("approvals: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx, `
		UPDATE core.approvals
		SET status='cancelled', decided_at=now(), decided_by=$2, reason=$3, version=version+1, updated_at=now()
		WHERE id=$1 AND status IN ('requested','pending') AND archived_at IS NULL
	`, id, actor, reason)
	if err != nil {
		return fmt.Errorf("approvals: cancel: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}

	hID, _ := uuid.NewV7()
	_, err = tx.Exec(ctx, `
		INSERT INTO core.approval_history (id, approval_id, actor_id, action, reason, from_status, to_status, created_at)
		VALUES ($1,$2,$3,'cancel',$4,'requested','cancelled',now())
	`, hID, id, actor, reason)
	if err != nil {
		return fmt.Errorf("approvals: history: %w", err)
	}

	if _, err := outbox.Write(ctx, tx, ev); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("approvals: commit: %w", err)
	}
	return nil
}

func (s *PGStore) ExpireWithEvent(ctx context.Context, id uuid.UUID, ev outbox.Event) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("approvals: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx, `
		UPDATE core.approvals
		SET status='expired', version=version+1, updated_at=now()
		WHERE id=$1 AND status IN ('requested','pending') AND archived_at IS NULL AND (due_at IS NULL OR due_at < now())
	`, id)
	if err != nil {
		return fmt.Errorf("approvals: expire: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}

	hID, _ := uuid.NewV7()
	_, err = tx.Exec(ctx, `
		INSERT INTO core.approval_history (id, approval_id, actor_id, action, reason, from_status, to_status, created_at)
		VALUES ($1,$2,NULL,'expire','','requested','expired',now())
	`, hID, id)
	if err != nil {
		return fmt.Errorf("approvals: history: %w", err)
	}

	if _, err := outbox.Write(ctx, tx, ev); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("approvals: commit: %w", err)
	}
	return nil
}

func (s *PGStore) ClaimExpired(ctx context.Context, limit int) ([]Approval, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, ref FROM core.approvals
		WHERE status IN ('requested','pending') AND archived_at IS NULL AND due_at IS NOT NULL AND due_at < now()
		ORDER BY due_at
		LIMIT $1
		FOR UPDATE SKIP LOCKED
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("approvals: claim expired: %w", err)
	}
	defer rows.Close()

	out := make([]Approval, 0, limit)
	for rows.Next() {
		var a Approval
		if err := rows.Scan(&a.ID, &a.Ref); err != nil {
			return nil, fmt.Errorf("approvals: scan expired: %w", err)
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// ExpireDue claims and expires overdue approvals in a single transaction. The
// claim and the expiry share one transaction on purpose: FOR UPDATE SKIP LOCKED
// only protects rows for the life of the transaction, so a claim that commits
// before the expiry would let a second worker pick the same row. Here the locks
// are held until every claimed row has been expired, and the rows are locked in
// due_at order so two workers take disjoint, deterministic slices.
func (s *PGStore) ExpireDue(ctx context.Context, limit int, buildEvent func(Approval) (outbox.Event, error)) ([]Approval, error) {
	if limit <= 0 {
		limit = 100
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("approvals: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := tx.Query(ctx, `
		SELECT id, ref, status, entity_type, requested_by, approver_role, approver_id
		FROM core.approvals
		WHERE status IN ('requested','pending') AND archived_at IS NULL
		  AND due_at IS NOT NULL AND due_at < now()
		ORDER BY due_at
		LIMIT $1
		FOR UPDATE SKIP LOCKED
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("approvals: claim expired: %w", err)
	}
	claimed := make([]Approval, 0, limit)
	for rows.Next() {
		var a Approval
		if err := rows.Scan(&a.ID, &a.Ref, &a.Status, &a.EntityType, &a.RequestedBy, &a.ApproverRole, &a.ApproverID); err != nil {
			rows.Close()
			return nil, fmt.Errorf("approvals: scan expired: %w", err)
		}
		claimed = append(claimed, a)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, fmt.Errorf("approvals: claim expired: %w", err)
	}

	for i := range claimed {
		a := &claimed[i]
		ev, err := buildEvent(*a)
		if err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE core.approvals
			SET status='expired', version=version+1, updated_at=now()
			WHERE id=$1
		`, a.ID); err != nil {
			return nil, fmt.Errorf("approvals: expire: %w", err)
		}
		hID, _ := uuid.NewV7()
		if _, err := tx.Exec(ctx, `
			INSERT INTO core.approval_history (id, approval_id, actor_id, action, reason, from_status, to_status, created_at)
			VALUES ($1,$2,NULL,'expire','',$3,'expired',now())
		`, hID, a.ID, string(a.Status)); err != nil {
			return nil, fmt.Errorf("approvals: history: %w", err)
		}
		if _, err := outbox.Write(ctx, tx, ev); err != nil {
			return nil, err
		}
		a.Status = StatusExpired
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("approvals: commit: %w", err)
	}
	return claimed, nil
}

func timePtr(t *time.Time) any {
	if t == nil {
		return nil
	}
	return *t
}
