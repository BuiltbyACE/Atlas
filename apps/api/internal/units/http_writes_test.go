package units

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
	units        []BusinessUnit
	unit         *BusinessUnit
	integrations []UnitIntegration
	integration  *UnitIntegration
	definitions  []UnitIndicatorDefinition
	definition   *UnitIndicatorDefinition

	gotUnitQuery         UnitQuery
	gotIntegrationQuery  IntegrationQuery
	gotDefinitionQuery   IndicatorDefinitionQuery
}

func newFakeStore() *fakeStore {
	return &fakeStore{}
}

func (f *fakeStore) ListUnits(_ context.Context, q UnitQuery, _ int, _ *Cursor) ([]BusinessUnit, error) {
	f.gotUnitQuery = q
	return f.units, nil
}

func (f *fakeStore) GetUnit(context.Context, uuid.UUID) (*BusinessUnit, error) {
	return f.unit, nil
}

func (f *fakeStore) ListIntegrations(_ context.Context, q IntegrationQuery, _ int, _ *Cursor) ([]UnitIntegration, error) {
	f.gotIntegrationQuery = q
	return f.integrations, nil
}

func (f *fakeStore) GetIntegration(context.Context, uuid.UUID) (*UnitIntegration, error) {
	return f.integration, nil
}

func (f *fakeStore) ListIndicatorDefinitions(_ context.Context, q IndicatorDefinitionQuery, _ int, _ *Cursor) ([]UnitIndicatorDefinition, error) {
	f.gotDefinitionQuery = q
	return f.definitions, nil
}

func (f *fakeStore) GetIndicatorDefinition(context.Context, uuid.UUID) (*UnitIndicatorDefinition, error) {
	return f.definition, nil
}

func (f *fakeStore) ListSnapshots(_ context.Context, q SnapshotQuery, _ int, _ *Cursor) ([]UnitSnapshot, error) {
	return nil, nil
}

func (f *fakeStore) GetSnapshot(context.Context, uuid.UUID) (*UnitSnapshot, error) {
	return nil, nil
}

func (f *fakeStore) ListFinanceSummaries(_ context.Context, q FinanceSummaryQuery, _ int, _ *Cursor) ([]UnitFinanceSummary, error) {
	return nil, nil
}

func (f *fakeStore) GetFinanceSummary(context.Context, uuid.UUID) (*UnitFinanceSummary, error) {
	return nil, nil
}

// fakeWriter is an in-memory Writer for write-handler tests.
type fakeWriter struct {
	*fakeStore
	conflict bool
}

func (w *fakeWriter) AllocateRef(_ context.Context, _ string, _ int) (string, error) {
	return "REF-TEST-1", nil
}

func (w *fakeWriter) CreateUnit(_ context.Context, u BusinessUnit, _ outbox.Event) (BusinessUnit, error) {
	if w.conflict {
		return BusinessUnit{}, ErrConflict
	}
	u.Ref, u.Version = "UNI-TEST-1", 1
	u.CreatedAt, u.UpdatedAt = time.Now().UTC(), time.Now().UTC()
	w.fakeStore.unit = &u
	return u, nil
}

func (w *fakeWriter) UpdateUnit(_ context.Context, u BusinessUnit, expectedVersion int, _ outbox.Event) (BusinessUnit, error) {
	if w.conflict {
		return BusinessUnit{}, ErrConflict
	}
	if w.unit != nil && w.unit.Version != expectedVersion {
		return BusinessUnit{}, withCode(ErrConflict, CodeUnitRetired)
	}
	u.Version++
	w.fakeStore.unit = &u
	return u, nil
}

func (w *fakeWriter) CreateIntegration(_ context.Context, i UnitIntegration, _ outbox.Event) (UnitIntegration, error) {
	if w.conflict {
		return UnitIntegration{}, ErrConflict
	}
	i.Version = 1
	i.CreatedAt, i.UpdatedAt = time.Now().UTC(), time.Now().UTC()
	w.fakeStore.integration = &i
	return i, nil
}

func (w *fakeWriter) UpdateIntegration(_ context.Context, i UnitIntegration, expectedVersion int, _ outbox.Event) (UnitIntegration, error) {
	if w.conflict {
		return UnitIntegration{}, ErrConflict
	}
	if w.integration != nil && w.integration.Version != expectedVersion {
		return UnitIntegration{}, withCode(ErrConflict, CodeIntegrationNotEditable)
	}
	i.Version++
	w.fakeStore.integration = &i
	return i, nil
}

func (w *fakeWriter) CreateIndicatorDefinition(_ context.Context, d UnitIndicatorDefinition, _ outbox.Event) (UnitIndicatorDefinition, error) {
	if w.conflict {
		return UnitIndicatorDefinition{}, ErrConflict
	}
	d.Version = 1
	d.CreatedAt, d.UpdatedAt = time.Now().UTC(), time.Now().UTC()
	w.fakeStore.definition = &d
	return d, nil
}

func (w *fakeWriter) UpdateIndicatorDefinition(_ context.Context, d UnitIndicatorDefinition, expectedVersion int, _ outbox.Event) (UnitIndicatorDefinition, error) {
	if w.conflict {
		return UnitIndicatorDefinition{}, ErrConflict
	}
	if w.definition != nil && w.definition.Version != expectedVersion {
		return UnitIndicatorDefinition{}, withCode(ErrConflict, CodeIndicatorNotEditable)
	}
	d.Version++
	w.fakeStore.definition = &d
	return d, nil
}

func unitWriteRouter(h *Handler) http.Handler {
	r := chi.NewRouter()
	r.Post("/api/v1/units", h.CreateUnit)
	r.Put("/api/v1/units/{id}", h.UpdateUnit)
	r.Post("/api/v1/units/integrations", h.CreateIntegration)
	r.Put("/api/v1/units/integrations/{id}", h.UpdateIntegration)
	r.Post("/api/v1/units/indicator-definitions", h.CreateIndicatorDefinition)
	r.Put("/api/v1/units/indicator-definitions/{id}", h.UpdateIndicatorDefinition)
	return r
}

func unitJSONRequest(method, target, body string, as *authz.Principal) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if as != nil {
		req = req.WithContext(authz.WithPrincipal(req.Context(), as))
	}
	return req
}

func serveUnit(t *testing.T, h *Handler, req *http.Request) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	unitWriteRouter(h).ServeHTTP(rec, req)
	return rec
}

func TestCreateUnitReturns201AndETag(t *testing.T) {
	w := &fakeWriter{fakeStore: &fakeStore{}}
	h := NewHandler(NewService(w, nil), nil)

	rec := serveUnit(t, h, unitJSONRequest(http.MethodPost, "/api/v1/units",
		`{"name":"Test Unit","code":"test-unit","kind":"school","owner_person_id":"00000000-0000-0000-0000-000000000001"}`,
		testPrincipal()))

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (%s)", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("ETag"); got != "\"1\"" {
		t.Errorf("ETag = %q, want \"1\"", got)
	}
	body := decodeJSON(t, rec)
	if body["name"] != "Test Unit" || body["code"] != "test-unit" {
		t.Errorf("body = %v", body)
	}
}

func TestCreateUnitRejectsEmptyName(t *testing.T) {
	w := &fakeWriter{fakeStore: &fakeStore{}}
	h := NewHandler(NewService(w, nil), nil)

	rec := serveUnit(t, h, unitJSONRequest(http.MethodPost, "/api/v1/units",
		`{"name":"","code":"test-unit","kind":"school"}`,
		testPrincipal()))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if code := decodeJSON(t, rec)["code"]; code != "VALIDATION_FAILED" {
		t.Errorf("code = %v, want VALIDATION_FAILED", code)
	}
}

func TestCreateUnitRejectsInvalidCode(t *testing.T) {
	w := &fakeWriter{fakeStore: &fakeStore{}}
	h := NewHandler(NewService(w, nil), nil)

	rec := serveUnit(t, h, unitJSONRequest(http.MethodPost, "/api/v1/units",
		`{"name":"Test Unit","code":"bad code","kind":"school"}`,
		testPrincipal()))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if code := decodeJSON(t, rec)["code"]; code != "VALIDATION_FAILED" {
		t.Errorf("code = %v, want VALIDATION_FAILED", code)
	}
}

func TestCreateUnitRejectsInvalidKind(t *testing.T) {
	w := &fakeWriter{fakeStore: &fakeStore{}}
	h := NewHandler(NewService(w, nil), nil)

	rec := serveUnit(t, h, unitJSONRequest(http.MethodPost, "/api/v1/units",
		`{"name":"Test Unit","code":"test-unit","kind":"bogus"}`,
		testPrincipal()))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if code := decodeJSON(t, rec)["code"]; code != "VALIDATION_FAILED" {
		t.Errorf("code = %v, want VALIDATION_FAILED", code)
	}
}

func TestUpdateUnitReturns200AndETag(t *testing.T) {
	w := &fakeWriter{fakeStore: &fakeStore{unit: &BusinessUnit{ID: uuid.Must(uuid.Parse("00000000-0000-0000-0000-000000000001")), Ref: "UNI-TEST-1", Name: "Test", Code: "test-unit", Version: 1}}}
	h := NewHandler(NewService(w, nil), nil)

	req := unitJSONRequest(http.MethodPut, "/api/v1/units/00000000-0000-0000-0000-000000000001",
		`{"name":"Updated","code":"testunit","kind":"school"}`,
		testPrincipal())
	req.Header.Set("If-Match", "\"1\"")

	rec := serveUnit(t, h, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("ETag"); got != "\"2\"" {
		t.Errorf("ETag = %q, want \"2\"", got)
	}
	body := decodeJSON(t, rec)
	if body["name"] != "Updated" {
		t.Errorf("body = %v", body)
	}
}

func TestUpdateUnitStaleVersionReturns412(t *testing.T) {
	w := &fakeWriter{fakeStore: &fakeStore{unit: &BusinessUnit{ID: uuid.Must(uuid.Parse("00000000-0000-0000-0000-000000000001")), Ref: "UNI-TEST-1", Version: 5}}}
	h := NewHandler(NewService(w, nil), nil)

	req := unitJSONRequest(http.MethodPut, "/api/v1/units/00000000-0000-0000-0000-000000000001",
		`{"name":"Updated","code":"test-unit","kind":"school"}`,
		testPrincipal())
	req.Header.Set("If-Match", "\"1\"")

	rec := serveUnit(t, h, req)

	if rec.Code != http.StatusPreconditionFailed {
		t.Fatalf("status = %d, want 412 (%s)", rec.Code, rec.Body.String())
	}
}

func TestCreateIntegrationReturns201AndETag(t *testing.T) {
	w := &fakeWriter{fakeStore: &fakeStore{}}
	h := NewHandler(NewService(w, nil), nil)

	rec := serveUnit(t, h, unitJSONRequest(http.MethodPost, "/api/v1/units/integrations",
		`{"name":"Test Integration","kind":"api","endpoint":"https://api.example.com","credential_ref":"sm://creds/key"}`,
		testPrincipal()))

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (%s)", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("ETag"); got != "\"1\"" {
		t.Errorf("ETag = %q, want \"1\"", got)
	}
	body := decodeJSON(t, rec)
	if body["name"] != "Test Integration" {
		t.Errorf("body = %v", body)
	}
}

func TestCreateIndicatorDefinitionReturns201AndETag(t *testing.T) {
	w := &fakeWriter{fakeStore: &fakeStore{}}
	h := NewHandler(NewService(w, nil), nil)

	rec := serveUnit(t, h, unitJSONRequest(http.MethodPost, "/api/v1/units/indicator-definitions",
		`{"name":"Test Indicator","kind":"count","source_field":"field1","target":"100","warning_threshold":"90","critical_threshold":"80"}`,
		testPrincipal()))

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (%s)", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("ETag"); got != "\"1\"" {
		t.Errorf("ETag = %q, want \"1\"", got)
	}
	body := decodeJSON(t, rec)
	if body["name"] != "Test Indicator" {
		t.Errorf("body = %v", body)
	}
}

func TestUpdateIndicatorDefinitionReturns200AndETag(t *testing.T) {
	w := &fakeWriter{fakeStore: &fakeStore{definition: &UnitIndicatorDefinition{ID: uuid.Must(uuid.Parse("00000000-0000-0000-0000-000000000001")), Name: "Test", Version: 1}}}
	h := NewHandler(NewService(w, nil), nil)

	req := unitJSONRequest(http.MethodPut, "/api/v1/units/indicator-definitions/00000000-0000-0000-0000-000000000001",
		`{"name":"Updated","kind":"count","source_field":"field1","target":"100","warning_threshold":"90","critical_threshold":"80"}`,
		testPrincipal())
	req.Header.Set("If-Match", "\"1\"")

	rec := serveUnit(t, h, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("ETag"); got != "\"2\"" {
		t.Errorf("ETag = %q, want \"2\"", got)
	}
	body := decodeJSON(t, rec)
	if body["name"] != "Updated" {
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
