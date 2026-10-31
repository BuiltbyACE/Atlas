// Package systems is the register of everything SafariStack runs and its health
// (docs/ATLAS.md §8.6).
//
// The package is the read side of the module: the register of systems, the
// checks watching them, the incidents they have had, and the domains,
// certificates and credentials that expire. The monitoring probes themselves
// live in atlas-worker; this package reads what they wrote.
//
// Go is the sole writer (ADR-002) and the reader never trusts a stored status
// it can derive — a check's current health is its latest result, not a column.
package systems

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
	ErrNotFound  = errors.New("systems: not found")
	ErrInvalid   = errors.New("systems: invalid")
	ErrConflict  = errors.New("systems: conflict")
	ErrForbidden = errors.New("systems: forbidden")
	ErrDuplicate = errors.New("systems: duplicate")
)

// Stable problem codes for the write lifecycle. Clients branch on `code`, so
// every refusal the service can raise has one constant here.
const (
	CodeSystemNotEditable           = "SYSTEMS_SYSTEM_NOT_EDITABLE"
	CodeSystemRetired               = "SYSTEMS_SYSTEM_RETIRED"
	CodeSystemHasOpenIncidents      = "SYSTEMS_SYSTEM_HAS_OPEN_INCIDENTS"
	CodeCheckNotEditable            = "SYSTEMS_CHECK_NOT_EDITABLE"
	CodeCheckExpectRequired         = "SYSTEMS_CHECK_EXPECT_REQUIRED"
	CodeCheckHeartbeatWindowMissing = "SYSTEMS_CHECK_HEARTBEAT_WINDOW_REQUIRED"
	CodeCheckThresholdsInvalid      = "SYSTEMS_CHECK_THRESHOLDS_INVALID"
	CodeIncidentNotAckable          = "SYSTEMS_INCIDENT_NOT_ACKNOWLEDGEABLE"
	CodeIncidentNotMitigable        = "SYSTEMS_INCIDENT_NOT_MITIGABLE"
	CodeIncidentNotResolvable       = "SYSTEMS_INCIDENT_NOT_RESOLVABLE"
	CodeIncidentNotClosable         = "SYSTEMS_INCIDENT_NOT_CLOSABLE"
	CodeIncidentNotReviewable       = "SYSTEMS_INCIDENT_NOT_REVIEWABLE"
	CodeIncidentSystemsRequired     = "SYSTEMS_INCIDENT_SYSTEMS_REQUIRED"
	CodeDetectedInFuture            = "SYSTEMS_DETECTED_IN_FUTURE"
	CodeReviewRequired              = "SYSTEMS_REVIEW_REQUIRED"
	CodeReasonRequired              = "SYSTEMS_REASON_REQUIRED"
	CodeRootCauseRequired           = "SYSTEMS_ROOT_CAUSE_REQUIRED"
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

// withCode attaches a stable problem code to err. Non-sentinel errors pass
// through untouched so wrapping never hides an unexpected failure.
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

// SystemType is what a system is (§8.6).
type SystemType string

const (
	TypeWebsite       SystemType = "website"
	TypeWebApp        SystemType = "web_app"
	TypeAPI           SystemType = "api"
	TypeMobileBackend SystemType = "mobile_backend"
	TypeIntegration   SystemType = "integration"
	TypeBackgroundJob SystemType = "background_job"
	TypeDatabase      SystemType = "database"
)

func validSystemType(t SystemType) bool {
	switch t {
	case TypeWebsite, TypeWebApp, TypeAPI, TypeMobileBackend, TypeIntegration, TypeBackgroundJob, TypeDatabase:
		return true
	}
	return false
}

// Environment is where a system runs.
type Environment string

const (
	EnvProd    Environment = "prod"
	EnvStaging Environment = "staging"
)

func validEnvironment(e Environment) bool { return e == EnvProd || e == EnvStaging }

// Criticality is how much a system matters: tier 1 is business-critical, tier 3
// is a convenience. It decides how loudly an outage is escalated (§8.6).
type Criticality string

const (
	Tier1 Criticality = "tier1"
	Tier2 Criticality = "tier2"
	Tier3 Criticality = "tier3"
)

func validCriticality(c Criticality) bool {
	return c == Tier1 || c == Tier2 || c == Tier3
}

// SystemStatus retires a system without deleting its incident history.
type SystemStatus string

const (
	SystemActive  SystemStatus = "active"
	SystemRetired SystemStatus = "retired"
)

func validSystemStatus(s SystemStatus) bool { return s == SystemActive || s == SystemRetired }

// CheckType is the kind of probe (§8.6). Readiness and content checks are
// first-class because a 200 OK on a broken page is not health.
type CheckType string

const (
	CheckHTTPStatus  CheckType = "http_status"
	CheckHTTPContent CheckType = "http_content"
	CheckTCP         CheckType = "tcp"
	CheckTLSExpiry   CheckType = "tls_expiry"
	CheckDNSExpiry   CheckType = "dns_expiry"
	CheckHeartbeat   CheckType = "heartbeat"
	CheckLatency     CheckType = "latency"
	CheckDependency  CheckType = "dependency"
)

func validCheckType(t CheckType) bool {
	switch t {
	case CheckHTTPStatus, CheckHTTPContent, CheckTCP, CheckTLSExpiry, CheckDNSExpiry,
		CheckHeartbeat, CheckLatency, CheckDependency:
		return true
	}
	return false
}

// CheckStatus is the outcome of one probe. `warn` is a degradation — slow but
// answering, or a threshold crossed — and does not open an incident on its own.
type CheckStatus string

const (
	CheckPass CheckStatus = "pass"
	CheckWarn CheckStatus = "warn"
	CheckFail CheckStatus = "fail"
)

func validCheckStatus(s CheckStatus) bool {
	return s == CheckPass || s == CheckWarn || s == CheckFail
}

// CheckSource is who observed the result: our probe, a heartbeat the job sent,
// or a person reporting it.
type CheckSource string

const (
	SourceAtlas          CheckSource = "atlas"
	SourceHeartbeat      CheckSource = "heartbeat"
	SourceClientReported CheckSource = "client_reported"
)

func validCheckSource(s CheckSource) bool {
	return s == SourceAtlas || s == SourceHeartbeat || s == SourceClientReported
}

// IncidentSeverity is how bad an incident is (§8.6). SEV1/SEV2 open an incident
// channel, page on-call and force a review on resolve.
type IncidentSeverity string

const (
	Sev1 IncidentSeverity = "sev1"
	Sev2 IncidentSeverity = "sev2"
	Sev3 IncidentSeverity = "sev3"
	Sev4 IncidentSeverity = "sev4"
)

func validIncidentSeverity(s IncidentSeverity) bool {
	return s == Sev1 || s == Sev2 || s == Sev3 || s == Sev4
}

// RequiresReview reports whether resolving at this severity forces a
// post-incident review with action items that become tasks (§8.6).
func (s IncidentSeverity) RequiresReview() bool { return s == Sev1 || s == Sev2 }

// IncidentStatus is where an incident is in its life.
type IncidentStatus string

const (
	IncidentOpen         IncidentStatus = "open"
	IncidentAcknowledged IncidentStatus = "acknowledged"
	IncidentMitigated    IncidentStatus = "mitigated"
	IncidentResolved     IncidentStatus = "resolved"
	IncidentClosed       IncidentStatus = "closed"
)

func validIncidentStatus(s IncidentStatus) bool {
	switch s {
	case IncidentOpen, IncidentAcknowledged, IncidentMitigated, IncidentResolved, IncidentClosed:
		return true
	}
	return false
}

// Open reports whether an incident is still running.
func (s IncidentStatus) Open() bool { return s != IncidentResolved && s != IncidentClosed }

// IncidentSource is how the incident was detected. Atlas detecting first is the
// target (§8.6), so the source is recorded and measured.
type IncidentSource string

const (
	DetectedByAtlas  IncidentSource = "atlas"
	DetectedByClient IncidentSource = "client_reported"
	DetectedByManual IncidentSource = "manual"
)

func validIncidentSource(s IncidentSource) bool {
	return s == DetectedByAtlas || s == DetectedByClient || s == DetectedByManual
}

// TimelineEntryType is what a timeline entry records.
type TimelineEntryType string

const (
	EntryDetected     TimelineEntryType = "detected"
	EntryAcknowledged TimelineEntryType = "acknowledged"
	EntryNote         TimelineEntryType = "note"
	EntryMitigated    TimelineEntryType = "mitigated"
	EntryResolved     TimelineEntryType = "resolved"
	EntryClosed       TimelineEntryType = "closed"
	EntryAction       TimelineEntryType = "action"
)

func validTimelineEntry(t TimelineEntryType) bool {
	switch t {
	case EntryDetected, EntryAcknowledged, EntryNote, EntryMitigated, EntryResolved, EntryClosed, EntryAction:
		return true
	}
	return false
}

// ActionStatus tracks a follow-up action from an incident review.
type ActionStatus string

const (
	ActionTodo    ActionStatus = "todo"
	ActionDoing   ActionStatus = "doing"
	ActionDone    ActionStatus = "done"
	ActionDropped ActionStatus = "dropped"
)

func validActionStatus(s ActionStatus) bool {
	switch s {
	case ActionTodo, ActionDoing, ActionDone, ActionDropped:
		return true
	}
	return false
}

// ── lifecycle predicates ────────────────────────────────────────────────────
//
// Each transition has one predicate. A refusal names the transition the caller
// attempted rather than silently doing nothing, and carries a stable code.

// CheckEditable: a retired system's checks are frozen with it.
func CheckEditable(s SystemStatus) bool { return s == SystemActive }

// IncidentAcknowledgeable: only an open incident can be acknowledged, and it is
// the acknowledgement that starts the MTTA clock's stop.
func IncidentAcknowledgeable(s IncidentStatus) bool { return s == IncidentOpen }

// IncidentMitigable: the impact is contained but the incident is still running.
func IncidentMitigable(s IncidentStatus) bool {
	return s == IncidentOpen || s == IncidentAcknowledged
}

// IncidentResolvable: a running incident can be resolved from any open state.
func IncidentResolvable(s IncidentStatus) bool { return s.Open() }

// IncidentClosable: only a resolved incident is closed, so the review cannot be
// skipped by closing straight from open.
func IncidentClosable(s IncidentStatus) bool { return s == IncidentResolved }

// ExpiryWarningDays are the days-out warnings for domains and certificates
// (§8.6): 30, 14 and 7. They are a package constant rather than a setting so a
// warning can never be configured away.
var ExpiryWarningDays = []int{30, 14, 7}

// ── entities ────────────────────────────────────────────────────────────────

// System is one registered system (§8.6).
type System struct {
	ID               uuid.UUID
	Ref              string
	Name             string
	SystemType       SystemType
	OwnerPersonID    *uuid.UUID
	OwnerName        string
	OrganizationID   *uuid.UUID
	OrganizationName string
	Environment      Environment
	Criticality      Criticality
	ContractID       *uuid.UUID
	ContractRef      string
	RunbookURL       string
	RepositoryURL    string
	Description      string
	Status           SystemStatus
	Version          int
	CreatedAt        time.Time
	UpdatedAt        time.Time

	// Derived, computed per read: the health a person actually wants at a
	// glance, never a stored counter that can drift.
	ChecksTotal    int
	ChecksFailing  int
	ChecksWarning  int
	OpenIncidents  int
	AvailabilityBP *int // basis points over AvailabilityWindow; nil when unprobed
}

// AvailabilityWindow is how far back a system's availability figure looks.
const AvailabilityWindow = 30 * 24 * time.Hour

// Check is one probe against a system (§8.6).
type Check struct {
	ID                     uuid.UUID
	SystemID               uuid.UUID
	SystemName             string
	SystemCriticality      Criticality
	Name                   string
	CheckType              CheckType
	Target                 string
	Expect                 string
	IntervalSeconds        int
	TimeoutSeconds         int
	Regions                []string
	WarnMS                 *int
	FailMS                 *int
	FailureThreshold       int
	HeartbeatWindowSeconds *int
	Active                 bool
	Version                int
	CreatedAt              time.Time
	UpdatedAt              time.Time

	// Derived: the latest result, and the run of consecutive failures that
	// decides whether an incident opens (flap protection, §8.6).
	LastStatus          *CheckStatus
	LastAt              *time.Time
	LastLatencyMS       *int
	ConsecutiveFailures int
}

// Unhealthy reports whether a check is failing or warning right now.
func (c Check) Unhealthy() bool {
	return c.LastStatus != nil && *c.LastStatus != CheckPass
}

// CheckResult is the outcome of one probe (§8.6).
type CheckResult struct {
	ID        uuid.UUID
	CheckID   uuid.UUID
	At        time.Time
	Status    CheckStatus
	LatencyMS *int
	Detail    string
	Source    CheckSource
	Region    string
}

// Incident is an outage or degradation (§8.6).
type Incident struct {
	ID                uuid.UUID
	Ref               string
	Title             string
	Severity          IncidentSeverity
	Status            IncidentStatus
	DetectedAt        time.Time
	DetectedSource    IncidentSource
	AcknowledgedAt    *time.Time
	MitigatedAt       *time.Time
	ResolvedAt        *time.Time
	ClosedAt          *time.Time
	CommanderPersonID *uuid.UUID
	CommanderName     string
	ClientImpact      string
	RootCause         string
	Summary           string
	ReviewRequired    bool
	ReviewAt          *time.Time
	Version           int
	CreatedAt         time.Time
	UpdatedAt         time.Time

	// SystemNames is the list projection: the names of the systems an incident
	// touched, so a row can say what it was about without a second request.
	SystemNames []string

	// Loaded on the detail read, not the list.
	Systems  []IncidentSystem
	Timeline []TimelineEntry
	Actions  []IncidentAction
}

// IncidentSystem is a system an incident touched, with the detail needed to
// render it without a second lookup.
type IncidentSystem struct {
	SystemID    uuid.UUID
	Ref         string
	Name        string
	Criticality Criticality
}

// TimelineEntry is one line of an incident's memory (§8.6).
type TimelineEntry struct {
	ID            uuid.UUID
	IncidentID    uuid.UUID
	At            time.Time
	ActorPersonID *uuid.UUID
	ActorName     string
	EntryType     TimelineEntryType
	Note          string
}

// IncidentAction is a follow-up action; it becomes a work task (§8.6).
type IncidentAction struct {
	ID            uuid.UUID
	IncidentID    uuid.UUID
	Title         string
	OwnerPersonID *uuid.UUID
	OwnerName     string
	DueOn         *time.Time
	Status        ActionStatus
	TaskID        *uuid.UUID
	Version       int
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// CredentialRecord says a credential exists and where it is kept — never the
// secret itself (§8.6).
type CredentialRecord struct {
	ID             uuid.UUID
	SystemID       uuid.UUID
	Label          string
	CredentialKind string
	StorePath      string
	OwnerPersonID  *uuid.UUID
	OwnerName      string
	RotatedOn      *time.Time
	NextRotationOn *time.Time
	Notes          string
	Active         bool
	Version        int
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// Domain is a domain name and when its registration lapses (§8.6). ExpiresOn is
// nullable because a domain can be recorded before its renewal date is known;
// such a domain simply never appears in the expiry warning.
type Domain struct {
	ID            uuid.UUID
	Name          string
	SystemID      *uuid.UUID
	SystemName    string
	Registrar     string
	RegisteredOn  *time.Time
	ExpiresOn     *time.Time
	AutoRenew     bool
	OwnerPersonID *uuid.UUID
	OwnerName     string
	Notes         string
	Version       int
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Certificate is a TLS certificate and when it expires (§8.6). ExpiresOn is
// required: a certificate record with no expiry cannot warn anyone.
type Certificate struct {
	ID            uuid.UUID
	Hostname      string
	SystemID      *uuid.UUID
	SystemName    string
	Issuer        string
	IssuedOn      *time.Time
	ExpiresOn     time.Time
	AutoRenew     bool
	OwnerPersonID *uuid.UUID
	OwnerName     string
	Notes         string
	Version       int
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// ExpiryKind tells a domain from a certificate from a credential rotation.
type ExpiryKind string

const (
	ExpiryDomain      ExpiryKind = "domain"
	ExpiryCertificate ExpiryKind = "certificate"
	ExpiryCredential  ExpiryKind = "credential"
)

// Expiry is one thing about to lapse: a domain registration, a certificate, a
// credential rotation, or a check that is itself an expiry probe. DaysOut is
// derived at read time so a stale row cannot lie about the urgency.
type Expiry struct {
	Kind        ExpiryKind
	ID          uuid.UUID
	Name        string
	ExpiresOn   time.Time
	DaysOut     int
	AutoRenew   bool
	SystemID    *uuid.UUID
	SystemName  string
	OwnerName   string
	WarningBand int // the tightest ExpiryWarningDays band it falls in (7/14/30)
}

// ── queries ─────────────────────────────────────────────────────────────────

// SystemQuery narrows the system register.
type SystemQuery struct {
	Type           *SystemType
	Environment    *Environment
	Criticality    *Criticality
	Status         *SystemStatus
	OwnerID        *uuid.UUID
	OrganizationID *uuid.UUID
	Search         string
	UnhealthyOnly  bool
}

// CheckQuery narrows the check list.
type CheckQuery struct {
	SystemID *uuid.UUID
	Type     *CheckType
	Active   *bool
	Status   *CheckStatus
	Search   string
}

// IncidentQuery narrows the incident list.
type IncidentQuery struct {
	Severity       *IncidentSeverity
	Status         *IncidentStatus
	SystemID       *uuid.UUID
	CommanderID    *uuid.UUID
	ReviewRequired *bool
	From           *time.Time
	To             *time.Time
	Search         string
	OpenOnly       bool
}

// DomainQuery narrows the domain list.
type DomainQuery struct {
	SystemID *uuid.UUID
	Search   string
}

// CertificateQuery narrows the certificate list.
type CertificateQuery struct {
	SystemID *uuid.UUID
	Search   string
}

// CredentialQuery narrows the credential-record list.
type CredentialQuery struct {
	SystemID *uuid.UUID
	Active   *bool
}

// Cursor is a keyset position, newest first (§6.6). At is the row's sort time:
// created_at for systems and checks, detected_at for incidents, at for results.
type Cursor struct {
	At time.Time
	ID uuid.UUID
}

// Overview is the systems headline: what is up, what is down, and how fast we
// are at noticing and fixing (§8.6 metrics).
type Overview struct {
	AsOf time.Time

	SystemsTotal   int
	SystemsTier1   int
	SystemsTier2   int
	SystemsTier3   int
	ChecksTotal    int
	ChecksFailing  int
	ChecksWarning  int
	ChecksUnprobed int

	OpenIncidents       int
	OpenSev1            int
	OpenSev2            int
	OpenIncidentsBySev  []SeverityCount
	Incidents30D        int
	DetectedByAtlas30D  int
	DetectedByClient30D int
	MTTASeconds         *int64
	MTTRSeconds         *int64
	ReviewsOutstanding  int
	ExpiringCount       int
	RotationsDueCount   int
	Availability        []SystemAvailability
}

// SeverityCount is one bucket of the incidents-by-severity breakdown.
type SeverityCount struct {
	Severity string
	Label    string
	Count    int
}

// SystemAvailability is one system's measured availability over the window.
type SystemAvailability struct {
	SystemID       uuid.UUID
	Ref            string
	Name           string
	Criticality    Criticality
	Passed         int
	Total          int
	AvailabilityBP int
}

// ── input validation (shared with the write pass) ───────────────────────────

func trim(s string) string { return strings.TrimSpace(s) }

func errInvalidField(field string) error {
	return fmt.Errorf("%w: %s is not valid", ErrInvalid, field)
}

// validAvailabilityBP reports whether a basis-point availability figure is in
// range. Availability is derived, so this only guards the arithmetic.
func validAvailabilityBP(bp int) bool { return bp >= 0 && bp <= 10000 }

// daysBetween is the whole-day distance from asOf to t, negative when t is past.
// It is computed in UTC so a server's local offset cannot shift a warning band.
func daysBetween(asOf, t time.Time) int {
	asOf = time.Date(asOf.UTC().Year(), asOf.UTC().Month(), asOf.UTC().Day(), 0, 0, 0, 0, time.UTC)
	t = time.Date(t.UTC().Year(), t.UTC().Month(), t.UTC().Day(), 0, 0, 0, 0, time.UTC)
	return int(t.Sub(asOf).Hours() / 24)
}

// warningBand returns the tightest configured band a days-out value falls in,
// or 0 when it is beyond every band. A lapsed item is always band 7.
//
// The bands are stored widest first (30/14/7, as §8.6 states them), so the walk
// goes backwards from the tightest: 20 days out is a 30-day warning, not a
// 7-day one.
func warningBand(daysOut int) int {
	if daysOut <= 0 {
		return 7
	}
	for i := len(ExpiryWarningDays) - 1; i >= 0; i-- {
		if daysOut <= ExpiryWarningDays[i] {
			return ExpiryWarningDays[i]
		}
	}
	return 0
}

// validLink reports whether a runbook or repository link is an absolute http(s)
// URL. An empty link is valid: a system may have neither.
func validLink(raw string) bool {
	if raw == "" {
		return true
	}
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	return (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

// validDomainName is a light shape check: a bare hostname with at least one dot,
// no scheme, no path and no spaces. The registrar is the authority on what is a
// real domain; this only catches obvious mistakes before they are stored.
func validDomainName(name string) bool {
	if name == "" || strings.ContainsAny(name, " \t/") || strings.Contains(name, "://") {
		return false
	}
	labels := strings.Split(name, ".")
	if len(labels) < 2 {
		return false
	}
	for _, label := range labels {
		if label == "" {
			return false
		}
	}
	return true
}
