// Package units is business-unit oversight (§8.7).
//
// The package is the read side of the module: business units, their
// integrations with systems of record, indicator definitions and snapshots,
// and finance summaries. The school management system remains the system of
// record; Atlas stores aggregates and indicators, not student-level personal
// data, unless a specific, approved need exists.
//
// Go is the sole writer (ADR-002). Each unit has its own scope: unit managers
// see their unit; CEOs see all. Sync failures raise units.sync.failed.v1 and
// alert the integration owner; indicator breaches raise
// units.indicator.breached.v1 to the unit manager and CEOs (§8.7).
package units

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Sentinel errors. The HTTP layer maps these onto problem responses.
var (
	ErrNotFound  = errors.New("units: not found")
	ErrInvalid   = errors.New("units: invalid")
	ErrConflict  = errors.New("units: conflict")
	ErrForbidden = errors.New("units: forbidden")
	ErrDuplicate = errors.New("units: duplicate")
)

// Stable problem codes for the write lifecycle. Clients branch on `code`, so
// every refusal the service can raise has one constant here.
const (
	CodeUnitNotEditable           = "UNITS_UNIT_NOT_EDITABLE"
	CodeUnitRetired               = "UNITS_UNIT_RETIRED"
	CodeUnitHasActiveIntegrations = "UNITS_UNIT_HAS_ACTIVE_INTEGRATIONS"
	CodeIntegrationNotEditable    = "UNITS_INTEGRATION_NOT_EDITABLE"
	CodeIndicatorNotEditable      = "UNITS_INDICATOR_NOT_EDITABLE"
	CodeIndicatorBreached         = "UNITS_INDICATOR_BREACHED"
	CodeSyncFailed                = "UNITS_SYNC_FAILED"
	CodeDuplicateCode             = "UNITS_DUPLICATE_CODE"
	CodeDuplicateName             = "UNITS_DUPLICATE_NAME"
	CodeInvalidUnitKind           = "UNITS_INVALID_UNIT_KIND"
	CodeInvalidIntegrationKind    = "UNITS_INVALID_INTEGRATION_KIND"
	CodeInvalidIndicatorKind      = "UNITS_INVALID_INDICATOR_KIND"
	CodeInvalidEndpoint           = "UNITS_INVALID_ENDPOINT"
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

// UnitKind is what a unit is (§8.7).
type UnitKind string

const (
	UnitSchool     UnitKind = "school"
	UnitDepartment UnitKind = "department"
	UnitProject    UnitKind = "project"
	UnitRegion     UnitKind = "region"
)

func validUnitKind(k UnitKind) bool {
	switch k {
	case UnitSchool, UnitDepartment, UnitProject, UnitRegion:
		return true
	}
	return false
}

// UnitStatus is whether a unit is active or retired.
type UnitStatus string

const (
	UnitActive  UnitStatus = "active"
	UnitRetired UnitStatus = "retired"
)

func validUnitStatus(s UnitStatus) bool { return s == UnitActive || s == UnitRetired }

// IntegrationKind is how Atlas connects to a unit's system of record (§8.7).
type IntegrationKind string

const (
	IntegrationAPI     IntegrationKind = "api"
	IntegrationWebhook IntegrationKind = "webhook"
	IntegrationFile    IntegrationKind = "file"
	IntegrationSFTP    IntegrationKind = "sftp"
)

func validIntegrationKind(k IntegrationKind) bool {
	switch k {
	case IntegrationAPI, IntegrationWebhook, IntegrationFile, IntegrationSFTP:
		return true
	}
	return false
}

// IntegrationStatus is the last known sync status.
type IntegrationStatus string

const (
	IntegrationOK     IntegrationStatus = "ok"
	IntegrationFailed IntegrationStatus = "failed"
	IntegrationNever  IntegrationStatus = "never"
)

func validIntegrationStatus(s IntegrationStatus) bool {
	return s == IntegrationOK || s == IntegrationFailed || s == IntegrationNever
}

// IndicatorKind is how an indicator's value is derived (§8.7).
type IndicatorKind string

const (
	IndicatorCount   IndicatorKind = "count"
	IndicatorRatio   IndicatorKind = "ratio"
	IndicatorAmount  IndicatorKind = "amount"
	IndicatorBoolean IndicatorKind = "boolean"
)

func validIndicatorKind(k IndicatorKind) bool {
	switch k {
	case IndicatorCount, IndicatorRatio, IndicatorAmount, IndicatorBoolean:
		return true
	}
	return false
}

// ── lifecycle predicates ────────────────────────────────────────────────────

// UnitEditable: a retired unit cannot be edited.
func UnitEditable(s UnitStatus) bool { return s == UnitActive }

// IntegrationEditable: an integration can be edited while active.
func IntegrationEditable(s IntegrationStatus) bool { return s == IntegrationOK }

// ── entities ────────────────────────────────────────────────────────────────

// BusinessUnit is one business unit (§8.7). The school is the first unit.
type BusinessUnit struct {
	ID            uuid.UUID
	Ref           string
	Name          string
	Code          string
	Kind          UnitKind
	OwnerPersonID *uuid.UUID
	OwnerName     string
	Description   string
	Status        UnitStatus
	Version       int
	CreatedAt     time.Time
	UpdatedAt     time.Time

	// Derived counts, computed per read.
	IntegrationCount   int
	ActiveIntegrations int
	IndicatorCount     int
}

// UnitIntegration is how Atlas connects to a unit's system of record (§8.7).
// Only the credential reference is stored — never the secret itself.
type UnitIntegration struct {
	ID             uuid.UUID
	UnitID         uuid.UUID
	UnitName       string
	Name           string
	Kind           IntegrationKind
	Endpoint       string
	CredentialRef  string
	Schedule       string
	LastSyncAt     *time.Time
	LastSyncStatus IntegrationStatus
	Active         bool
	Version        int
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// UnitIndicatorDefinition is what we measure for each unit (§8.7): name,
// formula/source field, target, thresholds.
type UnitIndicatorDefinition struct {
	ID                uuid.UUID
	UnitID            uuid.UUID
	UnitName          string
	Name              string
	Kind              IndicatorKind
	SourceField       string
	Formula           string
	Target            string
	WarningThreshold  string
	CriticalThreshold string
	Active            bool
	Version           int
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// UnitSnapshot is one indicator's value for one unit on one date (§8.7).
type UnitSnapshot struct {
	ID            uuid.UUID
	UnitID        uuid.UUID
	UnitName      string
	OnDate        time.Time
	IndicatorID   uuid.UUID
	IndicatorName string
	Value         string
	MetTarget     bool
	Notes         string
	CreatedAt     time.Time
}

// UnitFinanceSummary is a period roll-up of income, expenses, receivables
// (§8.7). Money is integer minor units (ADR-010).
type UnitFinanceSummary struct {
	ID               uuid.UUID
	UnitID           uuid.UUID
	UnitName         string
	PeriodStart      time.Time
	PeriodEnd        time.Time
	IncomeMinor      int64
	ExpensesMinor    int64
	ReceivablesMinor int64
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// ── queries ─────────────────────────────────────────────────────────────────

// UnitQuery narrows the business unit list.
type UnitQuery struct {
	Kind    *UnitKind
	Status  *UnitStatus
	OwnerID *uuid.UUID
	Search  string
	Active  *bool
}

// IntegrationQuery narrows the integration list.
type IntegrationQuery struct {
	UnitID *uuid.UUID
	Kind   *IntegrationKind
	Status *IntegrationStatus
	Active *bool
	Search string
}

// IndicatorDefinitionQuery narrows the indicator definition list.
type IndicatorDefinitionQuery struct {
	UnitID *uuid.UUID
	Kind   *IndicatorKind
	Active *bool
	Search string
}

// SnapshotQuery narrows the snapshot list.
type SnapshotQuery struct {
	UnitID      *uuid.UUID
	IndicatorID *uuid.UUID
	From        *time.Time
	To          *time.Time
	Search      string
}

// FinanceSummaryQuery narrows the finance summary list.
type FinanceSummaryQuery struct {
	UnitID *uuid.UUID
	From   *time.Time
	To     *time.Time
	Search string
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

// validUnitCode is a light shape check: lowercase letters, digits and dashes,
// at least one character, no spaces. A unit's code is its stable handle.
func validUnitCode(code string) bool {
	if code == "" || strings.ContainsAny(code, " \t/") || strings.Contains(code, "://") {
		return false
	}
	for _, r := range code {
		if !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') && r != '-' {
			return false
		}
	}
	return true
}

// validEndpoint is a light shape check: an absolute http(s) URL. An empty
// endpoint is valid for file/sftp integrations whose target is elsewhere.
func validEndpoint(raw string) bool {
	if raw == "" {
		return true
	}
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	return (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}
