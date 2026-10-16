package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

func TestCheckPermission(t *testing.T) {
	h := newHarness(t)
	userID := uuid.Must(uuid.NewV7())

	// The fake principal holds core.notification.read.
	decision, err := h.service.CheckPermission(context.Background(), userID, "core.notification.read")
	if err != nil {
		t.Fatalf("CheckPermission: %v", err)
	}
	if !decision.Allowed || decision.Reason != "granted" {
		t.Fatalf("held permission = %+v, want allowed/granted", decision)
	}

	decision, err = h.service.CheckPermission(context.Background(), userID, "work.task.read")
	if err != nil {
		t.Fatalf("CheckPermission: %v", err)
	}
	if decision.Allowed || decision.Reason != "missing_permission" {
		t.Fatalf("absent permission = %+v, want denied/missing_permission", decision)
	}

	h.store.principalErr = ErrUserNotFound
	if _, err := h.service.CheckPermission(context.Background(), userID, "work.task.read"); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("unknown user: err = %v, want ErrUserNotFound", err)
	}
}

func TestCheckHandler(t *testing.T) {
	h := newHarness(t)
	handler := NewHandler(h.service, HTTPConfig{CookieName: "atlas_session"}, nil)
	userID := uuid.Must(uuid.NewV7())

	t.Run("allowed", func(t *testing.T) {
		rec := httptest.NewRecorder()
		handler.Check(rec, checkRequest(userID.String(), "core.notification.read"))

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
		}
		var decision PermissionDecision
		if err := json.Unmarshal(rec.Body.Bytes(), &decision); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if !decision.Allowed {
			t.Fatalf("decision = %+v, want allowed", decision)
		}
	})

	t.Run("denied", func(t *testing.T) {
		rec := httptest.NewRecorder()
		handler.Check(rec, checkRequest(userID.String(), "work.project.manage"))

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
		}
		var decision PermissionDecision
		if err := json.Unmarshal(rec.Body.Bytes(), &decision); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if decision.Allowed {
			t.Fatalf("decision = %+v, want denied", decision)
		}
	})

	t.Run("bad user id", func(t *testing.T) {
		rec := httptest.NewRecorder()
		handler.Check(rec, checkRequest("not-a-uuid", "work.task.read"))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rec.Code)
		}
	})

	t.Run("missing permission", func(t *testing.T) {
		rec := httptest.NewRecorder()
		handler.Check(rec, checkRequest(userID.String(), ""))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", rec.Code)
		}
	})

	t.Run("unknown user", func(t *testing.T) {
		h.store.principalErr = ErrUserNotFound
		defer func() { h.store.principalErr = nil }()

		rec := httptest.NewRecorder()
		handler.Check(rec, checkRequest(userID.String(), "work.task.read"))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", rec.Code)
		}
	})
}

func checkRequest(userID, permission string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/internal/v1/authz/check", nil)
	q := req.URL.Query()
	q.Set("user_id", userID)
	q.Set("permission", permission)
	req.URL.RawQuery = q.Encode()
	return req
}
