package contracts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/safaristack/atlas/apps/api/internal/outbox"
)

// Cursor is a keyset position (created_at, id), newest first (§6.6).
type Cursor struct {
	At time.Time
	ID uuid.UUID
}

// Store is the contracts persistence interface.
type Store interface {
	// OrganizationExists reports whether a client company exists (§8.1).
	OrganizationExists(ctx context.Context, id uuid.UUID) (bool, error)

	// Contracts. CreateContract allocates the human ref inside the transaction so
	// the created event carries it without a second write.
	CreateContract(ctx context.Context, c Contract, build func(ref string) (outbox.Event, error)) (Contract, error)
	GetContract(ctx context.Context, id uuid.UUID) (*Contract, error)
	ListContracts(ctx context.Context, status *ContractStatus, orgID *uuid.UUID, limit int, before *Cursor) ([]Contract, error)
	UpdateContract(ctx context.Context, c Contract, expectedVersion int) (Contract, error)
	SetContractStatus(ctx context.Context, id uuid.UUID, expectedVersion int, status ContractStatus, ev *outbox.Event) (Contract, error)
	SignContract(ctx context.Context, id uuid.UUID, expectedVersion int, signatories json.RawMessage, signedAt time.Time, ev *outbox.Event) (Contract, error)
	NextContractVersionNo(ctx context.Context, contractID uuid.UUID) (int, error)
	AddContractVersion(ctx context.Context, v ContractVersion, ev *outbox.Event) error

	// Milestones.
	CreateMilestone(ctx context.Context, m Milestone) (Milestone, error)
	GetMilestone(ctx context.Context, id uuid.UUID) (*Milestone, error)
	ListMilestones(ctx context.Context, contractID uuid.UUID) ([]Milestone, error)

	// Deliverables.
	CreateDeliverable(ctx context.Context, d Deliverable) (Deliverable, error)
	ListDeliverables(ctx context.Context, contractID uuid.UUID) ([]Deliverable, error)

	// Obligations.
	CreateObligation(ctx context.Context, o Obligation) (Obligation, error)
	ListObligations(ctx context.Context, contractID uuid.UUID) ([]Obligation, error)

	// SLA terms.
	CreateSLATerm(ctx context.Context, t SLATerm) (SLATerm, error)
	ListSLATerms(ctx context.Context, contractID uuid.UUID) ([]SLATerm, error)

	// Change requests. CreateChangeRequest allocates the CR-… ref in-transaction.
	CreateChangeRequest(ctx context.Context, cr ChangeRequest, build func(ref string) (outbox.Event, error)) (ChangeRequest, error)
	GetChangeRequest(ctx context.Context, id uuid.UUID) (*ChangeRequest, error)
	ListChangeRequests(ctx context.Context, contractID uuid.UUID) ([]ChangeRequest, error)
	SetChangeRequestStatus(ctx context.Context, id uuid.UUID, expectedVersion int, status ChangeRequestStatus, approvalID *uuid.UUID, ev *outbox.Event) (ChangeRequest, error)
}

// PGStore is the Postgres-backed store.
type PGStore struct{ db *pgxpool.Pool }

// NewPGStore builds a store over the pool.
func NewPGStore(db *pgxpool.Pool) *PGStore { return &PGStore{db: db} }

// rowScanner is satisfied by both pgx.Row and pgx.Rows.
type rowScanner interface {
	Scan(dest ...any) error
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func (s *PGStore) OrganizationExists(ctx context.Context, id uuid.UUID) (bool, error) {
	var exists bool
	err := s.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM crm.organizations WHERE id = $1)`, id).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("contracts: check organization: %w", err)
	}
	return exists, nil
}

// ── contracts ───────────────────────────────────────────────────────────────

const contractCols = `id, ref, organization_id, opportunity_id, contract_type, status,
	value_minor, currency, start_at, end_at, COALESCE(payment_terms,''), COALESCE(renewal_terms,''),
	governing_docs, signatories, signed_at, version, created_at, updated_at`

func scanContract(row rowScanner) (Contract, error) {
	var c Contract
	var docs, sigs []byte
	err := row.Scan(&c.ID, &c.Ref, &c.OrganizationID, &c.OpportunityID, &c.ContractType, &c.Status,
		&c.ValueMinor, &c.Currency, &c.StartAt, &c.EndAt, &c.PaymentTerms, &c.RenewalTerms,
		&docs, &sigs, &c.SignedAt, &c.Version, &c.CreatedAt, &c.UpdatedAt)
	c.GoverningDocs = docs
	c.Signatories = sigs
	return c, err
}

func (s *PGStore) CreateContract(ctx context.Context, c Contract, build func(ref string) (outbox.Event, error)) (Contract, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Contract{}, fmt.Errorf("contracts: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var ref string
	if err := tx.QueryRow(ctx, `SELECT core.next_reference('ctr', $1)`, time.Now().UTC().Year()).Scan(&ref); err != nil {
		return Contract{}, fmt.Errorf("contracts: allocate ref: %w", err)
	}
	docs := c.GoverningDocs
	if len(docs) == 0 {
		docs = json.RawMessage("[]")
	}
	sigs := c.Signatories
	if len(sigs) == 0 {
		sigs = json.RawMessage("[]")
	}
	created, err := scanContract(tx.QueryRow(ctx, `
		INSERT INTO contracts.contracts (
			id, ref, organization_id, opportunity_id, contract_type, status,
			value_minor, currency, start_at, end_at, payment_terms, renewal_terms,
			governing_docs, signatories, version, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)
		RETURNING `+contractCols,
		c.ID, ref, c.OrganizationID, c.OpportunityID, string(c.ContractType), string(c.Status),
		c.ValueMinor, c.Currency, c.StartAt, c.EndAt, nullIfEmpty(c.PaymentTerms), nullIfEmpty(c.RenewalTerms),
		docs, sigs, c.Version, c.CreatedAt, c.UpdatedAt))
	if err != nil {
		return Contract{}, fmt.Errorf("contracts: create contract: %w", err)
	}
	if build != nil {
		ev, err := build(created.Ref)
		if err != nil {
			return Contract{}, err
		}
		if _, err := outbox.Write(ctx, tx, ev); err != nil {
			return Contract{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Contract{}, fmt.Errorf("contracts: commit: %w", err)
	}
	return created, nil
}

func (s *PGStore) GetContract(ctx context.Context, id uuid.UUID) (*Contract, error) {
	c, err := scanContract(s.db.QueryRow(ctx, `SELECT `+contractCols+` FROM contracts.contracts WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("contracts: get contract: %w", err)
	}
	return &c, nil
}

func (s *PGStore) ListContracts(ctx context.Context, status *ContractStatus, orgID *uuid.UUID, limit int, before *Cursor) ([]Contract, error) {
	args := []any{}
	where := "TRUE"
	if status != nil {
		args = append(args, string(*status))
		where += fmt.Sprintf(" AND status = $%d", len(args))
	}
	if orgID != nil {
		args = append(args, *orgID)
		where += fmt.Sprintf(" AND organization_id = $%d", len(args))
	}
	if before != nil {
		args = append(args, before.At, before.ID)
		where += fmt.Sprintf(" AND (created_at, id) < ($%d, $%d)", len(args)-1, len(args))
	}
	args = append(args, limit)
	rows, err := s.db.Query(ctx, `SELECT `+contractCols+` FROM contracts.contracts
		WHERE `+where+` ORDER BY created_at DESC, id DESC LIMIT $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, fmt.Errorf("contracts: list contracts: %w", err)
	}
	defer rows.Close()
	out := []Contract{}
	for rows.Next() {
		c, err := scanContract(rows)
		if err != nil {
			return nil, fmt.Errorf("contracts: scan contract: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *PGStore) UpdateContract(ctx context.Context, c Contract, expectedVersion int) (Contract, error) {
	docs := c.GoverningDocs
	if len(docs) == 0 {
		docs = json.RawMessage("[]")
	}
	sigs := c.Signatories
	if len(sigs) == 0 {
		sigs = json.RawMessage("[]")
	}
	updated, err := scanContract(s.db.QueryRow(ctx, `
		UPDATE contracts.contracts SET
			organization_id = $2, opportunity_id = $3, contract_type = $4, value_minor = $5,
			currency = $6, start_at = $7, end_at = $8, payment_terms = $9, renewal_terms = $10,
			governing_docs = $11, signatories = $12, version = version + 1
		WHERE id = $1 AND version = $13
		RETURNING `+contractCols,
		c.ID, c.OrganizationID, c.OpportunityID, string(c.ContractType), c.ValueMinor,
		c.Currency, c.StartAt, c.EndAt, nullIfEmpty(c.PaymentTerms), nullIfEmpty(c.RenewalTerms),
		docs, sigs, expectedVersion))
	if errors.Is(err, pgx.ErrNoRows) {
		return Contract{}, s.versionConflict(ctx, "contracts.contracts", c.ID, expectedVersion)
	}
	if err != nil {
		return Contract{}, fmt.Errorf("contracts: update contract: %w", err)
	}
	return updated, nil
}

func (s *PGStore) SetContractStatus(ctx context.Context, id uuid.UUID, expectedVersion int, status ContractStatus, ev *outbox.Event) (Contract, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Contract{}, fmt.Errorf("contracts: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	updated, err := scanContract(tx.QueryRow(ctx, `
		UPDATE contracts.contracts SET status = $2, version = version + 1
		WHERE id = $1 AND version = $3
		RETURNING `+contractCols, id, string(status), expectedVersion))
	if errors.Is(err, pgx.ErrNoRows) {
		return Contract{}, s.versionConflict(ctx, "contracts.contracts", id, expectedVersion)
	}
	if err != nil {
		return Contract{}, fmt.Errorf("contracts: set contract status: %w", err)
	}
	if ev != nil {
		if _, err := outbox.Write(ctx, tx, *ev); err != nil {
			return Contract{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Contract{}, fmt.Errorf("contracts: commit: %w", err)
	}
	return updated, nil
}

func (s *PGStore) SignContract(ctx context.Context, id uuid.UUID, expectedVersion int, signatories json.RawMessage, signedAt time.Time, ev *outbox.Event) (Contract, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Contract{}, fmt.Errorf("contracts: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if len(signatories) == 0 {
		signatories = json.RawMessage("[]")
	}
	updated, err := scanContract(tx.QueryRow(ctx, `
		UPDATE contracts.contracts SET status = 'signed', signatories = $2, signed_at = $3, version = version + 1
		WHERE id = $1 AND version = $4
		RETURNING `+contractCols, id, signatories, signedAt, expectedVersion))
	if errors.Is(err, pgx.ErrNoRows) {
		return Contract{}, s.versionConflict(ctx, "contracts.contracts", id, expectedVersion)
	}
	if err != nil {
		return Contract{}, fmt.Errorf("contracts: sign contract: %w", err)
	}
	if ev != nil {
		if _, err := outbox.Write(ctx, tx, *ev); err != nil {
			return Contract{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Contract{}, fmt.Errorf("contracts: commit: %w", err)
	}
	return updated, nil
}

func (s *PGStore) NextContractVersionNo(ctx context.Context, contractID uuid.UUID) (int, error) {
	var n int
	err := s.db.QueryRow(ctx, `SELECT COALESCE(MAX(version_no), 0) + 1 FROM contracts.contract_versions WHERE contract_id = $1`, contractID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("contracts: next version no: %w", err)
	}
	return n, nil
}

func (s *PGStore) AddContractVersion(ctx context.Context, v ContractVersion, ev *outbox.Event) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("contracts: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	_, err = tx.Exec(ctx, `
		INSERT INTO contracts.contract_versions (id, contract_id, version_no, document_file_id, change_summary, created_by, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		v.ID, v.ContractID, v.VersionNo, v.DocumentFileID, nullIfEmpty(v.ChangeSummary), v.CreatedBy, v.CreatedAt)
	if err != nil {
		return fmt.Errorf("contracts: add contract version: %w", err)
	}
	// A new version bumps the contract's own version so clients re-read it.
	if _, err := tx.Exec(ctx, `UPDATE contracts.contracts SET version = version + 1 WHERE id = $1`, v.ContractID); err != nil {
		return fmt.Errorf("contracts: bump contract version: %w", err)
	}
	if ev != nil {
		if _, err := outbox.Write(ctx, tx, *ev); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("contracts: commit: %w", err)
	}
	return nil
}

// ── milestones ──────────────────────────────────────────────────────────────

const milestoneCols = `id, contract_id, title, due_at, amount_minor, currency, project_milestone_id, version, created_at, updated_at`

func scanMilestone(row rowScanner) (Milestone, error) {
	var m Milestone
	err := row.Scan(&m.ID, &m.ContractID, &m.Title, &m.DueAt, &m.AmountMinor, &m.Currency,
		&m.ProjectMilestoneID, &m.Version, &m.CreatedAt, &m.UpdatedAt)
	return m, err
}

func (s *PGStore) CreateMilestone(ctx context.Context, m Milestone) (Milestone, error) {
	created, err := scanMilestone(s.db.QueryRow(ctx, `
		INSERT INTO contracts.milestones (
			id, contract_id, title, due_at, amount_minor, currency, project_milestone_id, version, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		RETURNING `+milestoneCols,
		m.ID, m.ContractID, m.Title, m.DueAt, m.AmountMinor, m.Currency, m.ProjectMilestoneID,
		m.Version, m.CreatedAt, m.UpdatedAt))
	if err != nil {
		return Milestone{}, fmt.Errorf("contracts: create milestone: %w", err)
	}
	return created, nil
}

func (s *PGStore) GetMilestone(ctx context.Context, id uuid.UUID) (*Milestone, error) {
	m, err := scanMilestone(s.db.QueryRow(ctx, `SELECT `+milestoneCols+` FROM contracts.milestones WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("contracts: get milestone: %w", err)
	}
	return &m, nil
}

func (s *PGStore) ListMilestones(ctx context.Context, contractID uuid.UUID) ([]Milestone, error) {
	rows, err := s.db.Query(ctx, `SELECT `+milestoneCols+` FROM contracts.milestones
		WHERE contract_id = $1 ORDER BY due_at NULLS LAST, created_at, id`, contractID)
	if err != nil {
		return nil, fmt.Errorf("contracts: list milestones: %w", err)
	}
	defer rows.Close()
	out := []Milestone{}
	for rows.Next() {
		m, err := scanMilestone(rows)
		if err != nil {
			return nil, fmt.Errorf("contracts: scan milestone: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ── deliverables ────────────────────────────────────────────────────────────

const deliverableCols = `id, contract_id, description, COALESCE(acceptance_criteria,''), milestone_id, version, created_at, updated_at`

func scanDeliverable(row rowScanner) (Deliverable, error) {
	var d Deliverable
	err := row.Scan(&d.ID, &d.ContractID, &d.Description, &d.AcceptanceCriteria, &d.MilestoneID,
		&d.Version, &d.CreatedAt, &d.UpdatedAt)
	return d, err
}

func (s *PGStore) CreateDeliverable(ctx context.Context, d Deliverable) (Deliverable, error) {
	created, err := scanDeliverable(s.db.QueryRow(ctx, `
		INSERT INTO contracts.deliverables (id, contract_id, description, acceptance_criteria, milestone_id, version, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		RETURNING `+deliverableCols,
		d.ID, d.ContractID, d.Description, nullIfEmpty(d.AcceptanceCriteria), d.MilestoneID,
		d.Version, d.CreatedAt, d.UpdatedAt))
	if err != nil {
		return Deliverable{}, fmt.Errorf("contracts: create deliverable: %w", err)
	}
	return created, nil
}

func (s *PGStore) ListDeliverables(ctx context.Context, contractID uuid.UUID) ([]Deliverable, error) {
	rows, err := s.db.Query(ctx, `SELECT `+deliverableCols+` FROM contracts.deliverables
		WHERE contract_id = $1 ORDER BY created_at, id`, contractID)
	if err != nil {
		return nil, fmt.Errorf("contracts: list deliverables: %w", err)
	}
	defer rows.Close()
	out := []Deliverable{}
	for rows.Next() {
		d, err := scanDeliverable(rows)
		if err != nil {
			return nil, fmt.Errorf("contracts: scan deliverable: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// ── obligations ─────────────────────────────────────────────────────────────

const obligationCols = `id, contract_id, party, description, due_at, COALESCE(recurrence,''), status, owner_person_id, version, created_at, updated_at`

func scanObligation(row rowScanner) (Obligation, error) {
	var o Obligation
	err := row.Scan(&o.ID, &o.ContractID, &o.Party, &o.Description, &o.DueAt, &o.Recurrence,
		&o.Status, &o.OwnerPersonID, &o.Version, &o.CreatedAt, &o.UpdatedAt)
	return o, err
}

func (s *PGStore) CreateObligation(ctx context.Context, o Obligation) (Obligation, error) {
	created, err := scanObligation(s.db.QueryRow(ctx, `
		INSERT INTO contracts.obligations (id, contract_id, party, description, due_at, recurrence, status, owner_person_id, version, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		RETURNING `+obligationCols,
		o.ID, o.ContractID, string(o.Party), o.Description, o.DueAt, nullIfEmpty(o.Recurrence),
		string(o.Status), o.OwnerPersonID, o.Version, o.CreatedAt, o.UpdatedAt))
	if err != nil {
		return Obligation{}, fmt.Errorf("contracts: create obligation: %w", err)
	}
	return created, nil
}

func (s *PGStore) ListObligations(ctx context.Context, contractID uuid.UUID) ([]Obligation, error) {
	rows, err := s.db.Query(ctx, `SELECT `+obligationCols+` FROM contracts.obligations
		WHERE contract_id = $1 ORDER BY due_at NULLS LAST, created_at, id`, contractID)
	if err != nil {
		return nil, fmt.Errorf("contracts: list obligations: %w", err)
	}
	defer rows.Close()
	out := []Obligation{}
	for rows.Next() {
		o, err := scanObligation(rows)
		if err != nil {
			return nil, fmt.Errorf("contracts: scan obligation: %w", err)
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// ── SLA terms ───────────────────────────────────────────────────────────────

const slaTermCols = `id, contract_id, metric, target, COALESCE(measurement_window,''), system_ids, version, created_at, updated_at`

func scanSLATerm(row rowScanner) (SLATerm, error) {
	var t SLATerm
	err := row.Scan(&t.ID, &t.ContractID, &t.Metric, &t.Target, &t.MeasurementWindow,
		&t.SystemIDs, &t.Version, &t.CreatedAt, &t.UpdatedAt)
	return t, err
}

func (s *PGStore) CreateSLATerm(ctx context.Context, t SLATerm) (SLATerm, error) {
	ids := t.SystemIDs
	if ids == nil {
		ids = []uuid.UUID{}
	}
	created, err := scanSLATerm(s.db.QueryRow(ctx, `
		INSERT INTO contracts.sla_terms (id, contract_id, metric, target, measurement_window, system_ids, version, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		RETURNING `+slaTermCols,
		t.ID, t.ContractID, t.Metric, t.Target, nullIfEmpty(t.MeasurementWindow), ids,
		t.Version, t.CreatedAt, t.UpdatedAt))
	if err != nil {
		return SLATerm{}, fmt.Errorf("contracts: create SLA term: %w", err)
	}
	return created, nil
}

func (s *PGStore) ListSLATerms(ctx context.Context, contractID uuid.UUID) ([]SLATerm, error) {
	rows, err := s.db.Query(ctx, `SELECT `+slaTermCols+` FROM contracts.sla_terms
		WHERE contract_id = $1 ORDER BY created_at, id`, contractID)
	if err != nil {
		return nil, fmt.Errorf("contracts: list SLA terms: %w", err)
	}
	defer rows.Close()
	out := []SLATerm{}
	for rows.Next() {
		t, err := scanSLATerm(rows)
		if err != nil {
			return nil, fmt.Errorf("contracts: scan SLA term: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ── change requests ─────────────────────────────────────────────────────────

const changeRequestCols = `id, ref, contract_id, COALESCE(requested_by,''), description, COALESCE(effort_impact,''),
	price_impact_minor, currency, deadline_impact_days, status, approval_id, version, created_at, updated_at`

func scanChangeRequest(row rowScanner) (ChangeRequest, error) {
	var c ChangeRequest
	err := row.Scan(&c.ID, &c.Ref, &c.ContractID, &c.RequestedBy, &c.Description, &c.EffortImpact,
		&c.PriceImpactMinor, &c.Currency, &c.DeadlineImpactDays, &c.Status, &c.ApprovalID,
		&c.Version, &c.CreatedAt, &c.UpdatedAt)
	return c, err
}

func (s *PGStore) CreateChangeRequest(ctx context.Context, cr ChangeRequest, build func(ref string) (outbox.Event, error)) (ChangeRequest, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return ChangeRequest{}, fmt.Errorf("contracts: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var ref string
	if err := tx.QueryRow(ctx, `SELECT core.next_reference('cr', $1)`, time.Now().UTC().Year()).Scan(&ref); err != nil {
		return ChangeRequest{}, fmt.Errorf("contracts: allocate ref: %w", err)
	}
	created, err := scanChangeRequest(tx.QueryRow(ctx, `
		INSERT INTO contracts.change_requests (
			id, ref, contract_id, requested_by, description, effort_impact, price_impact_minor,
			currency, deadline_impact_days, status, approval_id, version, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		RETURNING `+changeRequestCols,
		cr.ID, ref, cr.ContractID, nullIfEmpty(cr.RequestedBy), cr.Description, nullIfEmpty(cr.EffortImpact),
		cr.PriceImpactMinor, cr.Currency, cr.DeadlineImpactDays, string(cr.Status), cr.ApprovalID,
		cr.Version, cr.CreatedAt, cr.UpdatedAt))
	if err != nil {
		return ChangeRequest{}, fmt.Errorf("contracts: create change request: %w", err)
	}
	if build != nil {
		ev, err := build(created.Ref)
		if err != nil {
			return ChangeRequest{}, err
		}
		if _, err := outbox.Write(ctx, tx, ev); err != nil {
			return ChangeRequest{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return ChangeRequest{}, fmt.Errorf("contracts: commit: %w", err)
	}
	return created, nil
}

func (s *PGStore) GetChangeRequest(ctx context.Context, id uuid.UUID) (*ChangeRequest, error) {
	c, err := scanChangeRequest(s.db.QueryRow(ctx, `SELECT `+changeRequestCols+` FROM contracts.change_requests WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("contracts: get change request: %w", err)
	}
	return &c, nil
}

func (s *PGStore) ListChangeRequests(ctx context.Context, contractID uuid.UUID) ([]ChangeRequest, error) {
	rows, err := s.db.Query(ctx, `SELECT `+changeRequestCols+` FROM contracts.change_requests
		WHERE contract_id = $1 ORDER BY created_at DESC, id DESC`, contractID)
	if err != nil {
		return nil, fmt.Errorf("contracts: list change requests: %w", err)
	}
	defer rows.Close()
	out := []ChangeRequest{}
	for rows.Next() {
		c, err := scanChangeRequest(rows)
		if err != nil {
			return nil, fmt.Errorf("contracts: scan change request: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *PGStore) SetChangeRequestStatus(ctx context.Context, id uuid.UUID, expectedVersion int, status ChangeRequestStatus, approvalID *uuid.UUID, ev *outbox.Event) (ChangeRequest, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return ChangeRequest{}, fmt.Errorf("contracts: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	updated, err := scanChangeRequest(tx.QueryRow(ctx, `
		UPDATE contracts.change_requests SET status = $2, approval_id = $3, version = version + 1
		WHERE id = $1 AND version = $4
		RETURNING `+changeRequestCols, id, string(status), approvalID, expectedVersion))
	if errors.Is(err, pgx.ErrNoRows) {
		return ChangeRequest{}, s.versionConflict(ctx, "contracts.change_requests", id, expectedVersion)
	}
	if err != nil {
		return ChangeRequest{}, fmt.Errorf("contracts: set change request status: %w", err)
	}
	if ev != nil {
		if _, err := outbox.Write(ctx, tx, *ev); err != nil {
			return ChangeRequest{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return ChangeRequest{}, fmt.Errorf("contracts: commit: %w", err)
	}
	return updated, nil
}

// ── shared helpers ──────────────────────────────────────────────────────────

func (s *PGStore) versionConflict(ctx context.Context, table string, id uuid.UUID, expected int) error {
	var cur int
	err := s.db.QueryRow(ctx, `SELECT version FROM `+table+` WHERE id = $1`, id).Scan(&cur)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	if err != nil {
		return fmt.Errorf("contracts: read version: %w", err)
	}
	return fmt.Errorf("%w: version %d is stale (current %d)", ErrConflict, expected, cur)
}
