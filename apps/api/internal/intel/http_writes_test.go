package intel

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/safaristack/atlas/apps/api/internal/authz"
	"github.com/safaristack/atlas/apps/api/internal/outbox"
)

// testPrincipal returns an authenticated principal for handler tests.
func testPrincipal() *authz.Principal {
	return &authz.Principal{
		Kind:         authz.KindUser,
		Subject:      "user:test",
		UserID:       uuid.Must(uuid.Parse("00000000-0000-0000-0000-000000000001")),
		PersonID:     uuid.Must(uuid.Parse("00000000-0000-0000-0000-000000000001")),
		Permissions:  map[string]struct{}{},
	}
}

// fakeStore is an in-memory Store for handler tests.
type fakeStore struct {
	decisions   []Decision
	decision    *Decision
	risks       []Risk
	risk        *Risk
	briefs      []Brief
	brief       *Brief
	snapshots   []MetricSnapshot
	snapshot    *MetricSnapshot

	gotDecisionQuery DecisionQuery
	gotRiskQuery     RiskQuery
	gotBriefQuery    BriefQuery
	gotMetricQuery   MetricSnapshotQuery
}

func newFakeStore() *fakeStore {
	return &fakeStore{}
}

func (f *fakeStore) ListDecisions(ctx context.Context, q DecisionQuery, limit int, before *Cursor) ([]Decision, error) {
	f.gotDecisionQuery = q
	return f.decisions, nil
}

func (f *fakeStore) GetDecision(context.Context, uuid.UUID) (*Decision, error) {
	return f.decision, nil
}

func (f *fakeStore) ListRisks(ctx context.Context, q RiskQuery, limit int, before *Cursor) ([]Risk, error) {
	f.gotRiskQuery = q
	return f.risks, nil
}

func (f *fakeStore) GetRisk(context.Context, uuid.UUID) (*Risk, error) {
	return f.risk, nil
}

func (f *fakeStore) ListBriefs(ctx context.Context, q BriefQuery, limit int, before *Cursor) ([]Brief, error) {
	f.gotBriefQuery = q
	return f.briefs, nil
}

func (f *fakeStore) GetBrief(context.Context, uuid.UUID) (*Brief, error) {
	return f.brief, nil
}

func (f *fakeStore) ListMetricSnapshots(ctx context.Context, q MetricSnapshotQuery, limit int, before *Cursor) ([]MetricSnapshot, error) {
	f.gotMetricQuery = q
	return f.snapshots, nil
}

func (f *fakeStore) GetMetricSnapshot(context.Context, uuid.UUID) (*MetricSnapshot, error) {
	return f.snapshot, nil
}

// fakeWriter is an in-memory Writer for write-handler tests.
type fakeWriter struct {
	*fakeStore
	conflict bool
}

func (w *fakeWriter) AllocateRef(_ context.Context, _ string, _ int) (string, error) {
	return "REF-TEST-1", nil
}

func (w *fakeWriter) CreateDecision(_ context.Context, d Decision, _ outbox.Event) (Decision, error) {
	if w.conflict {
		return Decision{}, ErrConflict
	}
	d.Ref, d.Version = "DEC-TEST-1", 1
	d.CreatedAt, d.UpdatedAt = time.Now().UTC(), time.Now().UTC()
	w.fakeStore.decision = &d
	return d, nil
}

func (w *fakeWriter) UpdateDecision(_ context.Context, d Decision, expectedVersion int, _ outbox.Event) (Decision, error) {
	if w.conflict {
		return Decision{}, ErrConflict
	}
	if w.decision != nil && w.decision.Version != expectedVersion {
		return Decision{}, withCode(ErrConflict, CodeDecisionAlreadyDecided)
	}
	d.Version++
	w.fakeStore.decision = &d
	return d, nil
}

func (w *fakeWriter) DecideDecision(_ context.Context, d Decision, expectedVersion int, _ outbox.Event) (Decision, error) {
	if w.conflict {
		return Decision{}, ErrConflict
	}
	if w.decision != nil && w.decision.Version != expectedVersion {
		return Decision{}, withCode(ErrConflict, CodeDecisionAlreadyDecided)
	}
	if w.decision != nil && w.decision.Outcome != DecisionOpen {
		return Decision{}, withCode(ErrForbidden, CodeDecisionNotEditable)
	}
	d.Version++
	w.fakeStore.decision = &d
	return d, nil
}

func (w *fakeWriter) CreateRisk(_ context.Context, r Risk, _ outbox.Event) (Risk, error) {
	if w.conflict {
		return Risk{}, ErrConflict
	}
	r.Ref, r.Version = "RSK-TEST-1", 1
	r.CreatedAt, r.UpdatedAt = time.Now().UTC(), time.Now().UTC()
	w.fakeStore.risk = &r
	return r, nil
}

func (w *fakeWriter) UpdateRisk(_ context.Context, r Risk, expectedVersion int, _ outbox.Event) (Risk, error) {
	if w.conflict {
		return Risk{}, ErrConflict
	}
	if w.risk != nil && w.risk.Version != expectedVersion {
		return Risk{}, withCode(ErrConflict, CodeRiskNotEditable)
	}
	r.Version++
	w.fakeStore.risk = &r
	return r, nil
}

func (w *fakeWriter) CreateBrief(_ context.Context, b Brief, _ outbox.Event) (Brief, error) {
	if w.conflict {
		return Brief{}, ErrConflict
	}
	b.Ref, b.Version = "BRIEF-TEST-1", 1
	b.CreatedAt, b.UpdatedAt = time.Now().UTC(), time.Now().UTC()
	w.fakeStore.brief = &b
	return b, nil
}

func (w *fakeWriter) UpdateBrief(_ context.Context, b Brief, expectedVersion int, _ outbox.Event) (Brief, error) {
	if w.conflict {
		return Brief{}, ErrConflict
	}
	if w.brief != nil && w.brief.Version != expectedVersion {
		return Brief{}, withCode(ErrConflict, CodeBriefNotEditable)
	}
	b.Version++
	w.fakeStore.brief = &b
	return b, nil
}

func (w *fakeWriter) DeliverBrief(_ context.Context, b Brief, expectedVersion int, _ outbox.Event) (Brief, error) {
	if w.conflict {
		return Brief{}, ErrConflict
	}
	if w.brief != nil && w.brief.Version != expectedVersion {
		return Brief{}, withCode(ErrConflict, CodeBriefNotEditable)
	}
	if w.brief != nil && w.brief.Delivered {
		return Brief{}, withCode(ErrConflict, CodeBriefAlreadyDelivered)
	}
	b.Version++
	b.Delivered = true
	now := time.Now().UTC()
	b.DeliveredAt = &now
	w.fakeStore.brief = &b
	return b, nil
}

func (w *fakeWriter) CreateMetricSnapshot(_ context.Context, m MetricSnapshot, _ outbox.Event) (MetricSnapshot, error) {
	if w.conflict {
		return MetricSnapshot{}, ErrConflict
	}
	m.Ref, m.Version = "MET-TEST-1", 1
	m.CreatedAt = time.Now().UTC()
	w.fakeStore.snapshot = &m
	return m, nil
}

func intelWriteRouter(h *Handler) http.Handler {
	r := chi.NewRouter()
	r.Post("/api/v1/intel/decisions", h.CreateDecision)
	r.Put("/api/v1/intel/decisions/{id}", h.UpdateDecision)
	r.Post("/api/v1/intel/decisions/{id}/decide", h.DecideDecision)
	r.Post("/api/v1/intel/risks", h.CreateRisk)
	r.Put("/api/v1/intel/risks/{id}", h.UpdateRisk)
	r.Post("/api/v1/intel/briefs", h.CreateBrief)
	r.Put("/api/v1/intel/briefs/{id}", h.UpdateBrief)
	r.Post("/api/v1/intel/briefs/{id}/deliver", h.DeliverBrief)
	r.Post("/api/v1/intel/metric-snapshots", h.CreateMetricSnapshot)
	return r
}

func intelJSONRequest(method, target, body string, as *authz.Principal) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if as != nil {
		req = req.WithContext(authz.WithPrincipal(req.Context(), as))
	}
	return req
}

func serveIntel(t *testing.T, h *Handler, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	intelWriteRouter(h).ServeHTTP(rec, req)
	return rec
}

func TestCreateDecisionReturns201AndETag(t *testing.T) {
	w := &fakeWriter{fakeStore: &fakeStore{}}
	h := NewHandler(NewService(w, nil), nil)

	rec := serveIntel(t, h, intelJSONRequest(http.MethodPost, "/api/v1/intel/decisions",
		`{"question":"Should we migrate to cloud?","recommendation":"Yes","requested_by":"00000000-0000-0000-0000-000000000001","deciders":["00000000-0000-0000-0000-000000000002"],"due_on":"2026-11-01","outcome":"open"}`,
		testPrincipal()))

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (%s)", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("ETag"); got != "\"1\"" {
		t.Errorf("ETag = %q, want \"1\"", got)
	}
	body := decodeJSON(t, rec)
	if body["question"] != "Should we migrate to cloud?" {
		t.Errorf("body = %v", body)
	}
}

func TestCreateDecisionRequiresQuestion(t *testing.T) {
	w := &fakeWriter{fakeStore: &fakeStore{}}
	h := NewHandler(NewService(w, nil), nil)

	rec := serveIntel(t, h, intelJSONRequest(http.MethodPost, "/api/v1/intel/decisions",
		`{"outcome":"open"}`,
		testPrincipal()))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if code := decodeJSON(t, rec)["code"]; code != "VALIDATION_FAILED" {
		t.Errorf("code = %v, want VALIDATION_FAILED", code)
	}
}

func TestUpdateDecisionReturns200AndETag(t *testing.T) {
	w := &fakeWriter{fakeStore: &fakeStore{decision: &Decision{ID: uuid.Must(uuid.Parse("00000000-0000-0000-0000-000000000001")), Ref: "DEC-TEST-1", Question: "Test", Version: 1, Outcome: DecisionOpen}}}
	h := NewHandler(NewService(w, nil), nil)

	req := intelJSONRequest(http.MethodPut, "/api/v1/intel/decisions/00000000-0000-0000-0000-000000000001",
		`{"question":"Updated","recommendation":"Yes","outcome":"open"}`,
		testPrincipal())
	req.Header.Set("If-Match", "\"1\"")

	rec := serveIntel(t, h, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("ETag"); got != "\"2\"" {
		t.Errorf("ETag = %q, want \"2\"", got)
	}
	body := decodeJSON(t, rec)
	if body["question"] != "Updated" {
		t.Errorf("body = %v", body)
	}
}

func TestUpdateDecisionStaleVersionReturns412(t *testing.T) {
	w := &fakeWriter{fakeStore: &fakeStore{decision: &Decision{ID: uuid.Must(uuid.Parse("00000000-0000-0000-0000-000000000001")), Ref: "DEC-TEST-1", Version: 5}}}
	h := NewHandler(NewService(w, nil), nil)

	req := intelJSONRequest(http.MethodPut, "/api/v1/intel/decisions/00000000-0000-0000-0000-000000000001",
		`{"question":"Updated","recommendation":"Yes","outcome":"open"}`,
		testPrincipal())
	req.Header.Set("If-Match", "\"1\"")

	rec := serveIntel(t, h, req)

	if rec.Code != http.StatusPreconditionFailed {
		t.Fatalf("status = %d, want 412 (%s)", rec.Code, rec.Body.String())
	}
}

func TestDecideDecisionReturns200AndETag(t *testing.T) {
	w := &fakeWriter{fakeStore: &fakeStore{decision: &Decision{ID: uuid.Must(uuid.Parse("00000000-0000-0000-0000-000000000001")), Ref: "DEC-TEST-1", Question: "Test", Version: 1, Outcome: DecisionOpen}}}
	h := NewHandler(NewService(w, nil), nil)

	req := intelJSONRequest(http.MethodPost, "/api/v1/intel/decisions/00000000-0000-0000-0000-000000000001/decide",
		`{"outcome":"approved","rationale":"Looks good"}`,
		testPrincipal())
	req.Header.Set("If-Match", "\"1\"")

	rec := serveIntel(t, h, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("ETag"); got != "\"2\"" {
		t.Errorf("ETag = %q, want \"2\"", got)
	}
	body := decodeJSON(t, rec)
	if body["outcome"] != "approved" {
		t.Errorf("body = %v", body)
	}
}

func TestDecideDecisionAlreadyDecidedReturns403(t *testing.T) {
	w := &fakeWriter{fakeStore: &fakeStore{decision: &Decision{ID: uuid.Must(uuid.Parse("00000000-0000-0000-0000-000000000001")), Ref: "DEC-TEST-1", Question: "Test", Version: 1, Outcome: DecisionApproved}}}
	h := NewHandler(NewService(w, nil), nil)

	req := intelJSONRequest(http.MethodPost, "/api/v1/intel/decisions/00000000-0000-0000-0000-000000000001/decide",
		`{"outcome":"approved","rationale":"Already decided"}`,
		testPrincipal())
	req.Header.Set("If-Match", "\"1\"")

	rec := serveIntel(t, h, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 (%s)", rec.Code, rec.Body.String())
	}
}

func TestCreateRiskReturns201AndETag(t *testing.T) {
	w := &fakeWriter{fakeStore: &fakeStore{}}
	h := NewHandler(NewService(w, nil), nil)

	rec := serveIntel(t, h, intelJSONRequest(http.MethodPost, "/api/v1/intel/risks",
		`{"title":"Vendor lock-in","kind":"client","owner_person_id":"00000000-0000-0000-0000-000000000001","likelihood":"high","impact":"high","status":"open"}`,
		testPrincipal()))

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (%s)", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("ETag"); got != "\"1\"" {
		t.Errorf("ETag = %q, want \"1\"", got)
	}
	body := decodeJSON(t, rec)
	if body["title"] != "Vendor lock-in" {
		t.Errorf("body = %v", body)
	}
}

func TestCreateRiskRequiresTitle(t *testing.T) {
	w := &fakeWriter{fakeStore: &fakeStore{}}
	h := NewHandler(NewService(w, nil), nil)

	rec := serveIntel(t, h, intelJSONRequest(http.MethodPost, "/api/v1/intel/risks",
		`{"kind":"client","likelihood":"high","impact":"high","status":"open"}`,
		testPrincipal()))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if code := decodeJSON(t, rec)["code"]; code != "VALIDATION_FAILED" {
		t.Errorf("code = %v, want VALIDATION_FAILED", code)
	}
}

func TestCreateMetricSnapshotReturns201AndETag(t *testing.T) {
	w := &fakeWriter{fakeStore: &fakeStore{}}
	h := NewHandler(NewService(w, nil), nil)

	rec := serveIntel(t, h, intelJSONRequest(http.MethodPost, "/api/v1/intel/metric-snapshots",
		`{"metric_name":"revenue","metric_value":"1000000","as_of":"2026-10-01","source":"finance"}`,
		testPrincipal()))

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (%s)", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("ETag"); got != "\"1\"" {
		t.Errorf("ETag = %q, want \"1\"", got)
	}
	body := decodeJSON(t, rec)
	if body["metric_name"] != "revenue" {
		t.Errorf("body = %v", body)
	}
}

func decodeJSON(t *testing.T, rec *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var v map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return v
}
