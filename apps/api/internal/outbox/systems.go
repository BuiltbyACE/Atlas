package outbox

import (
	"time"

	"github.com/google/uuid"
)

// Systems event types (docs/ATLAS.md §7.4, §8.6). An incident's life is the one
// part of the module another surface reacts to: SEV1/SEV2 open a channel, page
// on-call and force a review, so the orchestrator needs to hear every step.
//
// Registering a system, editing a check or recording a credential has no
// consumer outside the module: those writes are audited (§6.6) and read back
// from Go, which is enough. Events exist where someone else must react.
const (
	TypeIncidentOpenedV1         = "systems.incident.opened.v1"
	TypeIncidentAcknowledgedV1   = "systems.incident.acknowledged.v1"
	TypeIncidentMitigatedV1      = "systems.incident.mitigated.v1"
	TypeIncidentResolvedV1       = "systems.incident.resolved.v1"
	TypeIncidentClosedV1         = "systems.incident.closed.v1"
	TypeIncidentReviewRecordedV1 = "systems.incident.review_recorded.v1"
)

// Aggregate types for systems events.
const (
	AggregateIncident = "incident"
)

// IncidentPayload is the body of every systems.incident.* event. It carries the
// identifiers and the state a consumer needs to route a page or invalidate a
// view — never the narrative fields (client impact, root cause), which the
// reader fetches from Go under permission (§7.5, ADR-009).
type IncidentPayload struct {
	IncidentID        uuid.UUID   `json:"incident_id"`
	Ref               string      `json:"ref"`
	Severity          string      `json:"severity"`
	Status            string      `json:"status"`
	SystemIDs         []uuid.UUID `json:"system_ids"`
	CommanderPersonID *uuid.UUID  `json:"commander_person_id,omitempty"`
	DetectedAt        time.Time   `json:"detected_at"`
	ResolvedAt        *time.Time  `json:"resolved_at,omitempty"`
	ReviewRequired    bool        `json:"review_required"`
	ActorID           *uuid.UUID  `json:"actor_id,omitempty"`
	OccurredAt        time.Time   `json:"occurred_at"`
}

// IncidentRef is the minimal incident view the builders need.
type IncidentRef struct {
	ID                uuid.UUID
	Ref               string
	Severity          string
	Status            string
	SystemIDs         []uuid.UUID
	CommanderPersonID *uuid.UUID
	DetectedAt        time.Time
	ResolvedAt        *time.Time
	ReviewRequired    bool
}

// NewIncidentOpened builds a systems.incident.opened.v1 event.
func NewIncidentOpened(actorID *uuid.UUID, in IncidentRef) (Event, error) {
	return incidentEvent(TypeIncidentOpenedV1, actorID, in)
}

// NewIncidentAcknowledged builds a systems.incident.acknowledged.v1 event: the
// moment the MTTA clock stops.
func NewIncidentAcknowledged(actorID *uuid.UUID, in IncidentRef) (Event, error) {
	return incidentEvent(TypeIncidentAcknowledgedV1, actorID, in)
}

// NewIncidentMitigated builds a systems.incident.mitigated.v1 event: impact
// contained, incident still running.
func NewIncidentMitigated(actorID *uuid.UUID, in IncidentRef) (Event, error) {
	return incidentEvent(TypeIncidentMitigatedV1, actorID, in)
}

// NewIncidentResolved builds a systems.incident.resolved.v1 event: the MTTR
// clock stops and, for SEV1/SEV2, the review becomes owed.
func NewIncidentResolved(actorID *uuid.UUID, in IncidentRef) (Event, error) {
	return incidentEvent(TypeIncidentResolvedV1, actorID, in)
}

// NewIncidentClosed builds a systems.incident.closed.v1 event.
func NewIncidentClosed(actorID *uuid.UUID, in IncidentRef) (Event, error) {
	return incidentEvent(TypeIncidentClosedV1, actorID, in)
}

// NewIncidentReviewRecorded builds a systems.incident.review_recorded.v1 event:
// the root cause and summary are on the record and the review is no longer owed.
func NewIncidentReviewRecorded(actorID *uuid.UUID, in IncidentRef) (Event, error) {
	return incidentEvent(TypeIncidentReviewRecordedV1, actorID, in)
}

func incidentEvent(eventType string, actorID *uuid.UUID, in IncidentRef) (Event, error) {
	systemIDs := in.SystemIDs
	if systemIDs == nil {
		systemIDs = []uuid.UUID{}
	}
	return buildEvent(eventType, AggregateIncident, in.ID, actorID, nil, IncidentPayload{
		IncidentID:        in.ID,
		Ref:               in.Ref,
		Severity:          in.Severity,
		Status:            in.Status,
		SystemIDs:         systemIDs,
		CommanderPersonID: in.CommanderPersonID,
		DetectedAt:        in.DetectedAt,
		ResolvedAt:        in.ResolvedAt,
		ReviewRequired:    in.ReviewRequired,
		ActorID:           actorID,
		OccurredAt:        time.Now().UTC(),
	})
}
