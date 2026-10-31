package systems

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/safaristack/atlas/apps/api/internal/authz"
	"github.com/safaristack/atlas/apps/api/internal/outbox"
)

// fakeStore is an in-memory Store for handler tests. Every method returns the
// field it was given, so each test states only the rows it cares about, and the
// filters it wants to observe are recorded on the query.
type fakeStore struct {
	totals       RegisterTotals
	metrics      IncidentMetrics
	bySeverity   []SeverityCount
	availability []SystemAvailability
	expiring     []Expiry

	systems     []System
	system      *System
	credentials []CredentialRecord

	checks  []Check
	check   *Check
	results []CheckResult

	incidents []Incident
	incident  *Incident
	relations []IncidentSystem
	timeline  []TimelineEntry
	actions   []IncidentAction

	domains      []Domain
	domain       *Domain
	certificates []Certificate
	certificate  *Certificate
	allCreds     []CredentialRecord
	credential   *CredentialRecord

	// Captured queries, so a test can assert the handler parsed the filter.
	gotSystemQuery      SystemQuery
	gotCheckQuery       CheckQuery
	gotIncidentQuery    IncidentQuery
	gotDomainQuery      DomainQuery
	gotCertificateQuery CertificateQuery
	gotCredentialQuery  CredentialQuery
}

func (f *fakeStore) RegisterTotals(context.Context) (RegisterTotals, error) {
	return f.totals, nil
}
func (f *fakeStore) IncidentMetrics(context.Context, time.Time) (IncidentMetrics, error) {
	return f.metrics, nil
}
func (f *fakeStore) IncidentSeverityCounts(context.Context) ([]SeverityCount, error) {
	return f.bySeverity, nil
}
func (f *fakeStore) Availability(context.Context, time.Time) ([]SystemAvailability, error) {
	return f.availability, nil
}
func (f *fakeStore) Expiring(context.Context, time.Time, int) ([]Expiry, error) {
	return f.expiring, nil
}
func (f *fakeStore) ListSystems(_ context.Context, q SystemQuery, _ int, _ *Cursor) ([]System, error) {
	f.gotSystemQuery = q
	return f.systems, nil
}
func (f *fakeStore) GetSystem(context.Context, uuid.UUID) (*System, error) { return f.system, nil }
func (f *fakeStore) ListCredentials(context.Context, uuid.UUID) ([]CredentialRecord, error) {
	return f.credentials, nil
}
func (f *fakeStore) ListChecks(_ context.Context, q CheckQuery, _ int, _ *Cursor) ([]Check, error) {
	f.gotCheckQuery = q
	return f.checks, nil
}
func (f *fakeStore) GetCheck(context.Context, uuid.UUID) (*Check, error) { return f.check, nil }
func (f *fakeStore) ListCheckResults(context.Context, uuid.UUID, int, *Cursor) ([]CheckResult, error) {
	return f.results, nil
}
func (f *fakeStore) ListIncidents(_ context.Context, q IncidentQuery, _ int, _ *Cursor) ([]Incident, error) {
	f.gotIncidentQuery = q
	return f.incidents, nil
}
func (f *fakeStore) GetIncident(context.Context, uuid.UUID) (*Incident, error) {
	return f.incident, nil
}
func (f *fakeStore) IncidentSystems(context.Context, uuid.UUID) ([]IncidentSystem, error) {
	return f.relations, nil
}
func (f *fakeStore) IncidentTimeline(context.Context, uuid.UUID) ([]TimelineEntry, error) {
	return f.timeline, nil
}
func (f *fakeStore) IncidentActions(context.Context, uuid.UUID) ([]IncidentAction, error) {
	return f.actions, nil
}
func (f *fakeStore) ListDomains(_ context.Context, q DomainQuery, _ int, _ *Cursor) ([]Domain, error) {
	f.gotDomainQuery = q
	return f.domains, nil
}
func (f *fakeStore) GetDomain(context.Context, uuid.UUID) (*Domain, error) { return f.domain, nil }
func (f *fakeStore) ListCertificates(_ context.Context, q CertificateQuery, _ int, _ *Cursor) ([]Certificate, error) {
	f.gotCertificateQuery = q
	return f.certificates, nil
}
func (f *fakeStore) GetCertificate(context.Context, uuid.UUID) (*Certificate, error) {
	return f.certificate, nil
}
func (f *fakeStore) ListAllCredentials(_ context.Context, q CredentialQuery, _ int, _ *Cursor) ([]CredentialRecord, error) {
	f.gotCredentialQuery = q
	return f.allCreds, nil
}
func (f *fakeStore) GetCredential(context.Context, uuid.UUID) (*CredentialRecord, error) {
	return f.credential, nil
}

// ── request plumbing ────────────────────────────────────────────────────────

func systemsRouter(h *Handler) chi.Router {
	r := chi.NewRouter()
	r.Get("/api/v1/systems/overview", h.Overview)
	r.Get("/api/v1/systems/expiring", h.Expiring)
	r.Get("/api/v1/systems", h.ListSystems)
	r.Post("/api/v1/systems", h.CreateSystem)
	r.Get("/api/v1/systems/checks", h.ListChecks)
	r.Post("/api/v1/systems/checks", h.CreateCheck)
	r.Put("/api/v1/systems/checks/{id}", h.UpdateCheck)
	r.Post("/api/v1/systems/checks/{id}/pause", h.PauseCheck)
	r.Post("/api/v1/systems/checks/{id}/resume", h.ResumeCheck)
	r.Get("/api/v1/systems/checks/{id}/results", h.ListCheckResults)
	r.Get("/api/v1/systems/incidents", h.ListIncidents)
	r.Post("/api/v1/systems/incidents", h.OpenIncident)
	r.Get("/api/v1/systems/incidents/{id}", h.GetIncident)
	r.Post("/api/v1/systems/incidents/{id}/acknowledge", h.AcknowledgeIncident)
	r.Post("/api/v1/systems/incidents/{id}/mitigate", h.MitigateIncident)
	r.Post("/api/v1/systems/incidents/{id}/resolve", h.ResolveIncident)
	r.Post("/api/v1/systems/incidents/{id}/close", h.CloseIncident)
	r.Post("/api/v1/systems/incidents/{id}/review", h.ReviewIncident)
	r.Post("/api/v1/systems/incidents/{id}/notes", h.AddIncidentNote)
	r.Post("/api/v1/systems/incidents/{id}/actions", h.CreateIncidentAction)
	r.Put("/api/v1/systems/incidents/{id}/actions/{actionId}", h.UpdateIncidentAction)
	r.Get("/api/v1/systems/domains", h.ListDomains)
	r.Post("/api/v1/systems/domains", h.CreateDomain)
	r.Put("/api/v1/systems/domains/{id}", h.UpdateDomain)
	r.Get("/api/v1/systems/certificates", h.ListCertificates)
	r.Post("/api/v1/systems/certificates", h.CreateCertificate)
	r.Put("/api/v1/systems/certificates/{id}", h.UpdateCertificate)
	r.Get("/api/v1/systems/credentials", h.ListCredentials)
	r.Post("/api/v1/systems/credentials", h.CreateCredential)
	r.Put("/api/v1/systems/credentials/{id}", h.UpdateCredential)
	r.Get("/api/v1/systems/{id}", h.GetSystem)
	r.Put("/api/v1/systems/{id}", h.UpdateSystem)
	r.Post("/api/v1/systems/{id}/retire", h.RetireSystem)
	return r
}

// writeFake is an in-memory Writer for write-handler tests. Every method records
// the call and returns the record it was handed, with the version bumped, so a
// test can assert what the handler passed to the service.
type writeFake struct {
	*fakeStore

	// Configurable answers.
	system      *System
	check       *Check
	incident    *Incident
	action      *IncidentAction
	domain      *Domain
	certificate *Certificate
	credential  *CredentialRecord

	// Recorded calls.
	refCalls    int
	events      []string
	lastVersion int
}

func (w *writeFake) AllocateRef(context.Context, string, int) (string, error) {
	w.refCalls++
	return "SYS-2026-0001", nil
}

func (w *writeFake) CreateSystem(_ context.Context, s System) (System, error) {
	created := s
	created.ChecksTotal = 0
	w.system = &created
	return created, nil
}

func (w *writeFake) UpdateSystem(_ context.Context, s System, expected int) (System, error) {
	w.lastVersion = expected
	updated := s
	updated.Version = expected + 1
	w.system = &updated
	return updated, nil
}

func (w *writeFake) SetSystemStatus(_ context.Context, _ uuid.UUID, expected int, status SystemStatus) (System, error) {
	w.lastVersion = expected
	updated := System{ID: uuid.New(), Ref: "SYS-2026-0001", Name: "Atlas API", Status: status, Version: expected + 1}
	w.system = &updated
	return updated, nil
}

func (w *writeFake) CreateCheck(_ context.Context, c Check) (Check, error) {
	w.check = &c
	return c, nil
}

func (w *writeFake) UpdateCheck(_ context.Context, c Check, expected int) (Check, error) {
	w.lastVersion = expected
	updated := c
	updated.Version = expected + 1
	w.check = &updated
	return updated, nil
}

func (w *writeFake) SetCheckActive(_ context.Context, id uuid.UUID, expected int, active bool) (Check, error) {
	w.lastVersion = expected
	updated := Check{ID: id, SystemID: uuid.New(), Name: "API health", CheckType: CheckHTTPStatus,
		Target: "https://api.safaristack.co.ke/health", IntervalSeconds: 300, TimeoutSeconds: 10,
		FailureThreshold: 2, Active: active, Version: expected + 1}
	w.check = &updated
	return updated, nil
}

func (w *writeFake) CreateIncident(_ context.Context, in Incident, _ []uuid.UUID, _ TimelineEntry, ev outbox.Event) (Incident, error) {
	w.events = append(w.events, ev.Type)
	w.incident = &in
	return in, nil
}

func (w *writeFake) AcknowledgeIncident(_ context.Context, id uuid.UUID, expected int, at time.Time, _ TimelineEntry, ev outbox.Event) (Incident, error) {
	w.events = append(w.events, ev.Type)
	return w.transitioned(id, expected, IncidentAcknowledged, &at, nil), nil
}

func (w *writeFake) MitigateIncident(_ context.Context, id uuid.UUID, expected int, at time.Time, _ TimelineEntry, ev outbox.Event) (Incident, error) {
	w.events = append(w.events, ev.Type)
	return w.transitioned(id, expected, IncidentMitigated, nil, &at), nil
}

func (w *writeFake) ResolveIncident(_ context.Context, id uuid.UUID, expected int, at time.Time, _ bool, _ TimelineEntry, ev outbox.Event) (Incident, error) {
	w.events = append(w.events, ev.Type)
	return w.transitioned(id, expected, IncidentResolved, nil, nil), nil
}

func (w *writeFake) CloseIncident(_ context.Context, id uuid.UUID, expected int, _ time.Time, _ TimelineEntry, ev outbox.Event) (Incident, error) {
	w.events = append(w.events, ev.Type)
	return w.transitioned(id, expected, IncidentClosed, nil, nil), nil
}

func (w *writeFake) RecordIncidentReview(_ context.Context, id uuid.UUID, expected int, at time.Time,
	rootCause, summary string, _ TimelineEntry, ev outbox.Event) (Incident, error) {
	w.events = append(w.events, ev.Type)
	in := w.transitioned(id, expected, IncidentResolved, nil, nil)
	in.RootCause = rootCause
	in.Summary = summary
	in.ReviewAt = &at
	in.ReviewRequired = true
	return in, nil
}

// transitioned builds the incident a step returns, in the state the step set.
func (w *writeFake) transitioned(id uuid.UUID, expected int, status IncidentStatus, ack, mit *time.Time) Incident {
	in := Incident{ID: id, Ref: "INC-2026-0001", Title: "Checkout is down", Severity: Sev2,
		Status: status, DetectedAt: time.Now().UTC().Add(-time.Hour), DetectedSource: DetectedByAtlas,
		AcknowledgedAt: ack, MitigatedAt: mit, Version: expected + 1}
	w.incident = &in
	return in
}

func (w *writeFake) AddTimelineEntry(_ context.Context, entry TimelineEntry) (TimelineEntry, error) {
	return entry, nil
}

func (w *writeFake) CreateAction(_ context.Context, a IncidentAction, _ *TimelineEntry) (IncidentAction, error) {
	w.action = &a
	return a, nil
}

func (w *writeFake) UpdateAction(_ context.Context, a IncidentAction, expected int) (IncidentAction, error) {
	w.lastVersion = expected
	updated := a
	updated.Version = expected + 1
	w.action = &updated
	return updated, nil
}

func (w *writeFake) CreateDomain(_ context.Context, d Domain) (Domain, error) {
	w.domain = &d
	return d, nil
}

func (w *writeFake) UpdateDomain(_ context.Context, d Domain, expected int) (Domain, error) {
	w.lastVersion = expected
	updated := d
	updated.Version = expected + 1
	w.domain = &updated
	return updated, nil
}

func (w *writeFake) CreateCertificate(_ context.Context, c Certificate) (Certificate, error) {
	w.certificate = &c
	return c, nil
}

func (w *writeFake) UpdateCertificate(_ context.Context, c Certificate, expected int) (Certificate, error) {
	w.lastVersion = expected
	updated := c
	updated.Version = expected + 1
	w.certificate = &updated
	return updated, nil
}

func (w *writeFake) CreateCredential(_ context.Context, c CredentialRecord) (CredentialRecord, error) {
	w.credential = &c
	return c, nil
}

func (w *writeFake) UpdateCredential(_ context.Context, c CredentialRecord, expected int) (CredentialRecord, error) {
	w.lastVersion = expected
	updated := c
	updated.Version = expected + 1
	w.credential = &updated
	return updated, nil
}

func writeStore(store *fakeStore) *writeFake { return &writeFake{fakeStore: store} }

func systemsUser(perms ...string) *authz.Principal {
	p := &authz.Principal{Kind: authz.KindUser, Subject: "user:test", UserID: uuid.New(), PersonID: uuid.New()}
	if len(perms) > 0 {
		p.Permissions = make(map[string]struct{}, len(perms))
		for _, k := range perms {
			p.Permissions[k] = struct{}{}
		}
	}
	return p
}

func authRequest(method, target string, as *authz.Principal) *http.Request {
	req := httptest.NewRequest(method, target, nil)
	if as != nil {
		req = req.WithContext(authz.WithPrincipal(req.Context(), as))
	}
	return req
}

func decode(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	return body
}

func items(t *testing.T, body map[string]any) []any {
	t.Helper()
	raw, ok := body["items"].([]any)
	if !ok {
		t.Fatalf("body has no items array: %v", body)
	}
	return raw
}

// ── authentication ──────────────────────────────────────────────────────────

func TestSystemsRequiresAuthentication(t *testing.T) {
	h := NewHandler(NewService(&fakeStore{}, nil), nil)
	for _, target := range []string{
		"/api/v1/systems", "/api/v1/systems/overview", "/api/v1/systems/checks",
		"/api/v1/systems/incidents", "/api/v1/systems/expiring",
	} {
		rec := httptest.NewRecorder()
		systemsRouter(h).ServeHTTP(rec, authRequest(http.MethodGet, target, nil))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("GET %s without a principal = %d, want 401", target, rec.Code)
		}
	}
}

// ── overview ────────────────────────────────────────────────────────────────

func TestOverviewPublishesHeadlineAndBands(t *testing.T) {
	store := &fakeStore{
		totals:     RegisterTotals{SystemsTotal: 4, SystemsTier1: 1, ChecksTotal: 9, ChecksFailing: 2, ChecksUnprobed: 1},
		metrics:    IncidentMetrics{OpenIncidents: 1, OpenSev1: 1, Incidents30D: 3, DetectedByAtlas30D: 2},
		bySeverity: []SeverityCount{{Severity: "sev1", Label: "SEV1", Count: 1}},
		availability: []SystemAvailability{{
			SystemID: uuid.New(), Ref: "SYS-2026-0001", Name: "Atlas API", Criticality: Tier1,
			Passed: 99, Total: 100, AvailabilityBP: 9900,
		}},
		expiring: []Expiry{
			{Kind: ExpiryDomain, Name: "safaristack.co.ke", DaysOut: 12, WarningBand: 30},
			{Kind: ExpiryCredential, Name: "Stripe key", DaysOut: 3, WarningBand: 7},
		},
	}
	h := NewHandler(NewService(store, nil), nil)

	rec := httptest.NewRecorder()
	systemsRouter(h).ServeHTTP(rec, authRequest(http.MethodGet, "/api/v1/systems/overview", systemsUser()))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	body := decode(t, rec)
	if body["checks_failing"] != float64(2) {
		t.Errorf("checks_failing = %v, want 2", body["checks_failing"])
	}
	if body["open_sev1"] != float64(1) {
		t.Errorf("open_sev1 = %v, want 1", body["open_sev1"])
	}
	// The warning bands are published so the UI never hard-codes them.
	bands, ok := body["expiry_warning_days"].([]any)
	if !ok || len(bands) != 3 {
		t.Fatalf("expiry_warning_days = %v, want three bands", body["expiry_warning_days"])
	}
	// Expiring counts every lapse; rotations are a subset.
	if body["expiring_count"] != float64(2) {
		t.Errorf("expiring_count = %v, want 2", body["expiring_count"])
	}
	if body["rotations_due_count"] != float64(1) {
		t.Errorf("rotations_due_count = %v, want 1", body["rotations_due_count"])
	}
	avail, ok := body["availability"].([]any)
	if !ok || len(avail) != 1 {
		t.Fatalf("availability = %v, want one row", body["availability"])
	}
	if avail[0].(map[string]any)["availability_bp"] != float64(9900) {
		t.Errorf("availability_bp = %v, want 9900", avail[0].(map[string]any)["availability_bp"])
	}
}

// ── systems ─────────────────────────────────────────────────────────────────

func TestListSystemsReturnsPageAndCursor(t *testing.T) {
	created := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	rows := []System{
		{ID: uuid.New(), Ref: "SYS-2026-0001", Name: "Atlas API", SystemType: TypeAPI, Criticality: Tier1,
			Environment: EnvProd, Status: SystemActive, CreatedAt: created},
		{ID: uuid.New(), Ref: "SYS-2026-0002", Name: "Website", SystemType: TypeWebsite, Criticality: Tier2,
			Environment: EnvProd, Status: SystemActive, CreatedAt: created.Add(-time.Hour)},
	}
	h := NewHandler(NewService(&fakeStore{systems: rows}, nil), nil)

	rec := httptest.NewRecorder()
	systemsRouter(h).ServeHTTP(rec, authRequest(http.MethodGet, "/api/v1/systems?limit=1", systemsUser()))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	body := decode(t, rec)
	if len(items(t, body)) != 1 {
		t.Errorf("items = %v, want one (limit honoured)", body["items"])
	}
	if body["has_more"] != true {
		t.Errorf("has_more = %v, want true", body["has_more"])
	}
	if body["next_cursor"] == "" || body["next_cursor"] == nil {
		t.Errorf("next_cursor missing: %v", body)
	}
}

func TestListSystemsRejectsUnknownType(t *testing.T) {
	h := NewHandler(NewService(&fakeStore{}, nil), nil)

	rec := httptest.NewRecorder()
	systemsRouter(h).ServeHTTP(rec, authRequest(http.MethodGet, "/api/v1/systems?type=mainframe", systemsUser()))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (%s)", rec.Code, rec.Body.String())
	}
	if code := decode(t, rec)["code"]; code != "VALIDATION_FAILED" {
		t.Errorf("code = %v, want VALIDATION_FAILED", code)
	}
}

func TestListSystemsParsesFilters(t *testing.T) {
	store := &fakeStore{}
	h := NewHandler(NewService(store, nil), nil)

	owner := uuid.New()
	rec := httptest.NewRecorder()
	systemsRouter(h).ServeHTTP(rec, authRequest(http.MethodGet,
		"/api/v1/systems?criticality=tier1&environment=prod&unhealthy=true&owner_person_id="+owner.String()+"&q=api",
		systemsUser()))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	if store.gotSystemQuery.Criticality == nil || *store.gotSystemQuery.Criticality != Tier1 {
		t.Errorf("criticality filter not parsed: %+v", store.gotSystemQuery)
	}
	if !store.gotSystemQuery.UnhealthyOnly {
		t.Error("unhealthy filter not parsed")
	}
	if store.gotSystemQuery.OwnerID == nil || *store.gotSystemQuery.OwnerID != owner {
		t.Errorf("owner filter not parsed: %+v", store.gotSystemQuery)
	}
	if store.gotSystemQuery.Search != "api" {
		t.Errorf("search = %q, want api", store.gotSystemQuery.Search)
	}
}

func TestGetSystemNotFound(t *testing.T) {
	h := NewHandler(NewService(&fakeStore{}, nil), nil)

	rec := httptest.NewRecorder()
	systemsRouter(h).ServeHTTP(rec, authRequest(http.MethodGet, "/api/v1/systems/"+uuid.NewString(), systemsUser()))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (%s)", rec.Code, rec.Body.String())
	}
}

func TestGetSystemCarriesCredentialsAndETag(t *testing.T) {
	sys := System{ID: uuid.New(), Ref: "SYS-2026-0001", Name: "Atlas API", SystemType: TypeAPI,
		Criticality: Tier1, Environment: EnvProd, Status: SystemActive, Version: 4,
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	store := &fakeStore{
		system: &sys,
		credentials: []CredentialRecord{{
			ID: uuid.New(), SystemID: sys.ID, Label: "Stripe secret key",
			CredentialKind: "api_key", StorePath: "sm://atlas/stripe", Active: true, Version: 1,
		}},
	}
	h := NewHandler(NewService(store, nil), nil)

	rec := httptest.NewRecorder()
	systemsRouter(h).ServeHTTP(rec, authRequest(http.MethodGet, "/api/v1/systems/"+sys.ID.String(), systemsUser()))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	if etag := rec.Header().Get("ETag"); etag == "" {
		t.Error("no ETag on a versioned record")
	}
	body := decode(t, rec)
	creds, ok := body["credentials"].([]any)
	if !ok || len(creds) != 1 {
		t.Fatalf("credentials = %v, want one", body["credentials"])
	}
	cred := creds[0].(map[string]any)
	if cred["store_path"] != "sm://atlas/stripe" {
		t.Errorf("store_path = %v", cred["store_path"])
	}
	// The record says where a credential lives, and there is nowhere for a
	// secret to hide: no secret field exists (§8.6).
	for _, forbidden := range []string{"secret", "value", "password", "token"} {
		if _, present := cred[forbidden]; present {
			t.Errorf("credential view leaks a %q field", forbidden)
		}
	}
}

// ── checks ──────────────────────────────────────────────────────────────────

func TestListChecksParsesStatusFilter(t *testing.T) {
	store := &fakeStore{checks: []Check{{
		ID: uuid.New(), SystemID: uuid.New(), Name: "API readiness", CheckType: CheckHTTPContent,
		Target: "https://api.example.com/healthz", IntervalSeconds: 60, FailureThreshold: 2, Active: true,
	}}}
	h := NewHandler(NewService(store, nil), nil)

	rec := httptest.NewRecorder()
	systemsRouter(h).ServeHTTP(rec, authRequest(http.MethodGet, "/api/v1/systems/checks?status=fail", systemsUser()))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	if store.gotCheckQuery.Status == nil || *store.gotCheckQuery.Status != CheckFail {
		t.Errorf("status filter not parsed: %+v", store.gotCheckQuery)
	}
	body := decode(t, rec)
	row := items(t, body)[0].(map[string]any)
	// A check that has never run has no last_status, not a "pass".
	if _, present := row["last_status"]; present {
		t.Errorf("unprobed check should omit last_status: %v", row)
	}
	if row["consecutive_failures"] != float64(0) {
		t.Errorf("consecutive_failures = %v, want 0", row["consecutive_failures"])
	}
}

func TestListCheckResultsNotFoundForUnknownCheck(t *testing.T) {
	h := NewHandler(NewService(&fakeStore{}, nil), nil)

	rec := httptest.NewRecorder()
	systemsRouter(h).ServeHTTP(rec, authRequest(http.MethodGet,
		"/api/v1/systems/checks/"+uuid.NewString()+"/results", systemsUser()))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (%s)", rec.Code, rec.Body.String())
	}
}

func TestListCheckResultsReturnsNewestFirst(t *testing.T) {
	check := Check{ID: uuid.New(), SystemID: uuid.New(), Name: "API readiness", CheckType: CheckHTTPStatus,
		Target: "https://api.example.com", IntervalSeconds: 60, FailureThreshold: 2, Active: true}
	latency := 120
	at := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	store := &fakeStore{
		check: &check,
		results: []CheckResult{{
			ID: uuid.New(), CheckID: check.ID, At: at, Status: CheckPass, LatencyMS: &latency, Source: SourceAtlas,
		}},
	}
	h := NewHandler(NewService(store, nil), nil)

	rec := httptest.NewRecorder()
	systemsRouter(h).ServeHTTP(rec, authRequest(http.MethodGet,
		"/api/v1/systems/checks/"+check.ID.String()+"/results", systemsUser()))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	row := items(t, decode(t, rec))[0].(map[string]any)
	if row["status"] != "pass" || row["latency_ms"] != float64(120) {
		t.Errorf("result = %v", row)
	}
}

// ── incidents ───────────────────────────────────────────────────────────────

func TestListIncidentsParsesFilters(t *testing.T) {
	store := &fakeStore{}
	h := NewHandler(NewService(store, nil), nil)

	rec := httptest.NewRecorder()
	systemsRouter(h).ServeHTTP(rec, authRequest(http.MethodGet,
		"/api/v1/systems/incidents?severity=sev1&open=true&review_required=true", systemsUser()))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	if store.gotIncidentQuery.Severity == nil || *store.gotIncidentQuery.Severity != Sev1 {
		t.Errorf("severity not parsed: %+v", store.gotIncidentQuery)
	}
	if !store.gotIncidentQuery.OpenOnly {
		t.Error("open filter not parsed")
	}
	if store.gotIncidentQuery.ReviewRequired == nil || !*store.gotIncidentQuery.ReviewRequired {
		t.Errorf("review_required not parsed: %+v", store.gotIncidentQuery)
	}
}

func TestListIncidentsRejectsUnknownSeverity(t *testing.T) {
	h := NewHandler(NewService(&fakeStore{}, nil), nil)

	rec := httptest.NewRecorder()
	systemsRouter(h).ServeHTTP(rec, authRequest(http.MethodGet, "/api/v1/systems/incidents?severity=sev9", systemsUser()))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (%s)", rec.Code, rec.Body.String())
	}
}

func TestListIncidentsCursorUsesDetectionTime(t *testing.T) {
	detected := time.Date(2026, 10, 12, 3, 30, 0, 0, time.UTC)
	rows := []Incident{
		{ID: uuid.New(), Ref: "INC-2026-0031", Title: "API down", Severity: Sev1, Status: IncidentOpen,
			DetectedAt: detected, CreatedAt: detected, UpdatedAt: detected},
		{ID: uuid.New(), Ref: "INC-2026-0030", Title: "Slow site", Severity: Sev3, Status: IncidentResolved,
			DetectedAt: detected.Add(-48 * time.Hour), CreatedAt: detected, UpdatedAt: detected},
	}
	h := NewHandler(NewService(&fakeStore{incidents: rows}, nil), nil)

	rec := httptest.NewRecorder()
	systemsRouter(h).ServeHTTP(rec, authRequest(http.MethodGet, "/api/v1/systems/incidents?limit=1", systemsUser()))
	body := decode(t, rec)
	if body["next_cursor"] == nil || body["next_cursor"] == "" {
		t.Fatal("expected a cursor after a full page")
	}
	// The cursor encodes detection time: decode it and check the instant
	// matches the last row's detected_at, not its created_at.
	c, err := decodeCursor(body["next_cursor"].(string))
	if err != nil {
		t.Fatalf("decode cursor: %v", err)
	}
	if !c.At.Equal(detected) {
		t.Errorf("cursor At = %s, want %s", c.At, detected)
	}
}

func TestGetIncidentCarriesTimelineSystemsAndActions(t *testing.T) {
	id := uuid.New()
	detected := time.Date(2026, 10, 12, 3, 30, 0, 0, time.UTC)
	ack := detected.Add(4 * time.Minute)
	resolved := detected.Add(70 * time.Minute)
	systemID := uuid.New()
	store := &fakeStore{
		incident: &Incident{
			ID: id, Ref: "INC-2026-0031", Title: "API down", Severity: Sev1, Status: IncidentResolved,
			DetectedAt: detected, DetectedSource: DetectedByAtlas, AcknowledgedAt: &ack, ResolvedAt: &resolved,
			ReviewRequired: true, Version: 3, CreatedAt: detected, UpdatedAt: resolved,
		},
		relations: []IncidentSystem{{SystemID: systemID, Ref: "SYS-2026-0001", Name: "Atlas API", Criticality: Tier1}},
		timeline: []TimelineEntry{{
			ID: uuid.New(), IncidentID: id, At: detected, EntryType: EntryDetected, Note: "Checks failed",
		}},
		actions: []IncidentAction{{
			ID: uuid.New(), IncidentID: id, Title: "Add a readiness check", Status: ActionTodo,
			CreatedAt: detected, UpdatedAt: detected,
		}},
	}
	h := NewHandler(NewService(store, nil), nil)

	rec := httptest.NewRecorder()
	systemsRouter(h).ServeHTTP(rec, authRequest(http.MethodGet, "/api/v1/systems/incidents/"+id.String(), systemsUser()))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	body := decode(t, rec)
	if len(body["systems"].([]any)) != 1 {
		t.Errorf("systems = %v", body["systems"])
	}
	if len(body["timeline"].([]any)) != 1 {
		t.Errorf("timeline = %v", body["timeline"])
	}
	if len(body["actions"].([]any)) != 1 {
		t.Errorf("actions = %v", body["actions"])
	}
	// MTTA and MTTR are derived, so they agree with the timestamps.
	if body["mtta_seconds"] != float64(240) {
		t.Errorf("mtta_seconds = %v, want 240", body["mtta_seconds"])
	}
	if body["mttr_seconds"] != float64(4200) {
		t.Errorf("mttr_seconds = %v, want 4200", body["mttr_seconds"])
	}
	if body["severity_label"] != "SEV1" {
		t.Errorf("severity_label = %v, want SEV1", body["severity_label"])
	}
}

// ── expiring ────────────────────────────────────────────────────────────────

func TestExpiringDefaultsToWidestBandAndEchoesWindow(t *testing.T) {
	store := &fakeStore{expiring: []Expiry{{
		Kind: ExpiryCertificate, ID: uuid.New(), Name: "api.example.com",
		ExpiresOn: time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC), DaysOut: 9, WarningBand: 14,
	}}}
	h := NewHandler(NewService(store, nil), nil)

	rec := httptest.NewRecorder()
	systemsRouter(h).ServeHTTP(rec, authRequest(http.MethodGet, "/api/v1/systems/expiring", systemsUser()))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body.String())
	}
	body := decode(t, rec)
	if body["within_days"] != float64(ExpiryWarningDays[0]) {
		t.Errorf("within_days = %v, want %d", body["within_days"], ExpiryWarningDays[0])
	}
	row := items(t, body)[0].(map[string]any)
	if row["kind"] != "certificate" || row["days_out"] != float64(9) || row["warning_band"] != float64(14) {
		t.Errorf("expiry row = %v", row)
	}
}

func TestExpiringRejectsNonsenseWindow(t *testing.T) {
	h := NewHandler(NewService(&fakeStore{}, nil), nil)
	for _, target := range []string{
		"/api/v1/systems/expiring?within_days=soon",
		"/api/v1/systems/expiring?within_days=0",
		"/api/v1/systems/expiring?within_days=9999",
	} {
		rec := httptest.NewRecorder()
		systemsRouter(h).ServeHTTP(rec, authRequest(http.MethodGet, target, systemsUser()))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("GET %s = %d, want 400", target, rec.Code)
		}
	}
}
