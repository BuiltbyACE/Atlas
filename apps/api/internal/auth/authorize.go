package auth

import (
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/safaristack/atlas/apps/api/internal/platform/httpx"
)

// PermissionDecision is the answer to an internal authorisation check (§7.5).
type PermissionDecision struct {
	// Allowed is true when the user holds the permission the topic requires.
	Allowed bool `json:"allowed"`
	// Reason is a stable, non-sensitive explanation ("granted",
	// "missing_permission") for logs; clients branch on Allowed, not Reason.
	Reason string `json:"reason"`
}

// Check handles GET /internal/v1/authz/check. The Elixir orchestrator calls it
// when a client joins a project or dashboard topic so that Go remains the
// authority on who may receive what (§7.5, ADR-009). It is service-to-service
// and gated by core.authz.check.
//
// The orchestrator presents a user id and the permission the topic requires
// (for example work.task.read for work:project:{id}); Go resolves the user's
// roles to effective permissions and answers yes or no.
func (h *Handler) Check(w http.ResponseWriter, r *http.Request) {
	userID, err := uuid.Parse(strings.TrimSpace(r.URL.Query().Get("user_id")))
	if err != nil {
		httpx.WriteProblem(w, r, httpx.BadRequest("user_id must be a UUID."))
		return
	}

	permission := strings.TrimSpace(r.URL.Query().Get("permission"))
	if permission == "" {
		httpx.WriteProblem(w, r, httpx.BadRequest("permission is required."))
		return
	}

	decision, err := h.service.CheckPermission(r.Context(), userID, permission)
	if err != nil {
		if errors.Is(err, ErrUserNotFound) {
			httpx.WriteProblem(w, r, httpx.NotFound("No such user."))
			return
		}
		h.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, decision)
}
