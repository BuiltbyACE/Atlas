package intel

import (
	"encoding/json"
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

// ── write handlers ──────────────────────────────────────────────────────────

// CreateDecision handles POST /api/v1/intel/decisions.
func (h *Handler) CreateDecision(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	var req decisionReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	in, err := req.decisionInput()
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	created, err := h.service.CreateDecision(r.Context(), principal.PersonID, in)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(created.Version))
	httpx.WriteJSON(w, http.StatusCreated, decisionOf(created))
}

// UpdateDecision handles PUT /api/v1/intel/decisions/{id}.
func (h *Handler) UpdateDecision(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "decision")
	if !ok {
		return
	}
	current, err := h.service.Decision(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	if err := httpx.RequireIfMatch(r, int64(current.Version)); err != nil {
		h.writeError(w, r, err)
		return
	}
	var req decisionReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	in, err := req.decisionInput()
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	updated, err := h.service.UpdateDecision(r.Context(), principal.PersonID, id, current.Version, in)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(updated.Version))
	httpx.WriteJSON(w, http.StatusOK, decisionOf(updated))
}

// DecideDecision handles POST /api/v1/intel/decisions/{id}/decide.
func (h *Handler) DecideDecision(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "decision")
	if !ok {
		return
	}
	current, err := h.service.Decision(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	if err := httpx.RequireIfMatch(r, int64(current.Version)); err != nil {
		h.writeError(w, r, err)
		return
	}
	var req decideReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	outcome := DecisionOutcome(trim(req.Outcome))
	if !validDecisionOutcome(outcome) {
		h.writeError(w, r, withCode(errInvalidField("outcome"), CodeInvalidDecisionOutcome))
		return
	}
	updated, err := h.service.DecideDecision(r.Context(), principal.PersonID, id, current.Version, outcome, trim(req.Rationale))
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(updated.Version))
	httpx.WriteJSON(w, http.StatusOK, decisionOf(updated))
}

// CreateRisk handles POST /api/v1/intel/risks.
func (h *Handler) CreateRisk(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	var req riskReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	in, err := req.riskInput()
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	created, err := h.service.CreateRisk(r.Context(), principal.PersonID, in)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(created.Version))
	httpx.WriteJSON(w, http.StatusCreated, riskOf(created))
}

// UpdateRisk handles PUT /api/v1/intel/risks/{id}.
func (h *Handler) UpdateRisk(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "risk")
	if !ok {
		return
	}
	current, err := h.service.Risk(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	if err := httpx.RequireIfMatch(r, int64(current.Version)); err != nil {
		h.writeError(w, r, err)
		return
	}
	var req riskReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	in, err := req.riskInput()
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	updated, err := h.service.UpdateRisk(r.Context(), principal.PersonID, id, current.Version, in)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(updated.Version))
	httpx.WriteJSON(w, http.StatusOK, riskOf(updated))
}

// CreateBrief handles POST /api/v1/intel/briefs.
func (h *Handler) CreateBrief(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	var req briefReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	in, err := req.briefInput()
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	created, err := h.service.CreateBrief(r.Context(), principal.PersonID, in)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(created.Version))
	httpx.WriteJSON(w, http.StatusCreated, briefOf(created))
}

// UpdateBrief handles PUT /api/v1/intel/briefs/{id}.
func (h *Handler) UpdateBrief(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "brief")
	if !ok {
		return
	}
	current, err := h.service.Brief(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	if err := httpx.RequireIfMatch(r, int64(current.Version)); err != nil {
		h.writeError(w, r, err)
		return
	}
	var req briefReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	in, err := req.briefInput()
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	updated, err := h.service.UpdateBrief(r.Context(), principal.PersonID, id, current.Version, in)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(updated.Version))
	httpx.WriteJSON(w, http.StatusOK, briefOf(updated))
}

// DeliverBrief handles POST /api/v1/intel/briefs/{id}/deliver.
func (h *Handler) DeliverBrief(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "brief")
	if !ok {
		return
	}
	current, err := h.service.Brief(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	if err := httpx.RequireIfMatch(r, int64(current.Version)); err != nil {
		h.writeError(w, r, err)
		return
	}
	updated, err := h.service.DeliverBrief(r.Context(), principal.PersonID, id, current.Version)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(updated.Version))
	httpx.WriteJSON(w, http.StatusOK, briefOf(updated))
}

// CreateMetricSnapshot handles POST /api/v1/intel/metric-snapshots.
func (h *Handler) CreateMetricSnapshot(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	var req metricSnapshotReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	in, err := req.metricSnapshotInput()
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	created, err := h.service.CreateMetricSnapshot(r.Context(), principal.PersonID, in)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(created.Version))
	httpx.WriteJSON(w, http.StatusCreated, metricSnapshotOf(created))
}

// ── request types and parsing ───────────────────────────────────────────────

type decisionReq struct {
	Question        *string  `json:"question"`
	ContextLinks    []string `json:"context_links"`
	Options         []string `json:"options"`
	Recommendation  string   `json:"recommendation"`
	RequestedBy     *string  `json:"requested_by"`
	RequestedByName string   `json:"requested_by_name"`
	Deciders        []string `json:"deciders"`
	DueOn           *string  `json:"due_on"`
	Outcome         string   `json:"outcome"`
	Rationale       string   `json:"rationale"`
}

func (r decisionReq) decisionInput() (DecisionInput, error) {
	in := DecisionInput{
		Question:        trim(strOr(r.Question, "")),
		ContextLinks:    r.ContextLinks,
		Options:         r.Options,
		Recommendation:  r.Recommendation,
		RequestedByName: r.RequestedByName,
		Deciders:        []uuid.UUID{},
		DueOn:           nil,
		Outcome:         string(DecisionOpen),
		Rationale:       r.Rationale,
	}
	if r.RequestedBy != nil {
		id, err := uuid.Parse(trim(*r.RequestedBy))
		if err != nil {
			return in, errInvalidField("requested_by")
		}
		in.RequestedBy = &id
	}
	for _, raw := range r.Deciders {
		id, err := uuid.Parse(trim(raw))
		if err != nil {
			return in, errInvalidField("deciders")
		}
		in.Deciders = append(in.Deciders, id)
	}
	if r.DueOn != nil {
		t, err := time.Parse("2006-01-02", trim(*r.DueOn))
		if err != nil {
			return in, errInvalidField("due_on")
		}
		in.DueOn = &t
	}
	return in, nil
}

type decideReq struct {
	Outcome   string `json:"outcome"`
	Rationale string `json:"rationale"`
}

type riskReq struct {
	Title         *string `json:"title"`
	Kind          string  `json:"kind"`
	OwnerPersonID *string `json:"owner_person_id"`
	Likelihood    string  `json:"likelihood"`
	Impact        string  `json:"impact"`
	Mitigation    string  `json:"mitigation"`
	Status        string  `json:"status"`
}

func (r riskReq) riskInput() (RiskInput, error) {
	in := RiskInput{
		Title:         trim(strOr(r.Title, "")),
		Kind:          trim(r.Kind),
		OwnerPersonID: nil,
		Likelihood:    trim(r.Likelihood),
		Impact:        trim(r.Impact),
		Mitigation:    r.Mitigation,
		Status:        string(RiskOpen),
	}
	if r.OwnerPersonID != nil {
		id, err := uuid.Parse(trim(*r.OwnerPersonID))
		if err != nil {
			return in, errInvalidField("owner_person_id")
		}
		in.OwnerPersonID = &id
	}
	return in, nil
}

type briefReq struct {
	ForDate  *string `json:"for_date"`
	Summary  string  `json:"summary"`
	Sections string  `json:"sections"`
}

func (r briefReq) briefInput() (BriefInput, error) {
	in := BriefInput{
		ForDate:  time.Now().UTC(),
		Summary:  r.Summary,
		Sections: map[string]any{},
	}
	if r.ForDate != nil {
		t, err := time.Parse("2006-01-02", trim(*r.ForDate))
		if err != nil {
			return in, errInvalidField("for_date")
		}
		in.ForDate = t
	}
	if r.Sections != "" {
		if err := json.Unmarshal([]byte(r.Sections), &in.Sections); err != nil {
			return in, errInvalidField("sections")
		}
	}
	return in, nil
}

type metricSnapshotReq struct {
	MetricName  string  `json:"metric_name"`
	MetricValue string  `json:"metric_value"`
	AsOf        *string `json:"as_of"`
	Source      string  `json:"source"`
}

func (r metricSnapshotReq) metricSnapshotInput() (MetricSnapshotInput, error) {
	in := MetricSnapshotInput{
		MetricName:  trim(r.MetricName),
		MetricValue: trim(r.MetricValue),
		AsOf:        time.Now().UTC(),
		Source:      r.Source,
	}
	if r.AsOf != nil {
		t, err := time.Parse("2006-01-02", trim(*r.AsOf))
		if err != nil {
			return in, errInvalidField("as_of")
		}
		in.AsOf = t
	}
	return in, nil
}

// ── helpers ─────────────────────────────────────────────────────────────────

func strOr(s *string, fallback string) string {
	if s == nil {
		return fallback
	}
	return trim(*s)
}

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
