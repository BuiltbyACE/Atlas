package people

import (
	"encoding/base64"
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

// Handler serves the people-module HTTP endpoints for daily reports and leave
// (docs/ATLAS.md §8.5).
type Handler struct {
	service *Service
	logger  *slog.Logger
}

// NewHandler builds the handler.
func NewHandler(service *Service, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{service: service, logger: logger}
}

// ── views ───────────────────────────────────────────────────────────────────

type dailyReportView struct {
	ID          string     `json:"id"`
	ReporterID  string     `json:"reporter_person_id"`
	ReportDate  string     `json:"report_date"`
	Status      string     `json:"status"`
	WorkDone    string     `json:"work_done,omitempty"`
	Blockers    string     `json:"blockers,omitempty"`
	Risks       string     `json:"risks,omitempty"`
	NextDay     string     `json:"next_day,omitempty"`
	HoursWorked *float64   `json:"hours_worked,omitempty"`
	SubmittedAt *time.Time `json:"submitted_at,omitempty"`
	Version     int        `json:"version"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type leaveRequestView struct {
	ID           string     `json:"id"`
	PersonID     string     `json:"person_id"`
	Type         string     `json:"leave_type"`
	FromDate     string     `json:"from_date"`
	ToDate       string     `json:"to_date"`
	Reason       string     `json:"reason,omitempty"`
	Status       string     `json:"status"`
	ApproverID   string     `json:"approver_person_id,omitempty"`
	ApprovedAt   *time.Time `json:"approved_at,omitempty"`
	DeclinedAt   *time.Time `json:"declined_at,omitempty"`
	CancelReason string     `json:"cancel_reason,omitempty"`
	Version      int        `json:"version"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// ── requests ────────────────────────────────────────────────────────────────

type dailyReportReq struct {
	ReportDate  string   `json:"report_date"`
	WorkDone    string   `json:"work_done"`
	Blockers    string   `json:"blockers"`
	Risks       string   `json:"risks"`
	NextDay     string   `json:"next_day"`
	HoursWorked *float64 `json:"hours_worked"`
}

type leaveRequestReq struct {
	Type     string `json:"leave_type"`
	FromDate string `json:"from_date"`
	ToDate   string `json:"to_date"`
	Reason   string `json:"reason"`
}

type leaveDecisionReq struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
}

// ── daily reports ───────────────────────────────────────────────────────────

// SubmitDailyReport handles POST /api/v1/daily-reports.
func (h *Handler) SubmitDailyReport(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	var req dailyReportReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	date, err := optDate(req.ReportDate)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	in := DailyReportInput{
		WorkDone: req.WorkDone, Blockers: req.Blockers, Risks: req.Risks,
		NextDay: req.NextDay, HoursWorked: req.HoursWorked,
	}
	if date != nil {
		in.ReportDate = *date
	}
	rep, err := h.service.SubmitDailyReport(r.Context(), principal.PersonID, in)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(rep.Version))
	httpx.WriteJSON(w, http.StatusCreated, dailyReportOf(rep))
}

// ListDailyReports handles GET /api/v1/daily-reports.
func (h *Handler) ListDailyReports(w http.ResponseWriter, r *http.Request) {
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
	q := ListDailyReportsQuery{Status: DailyReportStatus(strings.TrimSpace(r.URL.Query().Get("status")))}
	if raw := strings.TrimSpace(r.URL.Query().Get("reporter_person_id")); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			httpx.WriteProblem(w, r, httpx.BadRequest("reporter_person_id is not a valid id."))
			return
		}
		q.ReporterID = &id
	}
	if q.FromDate, err = optDate(r.URL.Query().Get("from_date")); err != nil {
		h.writeError(w, r, err)
		return
	}
	if q.ToDate, err = optDate(r.URL.Query().Get("to_date")); err != nil {
		h.writeError(w, r, err)
		return
	}

	items, err := h.service.ListDailyReports(r.Context(), q, limit+1, before)
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
	views := make([]dailyReportView, 0, len(items))
	for _, rep := range items {
		views = append(views, dailyReportOf(rep))
	}
	httpx.WriteJSON(w, http.StatusOK, httpx.NewPage(views, next, limit))
}

// GetTodayDailyReport handles GET /api/v1/daily-reports/today: the caller's
// report for today, or 404 when they have not filed one yet.
func (h *Handler) GetTodayDailyReport(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	rep, err := h.service.TodayReport(r.Context(), principal.PersonID)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(rep.Version))
	httpx.WriteJSON(w, http.StatusOK, dailyReportOf(rep))
}

// GetDailyReport handles GET /api/v1/daily-reports/{id}.
func (h *Handler) GetDailyReport(w http.ResponseWriter, r *http.Request) {
	id, ok := h.idParam(w, r, "id", "daily report")
	if !ok {
		return
	}
	rep, err := h.service.GetDailyReport(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(rep.Version))
	httpx.WriteJSON(w, http.StatusOK, dailyReportOf(rep))
}

// ── leave requests ──────────────────────────────────────────────────────────

// RequestLeave handles POST /api/v1/people/leave-requests.
func (h *Handler) RequestLeave(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	var req leaveRequestReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	from, err := requireDate(req.FromDate, "from_date")
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	to, err := requireDate(req.ToDate, "to_date")
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	l, err := h.service.RequestLeave(r.Context(), principal.PersonID, LeaveRequestInput{
		Type: LeaveType(strings.TrimSpace(req.Type)), FromDate: from, ToDate: to, Reason: req.Reason,
	})
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(l.Version))
	httpx.WriteJSON(w, http.StatusCreated, leaveRequestOf(l))
}

// ListLeaveRequests handles GET /api/v1/people/leave-requests.
func (h *Handler) ListLeaveRequests(w http.ResponseWriter, r *http.Request) {
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
	q := ListLeaveRequestsQuery{Status: LeaveStatus(strings.TrimSpace(r.URL.Query().Get("status")))}
	if raw := strings.TrimSpace(r.URL.Query().Get("person_id")); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			httpx.WriteProblem(w, r, httpx.BadRequest("person_id is not a valid id."))
			return
		}
		q.PersonID = &id
	}
	if q.FromDate, err = optDate(r.URL.Query().Get("from_date")); err != nil {
		h.writeError(w, r, err)
		return
	}
	if q.ToDate, err = optDate(r.URL.Query().Get("to_date")); err != nil {
		h.writeError(w, r, err)
		return
	}

	items, err := h.service.ListLeaveRequests(r.Context(), q, limit+1, before)
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
	views := make([]leaveRequestView, 0, len(items))
	for _, l := range items {
		views = append(views, leaveRequestOf(l))
	}
	httpx.WriteJSON(w, http.StatusOK, httpx.NewPage(views, next, limit))
}

// GetLeaveRequest handles GET /api/v1/people/leave-requests/{id}.
func (h *Handler) GetLeaveRequest(w http.ResponseWriter, r *http.Request) {
	id, ok := h.idParam(w, r, "id", "leave request")
	if !ok {
		return
	}
	l, err := h.service.GetLeaveRequest(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(l.Version))
	httpx.WriteJSON(w, http.StatusOK, leaveRequestOf(l))
}

// DecideLeaveRequest handles POST /api/v1/people/leave-requests/{id}/decide with
// body {"decision": "approve"|"decline", "reason": "..."}.
func (h *Handler) DecideLeaveRequest(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "leave request")
	if !ok {
		return
	}
	var req leaveDecisionReq
	if r.ContentLength != 0 {
		if err := httpx.DecodeJSON(w, r, &req); err != nil {
			h.writeError(w, r, err)
			return
		}
	}
	var approve bool
	switch strings.ToLower(strings.TrimSpace(req.Decision)) {
	case "approve", "approved":
		approve = true
	case "decline", "declined":
		approve = false
	default:
		httpx.WriteProblem(w, r, httpx.BadRequest("decision must be approve or decline."))
		return
	}
	l, err := h.service.DecideLeaveRequest(r.Context(), principal.PersonID, id, approve, req.Reason)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(l.Version))
	httpx.WriteJSON(w, http.StatusOK, leaveRequestOf(l))
}

// CancelLeaveRequest handles POST /api/v1/people/leave-requests/{id}/cancel.
func (h *Handler) CancelLeaveRequest(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "leave request")
	if !ok {
		return
	}
	var req leaveDecisionReq
	if r.ContentLength != 0 {
		if err := httpx.DecodeJSON(w, r, &req); err != nil {
			h.writeError(w, r, err)
			return
		}
	}
	l, err := h.service.CancelLeaveRequest(r.Context(), principal.PersonID, id, req.Reason)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(l.Version))
	httpx.WriteJSON(w, http.StatusOK, leaveRequestOf(l))
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
	id, err := uuid.Parse(strings.TrimSpace(chi.URLParam(r, param)))
	if err != nil {
		httpx.WriteProblem(w, r, httpx.BadRequest("id is not a valid "+what+" id."))
		return uuid.Nil, false
	}
	return id, true
}

func (h *Handler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.WriteProblem(w, r, httpx.NotFound("No such people record."))
	case errors.Is(err, ErrInvalid):
		httpx.WriteProblem(w, r, httpx.BadRequest(err.Error()))
	case errors.Is(err, ErrForbidden):
		httpx.WriteProblem(w, r, httpx.Forbidden(err.Error()))
	case errors.Is(err, ErrConflict):
		httpx.WriteProblem(w, r, httpx.PreconditionFailed(err.Error()))
	default:
		httpx.WriteError(w, r, h.logger, err)
	}
}

// optDate parses a plain calendar date (YYYY-MM-DD) or returns nil.
func optDate(raw string) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	d, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return nil, errInvalidField("date")
	}
	return &d, nil
}

// requireDate parses a required calendar date.
func requireDate(raw, field string) (time.Time, error) {
	d, err := optDate(raw)
	if err != nil {
		return time.Time{}, errInvalidField(field)
	}
	if d == nil {
		return time.Time{}, errInvalidField(field)
	}
	return *d, nil
}

func errInvalidField(field string) error {
	return &fieldError{field: field}
}

type fieldError struct{ field string }

func (e *fieldError) Error() string { return "invalid " + e.field }
func (e *fieldError) Is(target error) bool {
	return target == ErrInvalid
}

func cursorOrNil(cursor string) (*Cursor, error) {
	if cursor == "" {
		return nil, nil
	}
	c, err := decodeCursor(cursor)
	if err != nil {
		return nil, &fieldError{field: "cursor"}
	}
	return c, nil
}

func dailyReportOf(r DailyReport) dailyReportView {
	return dailyReportView{
		ID:          r.ID.String(),
		ReporterID:  r.ReporterID.String(),
		ReportDate:  dateString(r.ReportDate),
		Status:      string(r.Status),
		WorkDone:    r.WorkDone,
		Blockers:    r.Blockers,
		Risks:       r.Risks,
		NextDay:     r.NextDay,
		HoursWorked: r.HoursWorked,
		SubmittedAt: r.SubmittedAt,
		Version:     r.Version,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
	}
}

func leaveRequestOf(l LeaveRequest) leaveRequestView {
	v := leaveRequestView{
		ID:           l.ID.String(),
		PersonID:     l.PersonID.String(),
		Type:         string(l.Type),
		FromDate:     dateString(l.FromDate),
		ToDate:       dateString(l.ToDate),
		Reason:       l.Reason,
		Status:       string(l.Status),
		ApprovedAt:   l.ApprovedAt,
		DeclinedAt:   l.DeclinedAt,
		CancelReason: l.CancelReason,
		Version:      l.Version,
		CreatedAt:    l.CreatedAt,
		UpdatedAt:    l.UpdatedAt,
	}
	if l.ApproverID != nil {
		v.ApproverID = l.ApproverID.String()
	}
	return v
}

func encodeCursor(at time.Time, id uuid.UUID) string {
	raw := strconv.FormatInt(at.UTC().UnixNano(), 10) + ":" + id.String()
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeCursor(encoded string) (*Cursor, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, err
	}
	parts := strings.SplitN(string(decoded), ":", 2)
	if len(parts) != 2 {
		return nil, errors.New("malformed cursor")
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
