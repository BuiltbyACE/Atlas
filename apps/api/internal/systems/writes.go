package systems

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/safaristack/atlas/apps/api/internal/audit"
	"github.com/safaristack/atlas/apps/api/internal/outbox"
)

// The systems write lifecycle (§8.6). Each method follows the shape the rest of
// Atlas uses: read the record, check the one rule that governs the transition,
// write it with the caller's version, then leave a footprint — an audit row
// naming the actor (§6.6), and an outbox event where another module reacts
// (ADR-005).
//
// Two rules are structural rather than cosmetic:
//
//   - A retired system is history: it cannot be edited, and its checks freeze
//     with it. Retiring is the only way out, and it is refused while an incident
//     is still open, so a system is never retired mid-fire.
//   - Closing a resolved SEV1/SEV2 without a post-incident review is refused.
//     The review is not paperwork; it is where the follow-up actions that become
//     tasks come from.

// ── inputs ──────────────────────────────────────────────────────────────────

// SystemInput is what a caller may set on a system. The reference, status,
// version and timestamps are Go's.
type SystemInput struct {
	Name           string
	SystemType     SystemType
	Environment    Environment
	Criticality    Criticality
	OwnerPersonID  *uuid.UUID
	OrganizationID *uuid.UUID
	ContractID     *uuid.UUID
	RunbookURL     string
	RepositoryURL  string
	Description    string
}

// CheckInput is what a caller may set on a probe.
type CheckInput struct {
	SystemID               uuid.UUID
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
}

// IncidentInput opens an incident by hand. Atlas-detected incidents are written
// by the monitoring worker with detected_source "atlas"; a person opening one
// after a client call records it as "manual".
type IncidentInput struct {
	Title             string
	Severity          IncidentSeverity
	SystemIDs         []uuid.UUID
	DetectedAt        *time.Time
	DetectedSource    IncidentSource
	CommanderPersonID *uuid.UUID
	ClientImpact      string
}

// IncidentStepInput is the optional note a lifecycle step carries.
type IncidentStepInput struct {
	Note string
}

// IncidentReviewInput is the post-incident review. Root cause is required; the
// summary is the short version a reader sees first.
type IncidentReviewInput struct {
	RootCause string
	Summary   string
}

// IncidentNoteInput is a timeline note.
type IncidentNoteInput struct {
	Note string
}

// IncidentActionInput is a follow-up action. TaskID is set once the action has
// become a work task; Go does not create the task itself.
type IncidentActionInput struct {
	Title         string
	OwnerPersonID *uuid.UUID
	DueOn         *time.Time
	Status        ActionStatus
	TaskID        *uuid.UUID
}

// DomainInput is a domain registration.
type DomainInput struct {
	Name          string
	SystemID      *uuid.UUID
	Registrar     string
	RegisteredOn  *time.Time
	ExpiresOn     *time.Time
	AutoRenew     bool
	OwnerPersonID *uuid.UUID
	Notes         string
}

// CertificateInput is a TLS certificate.
type CertificateInput struct {
	Hostname      string
	SystemID      *uuid.UUID
	Issuer        string
	IssuedOn      *time.Time
	ExpiresOn     *time.Time
	AutoRenew     bool
	OwnerPersonID *uuid.UUID
	Notes         string
}

// CredentialInput records that a credential exists and where it is stored. It
// has no field for the secret, by design (§8.6).
type CredentialInput struct {
	SystemID       uuid.UUID
	Label          string
	CredentialKind string
	StorePath      string
	OwnerPersonID  *uuid.UUID
	RotatedOn      *time.Time
	NextRotationOn *time.Time
	Notes          string
	Active         bool
}

// ── plumbing ────────────────────────────────────────────────────────────────

// writes returns the write store, or an internal error when it was never wired.
func (s *Service) writes() (Writer, error) {
	if s.writer == nil {
		return nil, fmt.Errorf("systems: the write store is not configured")
	}
	return s.writer, nil
}

func newID() (uuid.UUID, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return uuid.Nil, fmt.Errorf("systems: new id: %w", err)
	}
	return id, nil
}

// actorRef is the person a timeline entry is attributed to. A principal always
// has a person in this module (writes require a user session), but a zero id is
// stored as NULL rather than as an all-zero person.
func actorRef(actor uuid.UUID) *uuid.UUID {
	if actor == uuid.Nil {
		return nil
	}
	return &actor
}

// noteOr gives a lifecycle step a default timeline line when the caller sends
// none, so every state change is legible in the incident's memory.
func noteOr(note, fallback string) string {
	if n := trim(note); n != "" {
		return n
	}
	return fallback
}

func (s *Service) writeAudit(ctx context.Context, actor uuid.UUID, action, entityType string, entityID uuid.UUID, before, after any) {
	if s.audit == nil {
		return
	}
	id := entityID
	if err := s.audit.Write(ctx, nil, audit.Event{
		ActorType: audit.ActorUser, ActorID: actor.String(), Action: action,
		EntityType: entityType, EntityID: &id, Before: before, After: after,
	}); err != nil {
		s.logger.ErrorContext(ctx, "systems: audit write failed",
			slog.String("action", action), slog.String("error", err.Error()))
	}
}

// ── systems ─────────────────────────────────────────────────────────────────

// CreateSystem registers a system and gives it its SYS reference.
func (s *Service) CreateSystem(ctx context.Context, actor uuid.UUID, in SystemInput) (System, error) {
	w, err := s.writes()
	if err != nil {
		return System{}, err
	}
	if err := validateSystemInput(in); err != nil {
		return System{}, err
	}
	id, err := newID()
	if err != nil {
		return System{}, err
	}
	ref, err := w.AllocateRef(ctx, "sys", s.now().Year())
	if err != nil {
		return System{}, err
	}
	now := s.now()
	sys := System{
		ID: id, Ref: ref, Name: trim(in.Name), SystemType: in.SystemType,
		OwnerPersonID: in.OwnerPersonID, OrganizationID: in.OrganizationID,
		Environment: in.Environment, Criticality: in.Criticality, ContractID: in.ContractID,
		RunbookURL: trim(in.RunbookURL), RepositoryURL: trim(in.RepositoryURL),
		Description: trim(in.Description), Status: SystemActive, Version: 1,
		CreatedAt: now, UpdatedAt: now,
	}
	created, err := w.CreateSystem(ctx, sys)
	if err != nil {
		return System{}, err
	}
	s.writeAudit(ctx, actor, "systems.system.created", "systems.system", created.ID, nil, systemSnapshot(created))
	return created, nil
}

// UpdateSystem rewrites an active system's details. A retired system is frozen.
func (s *Service) UpdateSystem(ctx context.Context, actor uuid.UUID, id uuid.UUID, expectedVersion int, in SystemInput) (System, error) {
	w, err := s.writes()
	if err != nil {
		return System{}, err
	}
	if err := validateSystemInput(in); err != nil {
		return System{}, err
	}
	current, err := s.System(ctx, id)
	if err != nil {
		return System{}, err
	}
	if current.Status != SystemActive {
		return System{}, withCode(fmt.Errorf("%w: a retired system cannot be edited", ErrInvalid), CodeSystemNotEditable)
	}
	next := current
	next.Name = trim(in.Name)
	next.SystemType = in.SystemType
	next.OwnerPersonID = in.OwnerPersonID
	next.OrganizationID = in.OrganizationID
	next.Environment = in.Environment
	next.Criticality = in.Criticality
	next.ContractID = in.ContractID
	next.RunbookURL = trim(in.RunbookURL)
	next.RepositoryURL = trim(in.RepositoryURL)
	next.Description = trim(in.Description)

	updated, err := w.UpdateSystem(ctx, next, expectedVersion)
	if err != nil {
		return System{}, err
	}
	s.writeAudit(ctx, actor, "systems.system.updated", "systems.system", id, systemSnapshot(current), systemSnapshot(updated))
	return updated, nil
}

// RetireSystem retires a system without deleting it: the incident history stays
// readable, and its checks are paused with it.
func (s *Service) RetireSystem(ctx context.Context, actor uuid.UUID, id uuid.UUID, expectedVersion int) (System, error) {
	w, err := s.writes()
	if err != nil {
		return System{}, err
	}
	current, err := s.System(ctx, id)
	if err != nil {
		return System{}, err
	}
	if current.Status == SystemRetired {
		return System{}, withCode(fmt.Errorf("%w: the system is already retired", ErrInvalid), CodeSystemRetired)
	}
	if current.OpenIncidents > 0 {
		return System{}, withCode(fmt.Errorf("%w: %d incident(s) are still open", ErrInvalid, current.OpenIncidents), CodeSystemHasOpenIncidents)
	}
	retired, err := w.SetSystemStatus(ctx, id, expectedVersion, SystemRetired)
	if err != nil {
		return System{}, err
	}
	s.writeAudit(ctx, actor, "systems.system.retired", "systems.system", id, systemSnapshot(current), systemSnapshot(retired))
	return retired, nil
}

func validateSystemInput(in SystemInput) error {
	if n := trim(in.Name); n == "" || len(n) > 160 {
		return errInvalidField("name")
	}
	if !validSystemType(in.SystemType) {
		return errInvalidField("system_type")
	}
	if !validEnvironment(in.Environment) {
		return errInvalidField("environment")
	}
	if !validCriticality(in.Criticality) {
		return errInvalidField("criticality")
	}
	if !validLink(trim(in.RunbookURL)) {
		return errInvalidField("runbook_url")
	}
	if !validLink(trim(in.RepositoryURL)) {
		return errInvalidField("repository_url")
	}
	if len(trim(in.Description)) > 2000 {
		return errInvalidField("description")
	}
	return nil
}

// ── checks ──────────────────────────────────────────────────────────────────

// CreateCheck adds a probe to an active system.
func (s *Service) CreateCheck(ctx context.Context, actor uuid.UUID, in CheckInput) (Check, error) {
	w, err := s.writes()
	if err != nil {
		return Check{}, err
	}
	sys, err := s.System(ctx, in.SystemID)
	if err != nil {
		return Check{}, err
	}
	if !CheckEditable(sys.Status) {
		return Check{}, withCode(fmt.Errorf("%w: a retired system's checks are frozen", ErrInvalid), CodeCheckNotEditable)
	}
	if err := validateCheckInput(in); err != nil {
		return Check{}, err
	}
	id, err := newID()
	if err != nil {
		return Check{}, err
	}
	now := s.now()
	c := Check{
		ID: id, SystemID: in.SystemID, Name: trim(in.Name), CheckType: in.CheckType,
		Target: trim(in.Target), Expect: trim(in.Expect), IntervalSeconds: in.IntervalSeconds,
		TimeoutSeconds: in.TimeoutSeconds, Regions: cleanRegions(in.Regions), WarnMS: in.WarnMS,
		FailMS: in.FailMS, FailureThreshold: in.FailureThreshold,
		HeartbeatWindowSeconds: in.HeartbeatWindowSeconds, Active: in.Active, Version: 1,
		CreatedAt: now, UpdatedAt: now,
	}
	created, err := w.CreateCheck(ctx, c)
	if err != nil {
		return Check{}, err
	}
	s.writeAudit(ctx, actor, "systems.check.created", "systems.check", created.ID, nil, checkSnapshot(created))
	return created, nil
}

// UpdateCheck rewrites a probe. A check belonging to a retired system is frozen.
func (s *Service) UpdateCheck(ctx context.Context, actor uuid.UUID, id uuid.UUID, expectedVersion int, in CheckInput) (Check, error) {
	w, err := s.writes()
	if err != nil {
		return Check{}, err
	}
	current, err := s.store.GetCheck(ctx, id)
	if err != nil {
		return Check{}, err
	}
	if current == nil {
		return Check{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	sys, err := s.System(ctx, current.SystemID)
	if err != nil {
		return Check{}, err
	}
	if !CheckEditable(sys.Status) {
		return Check{}, withCode(fmt.Errorf("%w: a retired system's checks are frozen", ErrInvalid), CodeCheckNotEditable)
	}
	if err := validateCheckInput(in); err != nil {
		return Check{}, err
	}
	next := *current
	next.Name = trim(in.Name)
	next.CheckType = in.CheckType
	next.Target = trim(in.Target)
	next.Expect = trim(in.Expect)
	next.IntervalSeconds = in.IntervalSeconds
	next.TimeoutSeconds = in.TimeoutSeconds
	next.Regions = cleanRegions(in.Regions)
	next.WarnMS = in.WarnMS
	next.FailMS = in.FailMS
	next.FailureThreshold = in.FailureThreshold
	next.HeartbeatWindowSeconds = in.HeartbeatWindowSeconds
	next.Active = in.Active

	updated, err := w.UpdateCheck(ctx, next, expectedVersion)
	if err != nil {
		return Check{}, err
	}
	s.writeAudit(ctx, actor, "systems.check.updated", "systems.check", id, checkSnapshot(*current), checkSnapshot(updated))
	return updated, nil
}

// SetCheckActive pauses or resumes a probe, leaving its result history intact.
func (s *Service) SetCheckActive(ctx context.Context, actor uuid.UUID, id uuid.UUID, expectedVersion int, active bool) (Check, error) {
	w, err := s.writes()
	if err != nil {
		return Check{}, err
	}
	current, err := s.store.GetCheck(ctx, id)
	if err != nil {
		return Check{}, err
	}
	if current == nil {
		return Check{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	sys, err := s.System(ctx, current.SystemID)
	if err != nil {
		return Check{}, err
	}
	if !CheckEditable(sys.Status) {
		return Check{}, withCode(fmt.Errorf("%w: a retired system's checks are frozen", ErrInvalid), CodeCheckNotEditable)
	}
	updated, err := w.SetCheckActive(ctx, id, expectedVersion, active)
	if err != nil {
		return Check{}, err
	}
	action := "systems.check.paused"
	if active {
		action = "systems.check.resumed"
	}
	s.writeAudit(ctx, actor, action, "systems.check", id, checkSnapshot(*current), checkSnapshot(updated))
	return updated, nil
}

func validateCheckInput(in CheckInput) error {
	if n := trim(in.Name); n == "" || len(n) > 160 {
		return errInvalidField("name")
	}
	if !validCheckType(in.CheckType) {
		return errInvalidField("check_type")
	}
	if t := trim(in.Target); t == "" || len(t) > 2048 {
		return errInvalidField("target")
	}
	if in.IntervalSeconds <= 0 {
		return errInvalidField("interval_seconds")
	}
	if in.TimeoutSeconds <= 0 {
		return errInvalidField("timeout_seconds")
	}
	if in.FailureThreshold <= 0 {
		return errInvalidField("failure_threshold")
	}
	if in.WarnMS != nil && *in.WarnMS < 0 {
		return errInvalidField("warn_ms")
	}
	if in.FailMS != nil && *in.FailMS < 0 {
		return errInvalidField("fail_ms")
	}
	if in.WarnMS != nil && in.FailMS != nil && *in.WarnMS >= *in.FailMS {
		return withCode(fmt.Errorf("%w: warn_ms must be below fail_ms", ErrInvalid), CodeCheckThresholdsInvalid)
	}
	// A content probe without a marker can only ever test reachability, which
	// is a different check: refuse the ambiguity rather than silently downgrade
	// it (§8.6).
	if in.CheckType == CheckHTTPContent && trim(in.Expect) == "" {
		return withCode(fmt.Errorf("%w: a content check needs expect", ErrInvalid), CodeCheckExpectRequired)
	}
	if in.CheckType == CheckHeartbeat && (in.HeartbeatWindowSeconds == nil || *in.HeartbeatWindowSeconds <= 0) {
		return withCode(fmt.Errorf("%w: a heartbeat check needs heartbeat_window_seconds", ErrInvalid), CodeCheckHeartbeatWindowMissing)
	}
	return nil
}

// cleanRegions trims, drops blanks and de-duplicates the region list.
func cleanRegions(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, region := range in {
		r := trim(region)
		if r == "" || seen[r] {
			continue
		}
		seen[r] = true
		out = append(out, r)
	}
	return out
}

// ── incidents ───────────────────────────────────────────────────────────────

// OpenIncident records an incident a person detected.
func (s *Service) OpenIncident(ctx context.Context, actor uuid.UUID, in IncidentInput) (Incident, error) {
	w, err := s.writes()
	if err != nil {
		return Incident{}, err
	}
	if title := trim(in.Title); title == "" || len(title) > 200 {
		return Incident{}, errInvalidField("title")
	}
	if !validIncidentSeverity(in.Severity) {
		return Incident{}, errInvalidField("severity")
	}
	if len(in.SystemIDs) == 0 {
		return Incident{}, withCode(fmt.Errorf("%w: an incident must name at least one system", ErrInvalid), CodeIncidentSystemsRequired)
	}
	source := in.DetectedSource
	if source == "" {
		source = DetectedByManual
	}
	if !validIncidentSource(source) {
		return Incident{}, errInvalidField("detected_source")
	}
	detectedAt := s.now()
	if in.DetectedAt != nil {
		detectedAt = in.DetectedAt.UTC()
		if detectedAt.After(s.now()) {
			return Incident{}, withCode(fmt.Errorf("%w: detected_at cannot be in the future", ErrInvalid), CodeDetectedInFuture)
		}
	}
	// Every named system must exist and still be running; an incident about a
	// retired system is a data error, not a story.
	systemIDs := make([]uuid.UUID, 0, len(in.SystemIDs))
	seen := map[uuid.UUID]bool{}
	for _, systemID := range in.SystemIDs {
		if seen[systemID] {
			continue
		}
		seen[systemID] = true
		sys, err := s.System(ctx, systemID)
		if err != nil {
			return Incident{}, err
		}
		if sys.Status != SystemActive {
			return Incident{}, fmt.Errorf("%w: %s is retired", ErrInvalid, sys.Ref)
		}
		systemIDs = append(systemIDs, systemID)
	}

	id, err := newID()
	if err != nil {
		return Incident{}, err
	}
	ref, err := w.AllocateRef(ctx, "inc", detectedAt.Year())
	if err != nil {
		return Incident{}, err
	}
	commander := in.CommanderPersonID
	if commander == nil {
		commander = actorRef(actor)
	}
	now := s.now()
	incoming := Incident{
		ID: id, Ref: ref, Title: trim(in.Title), Severity: in.Severity, Status: IncidentOpen,
		DetectedAt: detectedAt, DetectedSource: source, CommanderPersonID: commander,
		ClientImpact: trim(in.ClientImpact), Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	entry := TimelineEntry{
		ID: mustID(), IncidentID: id, At: detectedAt, ActorPersonID: actorRef(actor),
		EntryType: EntryDetected, Note: fmt.Sprintf("Incident opened (%s).", severityLabel(in.Severity)),
	}
	ev, err := outbox.NewIncidentOpened(actorRef(actor), incidentRef(incoming, systemIDs, nil))
	if err != nil {
		return Incident{}, err
	}
	if _, err := w.CreateIncident(ctx, incoming, systemIDs, entry, ev); err != nil {
		return Incident{}, err
	}
	s.writeAudit(ctx, actor, "systems.incident.opened", "systems.incident", id, nil, incidentSnapshot(incoming))
	return s.Incident(ctx, id)
}

// AcknowledgeIncident stops the MTTA clock.
func (s *Service) AcknowledgeIncident(ctx context.Context, actor uuid.UUID, id uuid.UUID, expectedVersion int, note string) (Incident, error) {
	w, err := s.writes()
	if err != nil {
		return Incident{}, err
	}
	current, err := s.Incident(ctx, id)
	if err != nil {
		return Incident{}, err
	}
	if !IncidentAcknowledgeable(current.Status) {
		return Incident{}, withCode(fmt.Errorf("%w: a %s incident cannot be acknowledged", ErrInvalid, current.Status), CodeIncidentNotAckable)
	}
	at := s.now()
	entry := stepEntry(id, actor, at, EntryAcknowledged, noteOr(note, "Acknowledged."))
	ev, err := outbox.NewIncidentAcknowledged(actorRef(actor), incidentRef(current, incidentSystemIDs(current), nil))
	if err != nil {
		return Incident{}, err
	}
	if _, err := w.AcknowledgeIncident(ctx, id, expectedVersion, at, entry, ev); err != nil {
		return Incident{}, err
	}
	updated, err := s.Incident(ctx, id)
	if err != nil {
		return Incident{}, err
	}
	s.writeAudit(ctx, actor, "systems.incident.acknowledged", "systems.incident", id, incidentSnapshot(current), incidentSnapshot(updated))
	return updated, nil
}

// MitigateIncident records that impact is contained while the incident runs on.
func (s *Service) MitigateIncident(ctx context.Context, actor uuid.UUID, id uuid.UUID, expectedVersion int, note string) (Incident, error) {
	w, err := s.writes()
	if err != nil {
		return Incident{}, err
	}
	current, err := s.Incident(ctx, id)
	if err != nil {
		return Incident{}, err
	}
	if !IncidentMitigable(current.Status) {
		return Incident{}, withCode(fmt.Errorf("%w: a %s incident cannot be mitigated", ErrInvalid, current.Status), CodeIncidentNotMitigable)
	}
	at := s.now()
	entry := stepEntry(id, actor, at, EntryMitigated, noteOr(note, "Impact contained."))
	ev, err := outbox.NewIncidentMitigated(actorRef(actor), incidentRef(current, incidentSystemIDs(current), nil))
	if err != nil {
		return Incident{}, err
	}
	if _, err := w.MitigateIncident(ctx, id, expectedVersion, at, entry, ev); err != nil {
		return Incident{}, err
	}
	updated, err := s.Incident(ctx, id)
	if err != nil {
		return Incident{}, err
	}
	s.writeAudit(ctx, actor, "systems.incident.mitigated", "systems.incident", id, incidentSnapshot(current), incidentSnapshot(updated))
	return updated, nil
}

// ResolveIncident stops the MTTR clock. Resolving a SEV1/SEV2 marks the review
// owed; closing is refused until it is recorded.
func (s *Service) ResolveIncident(ctx context.Context, actor uuid.UUID, id uuid.UUID, expectedVersion int, note string) (Incident, error) {
	w, err := s.writes()
	if err != nil {
		return Incident{}, err
	}
	current, err := s.Incident(ctx, id)
	if err != nil {
		return Incident{}, err
	}
	if !IncidentResolvable(current.Status) {
		return Incident{}, withCode(fmt.Errorf("%w: a %s incident cannot be resolved", ErrInvalid, current.Status), CodeIncidentNotResolvable)
	}
	at := s.now()
	reviewRequired := current.ReviewRequired || current.Severity.RequiresReview()
	entry := stepEntry(id, actor, at, EntryResolved, noteOr(note, "Resolved."))
	ev, err := outbox.NewIncidentResolved(actorRef(actor), incidentRef(current, incidentSystemIDs(current), &at))
	if err != nil {
		return Incident{}, err
	}
	if _, err := w.ResolveIncident(ctx, id, expectedVersion, at, reviewRequired, entry, ev); err != nil {
		return Incident{}, err
	}
	updated, err := s.Incident(ctx, id)
	if err != nil {
		return Incident{}, err
	}
	s.writeAudit(ctx, actor, "systems.incident.resolved", "systems.incident", id, incidentSnapshot(current), incidentSnapshot(updated))
	return updated, nil
}

// CloseIncident closes a resolved incident, unless its review is still owed.
func (s *Service) CloseIncident(ctx context.Context, actor uuid.UUID, id uuid.UUID, expectedVersion int, note string) (Incident, error) {
	w, err := s.writes()
	if err != nil {
		return Incident{}, err
	}
	current, err := s.Incident(ctx, id)
	if err != nil {
		return Incident{}, err
	}
	if !IncidentClosable(current.Status) {
		return Incident{}, withCode(fmt.Errorf("%w: only a resolved incident can be closed", ErrInvalid), CodeIncidentNotClosable)
	}
	if current.ReviewRequired && current.ReviewAt == nil {
		return Incident{}, withCode(fmt.Errorf("%w: record the post-incident review before closing", ErrInvalid), CodeReviewRequired)
	}
	at := s.now()
	entry := stepEntry(id, actor, at, EntryClosed, noteOr(note, "Closed."))
	ev, err := outbox.NewIncidentClosed(actorRef(actor), incidentRef(current, incidentSystemIDs(current), nil))
	if err != nil {
		return Incident{}, err
	}
	if _, err := w.CloseIncident(ctx, id, expectedVersion, at, entry, ev); err != nil {
		return Incident{}, err
	}
	updated, err := s.Incident(ctx, id)
	if err != nil {
		return Incident{}, err
	}
	s.writeAudit(ctx, actor, "systems.incident.closed", "systems.incident", id, incidentSnapshot(current), incidentSnapshot(updated))
	return updated, nil
}

// ReviewIncident records the post-incident review: what actually happened and
// why. It may be recorded on a resolved incident, or amended after closing.
func (s *Service) ReviewIncident(ctx context.Context, actor uuid.UUID, id uuid.UUID, expectedVersion int, in IncidentReviewInput) (Incident, error) {
	w, err := s.writes()
	if err != nil {
		return Incident{}, err
	}
	current, err := s.Incident(ctx, id)
	if err != nil {
		return Incident{}, err
	}
	if current.Status != IncidentResolved && current.Status != IncidentClosed {
		return Incident{}, withCode(fmt.Errorf("%w: a %s incident has nothing to review yet", ErrInvalid, current.Status), CodeIncidentNotReviewable)
	}
	rootCause := trim(in.RootCause)
	if rootCause == "" {
		return Incident{}, withCode(fmt.Errorf("%w: root_cause is required", ErrInvalid), CodeRootCauseRequired)
	}
	at := s.now()
	entry := stepEntry(id, actor, at, EntryNote, "Post-incident review recorded.")
	ev, err := outbox.NewIncidentReviewRecorded(actorRef(actor), incidentRef(current, incidentSystemIDs(current), nil))
	if err != nil {
		return Incident{}, err
	}
	if _, err := w.RecordIncidentReview(ctx, id, expectedVersion, at, rootCause, trim(in.Summary), entry, ev); err != nil {
		return Incident{}, err
	}
	updated, err := s.Incident(ctx, id)
	if err != nil {
		return Incident{}, err
	}
	s.writeAudit(ctx, actor, "systems.incident.reviewed", "systems.incident", id, incidentSnapshot(current), incidentSnapshot(updated))
	return updated, nil
}

// AddIncidentNote appends a line to the incident's memory. Notes do not move
// the incident's version: they are added facts, not changes to the record.
func (s *Service) AddIncidentNote(ctx context.Context, actor uuid.UUID, id uuid.UUID, in IncidentNoteInput) (TimelineEntry, error) {
	w, err := s.writes()
	if err != nil {
		return TimelineEntry{}, err
	}
	if _, err := s.Incident(ctx, id); err != nil {
		return TimelineEntry{}, err
	}
	note := trim(in.Note)
	if note == "" || len(note) > 4000 {
		return TimelineEntry{}, errInvalidField("note")
	}
	entry := stepEntry(id, actor, s.now(), EntryNote, note)
	if _, err := w.AddTimelineEntry(ctx, entry); err != nil {
		return TimelineEntry{}, err
	}
	s.writeAudit(ctx, actor, "systems.incident.noted", "systems.incident", id, nil, map[string]any{"note": note})
	return entry, nil
}

// CreateIncidentAction adds a follow-up action to an incident.
func (s *Service) CreateIncidentAction(ctx context.Context, actor uuid.UUID, id uuid.UUID, in IncidentActionInput) (IncidentAction, error) {
	w, err := s.writes()
	if err != nil {
		return IncidentAction{}, err
	}
	if _, err := s.Incident(ctx, id); err != nil {
		return IncidentAction{}, err
	}
	if err := validateActionInput(in); err != nil {
		return IncidentAction{}, err
	}
	actionID, err := newID()
	if err != nil {
		return IncidentAction{}, err
	}
	status := in.Status
	if status == "" {
		status = ActionTodo
	}
	now := s.now()
	a := IncidentAction{
		ID: actionID, IncidentID: id, Title: trim(in.Title), OwnerPersonID: in.OwnerPersonID,
		DueOn: in.DueOn, Status: status, TaskID: in.TaskID, Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	entry := stepEntry(id, actor, now, EntryAction, fmt.Sprintf("Follow-up action added: %s.", trim(in.Title)))
	created, err := w.CreateAction(ctx, a, &entry)
	if err != nil {
		return IncidentAction{}, err
	}
	s.writeAudit(ctx, actor, "systems.incident.action_added", "systems.incident", id, nil, actionSnapshot(created))
	return created, nil
}

// UpdateIncidentAction moves a follow-up action along, or links the task it
// became.
func (s *Service) UpdateIncidentAction(ctx context.Context, actor uuid.UUID, incidentID, actionID uuid.UUID, expectedVersion int, in IncidentActionInput) (IncidentAction, error) {
	w, err := s.writes()
	if err != nil {
		return IncidentAction{}, err
	}
	if _, err := s.Incident(ctx, incidentID); err != nil {
		return IncidentAction{}, err
	}
	if err := validateActionInput(in); err != nil {
		return IncidentAction{}, err
	}
	actions, err := s.store.IncidentActions(ctx, incidentID)
	if err != nil {
		return IncidentAction{}, err
	}
	var current *IncidentAction
	for i := range actions {
		if actions[i].ID == actionID {
			current = &actions[i]
			break
		}
	}
	if current == nil {
		return IncidentAction{}, fmt.Errorf("%w: %s", ErrNotFound, actionID)
	}
	status := in.Status
	if status == "" {
		status = current.Status
	}
	next := *current
	next.Title = trim(in.Title)
	next.OwnerPersonID = in.OwnerPersonID
	next.DueOn = in.DueOn
	next.Status = status
	next.TaskID = in.TaskID

	updated, err := w.UpdateAction(ctx, next, expectedVersion)
	if err != nil {
		return IncidentAction{}, err
	}
	s.writeAudit(ctx, actor, "systems.incident.action_updated", "systems.incident", incidentID, actionSnapshot(*current), actionSnapshot(updated))
	return updated, nil
}

func validateActionInput(in IncidentActionInput) error {
	if t := trim(in.Title); t == "" || len(t) > 200 {
		return errInvalidField("title")
	}
	if in.Status != "" && !validActionStatus(in.Status) {
		return errInvalidField("status")
	}
	return nil
}

// ── domains and certificates ────────────────────────────────────────────────

// CreateDomain records a domain registration.
func (s *Service) CreateDomain(ctx context.Context, actor uuid.UUID, in DomainInput) (Domain, error) {
	w, err := s.writes()
	if err != nil {
		return Domain{}, err
	}
	if err := validateDomainInput(in); err != nil {
		return Domain{}, err
	}
	id, err := newID()
	if err != nil {
		return Domain{}, err
	}
	now := s.now()
	d := Domain{
		ID: id, Name: trimLower(in.Name), SystemID: in.SystemID, Registrar: trim(in.Registrar),
		RegisteredOn: in.RegisteredOn, ExpiresOn: in.ExpiresOn, AutoRenew: in.AutoRenew,
		OwnerPersonID: in.OwnerPersonID, Notes: trim(in.Notes), Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	created, err := w.CreateDomain(ctx, d)
	if err != nil {
		return Domain{}, err
	}
	s.writeAudit(ctx, actor, "systems.domain.created", "systems.domain", created.ID, nil, domainSnapshot(created))
	return created, nil
}

// UpdateDomain rewrites a domain registration.
func (s *Service) UpdateDomain(ctx context.Context, actor uuid.UUID, id uuid.UUID, expectedVersion int, in DomainInput) (Domain, error) {
	w, err := s.writes()
	if err != nil {
		return Domain{}, err
	}
	if err := validateDomainInput(in); err != nil {
		return Domain{}, err
	}
	current := s.domain(ctx, id)
	if current == nil {
		return Domain{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	next := *current
	next.Name = trimLower(in.Name)
	next.SystemID = in.SystemID
	next.Registrar = trim(in.Registrar)
	next.RegisteredOn = in.RegisteredOn
	next.ExpiresOn = in.ExpiresOn
	next.AutoRenew = in.AutoRenew
	next.OwnerPersonID = in.OwnerPersonID
	next.Notes = trim(in.Notes)

	updated, err := w.UpdateDomain(ctx, next, expectedVersion)
	if err != nil {
		return Domain{}, err
	}
	s.writeAudit(ctx, actor, "systems.domain.updated", "systems.domain", id, domainSnapshot(*current), domainSnapshot(updated))
	return updated, nil
}

func validateDomainInput(in DomainInput) error {
	if !validDomainName(trimLower(in.Name)) {
		return errInvalidField("name")
	}
	if in.RegisteredOn != nil && in.ExpiresOn != nil && in.ExpiresOn.Before(*in.RegisteredOn) {
		return errInvalidField("expires_on")
	}
	if len(trim(in.Notes)) > 2000 {
		return errInvalidField("notes")
	}
	return nil
}

// CreateCertificate records a TLS certificate.
func (s *Service) CreateCertificate(ctx context.Context, actor uuid.UUID, in CertificateInput) (Certificate, error) {
	w, err := s.writes()
	if err != nil {
		return Certificate{}, err
	}
	if err := validateCertificateInput(in); err != nil {
		return Certificate{}, err
	}
	id, err := newID()
	if err != nil {
		return Certificate{}, err
	}
	now := s.now()
	c := Certificate{
		ID: id, Hostname: trimLower(in.Hostname), SystemID: in.SystemID, Issuer: trim(in.Issuer),
		IssuedOn: in.IssuedOn, ExpiresOn: *in.ExpiresOn, AutoRenew: in.AutoRenew,
		OwnerPersonID: in.OwnerPersonID, Notes: trim(in.Notes), Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	created, err := w.CreateCertificate(ctx, c)
	if err != nil {
		return Certificate{}, err
	}
	s.writeAudit(ctx, actor, "systems.certificate.created", "systems.certificate", created.ID, nil, certificateSnapshot(created))
	return created, nil
}

// UpdateCertificate rewrites a certificate record.
func (s *Service) UpdateCertificate(ctx context.Context, actor uuid.UUID, id uuid.UUID, expectedVersion int, in CertificateInput) (Certificate, error) {
	w, err := s.writes()
	if err != nil {
		return Certificate{}, err
	}
	if err := validateCertificateInput(in); err != nil {
		return Certificate{}, err
	}
	current := s.certificate(ctx, id)
	if current == nil {
		return Certificate{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	next := *current
	next.Hostname = trimLower(in.Hostname)
	next.SystemID = in.SystemID
	next.Issuer = trim(in.Issuer)
	next.IssuedOn = in.IssuedOn
	next.ExpiresOn = *in.ExpiresOn
	next.AutoRenew = in.AutoRenew
	next.OwnerPersonID = in.OwnerPersonID
	next.Notes = trim(in.Notes)

	updated, err := w.UpdateCertificate(ctx, next, expectedVersion)
	if err != nil {
		return Certificate{}, err
	}
	s.writeAudit(ctx, actor, "systems.certificate.updated", "systems.certificate", id, certificateSnapshot(*current), certificateSnapshot(updated))
	return updated, nil
}

func validateCertificateInput(in CertificateInput) error {
	host := trimLower(in.Hostname)
	if host == "" || len(host) > 255 || !validDomainName(host) {
		return errInvalidField("hostname")
	}
	if in.ExpiresOn == nil {
		return errInvalidField("expires_on")
	}
	if in.IssuedOn != nil && in.ExpiresOn.Before(*in.IssuedOn) {
		return errInvalidField("expires_on")
	}
	if len(trim(in.Notes)) > 2000 {
		return errInvalidField("notes")
	}
	return nil
}

// ── credential records ──────────────────────────────────────────────────────

// CreateCredential records that a credential exists and where it is stored.
// There is no secret in this type and no column for one (§8.6).
func (s *Service) CreateCredential(ctx context.Context, actor uuid.UUID, in CredentialInput) (CredentialRecord, error) {
	w, err := s.writes()
	if err != nil {
		return CredentialRecord{}, err
	}
	sys, err := s.System(ctx, in.SystemID)
	if err != nil {
		return CredentialRecord{}, err
	}
	if sys.Status != SystemActive {
		return CredentialRecord{}, fmt.Errorf("%w: %s is retired", ErrInvalid, sys.Ref)
	}
	if err := validateCredentialInput(in); err != nil {
		return CredentialRecord{}, err
	}
	id, err := newID()
	if err != nil {
		return CredentialRecord{}, err
	}
	now := s.now()
	c := CredentialRecord{
		ID: id, SystemID: in.SystemID, Label: trim(in.Label),
		CredentialKind: trim(in.CredentialKind), StorePath: trim(in.StorePath),
		OwnerPersonID: in.OwnerPersonID, RotatedOn: in.RotatedOn, NextRotationOn: in.NextRotationOn,
		Notes: trim(in.Notes), Active: in.Active, Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	created, err := w.CreateCredential(ctx, c)
	if err != nil {
		return CredentialRecord{}, err
	}
	s.writeAudit(ctx, actor, "systems.credential.created", "systems.credential", created.ID, nil, credentialSnapshot(created))
	return created, nil
}

// UpdateCredential rewrites a credential record — still never the secret.
func (s *Service) UpdateCredential(ctx context.Context, actor uuid.UUID, id uuid.UUID, expectedVersion int, in CredentialInput) (CredentialRecord, error) {
	w, err := s.writes()
	if err != nil {
		return CredentialRecord{}, err
	}
	if err := validateCredentialInput(in); err != nil {
		return CredentialRecord{}, err
	}
	current, err := s.store.GetCredential(ctx, id)
	if err != nil {
		return CredentialRecord{}, err
	}
	if current == nil {
		return CredentialRecord{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	next := *current
	next.SystemID = in.SystemID
	next.Label = trim(in.Label)
	next.CredentialKind = trim(in.CredentialKind)
	next.StorePath = trim(in.StorePath)
	next.OwnerPersonID = in.OwnerPersonID
	next.RotatedOn = in.RotatedOn
	next.NextRotationOn = in.NextRotationOn
	next.Notes = trim(in.Notes)
	next.Active = in.Active

	updated, err := w.UpdateCredential(ctx, next, expectedVersion)
	if err != nil {
		return CredentialRecord{}, err
	}
	s.writeAudit(ctx, actor, "systems.credential.updated", "systems.credential", id, credentialSnapshot(*current), credentialSnapshot(updated))
	return updated, nil
}

func validateCredentialInput(in CredentialInput) error {
	if trim(in.Label) == "" || len(trim(in.Label)) > 160 {
		return errInvalidField("label")
	}
	if trim(in.CredentialKind) == "" || len(trim(in.CredentialKind)) > 80 {
		return errInvalidField("credential_kind")
	}
	if trim(in.StorePath) == "" || len(trim(in.StorePath)) > 500 {
		return errInvalidField("store_path")
	}
	if len(trim(in.Notes)) > 2000 {
		return errInvalidField("notes")
	}
	return nil
}

// ── directory reads and accessors ───────────────────────────────────────────

// Domains lists the domain registrations.
func (s *Service) Domains(ctx context.Context, q DomainQuery, limit int, before *Cursor) ([]Domain, error) {
	return s.store.ListDomains(ctx, q, limit, before)
}

// Domain returns one domain, or ErrNotFound.
func (s *Service) Domain(ctx context.Context, id uuid.UUID) (Domain, error) {
	d := s.domain(ctx, id)
	if d == nil {
		return Domain{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	return *d, nil
}

func (s *Service) domain(ctx context.Context, id uuid.UUID) *Domain {
	d, err := s.store.GetDomain(ctx, id)
	if err != nil {
		return nil
	}
	return d
}

// Certificates lists the certificate records.
func (s *Service) Certificates(ctx context.Context, q CertificateQuery, limit int, before *Cursor) ([]Certificate, error) {
	return s.store.ListCertificates(ctx, q, limit, before)
}

// Certificate returns one certificate, or ErrNotFound.
func (s *Service) Certificate(ctx context.Context, id uuid.UUID) (Certificate, error) {
	c := s.certificate(ctx, id)
	if c == nil {
		return Certificate{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	return *c, nil
}

func (s *Service) certificate(ctx context.Context, id uuid.UUID) *Certificate {
	c, err := s.store.GetCertificate(ctx, id)
	if err != nil {
		return nil
	}
	return c
}

// AllCredentials lists credential records across the register.
func (s *Service) AllCredentials(ctx context.Context, q CredentialQuery, limit int, before *Cursor) ([]CredentialRecord, error) {
	return s.store.ListAllCredentials(ctx, q, limit, before)
}

// Credential returns one credential record, or ErrNotFound.
func (s *Service) Credential(ctx context.Context, id uuid.UUID) (CredentialRecord, error) {
	c, err := s.store.GetCredential(ctx, id)
	if err != nil {
		return CredentialRecord{}, err
	}
	if c == nil {
		return CredentialRecord{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	return *c, nil
}

// ── small helpers ───────────────────────────────────────────────────────────

// mustID generates an id for a journal row, where a UUIDv7 failure is a broken
// process rather than a client problem.
func mustID() uuid.UUID {
	return uuid.Must(uuid.NewV7())
}

func stepEntry(incidentID, actor uuid.UUID, at time.Time, entryType TimelineEntryType, note string) TimelineEntry {
	return TimelineEntry{
		ID: mustID(), IncidentID: incidentID, At: at, ActorPersonID: actorRef(actor),
		EntryType: entryType, Note: note,
	}
}

func incidentSystemIDs(in Incident) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(in.Systems))
	for _, s := range in.Systems {
		out = append(out, s.SystemID)
	}
	return out
}

// incidentRef is the event view of an incident: ids and state, never narrative.
func incidentRef(in Incident, systemIDs []uuid.UUID, resolvedAt *time.Time) outbox.IncidentRef {
	if resolvedAt == nil {
		resolvedAt = in.ResolvedAt
	}
	return outbox.IncidentRef{
		ID: in.ID, Ref: in.Ref, Severity: string(in.Severity), Status: string(in.Status),
		SystemIDs: systemIDs, CommanderPersonID: in.CommanderPersonID, DetectedAt: in.DetectedAt,
		ResolvedAt: resolvedAt, ReviewRequired: in.ReviewRequired,
	}
}

// ── snapshots (audit payloads) ──────────────────────────────────────────────

// A snapshot carries what a reviewer needs to see what changed: the state and
// the links, never the whole row (§6.3). A credential snapshot has no secret to
// carry, because a credential record never held one.
func systemSnapshot(s System) map[string]any {
	return map[string]any{
		"ref": s.Ref, "name": s.Name, "system_type": string(s.SystemType),
		"environment": string(s.Environment), "criticality": string(s.Criticality),
		"owner_person_id": idOrEmpty(s.OwnerPersonID), "organization_id": idOrEmpty(s.OrganizationID),
		"contract_id": idOrEmpty(s.ContractID), "status": string(s.Status),
	}
}

func checkSnapshot(c Check) map[string]any {
	return map[string]any{
		"system_id": c.SystemID.String(), "name": c.Name, "check_type": string(c.CheckType),
		"target": c.Target, "interval_seconds": c.IntervalSeconds,
		"failure_threshold": c.FailureThreshold, "active": c.Active,
	}
}

func incidentSnapshot(in Incident) map[string]any {
	return map[string]any{
		"ref": in.Ref, "title": in.Title, "severity": string(in.Severity), "status": string(in.Status),
		"detected_at":         in.DetectedAt.UTC().Format(time.RFC3339),
		"commander_person_id": idOrEmpty(in.CommanderPersonID), "review_required": in.ReviewRequired,
	}
}

func actionSnapshot(a IncidentAction) map[string]any {
	return map[string]any{
		"incident_id": a.IncidentID.String(), "title": a.Title, "status": string(a.Status),
		"owner_person_id": idOrEmpty(a.OwnerPersonID), "task_id": idOrEmpty(a.TaskID),
	}
}

func domainSnapshot(d Domain) map[string]any {
	return map[string]any{
		"name": d.Name, "system_id": idOrEmpty(d.SystemID), "registrar": d.Registrar,
		"expires_on": day(d.ExpiresOn), "auto_renew": d.AutoRenew,
	}
}

func certificateSnapshot(c Certificate) map[string]any {
	return map[string]any{
		"hostname": c.Hostname, "system_id": idOrEmpty(c.SystemID), "issuer": c.Issuer,
		"expires_on": day(&c.ExpiresOn), "auto_renew": c.AutoRenew,
	}
}

func credentialSnapshot(c CredentialRecord) map[string]any {
	return map[string]any{
		"system_id": c.SystemID.String(), "label": c.Label, "credential_kind": c.CredentialKind,
		"store_path": c.StorePath, "active": c.Active,
		"next_rotation_on": day(c.NextRotationOn),
	}
}

func idOrEmpty(id *uuid.UUID) string {
	if id == nil {
		return ""
	}
	return id.String()
}

// day renders an optional date as YYYY-MM-DD, or "" when unset.
func day(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format("2006-01-02")
}

func trimLower(s string) string {
	return strings.ToLower(trim(s))
}
