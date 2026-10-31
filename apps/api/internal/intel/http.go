package intel

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

// Handler serves the intel HTTP surface (§8.8).
type Handler struct {
	service *Service
	logger  *slog.Logger
}

// NewHandler builds an intel handler.
func NewHandler(service *Service, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{service: service, logger: logger}
}

// ── views ───────────────────────────────────────────────────────────────────

type decisionView struct {
	ID             string         `json:"id"`
	Ref            string         `json:"ref"`
	Question       string         `json:"question"`
	ContextLinks   []string       `json:"context_links"`
	Options        []string       `json:"options"`
	Recommendation string         `json:"recommendation,omitempty"`
	RequestedBy    *personRefView `json:"requested_by,omitempty"`
	Deciders       []string       `json:"deciders"`
	DueOn          string         `json:"due_on,omitempty"`
	Outcome        string         `json:"outcome"`
	Rationale      string         `json:"rationale,omitempty"`
	Version        int            `json:"version"`
	CreatedAt      string         `json:"created_at"`
	UpdatedAt      string         `json:"updated_at"`
}

type personRefView struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type riskView struct {
	ID         string         `json:"id"`
	Ref        string         `json:"ref"`
	Title      string         `json:"title"`
	Kind       string         `json:"kind"`
	Owner      *personRefView `json:"owner,omitempty"`
	Likelihood string         `json:"likelihood"`
	Impact     string         `json:"impact"`
	Mitigation string         `json:"mitigation,omitempty"`
	Status     string         `json:"status"`
	Version    int            `json:"version"`
	CreatedAt  string         `json:"created_at"`
	UpdatedAt  string         `json:"updated_at"`
}

type briefView struct {
	ID          string         `json:"id"`
	Ref         string         `json:"ref"`
	ForDate     string         `json:"for_date"`
	Summary     string         `json:"summary,omitempty"`
	Sections    map[string]any `json:"sections,omitempty"`
	Delivered   bool           `json:"delivered"`
	DeliveredAt string         `json:"delivered_at,omitempty"`
	Version     int            `json:"version"`
	CreatedAt   string         `json:"created_at"`
	UpdatedAt   string         `json:"updated_at"`
}

type metricSnapshotView struct {
	ID          string `json:"id"`
	Ref         string `json:"ref"`
	MetricName  string `json:"metric_name"`
	MetricValue string `json:"metric_value"`
	AsOf        string `json:"as_of"`
	Source      string `json:"source,omitempty"`
	Version     int    `json:"version"`
	CreatedAt   string `json:"created_at"`
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

func decisionOf(d Decision) decisionView {
	deciders := make([]string, 0, len(d.Deciders))
	for _, id := range d.Deciders {
		deciders = append(deciders, id.String())
	}
	return decisionView{
		ID:             d.ID.String(),
		Ref:            d.Ref,
		Question:       d.Question,
		ContextLinks:   d.ContextLinks,
		Options:        d.Options,
		Recommendation: d.Recommendation,
		RequestedBy:    personRef(d.RequestedBy, d.RequestedByName),
		Deciders:       deciders,
		DueOn:          dayStr(d.DueOn),
		Outcome:        string(d.Outcome),
		Rationale:      d.Rationale,
		Version:        d.Version,
		CreatedAt:      rfc(d.CreatedAt),
		UpdatedAt:      rfc(d.UpdatedAt),
	}
}

func riskOf(r Risk) riskView {
	return riskView{
		ID:         r.ID.String(),
		Ref:        r.Ref,
		Title:      r.Title,
		Kind:       string(r.Kind),
		Owner:      personRef(r.OwnerPersonID, r.OwnerName),
		Likelihood: string(r.Likelihood),
		Impact:     string(r.Impact),
		Mitigation: r.Mitigation,
		Status:     string(r.Status),
		Version:    r.Version,
		CreatedAt:  rfc(r.CreatedAt),
		UpdatedAt:  rfc(r.UpdatedAt),
	}
}

func briefOf(b Brief) briefView {
	return briefView{
		ID:          b.ID.String(),
		Ref:         b.Ref,
		ForDate:     b.ForDate.UTC().Format("2006-01-02"),
		Summary:     b.Summary,
		Sections:    b.Sections,
		Delivered:   b.Delivered,
		DeliveredAt: rfcPtr(b.DeliveredAt),
		Version:     b.Version,
		CreatedAt:   rfc(b.CreatedAt),
		UpdatedAt:   rfc(b.UpdatedAt),
	}
}

func metricSnapshotOf(m MetricSnapshot) metricSnapshotView {
	return metricSnapshotView{
		ID:          m.ID.String(),
		Ref:         m.Ref,
		MetricName:  m.MetricName,
		MetricValue: m.MetricValue,
		AsOf:        m.AsOf.UTC().Format("2006-01-02"),
		Source:      m.Source,
		Version:     m.Version,
		CreatedAt:   rfc(m.CreatedAt),
	}
}

// ── handlers ────────────────────────────────────────────────────────────────

// ListDecisions handles GET /api/v1/intel/decisions.
func (h *Handler) ListDecisions(w http.ResponseWriter, r *http.Request) {
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
	var q DecisionQuery
	var ok bool
	if raw := trim(r.URL.Query().Get("outcome")); raw != "" {
		o := DecisionOutcome(raw)
		q.Outcome = &o
	}
	if q.Decider, ok = h.uuidParam(w, r, "decider"); !ok {
		return
	}
	if q.From, ok = h.timeParam(w, r, "from"); !ok {
		return
	}
	if q.To, ok = h.timeParam(w, r, "to"); !ok {
		return
	}
	q.Search = trim(r.URL.Query().Get("q"))

	items, err := h.service.Decisions(r.Context(), q, limit+1, before)
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
	views := make([]decisionView, 0, len(items))
	for _, d := range items {
		views = append(views, decisionOf(d))
	}
	httpx.WriteJSON(w, http.StatusOK, httpx.NewPage(views, next, limit))
}

// GetDecision handles GET /api/v1/intel/decisions/{id}.
func (h *Handler) GetDecision(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.principal(w, r); !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "decision")
	if !ok {
		return
	}
	d, err := h.service.Decision(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(d.Version))
	httpx.WriteJSON(w, http.StatusOK, decisionOf(d))
}

// ListRisks handles GET /api/v1/intel/risks.
func (h *Handler) ListRisks(w http.ResponseWriter, r *http.Request) {
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
	var q RiskQuery
	var ok bool
	if raw := trim(r.URL.Query().Get("kind")); raw != "" {
		k := RiskKind(raw)
		q.Kind = &k
	}
	if raw := trim(r.URL.Query().Get("status")); raw != "" {
		s := RiskStatus(raw)
		q.Status = &s
	}
	if q.OwnerID, ok = h.uuidParam(w, r, "owner_person_id"); !ok {
		return
	}
	q.Search = trim(r.URL.Query().Get("q"))

	items, err := h.service.Risks(r.Context(), q, limit+1, before)
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
	views := make([]riskView, 0, len(items))
	for _, r := range items {
		views = append(views, riskOf(r))
	}
	httpx.WriteJSON(w, http.StatusOK, httpx.NewPage(views, next, limit))
}

// GetRisk handles GET /api/v1/intel/risks/{id}.
func (h *Handler) GetRisk(w http.ResponseWriter, req *http.Request) {
	if _, ok := h.principal(w, req); !ok {
		return
	}
	id, ok := h.idParam(w, req, "id", "risk")
	if !ok {
		return
	}
	risk, err := h.service.Risk(req.Context(), id)
	if err != nil {
		h.writeError(w, req, err)
		return
	}
	httpx.SetETag(w, int64(risk.Version))
	httpx.WriteJSON(w, http.StatusOK, riskOf(risk))
}

// ListBriefs handles GET /api/v1/intel/briefs.
func (h *Handler) ListBriefs(w http.ResponseWriter, r *http.Request) {
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
	var q BriefQuery
	var ok bool
	if q.From, ok = h.timeParam(w, r, "from"); !ok {
		return
	}
	if q.To, ok = h.timeParam(w, r, "to"); !ok {
		return
	}
	q.Search = trim(r.URL.Query().Get("q"))

	items, err := h.service.Briefs(r.Context(), q, limit+1, before)
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
	views := make([]briefView, 0, len(items))
	for _, b := range items {
		views = append(views, briefOf(b))
	}
	httpx.WriteJSON(w, http.StatusOK, httpx.NewPage(views, next, limit))
}

// GetBrief handles GET /api/v1/intel/briefs/{id}.
func (h *Handler) GetBrief(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.principal(w, r); !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "brief")
	if !ok {
		return
	}
	b, err := h.service.Brief(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, briefOf(b))
}

// ListMetricSnapshots handles GET /api/v1/intel/metric-snapshots.
func (h *Handler) ListMetricSnapshots(w http.ResponseWriter, r *http.Request) {
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
	var q MetricSnapshotQuery
	var ok bool
	if raw := trim(r.URL.Query().Get("metric_name")); raw != "" {
		q.MetricName = &raw
	}
	if q.From, ok = h.timeParam(w, r, "from"); !ok {
		return
	}
	if q.To, ok = h.timeParam(w, r, "to"); !ok {
		return
	}
	q.Search = trim(r.URL.Query().Get("q"))

	items, err := h.service.MetricSnapshots(r.Context(), q, limit+1, before)
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
	views := make([]metricSnapshotView, 0, len(items))
	for _, m := range items {
		views = append(views, metricSnapshotOf(m))
	}
	httpx.WriteJSON(w, http.StatusOK, httpx.NewPage(views, next, limit))
}

// GetMetricSnapshot handles GET /api/v1/intel/metric-snapshots/{id}.
func (h *Handler) GetMetricSnapshot(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.principal(w, r); !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "metric snapshot")
	if !ok {
		return
	}
	m, err := h.service.MetricSnapshot(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, metricSnapshotOf(m))
}

// ── write stubs ─────────────────────────────────────────────────────────────
//
// Write handlers are stubbed here so the routes compile. The write pass
// fills them in with the decision/risk/brief lifecycle rules (§8.8).

// CreateDecision handles POST /api/v1/intel/decisions.
func (h *Handler) CreateDecision(w http.ResponseWriter, r *http.Request) {
	httpx.WriteProblem(w, r, httpx.New(http.StatusNotImplemented, "NOT_IMPLEMENTED", "Decision creation is implemented in the Intel write pass."))
}

// UpdateDecision handles PUT /api/v1/intel/decisions/{id}.
func (h *Handler) UpdateDecision(w http.ResponseWriter, r *http.Request) {
	httpx.WriteProblem(w, r, httpx.New(http.StatusNotImplemented, "NOT_IMPLEMENTED", "Decision updates are implemented in the Intel write pass."))
}

// DecideDecision handles POST /api/v1/intel/decisions/{id}/decide.
func (h *Handler) DecideDecision(w http.ResponseWriter, r *http.Request) {
	httpx.WriteProblem(w, r, httpx.New(http.StatusNotImplemented, "NOT_IMPLEMENTED", "Decision decision is implemented in the Intel write pass."))
}

// CreateRisk handles POST /api/v1/intel/risks.
func (h *Handler) CreateRisk(w http.ResponseWriter, r *http.Request) {
	httpx.WriteProblem(w, r, httpx.New(http.StatusNotImplemented, "NOT_IMPLEMENTED", "Risk creation is implemented in the Intel write pass."))
}

// UpdateRisk handles PUT /api/v1/intel/risks/{id}.
func (h *Handler) UpdateRisk(w http.ResponseWriter, r *http.Request) {
	httpx.WriteProblem(w, r, httpx.New(http.StatusNotImplemented, "NOT_IMPLEMENTED", "Risk updates are implemented in the Intel write pass."))
}

// CreateBrief handles POST /api/v1/intel/briefs.
func (h *Handler) CreateBrief(w http.ResponseWriter, r *http.Request) {
	httpx.WriteProblem(w, r, httpx.New(http.StatusNotImplemented, "NOT_IMPLEMENTED", "Brief creation is implemented in the Intel write pass."))
}

// UpdateBrief handles PUT /api/v1/intel/briefs/{id}.
func (h *Handler) UpdateBrief(w http.ResponseWriter, r *http.Request) {
	httpx.WriteProblem(w, r, httpx.New(http.StatusNotImplemented, "NOT_IMPLEMENTED", "Brief updates are implemented in the Intel write pass."))
}

// DeliverBrief handles POST /api/v1/intel/briefs/{id}/deliver.
func (h *Handler) DeliverBrief(w http.ResponseWriter, r *http.Request) {
	httpx.WriteProblem(w, r, httpx.New(http.StatusNotImplemented, "NOT_IMPLEMENTED", "Brief delivery is implemented in the Intel write pass."))
}

// CreateMetricSnapshot handles POST /api/v1/intel/metric-snapshots.
func (h *Handler) CreateMetricSnapshot(w http.ResponseWriter, r *http.Request) {
	httpx.WriteProblem(w, r, httpx.New(http.StatusNotImplemented, "NOT_IMPLEMENTED", "Metric snapshot creation is implemented in the Intel write pass."))
}

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

func (h *Handler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.WriteProblem(w, r, httpx.NotFound("No such intel record."))
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
		return nil, errors.New("intel: malformed cursor")
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
