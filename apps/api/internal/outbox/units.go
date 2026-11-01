package outbox

import (
	"time"

	"github.com/google/uuid"
)

// Units event types (docs/ATLAS.md §7.4, §8.7). A unit's creation and its
// integration/indicator writes are the facts another surface might react to:
// a new unit moves the units board, a breached indicator nudges the dashboard.
//
// Registering a unit, editing an integration or recording a snapshot has no
// consumer outside the module beyond the board and dashboard: those writes are
// audited (§6.6) and read back from Go, which is enough.
const (
	TypeUnitCreatedV1            = "units.unit.created.v1"
	TypeUnitUpdatedV1            = "units.unit.updated.v1"
	TypeIntegrationCreatedV1     = "units.integration.created.v1"
	TypeIntegrationUpdatedV1     = "units.integration.updated.v1"
	TypeIndicatorCreatedV1       = "units.indicator.created.v1"
	TypeIndicatorUpdatedV1       = "units.indicator.updated.v1"
	TypeIndicatorBreachedV1      = "units.indicator.breached.v1"
)

// Aggregate types for units events.
const (
	AggregateIntegration  = "integration"
	AggregateIndicator    = "indicator"
)

// UnitPayload is the body of units.unit.* events.
type UnitPayload struct {
	UnitID         uuid.UUID  `json:"unit_id"`
	Ref            string     `json:"ref"`
	Name           string     `json:"name"`
	Code           string     `json:"code"`
	Kind           string     `json:"kind"`
	Status         string     `json:"status"`
	OwnerPersonID  *uuid.UUID `json:"owner_person_id,omitempty"`
	Version        int        `json:"version"`
	OccurredAt     time.Time  `json:"occurred_at"`
}

// IntegrationPayload is the body of units.integration.* events.
type IntegrationPayload struct {
	IntegrationID  uuid.UUID  `json:"integration_id"`
	UnitID         uuid.UUID  `json:"unit_id"`
	Name           string     `json:"name"`
	Kind           string     `json:"kind"`
	Endpoint       string     `json:"endpoint"`
	CredentialRef  string     `json:"credential_ref"`
	Active         bool       `json:"active"`
	Version        int        `json:"version"`
	OccurredAt     time.Time  `json:"occurred_at"`
}

// IndicatorPayload is the body of units.indicator.* events.
type IndicatorPayload struct {
	IndicatorID    uuid.UUID  `json:"indicator_id"`
	UnitID         uuid.UUID  `json:"unit_id"`
	Name           string     `json:"name"`
	Kind           string     `json:"kind"`
	SourceField    string     `json:"source_field"`
	Target         string     `json:"target"`
	Active         bool       `json:"active"`
	Version        int        `json:"version"`
	OccurredAt     time.Time  `json:"occurred_at"`
}

// BreachPayload is the body of units.indicator.breached.v1.
type BreachPayload struct {
	IndicatorID    uuid.UUID  `json:"indicator_id"`
	UnitID         uuid.UUID  `json:"unit_id"`
	Name           string     `json:"name"`
	Value          string     `json:"value"`
	MetTarget      bool       `json:"met_target"`
	OccurredAt     time.Time  `json:"occurred_at"`
}

// NewUnitCreated builds a units.unit.created.v1 event.
func NewUnitCreated(actorID *uuid.UUID, in UnitPayload) (Event, error) {
	return buildEvent(TypeUnitCreatedV1, AggregateUnit, in.UnitID, actorID, nil, in)
}

// NewUnitUpdated builds a units.unit.updated.v1 event.
func NewUnitUpdated(actorID *uuid.UUID, in UnitPayload) (Event, error) {
	return buildEvent(TypeUnitUpdatedV1, AggregateUnit, in.UnitID, actorID, nil, in)
}

// NewIntegrationCreated builds a units.integration.created.v1 event.
func NewIntegrationCreated(actorID *uuid.UUID, in IntegrationPayload) (Event, error) {
	return buildEvent(TypeIntegrationCreatedV1, AggregateIntegration, in.IntegrationID, actorID, nil, in)
}

// NewIntegrationUpdated builds a units.integration.updated.v1 event.
func NewIntegrationUpdated(actorID *uuid.UUID, in IntegrationPayload) (Event, error) {
	return buildEvent(TypeIntegrationUpdatedV1, AggregateIntegration, in.IntegrationID, actorID, nil, in)
}

// NewIndicatorCreated builds a units.indicator.created.v1 event.
func NewIndicatorCreated(actorID *uuid.UUID, in IndicatorPayload) (Event, error) {
	return buildEvent(TypeIndicatorCreatedV1, AggregateIndicator, in.IndicatorID, actorID, nil, in)
}

// NewIndicatorUpdated builds a units.indicator.updated.v1 event.
func NewIndicatorUpdated(actorID *uuid.UUID, in IndicatorPayload) (Event, error) {
	return buildEvent(TypeIndicatorUpdatedV1, AggregateIndicator, in.IndicatorID, actorID, nil, in)
}

// NewIndicatorBreached builds a units.indicator.breached.v1 event.
func NewIndicatorBreached(actorID *uuid.UUID, in BreachPayload) (Event, error) {
	return buildEvent(TypeIndicatorBreachedV1, AggregateIndicator, in.IndicatorID, actorID, nil, in)
}
