package integrations

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

// Work is the work module's contribution to linking pull requests to tasks.
// Implemented over work.Service in cmd wiring, so this package depends on the
// contract, not on work.
type Work interface {
	TaskIDByRef(ctx context.Context, ref string) (uuid.UUID, bool, error)
	AttachPullRequest(ctx context.Context, taskID uuid.UUID, pr PullRequest) (EvidenceLink, error)
}

// EvidenceLink is what attaching a merged PR as evidence produced.
type EvidenceLink struct {
	EvidenceID uuid.UUID
	ProjectID  uuid.UUID
}

// Service is the integrations read model's rules: it validates the filters,
// resolves absence into ErrNotFound. When the store also implements Writer,
// the same service carries the integration lifecycle (§8.9).
type Service struct {
	store  Store
	work   Work
	writer Writer
	audit  *audit.Writer
	now    func() time.Time
	logger *slog.Logger
}

// NewService builds the integrations service over a store. A store that
// implements Writer enables the write lifecycle; the read slice still works
// without one.
func NewService(store Store, work Work, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	s := &Service{
		store:  store,
		work:   work,
		now:    func() time.Time { return time.Now().UTC() },
		logger: logger,
	}
	if w, ok := store.(Writer); ok {
		s.writer = w
	}
	return s
}

// SetAudit attaches the audit writer that records who changed what (§6.6).
func (s *Service) SetAudit(aw *audit.Writer) { s.audit = aw }

// Integrations lists integration registrations.
func (s *Service) Integrations(ctx context.Context, q IntegrationQuery, limit int, before *Cursor) ([]Integration, error) {
	if err := validateIntegrationQuery(q); err != nil {
		return nil, err
	}
	out, err := s.store.ListIntegrations(ctx, q, limit, before)
	if err != nil {
		return nil, err
	}
	sortIntegrations(out)
	return out, nil
}

// Integration returns one integration, or ErrNotFound.
func (s *Service) Integration(ctx context.Context, id uuid.UUID) (Integration, error) {
	i, err := s.store.GetIntegration(ctx, id)
	if err != nil {
		return Integration{}, err
	}
	if i == nil {
		return Integration{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	return *i, nil
}

// WebhookInbox lists webhook inbox entries.
func (s *Service) WebhookInbox(ctx context.Context, q WebhookInboxQuery, limit int, before *Cursor) ([]WebhookInboxEntry, error) {
	if err := validateWebhookInboxQuery(q); err != nil {
		return nil, err
	}
	out, err := s.store.ListWebhookInbox(ctx, q, limit, before)
	if err != nil {
		return nil, err
	}
	sortWebhookInbox(out)
	return out, nil
}

// WebhookInboxEntry returns one webhook inbox entry, or ErrNotFound.
func (s *Service) WebhookInboxEntry(ctx context.Context, id uuid.UUID) (WebhookInboxEntry, error) {
	w, err := s.store.GetWebhookInbox(ctx, id)
	if err != nil {
		return WebhookInboxEntry{}, err
	}
	if w == nil {
		return WebhookInboxEntry{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	return *w, nil
}

// OutboundLog lists outbound log entries.
func (s *Service) OutboundLog(ctx context.Context, q OutboundLogQuery, limit int, before *Cursor) ([]OutboundLogEntry, error) {
	if err := validateOutboundLogQuery(q); err != nil {
		return nil, err
	}
	out, err := s.store.ListOutboundLog(ctx, q, limit, before)
	if err != nil {
		return nil, err
	}
	sortOutboundLog(out)
	return out, nil
}

// OutboundLogEntry returns one outbound log entry, or ErrNotFound.
func (s *Service) OutboundLogEntry(ctx context.Context, id uuid.UUID) (OutboundLogEntry, error) {
	o, err := s.store.GetOutboundLog(ctx, id)
	if err != nil {
		return OutboundLogEntry{}, err
	}
	if o == nil {
		return OutboundLogEntry{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	return *o, nil
}

// ── write lifecycle ─────────────────────────────────────────────────────────

// CreateIntegration registers an integration (§8.9). `integrations.registry.manage`.
func (s *Service) CreateIntegration(ctx context.Context, actor uuid.UUID, in IntegrationInput) (Integration, error) {
	i := Integration{
		Name:          trim(in.Name),
		Kind:          IntegrationKind(trim(in.Kind)),
		Direction:     Direction(trim(in.Direction)),
		OwnerPersonID: in.OwnerPersonID,
		CredentialRef: trim(in.CredentialRef),
		Scopes:        in.Scopes,
		Status:        IntegrationActive,
	}
	if err := validateIntegration(i); err != nil {
		return Integration{}, err
	}
	created, err := s.writer.CreateIntegration(ctx, i)
	if err != nil {
		return Integration{}, err
	}
	if s.audit != nil {
		s.writeAudit(ctx, actor, "integrations.integration.created", "integrations.integration", created.ID,
			nil, map[string]any{"ref": created.Ref, "name": created.Name, "kind": created.Kind})
	}
	return created, nil
}

// UpdateIntegration edits an integration (§8.9). `integrations.registry.manage`.
// Refused if the integration is in error state or the version is stale.
func (s *Service) UpdateIntegration(ctx context.Context, actor uuid.UUID, id uuid.UUID, expectedVersion int, in IntegrationInput) (Integration, error) {
	current, err := s.Integration(ctx, id)
	if err != nil {
		return Integration{}, err
	}
	if !IntegrationEditable(current.Status) {
		return Integration{}, withCode(ErrForbidden, CodeIntegrationNotEditable)
	}
	i := Integration{
		ID:            id,
		Ref:           current.Ref,
		Name:          trim(in.Name),
		Kind:          IntegrationKind(trim(in.Kind)),
		Direction:     Direction(trim(in.Direction)),
		OwnerPersonID: in.OwnerPersonID,
		CredentialRef: trim(in.CredentialRef),
		Scopes:        in.Scopes,
		Status:        IntegrationStatus(trim(in.Status)),
		Version:       expectedVersion,
	}
	if err := validateIntegration(i); err != nil {
		return Integration{}, err
	}
	updated, err := s.writer.UpdateIntegration(ctx, i, expectedVersion)
	if err != nil {
		return Integration{}, err
	}
	if s.audit != nil {
		s.writeAudit(ctx, actor, "integrations.integration.updated", "integrations.integration", updated.ID,
			map[string]any{"status": current.Status}, map[string]any{"status": updated.Status})
	}
	return updated, nil
}

// HandleGitHub processes one GitHub delivery. The signature is verified by the
// HTTP layer; this method is idempotent by delivery id.
func (s *Service) HandleGitHub(ctx context.Context, deliveryID, eventType string, body []byte) (Outcome, error) {
	deliveryID = strings.TrimSpace(deliveryID)
	if deliveryID == "" {
		return Outcome{}, fmt.Errorf("%w: X-GitHub-Delivery is required", ErrInvalid)
	}
	claimed, err := s.store.ClaimDelivery(ctx, deliveryID, eventType)
	if err != nil {
		return Outcome{}, err
	}
	if !claimed {
		return Outcome{Status: StatusDuplicate}, nil
	}

	outcome, err := s.processGitHub(ctx, deliveryID, eventType, body)
	if err != nil {
		if rerr := s.store.ReleaseDelivery(ctx, deliveryID); rerr != nil {
			s.logger.Warn("release github delivery", "delivery_id", deliveryID, "error", rerr)
		}
		return Outcome{}, err
	}
	return outcome, nil
}

func (s *Service) processGitHub(ctx context.Context, deliveryID, eventType string, body []byte) (Outcome, error) {
	if eventType != EventPullRequest {
		return Outcome{Status: StatusIgnored}, nil
	}
	pr, err := ParsePullRequest(body)
	if err != nil {
		return Outcome{}, err
	}
	if pr.Action != actionClosed || !pr.Merged {
		return Outcome{Status: StatusIgnored}, nil
	}

	refs := ExtractTaskRefs(pr.Title, pr.HeadRef)
	for _, ref := range refs {
		taskID, found, err := s.work.TaskIDByRef(ctx, ref)
		if err != nil {
			return Outcome{}, err
		}
		if !found {
			continue
		}
		link, err := s.work.AttachPullRequest(ctx, taskID, pr)
		if err != nil {
			return Outcome{}, err
		}
		ev, err := outbox.NewGitPullRequestMerged(outbox.PullRequestLink{
			EvidenceID: link.EvidenceID,
			TaskID:     taskID,
			ProjectID:  link.ProjectID,
			Provider:   ProviderGitHub,
			Repository: pr.Repository,
			Number:     pr.Number,
			Title:      pr.Title,
			URL:        pr.URL,
			CommitSHA:  pr.CommitSHA,
			Author:     pr.Author,
			MergedAt:   pr.MergedAt,
			LinkedAt:   s.now(),
		})
		if err != nil {
			return Outcome{}, err
		}
		if err := s.store.WriteEvent(ctx, ev); err != nil {
			return Outcome{}, err
		}
		s.logger.Info("linked merged pull request to task",
			"repository", pr.Repository, "number", pr.Number, "ref", ref, "task_id", taskID.String())
		return Outcome{Status: StatusRecorded, Refs: refs, TaskID: taskID, EvidenceID: link.EvidenceID}, nil
	}
	return Outcome{Status: StatusUnlinked, Refs: refs}, nil
}

// ── write input types ───────────────────────────────────────────────────────

// IntegrationInput is the create/update payload for an integration.
type IntegrationInput struct {
	Name          string
	Kind          string
	Direction     string
	OwnerPersonID *uuid.UUID
	CredentialRef string
	Scopes        []string
	Status        string
}

// ── write input validation ─────────────────────────────────────────────────

func validateIntegration(i Integration) error {
	if trim(i.Name) == "" {
		return errInvalidField("name")
	}
	if !validIntegrationKind(i.Kind) {
		return errInvalidField("kind")
	}
	if !validDirection(i.Direction) {
		return errInvalidField("direction")
	}
	if !validIntegrationStatus(i.Status) {
		return errInvalidField("status")
	}
	if !validRef(i.Ref) {
		return errInvalidField("ref")
	}
	return nil
}

// ── filter validation ───────────────────────────────────────────────────────

func validateIntegrationQuery(q IntegrationQuery) error {
	if q.Kind != nil && !validIntegrationKind(*q.Kind) {
		return fmt.Errorf("%w: unknown integration kind", ErrInvalid)
	}
	if q.Direction != nil && !validDirection(*q.Direction) {
		return fmt.Errorf("%w: unknown integration direction", ErrInvalid)
	}
	if q.Status != nil && !validIntegrationStatus(*q.Status) {
		return fmt.Errorf("%w: unknown integration status", ErrInvalid)
	}
	return nil
}

func validateWebhookInboxQuery(q WebhookInboxQuery) error {
	if q.ProcessingStatus != nil && !validProcessingStatus(*q.ProcessingStatus) {
		return fmt.Errorf("%w: unknown processing status", ErrInvalid)
	}
	return nil
}

func validateOutboundLogQuery(q OutboundLogQuery) error {
	if q.CircuitState != nil && !validCircuitState(*q.CircuitState) {
		return fmt.Errorf("%w: unknown circuit state", ErrInvalid)
	}
	return nil
}

// writeAudit records who changed what (§6.6).
func (s *Service) writeAudit(ctx context.Context, actor uuid.UUID, action, entityType string, entityID uuid.UUID, before, after any) {
	if s.audit == nil {
		return
	}
	id := entityID
	if err := s.audit.Write(ctx, nil, audit.Event{
		ActorType:  audit.ActorUser,
		ActorID:    actor.String(),
		Action:     action,
		EntityType: entityType,
		EntityID:   &id,
		Before:     before,
		After:      after,
	}); err != nil {
		s.logger.ErrorContext(ctx, "integrations: audit write failed",
			slog.String("action", action), slog.String("error", err.Error()))
	}
}
