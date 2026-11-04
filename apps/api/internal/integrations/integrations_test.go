package integrations

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/safaristack/atlas/apps/api/internal/outbox"
)

// fakeStore is an in-memory Store for the service tests.
type fakeStore struct {
	claims   map[string]bool
	released []string
	events   []outbox.Event
}

func newFakeStore() *fakeStore { return &fakeStore{claims: map[string]bool{}} }

func (s *fakeStore) ClaimDelivery(_ context.Context, deliveryID, _ string) (bool, error) {
	if s.claims[deliveryID] {
		return false, nil
	}
	s.claims[deliveryID] = true
	return true, nil
}

func (s *fakeStore) ReleaseDelivery(_ context.Context, deliveryID string) error {
	delete(s.claims, deliveryID)
	s.released = append(s.released, deliveryID)
	return nil
}

func (s *fakeStore) WriteEvent(_ context.Context, ev outbox.Event) error {
	s.events = append(s.events, ev)
	return nil
}

// GitHub installations (fake).
func (s *fakeStore) CreateInstallation(_ context.Context, in InstallationInput) (Installation, error) {
	return Installation{
		ID:            uuid.New(),
		InstallationID: in.InstallationID,
		AccountType:    in.AccountType,
		AccountLogin:   in.AccountLogin,
		AccountID:      in.AccountID,
		PersonID:       in.PersonID,
		Status:         in.Status,
		AuthorizedRepos: in.AuthorizedRepos,
	}, nil
}
func (s *fakeStore) GetInstallation(_ context.Context, id uuid.UUID) (*Installation, error) {
	return nil, nil
}
func (s *fakeStore) GetInstallationByGitHubID(_ context.Context, installationID int64) (*Installation, error) {
	return nil, nil
}
func (s *fakeStore) ListInstallations(_ context.Context, q InstallationQuery, limit int, before *Cursor) ([]Installation, error) {
	return nil, nil
}
func (s *fakeStore) UpdateInstallationStatus(_ context.Context, id uuid.UUID, status InstallationStatus) (Installation, error) {
	return Installation{}, nil
}
func (s *fakeStore) LinkPersonToInstallation(_ context.Context, id uuid.UUID, personID uuid.UUID) (Installation, error) {
	return Installation{}, nil
}

// GitHub user mappings (fake).
func (s *fakeStore) CreateUserMapping(_ context.Context, in UserMappingInput) (UserMapping, error) {
	return UserMapping{
		ID:           uuid.New(),
		PersonID:      in.PersonID,
		GitHubUserID:  in.GitHubUserID,
		GitHubLogin:   in.GitHubLogin,
		InstallationID: in.InstallationID,
		IsPrimary:     in.IsPrimary,
	}, nil
}
func (s *fakeStore) GetUserMapping(_ context.Context, id uuid.UUID) (*UserMapping, error) {
	return nil, nil
}
func (s *fakeStore) GetUserMappingByGitHubID(_ context.Context, githubUserID int64) (*UserMapping, error) {
	return nil, nil
}
func (s *fakeStore) ListUserMappings(_ context.Context, q UserMappingQuery) ([]UserMapping, error) {
	return nil, nil
}
func (s *fakeStore) SetPrimaryMapping(_ context.Context, id uuid.UUID) (UserMapping, error) {
	return UserMapping{}, nil
}
func (s *fakeStore) DeleteUserMapping(_ context.Context, id uuid.UUID) error {
	return nil
}

// GitHub repositories (fake).
func (s *fakeStore) CreateRepository(_ context.Context, in RepositoryInput) (Repository, error) {
	return Repository{
		ID:             uuid.New(),
		InstallationID: in.InstallationID,
		GitHubRepoID:   in.GitHubRepoID,
		FullName:       in.FullName,
		Name:           in.Name,
		IsPrivate:      in.IsPrivate,
		ProjectID:      in.ProjectID,
	}, nil
}
func (s *fakeStore) GetRepository(_ context.Context, id uuid.UUID) (*Repository, error) {
	return nil, nil
}
func (s *fakeStore) GetRepositoryByGitHubID(_ context.Context, installationID int64, githubRepoID int64) (*Repository, error) {
	return nil, nil
}
func (s *fakeStore) ListRepositories(_ context.Context, q RepositoryQuery) ([]Repository, error) {
	return nil, nil
}
func (s *fakeStore) LinkRepositoryToProject(_ context.Context, id uuid.UUID, projectID uuid.UUID) (Repository, error) {
	return Repository{}, nil
}
func (s *fakeStore) UpdateLastPushedAt(_ context.Context, id uuid.UUID, pushedAt time.Time) (Repository, error) {
	return Repository{}, nil
}

// GitHub events (fake).
func (s *fakeStore) CreateGitHubEvent(_ context.Context, in GitHubEventInput) (GitHubEvent, error) {
	return GitHubEvent{
		ID:             uuid.New(),
		InstallationID: in.InstallationID,
		DeliveryID:     in.DeliveryID,
		EventType:      in.EventType,
		Action:         in.Action,
		Repository:     in.Repository,
		SenderLogin:    in.SenderLogin,
		SenderID:       in.SenderID,
		Payload:        in.Payload,
	}, nil
}
func (s *fakeStore) GetGitHubEvent(_ context.Context, id uuid.UUID) (*GitHubEvent, error) {
	return nil, nil
}
func (s *fakeStore) GetGitHubEventByDeliveryID(_ context.Context, deliveryID string) (*GitHubEvent, error) {
	return nil, nil
}
func (s *fakeStore) ListGitHubEvents(_ context.Context, q GitHubEventQuery, limit int, before *Cursor) ([]GitHubEvent, error) {
	return nil, nil
}
func (s *fakeStore) MarkProcessed(_ context.Context, id uuid.UUID) error {
	return nil
}

func (s *fakeStore) ListIntegrations(ctx context.Context, q IntegrationQuery, limit int, before *Cursor) ([]Integration, error) {
	return nil, nil
}
func (s *fakeStore) GetIntegration(ctx context.Context, id uuid.UUID) (*Integration, error) {
	return nil, nil
}
func (s *fakeStore) ListWebhookInbox(ctx context.Context, q WebhookInboxQuery, limit int, before *Cursor) ([]WebhookInboxEntry, error) {
	return nil, nil
}
func (s *fakeStore) GetWebhookInbox(ctx context.Context, id uuid.UUID) (*WebhookInboxEntry, error) {
	return nil, nil
}
func (s *fakeStore) ListOutboundLog(ctx context.Context, q OutboundLogQuery, limit int, before *Cursor) ([]OutboundLogEntry, error) {
	return nil, nil
}
func (s *fakeStore) GetOutboundLog(ctx context.Context, id uuid.UUID) (*OutboundLogEntry, error) {
	return nil, nil
}

// fakeWork records the links the service asks for.
type fakeWork struct {
	refs       map[string]uuid.UUID
	attached   []uuid.UUID
	attachErr  error
	projectID  uuid.UUID
	evidenceID uuid.UUID
}

func (w *fakeWork) TaskIDByRef(_ context.Context, ref string) (uuid.UUID, bool, error) {
	id, ok := w.refs[ref]
	return id, ok, nil
}

func (w *fakeWork) AttachPullRequest(_ context.Context, taskID uuid.UUID, _ PullRequest) (EvidenceLink, error) {
	if w.attachErr != nil {
		return EvidenceLink{}, w.attachErr
	}
	w.attached = append(w.attached, taskID)
	return EvidenceLink{EvidenceID: w.evidenceID, ProjectID: w.projectID}, nil
}

func mergedPullRequestBody(title, headRef string) []byte {
	body, _ := json.Marshal(map[string]any{
		"action":     "closed",
		"number":     7,
		"repository": map[string]any{"full_name": "safaristack/atlas"},
		"pull_request": map[string]any{
			"html_url":         "https://github.com/safaristack/atlas/pull/7",
			"title":            title,
			"merged":           true,
			"merge_commit_sha": "abc123",
			"head":             map[string]any{"ref": headRef},
			"base":             map[string]any{"ref": "main"},
			"user":             map[string]any{"login": "octocat"},
		},
	})
	return body
}

func newTestService(store Store, work Work) *Service {
	return NewService(store, work, nil)
}

func TestHandleGitHubLinksMergedPullRequest(t *testing.T) {
	ctx := context.Background()
	taskID := uuid.New()
	work := &fakeWork{
		refs:       map[string]uuid.UUID{"PRJ-2026-007/T-0142": taskID},
		evidenceID: uuid.New(),
		projectID:  uuid.New(),
	}
	store := newFakeStore()
	svc := newTestService(store, work)

	// The branch is lowercase on purpose: extraction canonicalises it.
	outcome, err := svc.HandleGitHub(ctx, "delivery-1", EventPullRequest,
		mergedPullRequestBody("ship it", "prj-2026-007/t-0142-board"))
	if err != nil {
		t.Fatalf("HandleGitHub: %v", err)
	}
	if outcome.Status != StatusRecorded {
		t.Fatalf("status = %q, want %q", outcome.Status, StatusRecorded)
	}
	if len(work.attached) != 1 || work.attached[0] != taskID {
		t.Fatalf("attached = %v, want [%s]", work.attached, taskID)
	}
	if len(store.events) != 2 {
		t.Fatalf("events = %d, want 2 (generic PR + merged PR link)", len(store.events))
	}
	if store.events[0].Type != outbox.TypeGitHubPullRequestV1 {
		t.Fatalf("first event type = %q, want %q", store.events[0].Type, outbox.TypeGitHubPullRequestV1)
	}
	if store.events[1].Type != outbox.TypeGitPullRequestMergedV1 {
		t.Fatalf("second event type = %q, want %q", store.events[1].Type, outbox.TypeGitPullRequestMergedV1)
	}
	var payload outbox.GitPullRequestMergedPayload
	if err := json.Unmarshal(store.events[1].Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.TaskID != taskID || payload.EvidenceID != work.evidenceID {
		t.Fatalf("payload ids = %s/%s", payload.TaskID, payload.EvidenceID)
	}
	if payload.Repository != "safaristack/atlas" || payload.Number != 7 {
		t.Fatalf("payload pr = %s#%d", payload.Repository, payload.Number)
	}
}

func TestHandleGitHubIsIdempotentByDelivery(t *testing.T) {
	ctx := context.Background()
	work := &fakeWork{
		refs:       map[string]uuid.UUID{"PRJ-2026-007/T-0142": uuid.New()},
		evidenceID: uuid.New(),
	}
	store := newFakeStore()
	svc := newTestService(store, work)

	body := mergedPullRequestBody("ship it", "PRJ-2026-007/T-0142")
	if _, err := svc.HandleGitHub(ctx, "delivery-1", EventPullRequest, body); err != nil {
		t.Fatalf("first delivery: %v", err)
	}
	outcome, err := svc.HandleGitHub(ctx, "delivery-1", EventPullRequest, body)
	if err != nil {
		t.Fatalf("second delivery: %v", err)
	}
	if outcome.Status != StatusDuplicate {
		t.Fatalf("status = %q, want %q", outcome.Status, StatusDuplicate)
	}
	if len(work.attached) != 1 {
		t.Fatalf("attached %d times, want 1", len(work.attached))
	}
	if len(store.events) != 2 {
		t.Fatalf("events = %d, want 2 (generic PR + merged PR link)", len(store.events))
	}
}

func TestHandleGitHubIgnoresUnmerged(t *testing.T) {
	ctx := context.Background()
	work := &fakeWork{refs: map[string]uuid.UUID{}}
	store := newFakeStore()
	svc := newTestService(store, work)

	body, _ := json.Marshal(map[string]any{
		"action":     "opened",
		"repository": map[string]any{"full_name": "safaristack/atlas"},
		"pull_request": map[string]any{
			"title":  "PRJ-2026-007/T-0142 wip",
			"merged": false,
			"head":   map[string]any{"ref": "PRJ-2026-007/T-0142"},
		},
	})
	outcome, err := svc.HandleGitHub(ctx, "delivery-2", EventPullRequest, body)
	if err != nil {
		t.Fatalf("HandleGitHub: %v", err)
	}
	if outcome.Status != StatusRecorded {
		t.Fatalf("status = %q, want %q", outcome.Status, StatusRecorded)
	}
	if len(work.attached) != 0 || len(store.events) != 1 {
		t.Fatalf("an unmerged PR must not link but must store: attached=%d events=%d", len(work.attached), len(store.events))
	}
}

func TestHandleGitHubUnlinkedRef(t *testing.T) {
	ctx := context.Background()
	work := &fakeWork{refs: map[string]uuid.UUID{}}
	store := newFakeStore()
	svc := newTestService(store, work)

	outcome, err := svc.HandleGitHub(ctx, "delivery-3", EventPullRequest,
		mergedPullRequestBody("PRJ-2026-007/T-0999 no such task", "PRJ-2026-007/T-0999"))
	if err != nil {
		t.Fatalf("HandleGitHub: %v", err)
	}
	if outcome.Status != StatusRecorded {
		t.Fatalf("status = %q, want %q", outcome.Status, StatusRecorded)
	}
	if len(store.events) != 1 {
		t.Fatalf("an unlinked PR must store an event: events=%d", len(store.events))
	}
}

func TestHandleGitHubReleasesClaimOnFailure(t *testing.T) {
	ctx := context.Background()
	work := &fakeWork{
		refs:      map[string]uuid.UUID{"PRJ-2026-007/T-0142": uuid.New()},
		attachErr: errors.New("boom"),
	}
	store := newFakeStore()
	svc := newTestService(store, work)

	if _, err := svc.HandleGitHub(ctx, "delivery-4", EventPullRequest,
		mergedPullRequestBody("ship", "PRJ-2026-007/T-0142")); err == nil {
		t.Fatal("expected an error when linking fails")
	}
	if len(store.released) != 1 || store.released[0] != "delivery-4" {
		t.Fatalf("released = %v, want [delivery-4]", store.released)
	}
	// The claim is gone, so the provider's retry is processed rather than
	// swallowed as a duplicate.
	claimed, err := store.ClaimDelivery(ctx, "delivery-4", EventPullRequest)
	if err != nil || !claimed {
		t.Fatalf("claim after release = %v/%v, want true/nil", claimed, err)
	}
}

func TestHandleGitHubRequiresDeliveryID(t *testing.T) {
	svc := newTestService(newFakeStore(), &fakeWork{refs: map[string]uuid.UUID{}})
	if _, err := svc.HandleGitHub(context.Background(), "  ", EventPullRequest, nil); !errors.Is(err, ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
}
