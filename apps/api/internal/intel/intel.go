// Package intel is executive intelligence (§8.8).
//
// The package is the read side of the module: decisions (the company's memory
// of major decisions), risks (client, delivery, financial, people, systems),
// briefs (the daily executive brief) and metric snapshots (point-in-time KPI
// figures). All numbers are computed deterministically from records and link
// back to them — every figure is clickable to its source rows.
//
// Go is the sole writer (ADR-002).
package intel

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Sentinel errors. The HTTP layer maps these onto problem responses.
var (
	ErrNotFound  = errors.New("intel: not found")
	ErrInvalid   = errors.New("intel: invalid")
	ErrConflict  = errors.New("intel: conflict")
	ErrForbidden = errors.New("intel: forbidden")
	ErrDuplicate = errors.New("intel: duplicate")
)

// Stable problem codes for the write lifecycle. Clients branch on `code`, so
// every refusal the service can raise has one constant here.
const (
	CodeDecisionNotEditable    = "INTEL_DECISION_NOT_EDITABLE"
	CodeDecisionAlreadyDecided = "INTEL_DECISION_ALREADY_DECIDED"
	CodeInvalidDecisionOutcome = "INTEL_INVALID_DECISION_OUTCOME"
	CodeInvalidRiskKind        = "INTEL_INVALID_RISK_KIND"
	CodeInvalidRiskLikelihood  = "INTEL_INVALID_RISK_LIKELIHOOD"
	CodeInvalidRiskImpact      = "INTEL_INVALID_RISK_IMPACT"
	CodeRiskNotEditable        = "INTEL_RISK_NOT_EDITABLE"
	CodeBriefNotEditable       = "INTEL_BRIEF_NOT_EDITABLE"
	CodeBriefAlreadyDelivered  = "INTEL_BRIEF_ALREADY_DELIVERED"
	CodeMetricNotEditable      = "INTEL_METRIC_NOT_EDITABLE"
	CodeDuplicateRef           = "INTEL_DUPLICATE_REF"
)

// codedError overlays a stable problem code on top of a sentinel error. The
// HTTP layer still derives the status from the sentinel; the code is what the
// client keys on.
type codedError struct {
	err  error
	code string
}

func (e *codedError) Error() string { return e.err.Error() }
func (e *codedError) Unwrap() error { return e.err }

// withCode attaches a stable problem code to err.
func withCode(err error, code string) error {
	if err == nil {
		return nil
	}
	return &codedError{err: err, code: code}
}

// problemCode reports the stable problem code for err, or "" when it has none.
func problemCode(err error) string {
	var ce *codedError
	if errors.As(err, &ce) {
		return ce.code
	}
	return ""
}

// ── enums ───────────────────────────────────────────────────────────────────

// DecisionOutcome is where a decision is in its life.
type DecisionOutcome string

const (
	DecisionOpen     DecisionOutcome = "open"
	DecisionApproved DecisionOutcome = "approved"
	DecisionRejected DecisionOutcome = "rejected"
	DecisionDeferred DecisionOutcome = "deferred"
)

func validDecisionOutcome(o DecisionOutcome) bool {
	switch o {
	case DecisionOpen, DecisionApproved, DecisionRejected, DecisionDeferred:
		return true
	}
	return false
}

// DecisionStatus aliases outcome for compatibility with the UI's status field.
type DecisionStatus string

const (
	DecisionStatusOpen     DecisionStatus = "open"
	DecisionStatusApproved DecisionStatus = "approved"
	DecisionStatusRejected DecisionStatus = "rejected"
	DecisionStatusDeferred DecisionStatus = "deferred"
)

func validDecisionStatus(s DecisionStatus) bool {
	switch s {
	case DecisionStatusOpen, DecisionStatusApproved, DecisionStatusRejected, DecisionStatusDeferred:
		return true
	}
	return false
}

// RiskKind is the category of a risk (§8.8).
type RiskKind string

const (
	RiskClient    RiskKind = "client"
	RiskDelivery  RiskKind = "delivery"
	RiskFinancial RiskKind = "financial"
	RiskPeople    RiskKind = "people"
	RiskSystems   RiskKind = "systems"
)

func validRiskKind(k RiskKind) bool {
	switch k {
	case RiskClient, RiskDelivery, RiskFinancial, RiskPeople, RiskSystems:
		return true
	}
	return false
}

// RiskLikelihood is how likely a risk is to materialize.
type RiskLikelihood string

const (
	LikelihoodLow      RiskLikelihood = "low"
	LikelihoodMedium   RiskLikelihood = "medium"
	LikelihoodHigh     RiskLikelihood = "high"
	LikelihoodCritical RiskLikelihood = "critical"
)

func validRiskLikelihood(l RiskLikelihood) bool {
	switch l {
	case LikelihoodLow, LikelihoodMedium, LikelihoodHigh, LikelihoodCritical:
		return true
	}
	return false
}

// RiskImpact is how severe a risk's impact would be.
type RiskImpact string

const (
	ImpactLow      RiskImpact = "low"
	ImpactMedium   RiskImpact = "medium"
	ImpactHigh     RiskImpact = "high"
	ImpactCritical RiskImpact = "critical"
)

func validRiskImpact(i RiskImpact) bool {
	switch i {
	case ImpactLow, ImpactMedium, ImpactHigh, ImpactCritical:
		return true
	}
	return false
}

// RiskStatus is where a risk is in its life.
type RiskStatus string

const (
	RiskOpen       RiskStatus = "open"
	RiskMonitoring RiskStatus = "monitoring"
	RiskClosed     RiskStatus = "closed"
	RiskMitigated  RiskStatus = "mitigated"
)

func validRiskStatus(s RiskStatus) bool {
	switch s {
	case RiskOpen, RiskMonitoring, RiskClosed, RiskMitigated:
		return true
	}
	return false
}

// BriefStatus is whether a daily brief has been delivered.
type BriefStatus string

const (
	BriefPending   BriefStatus = "pending"
	BriefDelivered BriefStatus = "delivered"
)

func validBriefStatus(s BriefStatus) bool {
	switch s {
	case BriefPending, BriefDelivered:
		return true
	}
	return false
}

// ── lifecycle predicates ────────────────────────────────────────────────────

// DecisionEditable: only an open decision can be changed; once decided it is
// the company's memory and cannot be edited.
func DecisionEditable(o DecisionOutcome) bool { return o == DecisionOpen }

// RiskEditable: only an open or monitoring risk can be edited.
func RiskEditable(s RiskStatus) bool { return s == RiskOpen || s == RiskMonitoring }

// ── entities ────────────────────────────────────────────────────────────────

// Decision is the company's memory of a major decision (§8.8).
type Decision struct {
	ID              uuid.UUID
	Ref             string
	Question        string
	ContextLinks    []string
	Options         []string
	Recommendation  string
	RequestedBy     *uuid.UUID
	RequestedByName string
	Deciders        []uuid.UUID
	DueOn           *time.Time
	Outcome         DecisionOutcome
	Rationale       string
	Version         int
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// Risk is a client, delivery, financial, people or systems risk (§8.8).
type Risk struct {
	ID            uuid.UUID
	Ref           string
	Title         string
	Kind          RiskKind
	OwnerPersonID *uuid.UUID
	OwnerName     string
	Likelihood    RiskLikelihood
	Impact        RiskImpact
	Mitigation    string
	Status        RiskStatus
	Version       int
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Brief is the daily executive brief (§8.8).
type Brief struct {
	ID          uuid.UUID
	Ref         string
	ForDate     time.Time
	Summary     string
	Sections    map[string]any
	Delivered   bool
	DeliveredAt *time.Time
	Version     int
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// MetricSnapshot is a point-in-time KPI figure (§8.8).
type MetricSnapshot struct {
	ID          uuid.UUID
	Ref         string
	MetricName  string
	MetricValue string
	AsOf        time.Time
	Source      string
	Version     int
	CreatedAt   time.Time
}

// ── queries ─────────────────────────────────────────────────────────────────

// DecisionQuery narrows the decision list.
type DecisionQuery struct {
	Outcome *DecisionOutcome
	Decider *uuid.UUID
	From    *time.Time
	To      *time.Time
	Search  string
}

// RiskQuery narrows the risk list.
type RiskQuery struct {
	Kind    *RiskKind
	Status  *RiskStatus
	OwnerID *uuid.UUID
	Search  string
}

// BriefQuery narrows the brief list.
type BriefQuery struct {
	From   *time.Time
	To     *time.Time
	Search string
}

// MetricSnapshotQuery narrows the metric snapshot list.
type MetricSnapshotQuery struct {
	MetricName *string
	From       *time.Time
	To         *time.Time
	Search     string
}

// Cursor is a keyset position, newest first (§6.6).
type Cursor struct {
	At time.Time
	ID uuid.UUID
}

// ── input validation (shared with the write pass) ───────────────────────────

func trim(s string) string { return strings.TrimSpace(s) }

func errInvalidField(field string) error {
	return fmt.Errorf("%w: %s is not valid", ErrInvalid, field)
}
