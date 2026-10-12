package httpserver

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/safaristack/atlas/apps/api/internal/outbox"
	"github.com/safaristack/atlas/apps/api/internal/platform/httpx"
)

// mountDevRoutes registers local-development-only endpoints. They exist to make
// the Phase 0 acceptance demo reproducible (Appendix A): POST /dev/hello writes
// core.system.hello.v1 through the outbox and the browser sees it arrive live.
//
// Nothing in this file may be reachable outside ATLAS_ENV=local.
func mountDevRoutes(r chi.Router, d Deps) {
	r.Post("/dev/hello", devHelloHandler(d))
}

type helloRequest struct {
	Message string `json:"message"`
}

type helloResponse struct {
	EventID string `json:"event_id"`
	Type    string `json:"type"`
	Seq     int64  `json:"seq"`
	Note    string `json:"note"`
}

func devHelloHandler(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// An empty body is fine; the message is a nicety, not a requirement.
		var req helloRequest
		if r.Body != nil {
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
				httpx.WriteProblem(w, r, httpx.BadRequest("Request body must be JSON, e.g. {\"message\":\"hi\"}."))
				return
			}
		}
		if req.Message == "" {
			req.Message = "hello from atlas go"
		}

		event, err := outbox.NewHello(nil, nil, req.Message)
		if err != nil {
			httpx.WriteError(w, r, d.Logger, err)
			return
		}

		// The whole point of §7.1: the event is written in a transaction, and
		// the NOTIFY trigger wakes the orchestrator once that transaction commits.
		tx, err := d.DB.Begin(r.Context())
		if err != nil {
			httpx.WriteError(w, r, d.Logger, err)
			return
		}
		defer func() { _ = tx.Rollback(r.Context()) }()

		seq, err := outbox.Write(r.Context(), tx, event)
		if err != nil {
			httpx.WriteError(w, r, d.Logger, err)
			return
		}
		if err := tx.Commit(r.Context()); err != nil {
			httpx.WriteError(w, r, d.Logger, err)
			return
		}

		d.Logger.InfoContext(r.Context(), "hello event written",
			"event_id", event.ID.String(), "seq", seq, "request_id", httpx.RequestIDFromContext(r.Context()))

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(helloResponse{
			EventID: event.ID.String(),
			Type:    event.Type,
			Seq:     seq,
			Note:    "written to outbox; the orchestrator is notified once this transaction commits",
		})
	}
}
