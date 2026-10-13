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
	// DecideWithEvent updates approval to decided state, writes history and event in tx.
	DecideWithEvent(ctx context.Context, id uuid.UUID, actor uuid.UUID, approve bool, reason string, ev outbox.Event) error
	EscalateWithEvent(ctx context.Context, id uuid.UUID, actor uuid.UUID, toRole string, toPerson *uuid.UUID, reason string, ev outbox.Event) error
	CancelWithEvent(ctx context.Context, id uuid.UUID, actor uuid.UUID, reason string, ev outbox.Event) error
	ExpireWithEvent(ctx context.Context, id uuid.UUID, ev outbox.Event) error
	ClaimExpired(ctx context.Context, limit int) ([]Approval, error)
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
	approver_role, approver_id, status, policy_ref, due_at, decided_at, decided_by,
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
			approver_role, approver_id, status, policy_ref, due_at, reason, version, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,now(),now())
	`, a.ID, a.Ref, a.EntityType, a.EntityID, a.RequestedBy, a.AmountMinor, a.Currency,
		a.ApproverRole, a.ApproverID, string(a.Status), a.PolicyRef, timePtr(a.DueAt), a.Reason, a.Version)
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
			&a.ApproverRole, &a.ApproverID, &a.Status, &a.PolicyRef, &a.DueAt, &a.DecidedAt, &a.DecidedBy,
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
			&a.ApproverRole, &a.ApproverID, &a.Status, &a.PolicyRef, &a.DueAt, &a.DecidedAt, &a.DecidedBy,
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
			&a.ApproverRole, &a.ApproverID, &a.Status, &a.PolicyRef, &a.DueAt, &a.DecidedAt, &a.DecidedBy,
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
			&a.ApproverRole, &a.ApproverID, &a.Status, &a.PolicyRef, &a.DueAt, &a.DecidedAt, &a.DecidedBy,
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

func (s *PGStore) DecideWithEvent(ctx context.Context, id uuid.UUID, actor uuid.UUID, approve bool, reason string, ev outbox.Event) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("approvals: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var status Status
	if approve {
		status = StatusApproved
	} else {
		status = StatusDeclined
	}

	tag, err := tx.Exec(ctx, `
		UPDATE core.approvals
		SET status=$2, decided_at=now(), decided_by=$3, reason=$4, version=version+1, updated_at=now()
		WHERE id=$1 AND status IN ('requested','pending') AND archived_at IS NULL
	`, id, string(status), actor, reason)
	if err != nil {
		return fmt.Errorf("approvals: decide: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}

	hID, _ := uuid.NewV7()
	_, err = tx.Exec(ctx, `
		INSERT INTO core.approval_history (id, approval_id, actor_id, action, reason, from_status, to_status, created_at)
		VALUES ($1,$2,$3,$4,$5,'requested',$6,now())
	`, hID, id, actor, mapAction(status), reason, string(status))
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

func mapAction(s Status) string {
	switch s {
	case StatusApproved:
		return "approve"
	case StatusDeclined:
		return "decline"
	default:
		return "decide"
	}
}

func timePtr(t *time.Time) any {
	if t == nil {
		return nil
	}
	return *t
}
