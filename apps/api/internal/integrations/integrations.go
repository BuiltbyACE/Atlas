// Package integrations receives inbound provider webhooks (docs/ATLAS.md §11,
// §12) and turns them into Atlas facts.
//
// Decision D6 (docs/adr/ADR-018) implements GitHub: a merged pull request links
// to the task named by the Atlas task ref in its branch or title, and records
// §6.5 `pull_request` evidence. The provider authenticates with an HMAC rather
// than a principal, so the route is public and the signature is the gate;
// deliveries are de-duplicated by their GitHub delivery id.
//
// The package owns no business data beyond the delivery log; linking and
// evidence live in the work module, injected as a Work sink so this package
// depends on the contract, not on work.
package integrations

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/safaristack/atlas/apps/api/internal/outbox"
)

// ProviderGitHub is the provider id stored with a delivery.
const ProviderGitHub = "github"

// ErrInvalid marks a delivery Atlas cannot act on (missing delivery id, bad
// payload): the caller records it and answers 4xx.
var ErrInvalid = errors.New("integrations: invalid")

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

// Outcome is what one delivery accomplished.
type Outcome struct {
	Status     string
	Refs       []string
	TaskID     uuid.UUID
	EvidenceID uuid.UUID
}

// Work is the work module's contribution to a link: resolving a ref to a task
// and attaching the pull_request evidence. Implemented over work.Service in cmd
// wiring, so this package does not import work.
type Work interface {
	TaskIDByRef(ctx context.Context, ref string) (uuid.UUID, bool, error)
	AttachPullRequest(ctx context.Context, taskID uuid.UUID, pr PullRequest) (EvidenceLink, error)
}

// EvidenceLink is what attaching a merged PR as evidence produced.
type EvidenceLink struct {
	EvidenceID uuid.UUID
	ProjectID  uuid.UUID
}

// Store is the integrations persistence: the delivery log (replay protection)
// and the outbox write for the linked fact.
type Store interface {
	// ClaimDelivery records the id and reports whether it is new; false means
	// the delivery was already seen and must be a no-op.
	ClaimDelivery(ctx context.Context, deliveryID, eventType string) (bool, error)
	// ReleaseDelivery removes a claim after a failure so the provider's retry
	// is processed rather than swallowed.
	ReleaseDelivery(ctx context.Context, deliveryID string) error
	// WriteEvent appends the linked fact to the outbox.
	WriteEvent(ctx context.Context, ev outbox.Event) error
}

// Service applies delivered webhooks.
type Service struct {
	store  Store
	work   Work
	logger *slog.Logger
	now    func() time.Time
}

// NewService builds the integrations service.
func NewService(store Store, work Work, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{store: store, work: work, logger: logger, now: func() time.Time { return time.Now().UTC() }}
}

// HandleGitHub processes one GitHub delivery. The signature is verified by the
// HTTP layer; this method is idempotent by delivery id. A delivery it cannot
// act on returns an Outcome, not an error; only a fate Atlas cannot decide
// (claimed but failed) is an error, and the claim is released so a retry works.
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
