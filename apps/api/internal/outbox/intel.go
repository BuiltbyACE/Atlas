package outbox

import (
	"time"

	"github.com/google/uuid"
)

// Intel event types (docs/ATLAS.md §7.4, §8.8). A decision's life — open,
// decided — and a delivered brief are the facts another surface reacts to:
// a new decision or risk moves the intel board, a delivered brief nudges
// the dashboard.
//
// Creating or editing decisions, risks and briefs, and recording metric
// snapshots are audited (§6.6) and read back from Go. Events exist where
// another surface must react.
const (
	TypeDecisionCreatedV1      = "intel.decision.created.v1"
	TypeDecisionUpdatedV1      = "intel.decision.updated.v1"
	TypeDecisionDecidedV1      = "intel.decision.decided.v1"
	TypeRiskCreatedV1          = "intel.risk.created.v1"
	TypeRiskUpdatedV1          = "intel.risk.updated.v1"
	TypeBriefCreatedV1         = "intel.brief.created.v1"
	TypeBriefUpdatedV1         = "intel.brief.updated.v1"
	TypeBriefDeliveredV1       = "intel.brief.delivered.v1"
	TypeMetricSnapshotCreatedV1 = "intel.metric_snapshot.created.v1"
)

// Aggregate types for intel events.
const (
	AggregateDecision   = "decision"
	AggregateRisk       = "risk"
	AggregateBrief      = "brief"
	AggregateMetric     = "metric"
)

// DecisionPayload is the body of intel.decision.* events.
type DecisionPayload struct {
	DecisionID     uuid.UUID   `json:"decision_id"`
	Ref            string      `json:"ref"`
	Question       string      `json:"question"`
	Outcome        string      `json:"outcome"`
	RequestedBy    *uuid.UUID  `json:"requested_by,omitempty"`
	Deciders       []uuid.UUID `json:"deciders"`
	DueOn          *time.Time  `json:"due_on,omitempty"`
	Version        int         `json:"version"`
	OccurredAt     time.Time   `json:"occurred_at"`
}

// RiskPayload is the body of intel.risk.* events.
type RiskPayload struct {
	RiskID         uuid.UUID  `json:"risk_id"`
	Ref            string     `json:"ref"`
	Title          string     `json:"title"`
	Kind           string     `json:"kind"`
	Likelihood     string     `json:"likelihood"`
	Impact         string     `json:"impact"`
	Status         string     `json:"status"`
	OwnerPersonID  *uuid.UUID `json:"owner_person_id,omitempty"`
	Version        int        `json:"version"`
	OccurredAt     time.Time  `json:"occurred_at"`
}

// BriefPayload is the body of intel.brief.* events.
type BriefPayload struct {
	BriefID       uuid.UUID  `json:"brief_id"`
	Ref           string     `json:"ref"`
	ForDate       time.Time  `json:"for_date"`
	Summary       string     `json:"summary"`
	Delivered     bool       `json:"delivered"`
	Version       int        `json:"version"`
	OccurredAt    time.Time  `json:"occurred_at"`
}

// MetricSnapshotPayload is the body of intel.metric_snapshot.created.v1.
type MetricSnapshotPayload struct {
	SnapshotID   uuid.UUID  `json:"snapshot_id"`
	Ref          string     `json:"ref"`
	MetricName   string     `json:"metric_name"`
	MetricValue  string     `json:"metric_value"`
	AsOf         time.Time  `json:"as_of"`
	Source       string     `json:"source"`
	Version      int        `json:"version"`
	OccurredAt   time.Time  `json:"occurred_at"`
}

// NewDecisionCreated builds a intel.decision.created.v1 event.
func NewDecisionCreated(actorID *uuid.UUID, in DecisionPayload) (Event, error) {
	return buildEvent(TypeDecisionCreatedV1, AggregateDecision, in.DecisionID, actorID, nil, in)
}

// NewDecisionUpdated builds a intel.decision.updated.v1 event.
func NewDecisionUpdated(actorID *uuid.UUID, in DecisionPayload) (Event, error) {
	return buildEvent(TypeDecisionUpdatedV1, AggregateDecision, in.DecisionID, actorID, nil, in)
}

// NewDecisionDecided builds a intel.decision.decided.v1 event.
func NewDecisionDecided(actorID *uuid.UUID, in DecisionPayload) (Event, error) {
	return buildEvent(TypeDecisionDecidedV1, AggregateDecision, in.DecisionID, actorID, nil, in)
}

// NewRiskCreated builds a intel.risk.created.v1 event.
func NewRiskCreated(actorID *uuid.UUID, in RiskPayload) (Event, error) {
	return buildEvent(TypeRiskCreatedV1, AggregateRisk, in.RiskID, actorID, nil, in)
}

// NewRiskUpdated builds a intel.risk.updated.v1 event.
func NewRiskUpdated(actorID *uuid.UUID, in RiskPayload) (Event, error) {
	return buildEvent(TypeRiskUpdatedV1, AggregateRisk, in.RiskID, actorID, nil, in)
}

// NewBriefCreated builds a intel.brief.created.v1 event.
func NewBriefCreated(actorID *uuid.UUID, in BriefPayload) (Event, error) {
	return buildEvent(TypeBriefCreatedV1, AggregateBrief, in.BriefID, actorID, nil, in)
}

// NewBriefUpdated builds a intel.brief.updated.v1 event.
func NewBriefUpdated(actorID *uuid.UUID, in BriefPayload) (Event, error) {
	return buildEvent(TypeBriefUpdatedV1, AggregateBrief, in.BriefID, actorID, nil, in)
}

// NewBriefDelivered builds a intel.brief.delivered.v1 event.
func NewBriefDelivered(actorID *uuid.UUID, in BriefPayload) (Event, error) {
	return buildEvent(TypeBriefDeliveredV1, AggregateBrief, in.BriefID, actorID, nil, in)
}

// NewMetricSnapshotCreated builds a intel.metric_snapshot.created.v1 event.
func NewMetricSnapshotCreated(actorID *uuid.UUID, in MetricSnapshotPayload) (Event, error) {
	return buildEvent(TypeMetricSnapshotCreatedV1, AggregateMetric, in.SnapshotID, actorID, nil, in)
}
