package crm

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/safaristack/atlas/apps/api/internal/identity"
	"github.com/safaristack/atlas/apps/api/internal/outbox"
)

// Cursor is a keyset position (created_at, id), newest first (§6.6).
type Cursor struct {
	At time.Time
	ID uuid.UUID
}

// ListLeadsQuery filters a lead listing.
type ListLeadsQuery struct {
	Status         *LeadStatus
	OwnerID        *uuid.UUID
	SourceKey      string
	OrganizationID *uuid.UUID
}

// ListFollowupsQuery filters a follow-up listing.
type ListFollowupsQuery struct {
	Status    *FollowupStatus
	OwnerID   *uuid.UUID
	LeadID    *uuid.UUID
	DueBefore *time.Time
}

// ListInteractionsQuery filters an interaction listing.
type ListInteractionsQuery struct {
	LeadID        *uuid.UUID
	OpportunityID *uuid.UUID
	ContactID     *uuid.UUID
}

// PipelineStageTotal is one row of the pipeline summary.
type PipelineStageTotal struct {
	StageKey   string
	Label      string
	SortOrder  int
	Count      int
	ValueMinor int64
}

// Store is the CRM persistence interface.
type Store interface {
	// AccountForPerson resolves a person to the user account a realtime nudge
	// is addressed to, or nil when the person has no account (§7.5).
	AccountForPerson(ctx context.Context, personID uuid.UUID) (*uuid.UUID, error)

	// Organizations.
	CreateOrganization(ctx context.Context, o Organization) (Organization, error)
	GetOrganization(ctx context.Context, id uuid.UUID) (*Organization, error)
	UpdateOrganization(ctx context.Context, o Organization, expectedVersion int) (Organization, error)
	ListOrganizations(ctx context.Context, q string, limit int, before *Cursor) ([]Organization, error)
	OrganizationByName(ctx context.Context, nameNormalized string) (*Organization, error)

	// Contacts.
	CreateContact(ctx context.Context, c Contact) (Contact, error)
	GetContact(ctx context.Context, id uuid.UUID) (*Contact, error)
	UpdateContact(ctx context.Context, c Contact, expectedVersion int) (Contact, error)
	ListContacts(ctx context.Context, organizationID *uuid.UUID, limit int, before *Cursor) ([]Contact, error)

	// Config.
	ListLeadSources(ctx context.Context) ([]LeadSource, error)
	ListPipelineStages(ctx context.Context) ([]PipelineStage, error)
	GetPipelineStage(ctx context.Context, key string) (*PipelineStage, error)
	LeadSourceExists(ctx context.Context, key string) (bool, error)

	// Leads. CreateLead allocates the human ref inside the transaction and calls
	// build with it, so the created event carries the ref without a second write.
	CreateLead(ctx context.Context, l Lead, build func(ref string) (outbox.Event, error)) (Lead, error)
	GetLead(ctx context.Context, id uuid.UUID) (*Lead, error)
	ListLeads(ctx context.Context, q ListLeadsQuery, limit int, before *Cursor) ([]Lead, error)
	UpdateLead(ctx context.Context, l Lead, expectedVersion int, ev *outbox.Event) (Lead, error)
	SetLeadOwner(ctx context.Context, id, owner uuid.UUID, nextAction string, dueAt time.Time, ev *outbox.Event) (Lead, error)
	SetLeadStatus(ctx context.Context, id uuid.UUID, status LeadStatus, lostReason *LostReason, ev *outbox.Event) (Lead, error)
	ConvertLead(ctx context.Context, leadID uuid.UUID, opp Opportunity, buildOpp func(ref string) (outbox.Event, error), leadEv *outbox.Event) (Opportunity, Lead, error)
	LeadOwnerAndRef(ctx context.Context, id uuid.UUID) (uuid.UUID, string, error)

	// Opportunities.
	CreateOpportunity(ctx context.Context, o Opportunity, build func(ref string) (outbox.Event, error)) (Opportunity, error)
	GetOpportunity(ctx context.Context, id uuid.UUID) (*Opportunity, error)
	ListOpportunities(ctx context.Context, stageKey *string, ownerID *uuid.UUID, limit int, before *Cursor) ([]Opportunity, error)
	SetOpportunityStage(ctx context.Context, id uuid.UUID, stageKey string, lostReason *LostReason, ev *outbox.Event) (Opportunity, error)

	// Interactions.
	CreateInteraction(ctx context.Context, i Interaction, ev *outbox.Event) (Interaction, error)
	GetInteraction(ctx context.Context, id uuid.UUID) (*Interaction, error)
	ListInteractions(ctx context.Context, q ListInteractionsQuery, limit int, before *Cursor) ([]Interaction, error)

	// Follow-ups.
	CreateFollowup(ctx context.Context, f Followup, ev *outbox.Event) (Followup, error)
	GetFollowup(ctx context.Context, id uuid.UUID) (*Followup, error)
	ListFollowups(ctx context.Context, q ListFollowupsQuery, limit int, before *Cursor) ([]Followup, error)
	ResolveFollowup(ctx context.Context, id uuid.UUID, status FollowupStatus, outcome string, ev *outbox.Event) (Followup, error)
	MissOverdueFollowups(ctx context.Context, now time.Time, limit int, build func(f Followup) (outbox.Event, error)) ([]Followup, error)

	// Proposals.
	CreateProposal(ctx context.Context, p Proposal, build func(ref string) (outbox.Event, error)) (Proposal, error)
	GetProposal(ctx context.Context, id uuid.UUID) (*Proposal, error)
	ListProposals(ctx context.Context, opportunityID uuid.UUID) ([]Proposal, error)
	SetProposalStatus(ctx context.Context, id uuid.UUID, status ProposalStatus, ev *outbox.Event) (Proposal, error)

	// Pipeline summary (commercial dashboard).
	PipelineSummary(ctx context.Context) ([]PipelineStageTotal, error)
}

// PGStore is the Postgres-backed store.
type PGStore struct{ db *pgxpool.Pool }

// NewPGStore builds a store over the pool.
func NewPGStore(db *pgxpool.Pool) *PGStore { return &PGStore{db: db} }

// AccountForPerson resolves the person's account via the shared identity bridge.
func (s *PGStore) AccountForPerson(ctx context.Context, personID uuid.UUID) (*uuid.UUID, error) {
	return identity.AccountForPerson(ctx, s.db, personID)
}

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

// ── organizations ───────────────────────────────────────────────────────────

const organizationCols = `id, name, name_normalized, COALESCE(industry,''), COALESCE(size,''),
	COALESCE(location,''), COALESCE(kra_pin,''), status, owner_person_id, version, created_at, updated_at`

func scanOrganization(row rowScanner) (Organization, error) {
	var o Organization
	err := row.Scan(&o.ID, &o.Name, &o.NameNormalized, &o.Industry, &o.Size, &o.Location,
		&o.KRAPin, &o.Status, &o.OwnerPersonID, &o.Version, &o.CreatedAt, &o.UpdatedAt)
	return o, err
}

func (s *PGStore) CreateOrganization(ctx context.Context, o Organization) (Organization, error) {
	created, err := scanOrganization(s.db.QueryRow(ctx, `
		INSERT INTO crm.organizations (
			id, name, name_normalized, industry, size, location, kra_pin, status,
			owner_person_id, version, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		RETURNING `+organizationCols,
		o.ID, o.Name, o.NameNormalized, nullIfEmpty(o.Industry), nullIfEmpty(o.Size),
		nullIfEmpty(o.Location), nullIfEmpty(o.KRAPin), string(o.Status), o.OwnerPersonID,
		o.Version, o.CreatedAt, o.UpdatedAt))
	if err != nil {
		return Organization{}, fmt.Errorf("crm: create organization: %w", err)
	}
	return created, nil
}

func (s *PGStore) GetOrganization(ctx context.Context, id uuid.UUID) (*Organization, error) {
	o, err := scanOrganization(s.db.QueryRow(ctx, `SELECT `+organizationCols+` FROM crm.organizations WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("crm: get organization: %w", err)
	}
	return &o, nil
}

func (s *PGStore) OrganizationByName(ctx context.Context, nameNormalized string) (*Organization, error) {
	o, err := scanOrganization(s.db.QueryRow(ctx, `SELECT `+organizationCols+` FROM crm.organizations WHERE name_normalized = $1`, nameNormalized))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("crm: find organization: %w", err)
	}
	return &o, nil
}

func (s *PGStore) UpdateOrganization(ctx context.Context, o Organization, expectedVersion int) (Organization, error) {
	updated, err := scanOrganization(s.db.QueryRow(ctx, `
		UPDATE crm.organizations SET
			name = $2, name_normalized = $3, industry = $4, size = $5, location = $6,
			kra_pin = $7, status = $8, owner_person_id = $9, version = version + 1
		WHERE id = $1 AND version = $10
		RETURNING `+organizationCols,
		o.ID, o.Name, o.NameNormalized, nullIfEmpty(o.Industry), nullIfEmpty(o.Size),
		nullIfEmpty(o.Location), nullIfEmpty(o.KRAPin), string(o.Status), o.OwnerPersonID, expectedVersion))
	if errors.Is(err, pgx.ErrNoRows) {
		return Organization{}, s.versionConflict(ctx, "crm.organizations", o.ID, expectedVersion)
	}
	if err != nil {
		return Organization{}, fmt.Errorf("crm: update organization: %w", err)
	}
	return updated, nil
}

func (s *PGStore) ListOrganizations(ctx context.Context, q string, limit int, before *Cursor) ([]Organization, error) {
	args := []any{}
	where := "TRUE"
	if q != "" {
		args = append(args, "%"+q+"%")
		where = fmt.Sprintf("name ILIKE $%d", len(args))
	}
	if before != nil {
		args = append(args, before.At, before.ID)
		where += fmt.Sprintf(" AND (created_at, id) < ($%d, $%d)", len(args)-1, len(args))
	}
	args = append(args, limit)
	rows, err := s.db.Query(ctx, `SELECT `+organizationCols+` FROM crm.organizations
		WHERE `+where+` ORDER BY created_at DESC, id DESC LIMIT $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, fmt.Errorf("crm: list organizations: %w", err)
	}
	defer rows.Close()
	out := []Organization{}
	for rows.Next() {
		o, err := scanOrganization(rows)
		if err != nil {
			return nil, fmt.Errorf("crm: scan organization: %w", err)
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// ── contacts ────────────────────────────────────────────────────────────────

const contactCols = `id, organization_id, name, COALESCE(role,''), phones, emails,
	COALESCE(whatsapp_number,''),
	consent_marketing, consent_marketing_at, consent_marketing_source,
	consent_whatsapp, consent_whatsapp_at, consent_whatsapp_source,
	consent_call_recording, consent_call_recording_at, consent_call_recording_source,
	version, created_at, updated_at`

func scanContact(row rowScanner) (Contact, error) {
	var c Contact
	err := row.Scan(&c.ID, &c.OrganizationID, &c.Name, &c.Role, &c.Phones, &c.Emails, &c.WhatsAppNumber,
		&c.ConsentMarketing, &c.ConsentMarketingAt, &c.ConsentMarketingSource,
		&c.ConsentWhatsApp, &c.ConsentWhatsAppAt, &c.ConsentWhatsAppSource,
		&c.ConsentRecording, &c.ConsentRecordingAt, &c.ConsentRecordingSource,
		&c.Version, &c.CreatedAt, &c.UpdatedAt)
	return c, err
}

func (s *PGStore) CreateContact(ctx context.Context, c Contact) (Contact, error) {
	created, err := scanContact(s.db.QueryRow(ctx, `
		INSERT INTO crm.contacts (
			id, organization_id, name, role, phones, emails, whatsapp_number,
			consent_marketing, consent_marketing_at, consent_marketing_source,
			consent_whatsapp, consent_whatsapp_at, consent_whatsapp_source,
			consent_call_recording, consent_call_recording_at, consent_call_recording_source,
			version, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)
		RETURNING `+contactCols,
		c.ID, c.OrganizationID, c.Name, nullIfEmpty(c.Role), c.Phones, c.Emails, nullIfEmpty(c.WhatsAppNumber),
		c.ConsentMarketing, c.ConsentMarketingAt, nullIfEmpty(c.ConsentMarketingSource),
		c.ConsentWhatsApp, c.ConsentWhatsAppAt, nullIfEmpty(c.ConsentWhatsAppSource),
		c.ConsentRecording, c.ConsentRecordingAt, nullIfEmpty(c.ConsentRecordingSource),
		c.Version, c.CreatedAt, c.UpdatedAt))
	if err != nil {
		return Contact{}, fmt.Errorf("crm: create contact: %w", err)
	}
	return created, nil
}

func (s *PGStore) GetContact(ctx context.Context, id uuid.UUID) (*Contact, error) {
	c, err := scanContact(s.db.QueryRow(ctx, `SELECT `+contactCols+` FROM crm.contacts WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("crm: get contact: %w", err)
	}
	return &c, nil
}

func (s *PGStore) UpdateContact(ctx context.Context, c Contact, expectedVersion int) (Contact, error) {
	updated, err := scanContact(s.db.QueryRow(ctx, `
		UPDATE crm.contacts SET
			organization_id = $2, name = $3, role = $4, phones = $5, emails = $6, whatsapp_number = $7,
			consent_marketing = $8, consent_marketing_at = $9, consent_marketing_source = $10,
			consent_whatsapp = $11, consent_whatsapp_at = $12, consent_whatsapp_source = $13,
			consent_call_recording = $14, consent_call_recording_at = $15, consent_call_recording_source = $16,
			version = version + 1
		WHERE id = $1 AND version = $17
		RETURNING `+contactCols,
		c.ID, c.OrganizationID, c.Name, nullIfEmpty(c.Role), c.Phones, c.Emails, nullIfEmpty(c.WhatsAppNumber),
		c.ConsentMarketing, c.ConsentMarketingAt, nullIfEmpty(c.ConsentMarketingSource),
		c.ConsentWhatsApp, c.ConsentWhatsAppAt, nullIfEmpty(c.ConsentWhatsAppSource),
		c.ConsentRecording, c.ConsentRecordingAt, nullIfEmpty(c.ConsentRecordingSource),
		expectedVersion))
	if errors.Is(err, pgx.ErrNoRows) {
		return Contact{}, s.versionConflict(ctx, "crm.contacts", c.ID, expectedVersion)
	}
	if err != nil {
		return Contact{}, fmt.Errorf("crm: update contact: %w", err)
	}
	return updated, nil
}

func (s *PGStore) ListContacts(ctx context.Context, organizationID *uuid.UUID, limit int, before *Cursor) ([]Contact, error) {
	args := []any{}
	where := "TRUE"
	if organizationID != nil {
		args = append(args, *organizationID)
		where = fmt.Sprintf("organization_id = $%d", len(args))
	}
	if before != nil {
		args = append(args, before.At, before.ID)
		where += fmt.Sprintf(" AND (created_at, id) < ($%d, $%d)", len(args)-1, len(args))
	}
	args = append(args, limit)
	rows, err := s.db.Query(ctx, `SELECT `+contactCols+` FROM crm.contacts WHERE `+where+
		` ORDER BY created_at DESC, id DESC LIMIT $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, fmt.Errorf("crm: list contacts: %w", err)
	}
	defer rows.Close()
	out := []Contact{}
	for rows.Next() {
		c, err := scanContact(rows)
		if err != nil {
			return nil, fmt.Errorf("crm: scan contact: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ── config ──────────────────────────────────────────────────────────────────

func (s *PGStore) ListLeadSources(ctx context.Context) ([]LeadSource, error) {
	rows, err := s.db.Query(ctx, `SELECT key, label, sort_order, active FROM crm.lead_sources WHERE active ORDER BY sort_order, key`)
	if err != nil {
		return nil, fmt.Errorf("crm: list lead sources: %w", err)
	}
	defer rows.Close()
	out := []LeadSource{}
	for rows.Next() {
		var l LeadSource
		if err := rows.Scan(&l.Key, &l.Label, &l.SortOrder, &l.Active); err != nil {
			return nil, fmt.Errorf("crm: scan lead source: %w", err)
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (s *PGStore) LeadSourceExists(ctx context.Context, key string) (bool, error) {
	var ok bool
	if err := s.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM crm.lead_sources WHERE key = $1 AND active)`, key).Scan(&ok); err != nil {
		return false, fmt.Errorf("crm: lead source exists: %w", err)
	}
	return ok, nil
}

func (s *PGStore) ListPipelineStages(ctx context.Context) ([]PipelineStage, error) {
	rows, err := s.db.Query(ctx, `SELECT key, label, sort_order, is_won, is_lost, active FROM crm.pipeline_stages WHERE active ORDER BY sort_order, key`)
	if err != nil {
		return nil, fmt.Errorf("crm: list pipeline stages: %w", err)
	}
	defer rows.Close()
	out := []PipelineStage{}
	for rows.Next() {
		var p PipelineStage
		if err := rows.Scan(&p.Key, &p.Label, &p.SortOrder, &p.IsWon, &p.IsLost, &p.Active); err != nil {
			return nil, fmt.Errorf("crm: scan pipeline stage: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *PGStore) GetPipelineStage(ctx context.Context, key string) (*PipelineStage, error) {
	var p PipelineStage
	err := s.db.QueryRow(ctx, `SELECT key, label, sort_order, is_won, is_lost, active FROM crm.pipeline_stages WHERE key = $1`, key).
		Scan(&p.Key, &p.Label, &p.SortOrder, &p.IsWon, &p.IsLost, &p.Active)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("crm: get pipeline stage: %w", err)
	}
	return &p, nil
}

// ── leads ───────────────────────────────────────────────────────────────────

const leadCols = `id, ref, source_key, COALESCE(source_detail,''), organization_id, contact_id,
	COALESCE(contact_name,''), COALESCE(contact_phone,''), COALESCE(contact_email,''),
	COALESCE(need_summary,''), estimated_value_minor, currency, priority, owner_person_id,
	status, COALESCE(next_action,''), next_action_due_at, COALESCE(qualification_notes,''),
	lost_reason, converted_opportunity_id, version, created_at, updated_at`

func scanLead(row rowScanner) (Lead, error) {
	var l Lead
	err := row.Scan(&l.ID, &l.Ref, &l.SourceKey, &l.SourceDetail, &l.OrganizationID, &l.ContactID,
		&l.ContactName, &l.ContactPhone, &l.ContactEmail, &l.NeedSummary, &l.EstimatedValueMinor,
		&l.Currency, &l.Priority, &l.OwnerPersonID, &l.Status, &l.NextAction, &l.NextActionDueAt,
		&l.QualificationNotes, &l.LostReason, &l.ConvertedOpportunityID, &l.Version, &l.CreatedAt, &l.UpdatedAt)
	return l, err
}

func (s *PGStore) CreateLead(ctx context.Context, l Lead, build func(ref string) (outbox.Event, error)) (Lead, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Lead{}, fmt.Errorf("crm: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	ref, err := allocateRefTx(ctx, tx, "lead", l.CreatedAt)
	if err != nil {
		return Lead{}, err
	}
	l.Ref = ref
	created, err := scanLead(tx.QueryRow(ctx, `
		INSERT INTO crm.leads (
			id, ref, source_key, source_detail, organization_id, contact_id, contact_name,
			contact_phone, contact_email, need_summary, estimated_value_minor, currency,
			priority, owner_person_id, status, next_action, next_action_due_at,
			qualification_notes, lost_reason, converted_opportunity_id, version, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23)
		RETURNING `+leadCols,
		l.ID, l.Ref, l.SourceKey, nullIfEmpty(l.SourceDetail), l.OrganizationID, l.ContactID,
		nullIfEmpty(l.ContactName), nullIfEmpty(l.ContactPhone), nullIfEmpty(l.ContactEmail),
		nullIfEmpty(l.NeedSummary), l.EstimatedValueMinor, l.Currency, string(l.Priority),
		l.OwnerPersonID, string(l.Status), nullIfEmpty(l.NextAction), l.NextActionDueAt,
		nullIfEmpty(l.QualificationNotes), l.LostReason, l.ConvertedOpportunityID,
		l.Version, l.CreatedAt, l.UpdatedAt))
	if err != nil {
		return Lead{}, fmt.Errorf("crm: create lead: %w", err)
	}
	if build != nil {
		ev, err := build(created.Ref)
		if err != nil {
			return Lead{}, err
		}
		if _, err := outbox.Write(ctx, tx, ev); err != nil {
			return Lead{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Lead{}, fmt.Errorf("crm: commit: %w", err)
	}
	return created, nil
}

func (s *PGStore) GetLead(ctx context.Context, id uuid.UUID) (*Lead, error) {
	l, err := scanLead(s.db.QueryRow(ctx, `SELECT `+leadCols+` FROM crm.leads WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("crm: get lead: %w", err)
	}
	return &l, nil
}

func (s *PGStore) LeadOwnerAndRef(ctx context.Context, id uuid.UUID) (uuid.UUID, string, error) {
	var owner *uuid.UUID
	var ref string
	err := s.db.QueryRow(ctx, `SELECT owner_person_id, ref FROM crm.leads WHERE id = $1`, id).Scan(&owner, &ref)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, "", fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	if err != nil {
		return uuid.Nil, "", fmt.Errorf("crm: lead owner: %w", err)
	}
	if owner == nil {
		return uuid.Nil, ref, nil
	}
	return *owner, ref, nil
}

func (s *PGStore) ListLeads(ctx context.Context, q ListLeadsQuery, limit int, before *Cursor) ([]Lead, error) {
	args := []any{}
	where := "TRUE"
	add := func(clause string, v any) {
		args = append(args, v)
		where += fmt.Sprintf(clause, len(args))
	}
	if q.Status != nil {
		add(" AND status = $%d", string(*q.Status))
	}
	if q.OwnerID != nil {
		add(" AND owner_person_id = $%d", *q.OwnerID)
	}
	if q.SourceKey != "" {
		add(" AND source_key = $%d", q.SourceKey)
	}
	if q.OrganizationID != nil {
		add(" AND organization_id = $%d", *q.OrganizationID)
	}
	if before != nil {
		args = append(args, before.At, before.ID)
		where += fmt.Sprintf(" AND (created_at, id) < ($%d, $%d)", len(args)-1, len(args))
	}
	args = append(args, limit)
	rows, err := s.db.Query(ctx, `SELECT `+leadCols+` FROM crm.leads WHERE `+where+
		` ORDER BY created_at DESC, id DESC LIMIT $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, fmt.Errorf("crm: list leads: %w", err)
	}
	defer rows.Close()
	out := []Lead{}
	for rows.Next() {
		l, err := scanLead(rows)
		if err != nil {
			return nil, fmt.Errorf("crm: scan lead: %w", err)
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (s *PGStore) UpdateLead(ctx context.Context, l Lead, expectedVersion int, ev *outbox.Event) (Lead, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Lead{}, fmt.Errorf("crm: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	updated, err := scanLead(tx.QueryRow(ctx, `
		UPDATE crm.leads SET
			source_key = $2, source_detail = $3, organization_id = $4, contact_id = $5,
			contact_name = $6, contact_phone = $7, contact_email = $8, need_summary = $9,
			estimated_value_minor = $10, currency = $11, priority = $12, owner_person_id = $13,
			status = $14, next_action = $15, next_action_due_at = $16, qualification_notes = $17,
			lost_reason = $18, version = version + 1
		WHERE id = $1 AND version = $19
		RETURNING `+leadCols,
		l.ID, l.SourceKey, nullIfEmpty(l.SourceDetail), l.OrganizationID, l.ContactID,
		nullIfEmpty(l.ContactName), nullIfEmpty(l.ContactPhone), nullIfEmpty(l.ContactEmail),
		nullIfEmpty(l.NeedSummary), l.EstimatedValueMinor, l.Currency, string(l.Priority),
		l.OwnerPersonID, string(l.Status), nullIfEmpty(l.NextAction), l.NextActionDueAt,
		nullIfEmpty(l.QualificationNotes), l.LostReason, expectedVersion))
	if errors.Is(err, pgx.ErrNoRows) {
		return Lead{}, s.versionConflict(ctx, "crm.leads", l.ID, expectedVersion)
	}
	if err != nil {
		return Lead{}, fmt.Errorf("crm: update lead: %w", err)
	}
	if ev != nil {
		if _, err := outbox.Write(ctx, tx, *ev); err != nil {
			return Lead{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Lead{}, fmt.Errorf("crm: commit: %w", err)
	}
	return updated, nil
}

// SetLeadOwner assigns an owner and a dated next action, advancing a new lead to
// assigned (§8.1). It runs read-modify-write in one transaction.
func (s *PGStore) SetLeadOwner(ctx context.Context, id, owner uuid.UUID, nextAction string, dueAt time.Time, ev *outbox.Event) (Lead, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Lead{}, fmt.Errorf("crm: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	cur, err := scanLead(tx.QueryRow(ctx, `SELECT `+leadCols+` FROM crm.leads WHERE id = $1 FOR UPDATE`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Lead{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	if err != nil {
		return Lead{}, fmt.Errorf("crm: lock lead: %w", err)
	}
	status := cur.Status
	if status == LeadNew {
		status = LeadAssigned
	}
	updated, err := scanLead(tx.QueryRow(ctx, `
		UPDATE crm.leads SET owner_person_id = $2, next_action = $3, next_action_due_at = $4,
			status = $5, version = version + 1
		WHERE id = $1 RETURNING `+leadCols,
		id, owner, nextAction, dueAt, string(status)))
	if err != nil {
		return Lead{}, fmt.Errorf("crm: assign lead: %w", err)
	}
	if ev != nil {
		if _, err := outbox.Write(ctx, tx, *ev); err != nil {
			return Lead{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Lead{}, fmt.Errorf("crm: commit: %w", err)
	}
	return updated, nil
}

// SetLeadStatus moves a lead to a new status, enforcing the lifecycle.
func (s *PGStore) SetLeadStatus(ctx context.Context, id uuid.UUID, status LeadStatus, lostReason *LostReason, ev *outbox.Event) (Lead, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Lead{}, fmt.Errorf("crm: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Lock the row so a concurrent transition cannot interleave.
	if _, err := scanLead(tx.QueryRow(ctx, `SELECT `+leadCols+` FROM crm.leads WHERE id = $1 FOR UPDATE`, id)); errors.Is(err, pgx.ErrNoRows) {
		return Lead{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	} else if err != nil {
		return Lead{}, fmt.Errorf("crm: lock lead: %w", err)
	}
	updated, err := scanLead(tx.QueryRow(ctx, `
		UPDATE crm.leads SET status = $2, lost_reason = $3, version = version + 1
		WHERE id = $1 RETURNING `+leadCols,
		id, string(status), lostReason))
	if err != nil {
		return Lead{}, fmt.Errorf("crm: set lead status: %w", err)
	}
	if ev != nil {
		if _, err := outbox.Write(ctx, tx, *ev); err != nil {
			return Lead{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Lead{}, fmt.Errorf("crm: commit: %w", err)
	}
	return updated, nil
}

// ConvertLead creates the opportunity and marks the lead converted in one
// transaction, so a convert never half-happens.
func (s *PGStore) ConvertLead(ctx context.Context, leadID uuid.UUID, opp Opportunity, buildOpp func(ref string) (outbox.Event, error), leadEv *outbox.Event) (Opportunity, Lead, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Opportunity{}, Lead{}, fmt.Errorf("crm: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `SELECT id FROM crm.leads WHERE id = $1 FOR UPDATE`, leadID); err != nil {
		return Opportunity{}, Lead{}, fmt.Errorf("crm: lock lead: %w", err)
	}

	oppRef, err := allocateRefTx(ctx, tx, "opp", opp.CreatedAt)
	if err != nil {
		return Opportunity{}, Lead{}, err
	}
	opp.Ref = oppRef
	createdOpp, err := scanOpportunity(tx.QueryRow(ctx, `
		INSERT INTO crm.opportunities (
			id, ref, organization_id, title, value_minor, currency, probability, stage_key,
			expected_close_at, owner_person_id, lost_reason, version, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		RETURNING `+opportunityCols,
		opp.ID, opp.Ref, opp.OrganizationID, opp.Title, opp.ValueMinor, opp.Currency,
		opp.Probability, opp.StageKey, opp.ExpectedCloseAt, opp.OwnerPersonID, opp.LostReason,
		opp.Version, opp.CreatedAt, opp.UpdatedAt))
	if err != nil {
		return Opportunity{}, Lead{}, fmt.Errorf("crm: create opportunity: %w", err)
	}

	updatedLead, err := scanLead(tx.QueryRow(ctx, `
		UPDATE crm.leads SET status = 'converted', converted_opportunity_id = $2, version = version + 1
		WHERE id = $1 RETURNING `+leadCols, leadID, createdOpp.ID))
	if err != nil {
		return Opportunity{}, Lead{}, fmt.Errorf("crm: mark lead converted: %w", err)
	}

	if buildOpp != nil {
		ev, err := buildOpp(createdOpp.Ref)
		if err != nil {
			return Opportunity{}, Lead{}, err
		}
		if _, err := outbox.Write(ctx, tx, ev); err != nil {
			return Opportunity{}, Lead{}, err
		}
	}
	if leadEv != nil {
		if _, err := outbox.Write(ctx, tx, *leadEv); err != nil {
			return Opportunity{}, Lead{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Opportunity{}, Lead{}, fmt.Errorf("crm: commit: %w", err)
	}
	return createdOpp, updatedLead, nil
}

// ── opportunities ───────────────────────────────────────────────────────────

const opportunityCols = `id, ref, organization_id, title, value_minor, currency, probability,
	stage_key, expected_close_at, owner_person_id, lost_reason, version, created_at, updated_at`

func scanOpportunity(row rowScanner) (Opportunity, error) {
	var o Opportunity
	err := row.Scan(&o.ID, &o.Ref, &o.OrganizationID, &o.Title, &o.ValueMinor, &o.Currency,
		&o.Probability, &o.StageKey, &o.ExpectedCloseAt, &o.OwnerPersonID, &o.LostReason,
		&o.Version, &o.CreatedAt, &o.UpdatedAt)
	return o, err
}

func (s *PGStore) CreateOpportunity(ctx context.Context, o Opportunity, build func(ref string) (outbox.Event, error)) (Opportunity, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Opportunity{}, fmt.Errorf("crm: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	ref, err := allocateRefTx(ctx, tx, "opp", o.CreatedAt)
	if err != nil {
		return Opportunity{}, err
	}
	o.Ref = ref
	created, err := scanOpportunity(tx.QueryRow(ctx, `
		INSERT INTO crm.opportunities (
			id, ref, organization_id, title, value_minor, currency, probability, stage_key,
			expected_close_at, owner_person_id, lost_reason, version, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		RETURNING `+opportunityCols,
		o.ID, o.Ref, o.OrganizationID, o.Title, o.ValueMinor, o.Currency, o.Probability,
		o.StageKey, o.ExpectedCloseAt, o.OwnerPersonID, o.LostReason, o.Version, o.CreatedAt, o.UpdatedAt))
	if err != nil {
		return Opportunity{}, fmt.Errorf("crm: create opportunity: %w", err)
	}
	if build != nil {
		ev, err := build(created.Ref)
		if err != nil {
			return Opportunity{}, err
		}
		if _, err := outbox.Write(ctx, tx, ev); err != nil {
			return Opportunity{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Opportunity{}, fmt.Errorf("crm: commit: %w", err)
	}
	return created, nil
}

func (s *PGStore) GetOpportunity(ctx context.Context, id uuid.UUID) (*Opportunity, error) {
	o, err := scanOpportunity(s.db.QueryRow(ctx, `SELECT `+opportunityCols+` FROM crm.opportunities WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("crm: get opportunity: %w", err)
	}
	return &o, nil
}

func (s *PGStore) ListOpportunities(ctx context.Context, stageKey *string, ownerID *uuid.UUID, limit int, before *Cursor) ([]Opportunity, error) {
	args := []any{}
	where := "TRUE"
	if stageKey != nil {
		args = append(args, *stageKey)
		where += fmt.Sprintf(" AND stage_key = $%d", len(args))
	}
	if ownerID != nil {
		args = append(args, *ownerID)
		where += fmt.Sprintf(" AND owner_person_id = $%d", len(args))
	}
	if before != nil {
		args = append(args, before.At, before.ID)
		where += fmt.Sprintf(" AND (created_at, id) < ($%d, $%d)", len(args)-1, len(args))
	}
	args = append(args, limit)
	rows, err := s.db.Query(ctx, `SELECT `+opportunityCols+` FROM crm.opportunities WHERE `+where+
		` ORDER BY created_at DESC, id DESC LIMIT $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, fmt.Errorf("crm: list opportunities: %w", err)
	}
	defer rows.Close()
	out := []Opportunity{}
	for rows.Next() {
		o, err := scanOpportunity(rows)
		if err != nil {
			return nil, fmt.Errorf("crm: scan opportunity: %w", err)
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func (s *PGStore) SetOpportunityStage(ctx context.Context, id uuid.UUID, stageKey string, lostReason *LostReason, ev *outbox.Event) (Opportunity, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Opportunity{}, fmt.Errorf("crm: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	updated, err := scanOpportunity(tx.QueryRow(ctx, `
		UPDATE crm.opportunities SET stage_key = $2, lost_reason = $3, version = version + 1
		WHERE id = $1 RETURNING `+opportunityCols, id, stageKey, lostReason))
	if errors.Is(err, pgx.ErrNoRows) {
		return Opportunity{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	if err != nil {
		return Opportunity{}, fmt.Errorf("crm: set opportunity stage: %w", err)
	}
	if ev != nil {
		if _, err := outbox.Write(ctx, tx, *ev); err != nil {
			return Opportunity{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Opportunity{}, fmt.Errorf("crm: commit: %w", err)
	}
	return updated, nil
}

// ── interactions ────────────────────────────────────────────────────────────

const interactionCols = `id, interaction_type, direction, contact_id, lead_id, opportunity_id,
	occurred_at, duration_minutes, outcome, COALESCE(summary,''), COALESCE(next_action,''),
	next_action_due_at, recording_file_id, recorded_by, version, created_at, updated_at`

func scanInteraction(row rowScanner) (Interaction, error) {
	var i Interaction
	err := row.Scan(&i.ID, &i.Type, &i.Direction, &i.ContactID, &i.LeadID, &i.OpportunityID,
		&i.OccurredAt, &i.DurationMinutes, &i.Outcome, &i.Summary, &i.NextAction, &i.NextActionDueAt,
		&i.RecordingFileID, &i.RecordedBy, &i.Version, &i.CreatedAt, &i.UpdatedAt)
	return i, err
}

func (s *PGStore) CreateInteraction(ctx context.Context, i Interaction, ev *outbox.Event) (Interaction, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Interaction{}, fmt.Errorf("crm: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	created, err := scanInteraction(tx.QueryRow(ctx, `
		INSERT INTO crm.interactions (
			id, interaction_type, direction, contact_id, lead_id, opportunity_id, occurred_at,
			duration_minutes, outcome, summary, next_action, next_action_due_at, recording_file_id,
			recorded_by, version, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)
		RETURNING `+interactionCols,
		i.ID, string(i.Type), string(i.Direction), i.ContactID, i.LeadID, i.OpportunityID,
		i.OccurredAt, i.DurationMinutes, i.Outcome, nullIfEmpty(i.Summary), nullIfEmpty(i.NextAction),
		i.NextActionDueAt, i.RecordingFileID, i.RecordedBy, i.Version, i.CreatedAt, i.UpdatedAt))
	if err != nil {
		return Interaction{}, fmt.Errorf("crm: create interaction: %w", err)
	}
	if ev != nil {
		if _, err := outbox.Write(ctx, tx, *ev); err != nil {
			return Interaction{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Interaction{}, fmt.Errorf("crm: commit: %w", err)
	}
	return created, nil
}

func (s *PGStore) GetInteraction(ctx context.Context, id uuid.UUID) (*Interaction, error) {
	i, err := scanInteraction(s.db.QueryRow(ctx, `SELECT `+interactionCols+` FROM crm.interactions WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("crm: get interaction: %w", err)
	}
	return &i, nil
}

func (s *PGStore) ListInteractions(ctx context.Context, q ListInteractionsQuery, limit int, before *Cursor) ([]Interaction, error) {
	args := []any{}
	where := "TRUE"
	if q.LeadID != nil {
		args = append(args, *q.LeadID)
		where += fmt.Sprintf(" AND lead_id = $%d", len(args))
	}
	if q.OpportunityID != nil {
		args = append(args, *q.OpportunityID)
		where += fmt.Sprintf(" AND opportunity_id = $%d", len(args))
	}
	if q.ContactID != nil {
		args = append(args, *q.ContactID)
		where += fmt.Sprintf(" AND contact_id = $%d", len(args))
	}
	if before != nil {
		args = append(args, before.At, before.ID)
		where += fmt.Sprintf(" AND (created_at, id) < ($%d, $%d)", len(args)-1, len(args))
	}
	args = append(args, limit)
	rows, err := s.db.Query(ctx, `SELECT `+interactionCols+` FROM crm.interactions WHERE `+where+
		` ORDER BY created_at DESC, id DESC LIMIT $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, fmt.Errorf("crm: list interactions: %w", err)
	}
	defer rows.Close()
	out := []Interaction{}
	for rows.Next() {
		i, err := scanInteraction(rows)
		if err != nil {
			return nil, fmt.Errorf("crm: scan interaction: %w", err)
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

// ── follow-ups ──────────────────────────────────────────────────────────────

const followupCols = `id, lead_id, opportunity_id, owner_person_id, due_at, channel, reason,
	status, COALESCE(outcome,''), completed_at, version, created_at, updated_at`

func scanFollowup(row rowScanner) (Followup, error) {
	var f Followup
	err := row.Scan(&f.ID, &f.LeadID, &f.OpportunityID, &f.OwnerPersonID, &f.DueAt, &f.Channel,
		&f.Reason, &f.Status, &f.Outcome, &f.CompletedAt, &f.Version, &f.CreatedAt, &f.UpdatedAt)
	return f, err
}

func (s *PGStore) CreateFollowup(ctx context.Context, f Followup, ev *outbox.Event) (Followup, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Followup{}, fmt.Errorf("crm: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	created, err := scanFollowup(tx.QueryRow(ctx, `
		INSERT INTO crm.followups (
			id, lead_id, opportunity_id, owner_person_id, due_at, channel, reason,
			status, outcome, completed_at, version, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		RETURNING `+followupCols,
		f.ID, f.LeadID, f.OpportunityID, f.OwnerPersonID, f.DueAt, string(f.Channel), f.Reason,
		string(f.Status), nullIfEmpty(f.Outcome), f.CompletedAt, f.Version, f.CreatedAt, f.UpdatedAt))
	if err != nil {
		return Followup{}, fmt.Errorf("crm: create followup: %w", err)
	}
	if ev != nil {
		if _, err := outbox.Write(ctx, tx, *ev); err != nil {
			return Followup{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Followup{}, fmt.Errorf("crm: commit: %w", err)
	}
	return created, nil
}

func (s *PGStore) GetFollowup(ctx context.Context, id uuid.UUID) (*Followup, error) {
	f, err := scanFollowup(s.db.QueryRow(ctx, `SELECT `+followupCols+` FROM crm.followups WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("crm: get followup: %w", err)
	}
	return &f, nil
}

func (s *PGStore) ListFollowups(ctx context.Context, q ListFollowupsQuery, limit int, before *Cursor) ([]Followup, error) {
	args := []any{}
	where := "TRUE"
	if q.Status != nil {
		args = append(args, string(*q.Status))
		where += fmt.Sprintf(" AND status = $%d", len(args))
	}
	if q.OwnerID != nil {
		args = append(args, *q.OwnerID)
		where += fmt.Sprintf(" AND owner_person_id = $%d", len(args))
	}
	if q.LeadID != nil {
		args = append(args, *q.LeadID)
		where += fmt.Sprintf(" AND lead_id = $%d", len(args))
	}
	if q.DueBefore != nil {
		args = append(args, *q.DueBefore)
		where += fmt.Sprintf(" AND due_at < $%d", len(args))
	}
	if before != nil {
		args = append(args, before.At, before.ID)
		where += fmt.Sprintf(" AND (due_at, id) < ($%d, $%d)", len(args)-1, len(args))
	}
	args = append(args, limit)
	// Follow-ups are listed by due date, soonest first, so the "My day" call
	// list reads naturally (§8.1).
	rows, err := s.db.Query(ctx, `SELECT `+followupCols+` FROM crm.followups WHERE `+where+
		` ORDER BY due_at ASC, id ASC LIMIT $`+fmt.Sprint(len(args)), args...)
	if err != nil {
		return nil, fmt.Errorf("crm: list followups: %w", err)
	}
	defer rows.Close()
	out := []Followup{}
	for rows.Next() {
		f, err := scanFollowup(rows)
		if err != nil {
			return nil, fmt.Errorf("crm: scan followup: %w", err)
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// ResolveFollowup completes or cancels a follow-up.
func (s *PGStore) ResolveFollowup(ctx context.Context, id uuid.UUID, status FollowupStatus, outcome string, ev *outbox.Event) (Followup, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Followup{}, fmt.Errorf("crm: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var completedAt *time.Time
	if status == FollowupDone {
		now := time.Now().UTC()
		completedAt = &now
	}
	updated, err := scanFollowup(tx.QueryRow(ctx, `
		UPDATE crm.followups SET status = $2, outcome = $3, completed_at = $4, version = version + 1
		WHERE id = $1 RETURNING `+followupCols, id, string(status), nullIfEmpty(outcome), completedAt))
	if errors.Is(err, pgx.ErrNoRows) {
		return Followup{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	if err != nil {
		return Followup{}, fmt.Errorf("crm: resolve followup: %w", err)
	}
	if ev != nil {
		if _, err := outbox.Write(ctx, tx, *ev); err != nil {
			return Followup{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Followup{}, fmt.Errorf("crm: commit: %w", err)
	}
	return updated, nil
}

// MissOverdueFollowups flips every scheduled follow-up past its due date to
// missed, writing an event per row. It is a single pass; the caller supplies the
// event builder so the missed event matches the module's other events.
func (s *PGStore) MissOverdueFollowups(ctx context.Context, now time.Time, limit int, build func(f Followup) (outbox.Event, error)) ([]Followup, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("crm: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := tx.Query(ctx, `SELECT `+followupCols+` FROM crm.followups
		WHERE status = 'scheduled' AND due_at < $1
		ORDER BY due_at ASC LIMIT $2 FOR UPDATE`, now, limit)
	if err != nil {
		return nil, fmt.Errorf("crm: sweep followups: %w", err)
	}
	var due []Followup
	for rows.Next() {
		f, err := scanFollowup(rows)
		if err != nil {
			rows.Close()
			return nil, fmt.Errorf("crm: scan followup: %w", err)
		}
		due = append(due, f)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]Followup, 0, len(due))
	for _, f := range due {
		updated, err := scanFollowup(tx.QueryRow(ctx, `
			UPDATE crm.followups SET status = 'missed', version = version + 1
			WHERE id = $1 RETURNING `+followupCols, f.ID))
		if err != nil {
			return nil, fmt.Errorf("crm: mark followup missed: %w", err)
		}
		ev, err := build(updated)
		if err != nil {
			return nil, err
		}
		if _, err := outbox.Write(ctx, tx, ev); err != nil {
			return nil, err
		}
		out = append(out, updated)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("crm: commit: %w", err)
	}
	return out, nil
}

// ── proposals ───────────────────────────────────────────────────────────────

const proposalCols = `id, ref, opportunity_id, version_no, COALESCE(scope_summary,''), line_items,
	price_minor, currency, discount_minor, valid_until, status, document_file_id, approval_id,
	version, created_at, updated_at`

func scanProposal(row rowScanner) (Proposal, error) {
	var p Proposal
	err := row.Scan(&p.ID, &p.Ref, &p.OpportunityID, &p.VersionNo, &p.ScopeSummary, &p.LineItems,
		&p.PriceMinor, &p.Currency, &p.DiscountMinor, &p.ValidUntil, &p.Status, &p.DocumentFileID,
		&p.ApprovalID, &p.Version, &p.CreatedAt, &p.UpdatedAt)
	return p, err
}

func (s *PGStore) CreateProposal(ctx context.Context, p Proposal, build func(ref string) (outbox.Event, error)) (Proposal, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Proposal{}, fmt.Errorf("crm: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	ref, err := allocateRefTx(ctx, tx, "prop", p.CreatedAt)
	if err != nil {
		return Proposal{}, err
	}
	p.Ref = ref
	created, err := scanProposal(tx.QueryRow(ctx, `
		INSERT INTO crm.proposals (
			id, ref, opportunity_id, version_no, scope_summary, line_items, price_minor, currency,
			discount_minor, valid_until, status, document_file_id, approval_id, version, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
		RETURNING `+proposalCols,
		p.ID, p.Ref, p.OpportunityID, p.VersionNo, nullIfEmpty(p.ScopeSummary), lineItemsOrEmpty(p.LineItems),
		p.PriceMinor, p.Currency, p.DiscountMinor, p.ValidUntil, string(p.Status), p.DocumentFileID,
		p.ApprovalID, p.Version, p.CreatedAt, p.UpdatedAt))
	if err != nil {
		return Proposal{}, fmt.Errorf("crm: create proposal: %w", err)
	}
	if build != nil {
		ev, err := build(created.Ref)
		if err != nil {
			return Proposal{}, err
		}
		if _, err := outbox.Write(ctx, tx, ev); err != nil {
			return Proposal{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Proposal{}, fmt.Errorf("crm: commit: %w", err)
	}
	return created, nil
}

func (s *PGStore) GetProposal(ctx context.Context, id uuid.UUID) (*Proposal, error) {
	p, err := scanProposal(s.db.QueryRow(ctx, `SELECT `+proposalCols+` FROM crm.proposals WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("crm: get proposal: %w", err)
	}
	return &p, nil
}

func (s *PGStore) ListProposals(ctx context.Context, opportunityID uuid.UUID) ([]Proposal, error) {
	rows, err := s.db.Query(ctx, `SELECT `+proposalCols+` FROM crm.proposals WHERE opportunity_id = $1 ORDER BY version_no DESC`, opportunityID)
	if err != nil {
		return nil, fmt.Errorf("crm: list proposals: %w", err)
	}
	defer rows.Close()
	out := []Proposal{}
	for rows.Next() {
		p, err := scanProposal(rows)
		if err != nil {
			return nil, fmt.Errorf("crm: scan proposal: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *PGStore) SetProposalStatus(ctx context.Context, id uuid.UUID, status ProposalStatus, ev *outbox.Event) (Proposal, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Proposal{}, fmt.Errorf("crm: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	updated, err := scanProposal(tx.QueryRow(ctx, `
		UPDATE crm.proposals SET status = $2, version = version + 1
		WHERE id = $1 RETURNING `+proposalCols, id, string(status)))
	if errors.Is(err, pgx.ErrNoRows) {
		return Proposal{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	if err != nil {
		return Proposal{}, fmt.Errorf("crm: set proposal status: %w", err)
	}
	if ev != nil {
		if _, err := outbox.Write(ctx, tx, *ev); err != nil {
			return Proposal{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Proposal{}, fmt.Errorf("crm: commit: %w", err)
	}
	return updated, nil
}

// ── pipeline ────────────────────────────────────────────────────────────────

// PipelineSummary totals open opportunities per stage. Won/lost stages are
// excluded because they are closed (§8.1).
func (s *PGStore) PipelineSummary(ctx context.Context) ([]PipelineStageTotal, error) {
	rows, err := s.db.Query(ctx, `
		SELECT ps.key, ps.label, ps.sort_order,
		       COUNT(o.id)::int AS count,
		       COALESCE(SUM(o.value_minor), 0)::bigint AS value_minor
		FROM crm.pipeline_stages ps
		LEFT JOIN crm.opportunities o ON o.stage_key = ps.key
		WHERE ps.active AND NOT ps.is_won AND NOT ps.is_lost
		GROUP BY ps.key, ps.label, ps.sort_order
		ORDER BY ps.sort_order, ps.key`)
	if err != nil {
		return nil, fmt.Errorf("crm: pipeline summary: %w", err)
	}
	defer rows.Close()
	out := []PipelineStageTotal{}
	for rows.Next() {
		var t PipelineStageTotal
		if err := rows.Scan(&t.StageKey, &t.Label, &t.SortOrder, &t.Count, &t.ValueMinor); err != nil {
			return nil, fmt.Errorf("crm: scan pipeline stage: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ── helpers ─────────────────────────────────────────────────────────────────

// allocateRefTx formats the next human reference for a type in the current year
// from the foundation core.sequences counter (LEAD-2026-0042, OPP-…, PROP-…).
func allocateRefTx(ctx context.Context, tx pgx.Tx, refType string, at time.Time) (string, error) {
	if at.IsZero() {
		at = time.Now().UTC()
	}
	var ref string
	if err := tx.QueryRow(ctx, `SELECT core.next_reference($1, $2)`, refType, at.UTC().Year()).Scan(&ref); err != nil {
		return "", fmt.Errorf("crm: allocate ref %q: %w", refType, err)
	}
	return ref, nil
}

// versionConflict turns an empty UPDATE into the precise error: not found, or a
// stale version (§6.6).
func (s *PGStore) versionConflict(ctx context.Context, table string, id uuid.UUID, expected int) error {
	var cur int
	err := s.db.QueryRow(ctx, `SELECT version FROM `+table+` WHERE id = $1`, id).Scan(&cur)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	if err != nil {
		return fmt.Errorf("crm: read version: %w", err)
	}
	return fmt.Errorf("%w: version %d is stale (current %d)", ErrConflict, expected, cur)
}
