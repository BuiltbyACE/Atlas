package integrations

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

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
	if len(store.events) != 1 {
		t.Fatalf("events = %d, want 1", len(store.events))
	}
	if store.events[0].Type != outbox.TypeGitPullRequestMergedV1 {
		t.Fatalf("event type = %q", store.events[0].Type)
	}
	var payload outbox.GitPullRequestMergedPayload
	if err := json.Unmarshal(store.events[0].Payload, &payload); err != nil {
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
	if len(store.events) != 1 {
		t.Fatalf("events = %d, want 1", len(store.events))
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
	if outcome.Status != StatusIgnored {
		t.Fatalf("status = %q, want %q", outcome.Status, StatusIgnored)
	}
	if len(work.attached) != 0 || len(store.events) != 0 {
		t.Fatal("an unmerged PR must not link or emit")
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
	if outcome.Status != StatusUnlinked {
		t.Fatalf("status = %q, want %q", outcome.Status, StatusUnlinked)
	}
	if len(store.events) != 0 {
		t.Fatal("an unlinked PR must not emit an event")
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
