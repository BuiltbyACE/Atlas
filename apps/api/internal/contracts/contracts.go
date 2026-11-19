// Package contracts owns Phase 3 contracts and commitments: the contract
// lifecycle, its milestones, deliverables, obligations and SLA terms, and the
// change requests that amend a signed agreement (docs/ATLAS.md §8.3).
//
// A contract is drafted from an accepted proposal (§8.1). Signing is the event
// that unlocks handover and the financial schedule (later phases). Money is
// integer minor units with a currency (ADR-010); human refs come from
// core.next_reference (CTR-…, CR-…). Go is the sole writer (ADR-002): every
// state change is audited and, where the orchestrator or finance must react, an
// outbox event is written in the same transaction (ADR-005, ADR-009).
package contracts

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Sentinel errors. The HTTP layer maps these onto problem responses.
var (
	ErrNotFound  = errors.New("contracts: not found")
	ErrInvalid   = errors.New("contracts: invalid")
	ErrConflict  = errors.New("contracts: conflict")
	ErrForbidden = errors.New("contracts: forbidden")
)

// ── enums ───────────────────────────────────────────────────────────────────

// ContractType is the commercial shape of an agreement (§8.3).
type ContractType string

const (
	TypeFixedPriceProject ContractType = "fixed_price_project"
	TypeRetainer          ContractType = "retainer"
	TypeSupportSLA        ContractType = "support_sla"
	TypeLicence           ContractType = "licence"
	TypeHosting           ContractType = "hosting"
)

func validContractType(t ContractType) bool {
	switch t {
	case TypeFixedPriceProject, TypeRetainer, TypeSupportSLA, TypeLicence, TypeHosting:
		return true
	}
	return false
}

// ContractStatus is the contract lifecycle state (§8.3).
type ContractStatus string

const (
	StatusDraft          ContractStatus = "draft"
	StatusInternalReview ContractStatus = "internal_review"
	StatusApproved       ContractStatus = "approved"
	StatusSent           ContractStatus = "sent"
	StatusSigned         ContractStatus = "signed"
	StatusActive         ContractStatus = "active"
	StatusRenewalDue     ContractStatus = "renewal_due"
	StatusRenewed        ContractStatus = "renewed"
	StatusCompleted      ContractStatus = "completed"
	StatusTerminated     ContractStatus = "terminated"
)

func validContractStatus(s ContractStatus) bool {
	switch s {
	case StatusDraft, StatusInternalReview, StatusApproved, StatusSent, StatusSigned,
		StatusActive, StatusRenewalDue, StatusRenewed, StatusCompleted, StatusTerminated:
		return true
	}
	return false
}

// allowedContractTransition encodes the §8.3 lifecycle diagram.
func allowedContractTransition(from, to ContractStatus) bool {
	switch from {
	case StatusDraft:
		return to == StatusInternalReview
	case StatusInternalReview:
		return to == StatusApproved || to == StatusDraft
	case StatusApproved:
		return to == StatusSent
	case StatusSent:
		return to == StatusSigned
	case StatusSigned:
		return to == StatusActive
	case StatusActive:
		return to == StatusRenewalDue || to == StatusCompleted || to == StatusTerminated
	case StatusRenewalDue:
		return to == StatusRenewed || to == StatusTerminated
	case StatusRenewed:
		return to == StatusActive || to == StatusTerminated
	}
	return false
}

// ObligationParty is who owes an obligation.
type ObligationParty string

const (
	PartySafariStack ObligationParty = "safaristack"
	PartyClient      ObligationParty = "client"
)

// ObligationStatus is an obligation's state.
type ObligationStatus string

const (
	ObligationOpen    ObligationStatus = "open"
	ObligationMet     ObligationStatus = "met"
	ObligationOverdue ObligationStatus = "overdue"
	ObligationWaived  ObligationStatus = "waived"
)

func validObligationStatus(s ObligationStatus) bool {
	switch s {
	case ObligationOpen, ObligationMet, ObligationOverdue, ObligationWaived:
		return true
	}
	return false
}

// ChangeRequestStatus is a change request's state (§8.3).
type ChangeRequestStatus string

const (
	CRDraft       ChangeRequestStatus = "draft"
	CRSubmitted   ChangeRequestStatus = "submitted"
	CRApproved    ChangeRequestStatus = "approved"
	CRRejected    ChangeRequestStatus = "rejected"
	CRImplemented ChangeRequestStatus = "implemented"
)

func validCRStatus(s ChangeRequestStatus) bool {
	switch s {
	case CRDraft, CRSubmitted, CRApproved, CRRejected, CRImplemented:
		return true
	}
	return false
}

// ── records ─────────────────────────────────────────────────────────────────

// Contract is a client agreement.
type Contract struct {
	ID             uuid.UUID
	Ref            string
	OrganizationID uuid.UUID
	OpportunityID  *uuid.UUID
	ContractType   ContractType
	Status         ContractStatus
	ValueMinor     *int64
	Currency       string
	StartAt        *time.Time
	EndAt          *time.Time
	PaymentTerms   string
	RenewalTerms   string
	GoverningDocs  json.RawMessage
	Signatories    json.RawMessage
	SignedAt       *time.Time
	Version        int
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// Milestone is a billing milestone on a contract.
type Milestone struct {
	ID                 uuid.UUID
	ContractID         uuid.UUID
	Title              string
	DueAt              *time.Time
	AmountMinor        *int64
	Currency           string
	ProjectMilestoneID *uuid.UUID
	Version            int
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// Deliverable is what a contract promises, with acceptance criteria.
type Deliverable struct {
	ID                 uuid.UUID
	ContractID         uuid.UUID
	Description        string
	AcceptanceCriteria string
	MilestoneID        *uuid.UUID
	Version            int
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// Obligation is something one party owes the other by a date.
type Obligation struct {
	ID            uuid.UUID
	ContractID    uuid.UUID
	Party         ObligationParty
	Description   string
	DueAt         *time.Time
	Recurrence    string
	Status        ObligationStatus
	OwnerPersonID *uuid.UUID
	Version       int
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// SLATerm is a measurable service promise on a contract.
type SLATerm struct {
	ID                uuid.UUID
	ContractID        uuid.UUID
	Metric            string
	Target            string
	MeasurementWindow string
	SystemIDs         []uuid.UUID
	Version           int
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// ContractVersion is one revision of a contract document, with a change
// summary (§8.3). Approving a change request appends a version.
type ContractVersion struct {
	ID             uuid.UUID
	ContractID     uuid.UUID
	VersionNo      int
	DocumentFileID *uuid.UUID
	ChangeSummary  string
	CreatedBy      *uuid.UUID
	CreatedAt      time.Time
}

// ChangeRequest is a proposed change to a signed contract.
type ChangeRequest struct {
	ID                 uuid.UUID
	Ref                string
	ContractID         uuid.UUID
	RequestedBy        string
	Description        string
	EffortImpact       string
	PriceImpactMinor   *int64
	Currency           string
	DeadlineImpactDays *int
	Status             ChangeRequestStatus
	ApprovalID         *uuid.UUID
	Version            int
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// ── helpers ─────────────────────────────────────────────────────────────────

func trim(s string) string { return strings.TrimSpace(s) }

func normalizeCurrency(c string) string {
	c = strings.ToUpper(trim(c))
	if c == "" {
		return "KES"
	}
	return c
}

func validateCurrency(c string) error {
	if len(c) != 3 {
		return fmt.Errorf("%w: currency must be a three-letter code", ErrInvalid)
	}
	for _, r := range c {
		if r < 'A' || r > 'Z' {
			return fmt.Errorf("%w: currency must be a three-letter code", ErrInvalid)
		}
	}
	return nil
}

// Money returns the minor-unit value, rejecting negative amounts.
func validateAmountMinor(v *int64) error {
	if v != nil && *v < 0 {
		return fmt.Errorf("%w: amount cannot be negative", ErrInvalid)
	}
	return nil
}
