package units

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/safaristack/atlas/apps/api/internal/authz"
	"github.com/safaristack/atlas/apps/api/internal/platform/httpx"
)

// Handler serves the units HTTP surface (§8.7).
type Handler struct {
	service *Service
	logger  *slog.Logger
}

// NewHandler builds a units handler.
func NewHandler(service *Service, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{service: service, logger: logger}
}

// ── views ───────────────────────────────────────────────────────────────────

// Every optional field is a pointer with omitempty: an absent value is
// omitted rather than sent as a zero.

type unitView struct {
	ID                 string         `json:"id"`
	Ref                string         `json:"ref"`
	Name               string         `json:"name"`
	Code               string         `json:"code"`
	Kind               string         `json:"kind"`
	Owner              *personRefView `json:"owner,omitempty"`
	Description        string         `json:"description,omitempty"`
	Status             string         `json:"status"`
	IntegrationCount   int            `json:"integration_count"`
	ActiveIntegrations int            `json:"active_integrations"`
	IndicatorCount     int            `json:"indicator_count"`
	Version            int            `json:"version"`
	CreatedAt          string         `json:"created_at"`
	UpdatedAt          string         `json:"updated_at"`
}

type personRefView struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type integrationView struct {
	ID             string `json:"id"`
	UnitID         string `json:"unit_id"`
	UnitName       string `json:"unit_name"`
	Name           string `json:"name"`
	Kind           string `json:"kind"`
	Endpoint       string `json:"endpoint"`
	CredentialRef  string `json:"credential_ref"`
	Schedule       string `json:"schedule,omitempty"`
	LastSyncAt     string `json:"last_sync_at,omitempty"`
	LastSyncStatus string `json:"last_sync_status"`
	Active         bool   `json:"active"`
	Version        int    `json:"version"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

type indicatorDefinitionView struct {
	ID                string `json:"id"`
	UnitID            string `json:"unit_id"`
	UnitName          string `json:"unit_name"`
	Name              string `json:"name"`
	Kind              string `json:"kind"`
	SourceField       string `json:"source_field"`
	Formula           string `json:"formula,omitempty"`
	Target            string `json:"target,omitempty"`
	WarningThreshold  string `json:"warning_threshold,omitempty"`
	CriticalThreshold string `json:"critical_threshold,omitempty"`
	Active            bool   `json:"active"`
	Version           int    `json:"version"`
	CreatedAt         string `json:"created_at"`
	UpdatedAt         string `json:"updated_at"`
}

type snapshotView struct {
	ID            string `json:"id"`
	UnitID        string `json:"unit_id"`
	UnitName      string `json:"unit_name"`
	OnDate        string `json:"on_date"`
	IndicatorID   string `json:"indicator_id"`
	IndicatorName string `json:"indicator_name"`
	Value         string `json:"value"`
	MetTarget     bool   `json:"met_target"`
	Notes         string `json:"notes,omitempty"`
	CreatedAt     string `json:"created_at"`
}

type financeSummaryView struct {
	ID               string `json:"id"`
	UnitID           string `json:"unit_id"`
	UnitName         string `json:"unit_name"`
	PeriodStart      string `json:"period_start"`
	PeriodEnd        string `json:"period_end"`
	IncomeMinor      int64  `json:"income_minor"`
	ExpensesMinor    int64  `json:"expenses_minor"`
	ReceivablesMinor int64  `json:"receivables_minor"`
	CreatedAt        string `json:"created_at"`
	UpdatedAt        string `json:"updated_at"`
}

// ── converters ──────────────────────────────────────────────────────────────

func rfc(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func rfcPtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return rfc(*t)
}

func dayStr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format("2006-01-02")
}

func personRef(id *uuid.UUID, name string) *personRefView {
	if id == nil {
		return nil
	}
	return &personRefView{ID: id.String(), Name: name}
}

func unitOf(u BusinessUnit) unitView {
	return unitView{
		ID:                 u.ID.String(),
		Ref:                u.Ref,
		Name:               u.Name,
		Code:               u.Code,
		Kind:               string(u.Kind),
		Owner:              personRef(u.OwnerPersonID, u.OwnerName),
		Description:        u.Description,
		Status:             string(u.Status),
		IntegrationCount:   u.IntegrationCount,
		ActiveIntegrations: u.ActiveIntegrations,
		IndicatorCount:     u.IndicatorCount,
		Version:            u.Version,
		CreatedAt:          rfc(u.CreatedAt),
		UpdatedAt:          rfc(u.UpdatedAt),
	}
}

func integrationOf(i UnitIntegration) integrationView {
	return integrationView{
		ID:             i.ID.String(),
		UnitID:         i.UnitID.String(),
		UnitName:       i.UnitName,
		Name:           i.Name,
		Kind:           string(i.Kind),
		Endpoint:       i.Endpoint,
		CredentialRef:  i.CredentialRef,
		Schedule:       i.Schedule,
		LastSyncAt:     rfcPtr(i.LastSyncAt),
		LastSyncStatus: string(i.LastSyncStatus),
		Active:         i.Active,
		Version:        i.Version,
		CreatedAt:      rfc(i.CreatedAt),
		UpdatedAt:      rfc(i.UpdatedAt),
	}
}

func indicatorDefinitionOf(d UnitIndicatorDefinition) indicatorDefinitionView {
	return indicatorDefinitionView{
		ID:                d.ID.String(),
		UnitID:            d.UnitID.String(),
		UnitName:          d.UnitName,
		Name:              d.Name,
		Kind:              string(d.Kind),
		SourceField:       d.SourceField,
		Formula:           d.Formula,
		Target:            d.Target,
		WarningThreshold:  d.WarningThreshold,
		CriticalThreshold: d.CriticalThreshold,
		Active:            d.Active,
		Version:           d.Version,
		CreatedAt:         rfc(d.CreatedAt),
		UpdatedAt:         rfc(d.UpdatedAt),
	}
}

func snapshotOf(sp UnitSnapshot) snapshotView {
	return snapshotView{
		ID:            sp.ID.String(),
		UnitID:        sp.UnitID.String(),
		UnitName:      sp.UnitName,
		OnDate:        sp.OnDate.UTC().Format("2006-01-02"),
		IndicatorID:   sp.IndicatorID.String(),
		IndicatorName: sp.IndicatorName,
		Value:         sp.Value,
		MetTarget:     sp.MetTarget,
		Notes:         sp.Notes,
		CreatedAt:     rfc(sp.CreatedAt),
	}
}

func financeSummaryOf(f UnitFinanceSummary) financeSummaryView {
	return financeSummaryView{
		ID:               f.ID.String(),
		UnitID:           f.UnitID.String(),
		UnitName:         f.UnitName,
		PeriodStart:      f.PeriodStart.UTC().Format("2006-01-02"),
		PeriodEnd:        f.PeriodEnd.UTC().Format("2006-01-02"),
		IncomeMinor:      f.IncomeMinor,
		ExpensesMinor:    f.ExpensesMinor,
		ReceivablesMinor: f.ReceivablesMinor,
		CreatedAt:        rfc(f.CreatedAt),
		UpdatedAt:        rfc(f.UpdatedAt),
	}
}

// ── handlers ────────────────────────────────────────────────────────────────

// ListUnits handles GET /api/v1/units.
func (h *Handler) ListUnits(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.principal(w, r); !ok {
		return
	}
	limit, cursor, err := httpx.ParsePage(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	before, err := cursorOrNil(cursor)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	var q UnitQuery
	var ok bool
	if raw := trim(r.URL.Query().Get("kind")); raw != "" {
		k := UnitKind(raw)
		q.Kind = &k
	}
	if raw := trim(r.URL.Query().Get("status")); raw != "" {
		s := UnitStatus(raw)
		q.Status = &s
	}
	if q.OwnerID, ok = h.uuidParam(w, r, "owner_person_id"); !ok {
		return
	}
	if active, ok := h.boolParam(w, r, "active"); !ok {
		return
	} else if trim(r.URL.Query().Get("active")) != "" {
		q.Active = &active
	}
	q.Search = trim(r.URL.Query().Get("q"))

	items, err := h.service.Units(r.Context(), q, limit+1, before)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	next := ""
	if len(items) > limit {
		items = items[:limit]
		last := items[len(items)-1]
		next = encodeCursor(last.CreatedAt, last.ID)
	}
	views := make([]unitView, 0, len(items))
	for _, u := range items {
		views = append(views, unitOf(u))
	}
	httpx.WriteJSON(w, http.StatusOK, httpx.NewPage(views, next, limit))
}

// GetUnit handles GET /api/v1/units/{id}.
func (h *Handler) GetUnit(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.principal(w, r); !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "unit")
	if !ok {
		return
	}
	u, err := h.service.Unit(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(u.Version))
	httpx.WriteJSON(w, http.StatusOK, unitOf(u))
}

// ListIntegrations handles GET /api/v1/units/integrations.
func (h *Handler) ListIntegrations(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.principal(w, r); !ok {
		return
	}
	limit, cursor, err := httpx.ParsePage(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	before, err := cursorOrNil(cursor)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	var q IntegrationQuery
	var ok bool
	if q.UnitID, ok = h.uuidParam(w, r, "unit_id"); !ok {
		return
	}
	if raw := trim(r.URL.Query().Get("kind")); raw != "" {
		k := IntegrationKind(raw)
		q.Kind = &k
	}
	if raw := trim(r.URL.Query().Get("status")); raw != "" {
		s := IntegrationStatus(raw)
		q.Status = &s
	}
	if active, ok := h.boolParam(w, r, "active"); !ok {
		return
	} else if trim(r.URL.Query().Get("active")) != "" {
		q.Active = &active
	}
	q.Search = trim(r.URL.Query().Get("q"))

	items, err := h.service.Integrations(r.Context(), q, limit+1, before)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	next := ""
	if len(items) > limit {
		items = items[:limit]
		last := items[len(items)-1]
		next = encodeCursor(last.CreatedAt, last.ID)
	}
	views := make([]integrationView, 0, len(items))
	for _, i := range items {
		views = append(views, integrationOf(i))
	}
	httpx.WriteJSON(w, http.StatusOK, httpx.NewPage(views, next, limit))
}

// GetIntegration handles GET /api/v1/units/integrations/{id}.
func (h *Handler) GetIntegration(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.principal(w, r); !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "integration")
	if !ok {
		return
	}
	i, err := h.service.Integration(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(i.Version))
	httpx.WriteJSON(w, http.StatusOK, integrationOf(i))
}

// ListIndicatorDefinitions handles GET /api/v1/units/indicator-definitions.
func (h *Handler) ListIndicatorDefinitions(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.principal(w, r); !ok {
		return
	}
	limit, cursor, err := httpx.ParsePage(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	before, err := cursorOrNil(cursor)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	var q IndicatorDefinitionQuery
	var ok bool
	if q.UnitID, ok = h.uuidParam(w, r, "unit_id"); !ok {
		return
	}
	if raw := trim(r.URL.Query().Get("kind")); raw != "" {
		k := IndicatorKind(raw)
		q.Kind = &k
	}
	if active, ok := h.boolParam(w, r, "active"); !ok {
		return
	} else if trim(r.URL.Query().Get("active")) != "" {
		q.Active = &active
	}
	q.Search = trim(r.URL.Query().Get("q"))

	items, err := h.service.IndicatorDefinitions(r.Context(), q, limit+1, before)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	next := ""
	if len(items) > limit {
		items = items[:limit]
		last := items[len(items)-1]
		next = encodeCursor(last.CreatedAt, last.ID)
	}
	views := make([]indicatorDefinitionView, 0, len(items))
	for _, d := range items {
		views = append(views, indicatorDefinitionOf(d))
	}
	httpx.WriteJSON(w, http.StatusOK, httpx.NewPage(views, next, limit))
}

// GetIndicatorDefinition handles GET /api/v1/units/indicator-definitions/{id}.
func (h *Handler) GetIndicatorDefinition(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.principal(w, r); !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "indicator definition")
	if !ok {
		return
	}
	d, err := h.service.IndicatorDefinition(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(d.Version))
	httpx.WriteJSON(w, http.StatusOK, indicatorDefinitionOf(d))
}

// ListSnapshots handles GET /api/v1/units/snapshots.
func (h *Handler) ListSnapshots(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.principal(w, r); !ok {
		return
	}
	limit, cursor, err := httpx.ParsePage(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	before, err := cursorOrNil(cursor)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	var q SnapshotQuery
	var ok bool
	if q.UnitID, ok = h.uuidParam(w, r, "unit_id"); !ok {
		return
	}
	if q.IndicatorID, ok = h.uuidParam(w, r, "indicator_id"); !ok {
		return
	}
	if q.From, ok = h.timeParam(w, r, "from"); !ok {
		return
	}
	if q.To, ok = h.timeParam(w, r, "to"); !ok {
		return
	}
	q.Search = trim(r.URL.Query().Get("q"))

	items, err := h.service.Snapshots(r.Context(), q, limit+1, before)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	next := ""
	if len(items) > limit {
		items = items[:limit]
		last := items[len(items)-1]
		next = encodeCursor(last.CreatedAt, last.ID)
	}
	views := make([]snapshotView, 0, len(items))
	for _, sp := range items {
		views = append(views, snapshotOf(sp))
	}
	httpx.WriteJSON(w, http.StatusOK, httpx.NewPage(views, next, limit))
}

// GetSnapshot handles GET /api/v1/units/snapshots/{id}.
func (h *Handler) GetSnapshot(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.principal(w, r); !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "snapshot")
	if !ok {
		return
	}
	sp, err := h.service.Snapshot(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, snapshotOf(sp))
}

// ListFinanceSummaries handles GET /api/v1/units/finance-summaries.
func (h *Handler) ListFinanceSummaries(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.principal(w, r); !ok {
		return
	}
	limit, cursor, err := httpx.ParsePage(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	before, err := cursorOrNil(cursor)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	var q FinanceSummaryQuery
	var ok bool
	if q.UnitID, ok = h.uuidParam(w, r, "unit_id"); !ok {
		return
	}
	if q.From, ok = h.timeParam(w, r, "from"); !ok {
		return
	}
	if q.To, ok = h.timeParam(w, r, "to"); !ok {
		return
	}
	q.Search = trim(r.URL.Query().Get("q"))

	items, err := h.service.FinanceSummaries(r.Context(), q, limit+1, before)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	next := ""
	if len(items) > limit {
		items = items[:limit]
		last := items[len(items)-1]
		next = encodeCursor(last.CreatedAt, last.ID)
	}
	views := make([]financeSummaryView, 0, len(items))
	for _, f := range items {
		views = append(views, financeSummaryOf(f))
	}
	httpx.WriteJSON(w, http.StatusOK, httpx.NewPage(views, next, limit))
}

// GetFinanceSummary handles GET /api/v1/units/finance-summaries/{id}.
func (h *Handler) GetFinanceSummary(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.principal(w, r); !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "finance summary")
	if !ok {
		return
	}
	f, err := h.service.FinanceSummary(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, financeSummaryOf(f))
}

// ── write stubs ─────────────────────────────────────────────────────────────
//
// Write handlers are stubbed here so the routes compile. The write pass
// fills them in with the unit lifecycle rules (§8.7): sync failures raise
// units.sync.failed.v1, indicator breaches raise units.indicator.breached.v1.

// CreateUnit handles POST /api/v1/units.
func (h *Handler) CreateUnit(w http.ResponseWriter, r *http.Request) {
	httpx.WriteProblem(w, r, httpx.New(http.StatusNotImplemented, "NOT_IMPLEMENTED", "Unit creation is implemented in the Units write pass."))
}

// UpdateUnit handles PUT /api/v1/units/{id}.
func (h *Handler) UpdateUnit(w http.ResponseWriter, r *http.Request) {
	httpx.WriteProblem(w, r, httpx.New(http.StatusNotImplemented, "NOT_IMPLEMENTED", "Unit updates are implemented in the Units write pass."))
}

// CreateIntegration handles POST /api/v1/units/integrations.
func (h *Handler) CreateIntegration(w http.ResponseWriter, r *http.Request) {
	httpx.WriteProblem(w, r, httpx.New(http.StatusNotImplemented, "NOT_IMPLEMENTED", "Integration creation is implemented in the Units write pass."))
}

// UpdateIntegration handles PUT /api/v1/units/integrations/{id}.
func (h *Handler) UpdateIntegration(w http.ResponseWriter, r *http.Request) {
	httpx.WriteProblem(w, r, httpx.New(http.StatusNotImplemented, "NOT_IMPLEMENTED", "Integration updates are implemented in the Units write pass."))
}

// CreateIndicatorDefinition handles POST /api/v1/units/indicator-definitions.
func (h *Handler) CreateIndicatorDefinition(w http.ResponseWriter, r *http.Request) {
	httpx.WriteProblem(w, r, httpx.New(http.StatusNotImplemented, "NOT_IMPLEMENTED", "Indicator definition creation is implemented in the Units write pass."))
}

// UpdateIndicatorDefinition handles PUT /api/v1/units/indicator-definitions/{id}.
func (h *Handler) UpdateIndicatorDefinition(w http.ResponseWriter, r *http.Request) {
	httpx.WriteProblem(w, r, httpx.New(http.StatusNotImplemented, "NOT_IMPLEMENTED", "Indicator definition updates are implemented in the Units write pass."))
}

// ListSnapshots and GetSnapshot are read-only; snapshots are written by the
// sync worker, not by API clients.

// ── helpers ─────────────────────────────────────────────────────────────────

func (h *Handler) principal(w http.ResponseWriter, r *http.Request) (*authz.Principal, bool) {
	principal, ok := authz.PrincipalFrom(r.Context())
	if !ok || !principal.IsUser() {
		httpx.WriteProblem(w, r, httpx.Unauthenticated("Authentication is required."))
		return nil, false
	}
	return principal, true
}

func (h *Handler) idParam(w http.ResponseWriter, r *http.Request, param, what string) (uuid.UUID, bool) {
	id, err := uuid.Parse(trim(chi.URLParam(r, param)))
	if err != nil {
		httpx.WriteProblem(w, r, httpx.BadRequest("id is not a valid "+what+" id."))
		return uuid.Nil, false
	}
	return id, true
}

func (h *Handler) uuidParam(w http.ResponseWriter, r *http.Request, key string) (*uuid.UUID, bool) {
	raw := trim(r.URL.Query().Get(key))
	if raw == "" {
		return nil, true
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		httpx.WriteProblem(w, r, httpx.BadRequest(key+" is not a valid id."))
		return nil, false
	}
	return &id, true
}

func (h *Handler) timeParam(w http.ResponseWriter, r *http.Request, key string) (*time.Time, bool) {
	raw := trim(r.URL.Query().Get(key))
	if raw == "" {
		return nil, true
	}
	if ts, err := time.Parse(time.RFC3339, raw); err == nil {
		ts = ts.UTC()
		return &ts, true
	}
	if d, err := time.Parse("2006-01-02", raw); err == nil {
		return &d, true
	}
	httpx.WriteProblem(w, r, httpx.BadRequest(key+" is not a valid date."))
	return nil, false
}

func (h *Handler) boolParam(w http.ResponseWriter, r *http.Request, key string) (bool, bool) {
	raw := trim(r.URL.Query().Get(key))
	if raw == "" {
		return false, true
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		httpx.WriteProblem(w, r, httpx.BadRequest(key+" must be true or false."))
		return false, false
	}
	return v, true
}

func (h *Handler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.WriteProblem(w, r, httpx.NotFound("No such units record."))
	case errors.Is(err, ErrInvalid):
		code := problemCode(err)
		if code == "" {
			code = httpx.CodeValidationFailed
		}
		httpx.WriteProblem(w, r, httpx.New(http.StatusBadRequest, code, err.Error()))
	case errors.Is(err, ErrForbidden):
		httpx.WriteProblem(w, r, httpx.Forbidden(err.Error()))
	case errors.Is(err, ErrDuplicate):
		httpx.WriteProblem(w, r, httpx.Conflict(err.Error()))
	case errors.Is(err, ErrConflict):
		httpx.WriteProblem(w, r, httpx.PreconditionFailed(err.Error()))
	default:
		httpx.WriteError(w, r, h.logger, err)
	}
}

func cursorOrNil(cursor string) (*Cursor, error) {
	if cursor == "" {
		return nil, nil
	}
	c, err := decodeCursor(cursor)
	if err != nil {
		return nil, errInvalidField("cursor")
	}
	return c, nil
}

func encodeCursor(at time.Time, id uuid.UUID) string {
	return strconv.FormatInt(at.UTC().UnixNano(), 10) + ":" + id.String()
}

func decodeCursor(encoded string) (*Cursor, error) {
	parts := strings.SplitN(encoded, ":", 2)
	if len(parts) != 2 {
		return nil, errors.New("units: malformed cursor")
	}
	nanos, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return nil, err
	}
	id, err := uuid.Parse(parts[1])
	if err != nil {
		return nil, err
	}
	return &Cursor{At: time.Unix(0, nanos).UTC(), ID: id}, nil
}
