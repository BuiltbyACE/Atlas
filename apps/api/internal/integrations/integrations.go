// Package integrations is the integration hub (§8.9, §11, §12).
//
// The package is the read side of the module: the integration registry, the
// webhook inbox for inbound provider payloads, and the outbound log for
// outbound call attempts. Go is the sole writer (ADR-002).
package integrations

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Sentinel errors. The HTTP layer maps these onto problem responses.
var (
	ErrNotFound  = errors.New("integrations: not found")
	ErrInvalid   = errors.New("integrations: invalid")
	ErrConflict  = errors.New("integrations: conflict")
	ErrForbidden = errors.New("integrations: forbidden")
	ErrDuplicate = errors.New("integrations: duplicate")
)

// Stable problem codes for the write lifecycle. Clients branch on `code`, so
// every refusal the service can raise has one constant here.
const (
	CodeIntegrationNotEditable = "INTEGRATIONS_INTEGRATION_NOT_EDITABLE"
	CodeIntegrationNotFound    = "INTEGRATIONS_INTEGRATION_NOT_FOUND"
	CodeDuplicateRef           = "INTEGRATIONS_DUPLICATE_REF"
	CodeInvalidIntegrationKind = "INTEGRATIONS_INVALID_INTEGRATION_KIND"
	CodeInvalidDirection       = "INTEGRATIONS_INVALID_DIRECTION"
	CodeInvalidStatus          = "INTEGRATIONS_INVALID_STATUS"
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

// IntegrationKind is how Atlas connects to an external system (§8.9).
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

// Direction is the direction of integration traffic (§8.9).
type Direction string

const (
	DirectionIn   Direction = "in"
	DirectionOut  Direction = "out"
	DirectionBoth Direction = "both"
)

func validDirection(d Direction) bool {
	switch d {
	case DirectionIn, DirectionOut, DirectionBoth:
		return true
	}
	return false
}

// IntegrationStatus is the operational status of an integration (§8.9).
type IntegrationStatus string

const (
	IntegrationActive IntegrationStatus = "active"
	IntegrationPaused IntegrationStatus = "paused"
	IntegrationError  IntegrationStatus = "error"
)

func validIntegrationStatus(s IntegrationStatus) bool {
	switch s {
	case IntegrationActive, IntegrationPaused, IntegrationError:
		return true
	}
	return false
}

// ProcessingStatus is the processing state of a webhook inbox entry (§8.9).
type ProcessingStatus string

const (
	ProcessingPending   ProcessingStatus = "pending"
	ProcessingProcessed ProcessingStatus = "processed"
	ProcessingFailed    ProcessingStatus = "failed"
)

func validProcessingStatus(s ProcessingStatus) bool {
	switch s {
	case ProcessingPending, ProcessingProcessed, ProcessingFailed:
		return true
	}
	return false
}

// CircuitState is the circuit breaker state of an outbound log entry (§8.9).
type CircuitState string

const (
	CircuitClosed   CircuitState = "closed"
	CircuitOpen     CircuitState = "open"
	CircuitHalfOpen CircuitState = "half_open"
)

func validCircuitState(s CircuitState) bool {
	switch s {
	case CircuitClosed, CircuitOpen, CircuitHalfOpen:
		return true
	}
	return false
}

// ── lifecycle predicates ────────────────────────────────────────────────────

// IntegrationEditable: an integration in error state cannot be edited.
func IntegrationEditable(s IntegrationStatus) bool { return s != IntegrationError }

// ── entities ────────────────────────────────────────────────────────────────

// Integration is the register entry for a connection to an external system
// (§8.9). Only the credential reference is stored — never the secret itself.
type Integration struct {
	ID            uuid.UUID
	Ref           string
	Name          string
	Kind          IntegrationKind
	Direction     Direction
	OwnerPersonID *uuid.UUID
	OwnerName     string
	CredentialRef string
	Scopes        []string
	Status        IntegrationStatus
	LastSuccess   *time.Time
	ErrorRate     float64
	Version       int
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// WebhookInboxEntry is a raw inbound provider payload awaiting processing
// (§8.9, §11).
type WebhookInboxEntry struct {
	ID               uuid.UUID
	Provider         string
	EventType        string
	DeliveryID       string
	Payload          []byte
	Headers          []byte
	ReceivedAt       time.Time
	ProcessedAt      *time.Time
	ProcessingStatus ProcessingStatus
	ErrorMessage     string
}

// OutboundLogEntry is a recorded outbound call attempt (§8.9).
type OutboundLogEntry struct {
	ID            uuid.UUID
	Provider      string
	Endpoint      string
	Method        string
	StatusCode    *int
	ErrorMessage  string
	Attempt       int
	MaxAttempts   int
	LastAttemptAt time.Time
	CircuitState  CircuitState
	CreatedAt     time.Time
}

// ── queries ─────────────────────────────────────────────────────────────────

// IntegrationQuery narrows the integration registry list.
type IntegrationQuery struct {
	Kind      *IntegrationKind
	Direction *Direction
	Status    *IntegrationStatus
	OwnerID   *uuid.UUID
	Search    string
}

// WebhookInboxQuery narrows the webhook inbox list.
type WebhookInboxQuery struct {
	Provider         *string
	EventType        *string
	DeliveryID       *string
	ProcessingStatus *ProcessingStatus
	From             *time.Time
	To               *time.Time
	Search           string
}

// OutboundLogQuery narrows the outbound log list.
type OutboundLogQuery struct {
	Provider     *string
	Endpoint     *string
	CircuitState *CircuitState
	From         *time.Time
	To           *time.Time
	Search       string
}

// Cursor is a keyset position, newest first (§6.6).
type Cursor struct {
	At time.Time
	ID uuid.UUID
}

// Outcome is what one delivery accomplished.
type Outcome struct {
	Status     string
	Refs       []string
	TaskID     uuid.UUID
	EvidenceID uuid.UUID
}

// Outcome statuses.
const (
	// StatusRecorded: a merged PR was linked and the event written.
	StatusRecorded = "recorded"
	// StatusDuplicate: the delivery id was already processed.
	StatusDuplicate = "duplicate"
	// StatusIgnored: the delivery was not a PR we act on (opened, unmerged).
	StatusIgnored = "ignored"
	// StatusUnlinked: a merged PR named a ref that resolved to no task.
	StatusUnlinked = "unlinked"
)

// ── input validation (shared with the write pass) ───────────────────────────

func trim(s string) string { return strings.TrimSpace(s) }

func errInvalidField(field string) error {
	return fmt.Errorf("%w: %s is not valid", ErrInvalid, field)
}

// PullRequest is the subset of a GitHub pull_request payload Atlas acts on.
type PullRequest struct {
	Action     string
	Merged     bool
	Repository string
	Number     int
	Title      string
	URL        string
	CommitSHA  string
	HeadRef    string
	BaseRef    string
	Author     string
	MergedAt   *time.Time
}

// validRef is a light shape check: lowercase letters, digits and dashes,
// at least one character, no spaces. A ref is the stable handle of an
// integration.
func validRef(ref string) bool {
	if ref == "" || strings.ContainsAny(ref, " \t/") || strings.Contains(ref, "://") {
		return false
	}
	for _, r := range ref {
		if !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') && r != '-' {
			return false
		}
	}
	return true
}
