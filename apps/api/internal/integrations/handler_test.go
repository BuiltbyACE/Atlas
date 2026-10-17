package integrations

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

func TestReceiveGitHubRequiresConfiguration(t *testing.T) {
	svc := newTestService(newFakeStore(), &fakeWork{refs: map[string]uuid.UUID{}})
	h := NewHandler(svc, "", nil)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", nil)
	rec := httptest.NewRecorder()
	h.ReceiveGitHub(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

func TestReceiveGitHubRejectsBadSignature(t *testing.T) {
	svc := newTestService(newFakeStore(), &fakeWork{refs: map[string]uuid.UUID{}})
	h := NewHandler(svc, "s3cr3t", nil)

	body := mergedPullRequestBody("ship", "PRJ-2026-007/T-0142")
	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewReader(body))
	req.Header.Set(SignatureHeader, "sha256=not-the-signature")
	req.Header.Set(DeliveryHeader, "delivery-x")
	req.Header.Set(EventHeader, EventPullRequest)

	rec := httptest.NewRecorder()
	h.ReceiveGitHub(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestReceiveGitHubAcceptsSignedDelivery(t *testing.T) {
	const secret = "s3cr3t"
	taskID := uuid.New()
	work := &fakeWork{
		refs:       map[string]uuid.UUID{"PRJ-2026-007/T-0142": taskID},
		evidenceID: uuid.New(),
		projectID:  uuid.New(),
	}
	svc := newTestService(newFakeStore(), work)
	h := NewHandler(svc, secret, nil)

	body := mergedPullRequestBody("ship", "PRJ-2026-007/T-0142")
	req := httptest.NewRequest(http.MethodPost, "/webhooks/github", bytes.NewReader(body))
	req.Header.Set(SignatureHeader, signBody(secret, body))
	req.Header.Set(DeliveryHeader, "delivery-y")
	req.Header.Set(EventHeader, EventPullRequest)

	rec := httptest.NewRecorder()
	h.ReceiveGitHub(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d (body %s)", rec.Code, http.StatusAccepted, rec.Body.String())
	}
	var resp struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Status != StatusRecorded {
		t.Fatalf("status = %q, want %q", resp.Status, StatusRecorded)
	}
	if len(work.attached) != 1 || work.attached[0] != taskID {
		t.Fatalf("attached = %v, want [%s]", work.attached, taskID)
	}
}
