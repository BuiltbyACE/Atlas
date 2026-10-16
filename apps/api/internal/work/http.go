package work

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

// Handler serves the work-orchestration HTTP endpoints (docs/ATLAS.md Phase 2).
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

type projectView struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	Description   string     `json:"description,omitempty"`
	ProjectType   string     `json:"project_type,omitempty"`
	Status        string     `json:"status"`
	OwnerPersonID string     `json:"owner_person_id,omitempty"`
	DepartmentID  string     `json:"department_id,omitempty"`
	StartedAt     *time.Time `json:"started_at,omitempty"`
	DueAt         *time.Time `json:"due_at,omitempty"`
	CompletedAt   *time.Time `json:"completed_at,omitempty"`
	Version       int        `json:"version"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

type milestoneView struct {
	ID          string     `json:"id"`
	ProjectID   string     `json:"project_id"`
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	Status      string     `json:"status"`
	DueAt       *time.Time `json:"due_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	Weight      int        `json:"weight"`
	Version     int        `json:"version"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type taskView struct {
	ID               string     `json:"id"`
	ProjectID        string     `json:"project_id"`
	MilestoneID      string     `json:"milestone_id,omitempty"`
	ParentID         string     `json:"parent_id,omitempty"`
	Title            string     `json:"title"`
	Description      string     `json:"description,omitempty"`
	AssigneePersonID string     `json:"assignee_person_id,omitempty"`
	ReporterPersonID string     `json:"reporter_person_id,omitempty"`
	Status           string     `json:"status"`
	Priority         string     `json:"priority"`
	DueAt            *time.Time `json:"due_at,omitempty"`
	StartedAt        *time.Time `json:"started_at,omitempty"`
	CompletedAt      *time.Time `json:"completed_at,omitempty"`
	Blocked          bool       `json:"blocked"`
	BlockedReason    string     `json:"blocked_reason,omitempty"`
	EstimateHours    *float64   `json:"estimate_hours,omitempty"`
	ActualHours      *float64   `json:"actual_hours,omitempty"`
	Version          int        `json:"version"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

type blockerView struct {
	ID         string     `json:"id"`
	TaskID     string     `json:"task_id"`
	Reason     string     `json:"reason"`
	CreatedBy  string     `json:"created_by,omitempty"`
	ResolvedBy string     `json:"resolved_by,omitempty"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

type memberView struct {
	PersonID string    `json:"person_id"`
	Role     string    `json:"role"`
	JoinedAt time.Time `json:"joined_at"`
}

type evidenceView struct {
	ID          string     `json:"id"`
	TaskID      string     `json:"task_id,omitempty"`
	ProjectID   string     `json:"project_id,omitempty"`
	FileID      string     `json:"file_id"`
	UploaderID  string     `json:"uploader_person_id"`
	Kind        string     `json:"kind,omitempty"`
	Description string     `json:"description,omitempty"`
	VerifiedBy  string     `json:"verified_by_person_id,omitempty"`
	VerifiedAt  *time.Time `json:"verified_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

// ── requests ────────────────────────────────────────────────────────────────

type projectReq struct {
	Name          string  `json:"name"`
	Description   string  `json:"description"`
	ProjectType   string  `json:"project_type"`
	Status        string  `json:"status"`
	OwnerPersonID *string `json:"owner_person_id"`
	DepartmentID  *string `json:"department_id"`
	StartedAt     *string `json:"started_at"`
	DueAt         *string `json:"due_at"`
}

type milestoneReq struct {
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Status      string  `json:"status"`
	DueAt       *string `json:"due_at"`
	Weight      int     `json:"weight"`
}

type taskReq struct {
	MilestoneID      *string  `json:"milestone_id"`
	ParentID         *string  `json:"parent_id"`
	Title            string   `json:"title"`
	Description      string   `json:"description"`
	AssigneePersonID *string  `json:"assignee_person_id"`
	Status           string   `json:"status"`
	Priority         string   `json:"priority"`
	DueAt            *string  `json:"due_at"`
	EstimateHours    *float64 `json:"estimate_hours"`
	ActualHours      *float64 `json:"actual_hours"`
}

type memberReq struct {
	PersonID string `json:"person_id"`
	Role     string `json:"role"`
}

type dependencyReq struct {
	PredecessorID string `json:"predecessor_id"`
	Type          string `json:"dependency_type"`
}

type blockerReq struct {
	Reason string `json:"reason"`
}

type evidenceReq struct {
	FileID      string `json:"file_id"`
	Kind        string `json:"kind"`
	Description string `json:"description"`
}

// ── projects ────────────────────────────────────────────────────────────────

// CreateProject handles POST /api/v1/work/projects.
func (h *Handler) CreateProject(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	var req projectReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	in, err := toProjectInput(req)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	p, err := h.service.CreateProject(r.Context(), principal.PersonID, in)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(p.Version))
	httpx.WriteJSON(w, http.StatusCreated, projectOf(p))
}

// ListProjects handles GET /api/v1/work/projects.
func (h *Handler) ListProjects(w http.ResponseWriter, r *http.Request) {
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
	q := ListProjectsQuery{Status: ProjectStatus(strings.TrimSpace(r.URL.Query().Get("status")))}
	if raw := strings.TrimSpace(r.URL.Query().Get("owner_person_id")); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			httpx.WriteProblem(w, r, httpx.BadRequest("owner_person_id is not a valid id."))
			return
		}
		q.OwnerID = &id
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("department_id")); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			httpx.WriteProblem(w, r, httpx.BadRequest("department_id is not a valid id."))
			return
		}
		q.Department = &id
	}

	items, err := h.service.ListProjects(r.Context(), q, limit+1, before)
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
	views := make([]projectView, 0, len(items))
	for _, p := range items {
		views = append(views, projectOf(p))
	}
	httpx.WriteJSON(w, http.StatusOK, httpx.NewPage(views, next, limit))
}

// GetProject handles GET /api/v1/work/projects/{id}.
func (h *Handler) GetProject(w http.ResponseWriter, r *http.Request) {
	id, ok := h.idParam(w, r, "id", "project")
	if !ok {
		return
	}
	p, err := h.service.GetProject(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(p.Version))
	httpx.WriteJSON(w, http.StatusOK, projectOf(p))
}

// UpdateProject handles PUT /api/v1/work/projects/{id}.
func (h *Handler) UpdateProject(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "project")
	if !ok {
		return
	}
	current, err := h.service.GetProject(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	if err := httpx.RequireIfMatch(r, int64(current.Version)); err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	var req projectReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	in, err := toProjectInput(req)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	p, err := h.service.UpdateProject(r.Context(), principal.PersonID, id, current.Version, in)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(p.Version))
	httpx.WriteJSON(w, http.StatusOK, projectOf(p))
}

// ListMembers handles GET /api/v1/work/projects/{id}/members.
func (h *Handler) ListMembers(w http.ResponseWriter, r *http.Request) {
	id, ok := h.idParam(w, r, "id", "project")
	if !ok {
		return
	}
	members, err := h.service.ListMembers(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	views := make([]memberView, 0, len(members))
	for _, m := range members {
		views = append(views, memberView{PersonID: m.PersonID.String(), Role: m.Role, JoinedAt: m.JoinedAt})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": views})
}

// AddMember handles POST /api/v1/work/projects/{id}/members.
func (h *Handler) AddMember(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "project")
	if !ok {
		return
	}
	var req memberReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	personID, err := uuid.Parse(strings.TrimSpace(req.PersonID))
	if err != nil {
		httpx.WriteProblem(w, r, httpx.BadRequest("person_id is not a valid id."))
		return
	}
	if err := h.service.AddMember(r.Context(), principal.PersonID, id, personID, req.Role); err != nil {
		h.writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// RemoveMember handles DELETE /api/v1/work/projects/{id}/members/{person_id}.
func (h *Handler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "project")
	if !ok {
		return
	}
	personID, err := uuid.Parse(strings.TrimSpace(chi.URLParam(r, "person_id")))
	if err != nil {
		httpx.WriteProblem(w, r, httpx.BadRequest("person_id is not a valid id."))
		return
	}
	if err := h.service.RemoveMember(r.Context(), principal.PersonID, id, personID); err != nil {
		h.writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── milestones ──────────────────────────────────────────────────────────────

// CreateMilestone handles POST /api/v1/work/projects/{id}/milestones.
func (h *Handler) CreateMilestone(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "project")
	if !ok {
		return
	}
	var req milestoneReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	in, err := toMilestoneInput(req)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	m, err := h.service.CreateMilestone(r.Context(), principal.PersonID, id, in)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(m.Version))
	httpx.WriteJSON(w, http.StatusCreated, milestoneOf(m))
}

// ListMilestones handles GET /api/v1/work/projects/{id}/milestones.
func (h *Handler) ListMilestones(w http.ResponseWriter, r *http.Request) {
	id, ok := h.idParam(w, r, "id", "project")
	if !ok {
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
	items, err := h.service.ListMilestones(r.Context(), id, limit+1, before)
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
	views := make([]milestoneView, 0, len(items))
	for _, m := range items {
		views = append(views, milestoneOf(m))
	}
	httpx.WriteJSON(w, http.StatusOK, httpx.NewPage(views, next, limit))
}

// GetMilestone handles GET /api/v1/work/milestones/{id}.
func (h *Handler) GetMilestone(w http.ResponseWriter, r *http.Request) {
	id, ok := h.idParam(w, r, "id", "milestone")
	if !ok {
		return
	}
	m, err := h.service.GetMilestone(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(m.Version))
	httpx.WriteJSON(w, http.StatusOK, milestoneOf(m))
}

// UpdateMilestone handles PUT /api/v1/work/milestones/{id}.
func (h *Handler) UpdateMilestone(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "milestone")
	if !ok {
		return
	}
	current, err := h.service.GetMilestone(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	if err := httpx.RequireIfMatch(r, int64(current.Version)); err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	var req milestoneReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	in, err := toMilestoneInput(req)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	m, err := h.service.UpdateMilestone(r.Context(), principal.PersonID, id, current.Version, in)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(m.Version))
	httpx.WriteJSON(w, http.StatusOK, milestoneOf(m))
}

// ── tasks ───────────────────────────────────────────────────────────────────

// CreateTask handles POST /api/v1/work/projects/{id}/tasks.
func (h *Handler) CreateTask(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "project")
	if !ok {
		return
	}
	var req taskReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	in, err := toTaskInput(req)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	t, err := h.service.CreateTask(r.Context(), principal.PersonID, id, in)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(t.Version))
	httpx.WriteJSON(w, http.StatusCreated, taskOf(t))
}

// ListTasks handles GET /api/v1/work/tasks.
func (h *Handler) ListTasks(w http.ResponseWriter, r *http.Request) {
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
	q := ListTasksQuery{Status: TaskStatus(strings.TrimSpace(r.URL.Query().Get("status")))}
	for _, pf := range []struct {
		param string
		dst   **uuid.UUID
	}{{"project_id", &q.ProjectID}, {"milestone_id", &q.MilestoneID}, {"assignee_person_id", &q.AssigneeID}} {
		if raw := strings.TrimSpace(r.URL.Query().Get(pf.param)); raw != "" {
			id, err := uuid.Parse(raw)
			if err != nil {
				httpx.WriteProblem(w, r, httpx.BadRequest(pf.param+" is not a valid id."))
				return
			}
			*pf.dst = &id
		}
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("blocked")); raw != "" {
		b := strings.EqualFold(raw, "true")
		q.Blocked = &b
	}
	q.Overdue = strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("overdue")), "true")

	items, err := h.service.ListTasks(r.Context(), q, limit+1, before)
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
	views := make([]taskView, 0, len(items))
	for _, t := range items {
		views = append(views, taskOf(t))
	}
	httpx.WriteJSON(w, http.StatusOK, httpx.NewPage(views, next, limit))
}

// GetTask handles GET /api/v1/work/tasks/{id}.
func (h *Handler) GetTask(w http.ResponseWriter, r *http.Request) {
	id, ok := h.idParam(w, r, "id", "task")
	if !ok {
		return
	}
	t, err := h.service.GetTask(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(t.Version))
	httpx.WriteJSON(w, http.StatusOK, taskOf(t))
}

// UpdateTask handles PUT /api/v1/work/tasks/{id}.
func (h *Handler) UpdateTask(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "task")
	if !ok {
		return
	}
	current, err := h.service.GetTask(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	if err := httpx.RequireIfMatch(r, int64(current.Version)); err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	var req taskReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	in, err := toTaskInput(req)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	t, err := h.service.UpdateTask(r.Context(), principal.PersonID, id, current.Version, in)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(t.Version))
	httpx.WriteJSON(w, http.StatusOK, taskOf(t))
}

// AddDependency handles POST /api/v1/work/tasks/{id}/dependencies.
func (h *Handler) AddDependency(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	successor, ok := h.idParam(w, r, "id", "task")
	if !ok {
		return
	}
	var req dependencyReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	predecessor, err := uuid.Parse(strings.TrimSpace(req.PredecessorID))
	if err != nil {
		httpx.WriteProblem(w, r, httpx.BadRequest("predecessor_id is not a valid id."))
		return
	}
	if err := h.service.AddDependency(r.Context(), principal.PersonID, predecessor, successor, DependencyType(strings.TrimSpace(req.Type))); err != nil {
		h.writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// RemoveDependency handles DELETE /api/v1/work/tasks/{id}/dependencies/{predecessor_id}.
func (h *Handler) RemoveDependency(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	successor, ok := h.idParam(w, r, "id", "task")
	if !ok {
		return
	}
	predecessor, err := uuid.Parse(strings.TrimSpace(chi.URLParam(r, "predecessor_id")))
	if err != nil {
		httpx.WriteProblem(w, r, httpx.BadRequest("predecessor_id is not a valid id."))
		return
	}
	if err := h.service.RemoveDependency(r.Context(), principal.PersonID, predecessor, successor); err != nil {
		h.writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ListBlockers handles GET /api/v1/work/tasks/{id}/blockers.
func (h *Handler) ListBlockers(w http.ResponseWriter, r *http.Request) {
	id, ok := h.idParam(w, r, "id", "task")
	if !ok {
		return
	}
	includeResolved := strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("include_resolved")), "true")
	blockers, err := h.service.ListBlockers(r.Context(), id, includeResolved)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	views := make([]blockerView, 0, len(blockers))
	for _, b := range blockers {
		views = append(views, blockerOf(b))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": views})
}

// RaiseBlocker handles POST /api/v1/work/tasks/{id}/blockers.
func (h *Handler) RaiseBlocker(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "task")
	if !ok {
		return
	}
	var req blockerReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	b, err := h.service.RaiseBlocker(r.Context(), principal.PersonID, id, req.Reason)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, blockerOf(b))
}

// ResolveBlocker handles POST /api/v1/work/blockers/{id}/resolve.
func (h *Handler) ResolveBlocker(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "blocker")
	if !ok {
		return
	}
	b, err := h.service.ResolveBlocker(r.Context(), principal.PersonID, id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, blockerOf(b))
}

// ── evidence ────────────────────────────────────────────────────────────────

// AttachTaskEvidence handles POST /api/v1/work/tasks/{id}/evidence.
func (h *Handler) AttachTaskEvidence(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	taskID, ok := h.idParam(w, r, "id", "task")
	if !ok {
		return
	}
	h.attachEvidence(w, r, principal.PersonID, &taskID, nil)
}

// AttachProjectEvidence handles POST /api/v1/work/projects/{id}/evidence.
func (h *Handler) AttachProjectEvidence(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	projectID, ok := h.idParam(w, r, "id", "project")
	if !ok {
		return
	}
	h.attachEvidence(w, r, principal.PersonID, nil, &projectID)
}

func (h *Handler) attachEvidence(w http.ResponseWriter, r *http.Request, actor uuid.UUID, taskID, projectID *uuid.UUID) {
	var req evidenceReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	fileID, err := uuid.Parse(strings.TrimSpace(req.FileID))
	if err != nil {
		httpx.WriteProblem(w, r, httpx.BadRequest("file_id is not a valid id."))
		return
	}
	e, err := h.service.AttachEvidence(r.Context(), actor, EvidenceInput{
		TaskID: taskID, ProjectID: projectID, FileID: fileID,
		Kind: req.Kind, Description: req.Description,
	})
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, evidenceOf(e))
}

// ListTaskEvidence handles GET /api/v1/work/tasks/{id}/evidence.
func (h *Handler) ListTaskEvidence(w http.ResponseWriter, r *http.Request) {
	taskID, ok := h.idParam(w, r, "id", "task")
	if !ok {
		return
	}
	h.listEvidence(w, r, ListEvidenceQuery{TaskID: &taskID})
}

// ListProjectEvidence handles GET /api/v1/work/projects/{id}/evidence.
func (h *Handler) ListProjectEvidence(w http.ResponseWriter, r *http.Request) {
	projectID, ok := h.idParam(w, r, "id", "project")
	if !ok {
		return
	}
	h.listEvidence(w, r, ListEvidenceQuery{ProjectID: &projectID})
}

func (h *Handler) listEvidence(w http.ResponseWriter, r *http.Request, q ListEvidenceQuery) {
	items, err := h.service.ListEvidence(r.Context(), q)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	views := make([]evidenceView, 0, len(items))
	for _, e := range items {
		views = append(views, evidenceOf(e))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": views})
}

// GetEvidence handles GET /api/v1/work/evidence/{id}.
func (h *Handler) GetEvidence(w http.ResponseWriter, r *http.Request) {
	id, ok := h.idParam(w, r, "id", "evidence")
	if !ok {
		return
	}
	e, err := h.service.GetEvidence(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, evidenceOf(e))
}

// VerifyEvidence handles POST /api/v1/work/evidence/{id}/verify.
func (h *Handler) VerifyEvidence(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "evidence")
	if !ok {
		return
	}
	e, err := h.service.VerifyEvidence(r.Context(), principal.PersonID, id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, evidenceOf(e))
}

// DeleteEvidence handles DELETE /api/v1/work/evidence/{id}.
func (h *Handler) DeleteEvidence(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "evidence")
	if !ok {
		return
	}
	if err := h.service.DeleteEvidence(r.Context(), principal.PersonID, id); err != nil {
		h.writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
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
		httpx.WriteProblem(w, r, httpx.NotFound("No such work record."))
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

func toProjectInput(req projectReq) (ProjectInput, error) {
	in := ProjectInput{
		Name:        req.Name,
		Description: req.Description,
		ProjectType: req.ProjectType,
		Status:      ProjectStatus(strings.TrimSpace(req.Status)),
	}
	var err error
	if in.OwnerPersonID, err = optUUID(req.OwnerPersonID, "owner_person_id"); err != nil {
		return in, err
	}
	if in.DepartmentID, err = optUUID(req.DepartmentID, "department_id"); err != nil {
		return in, err
	}
	if in.StartedAt, err = optTime(req.StartedAt, "started_at"); err != nil {
		return in, err
	}
	if in.DueAt, err = optTime(req.DueAt, "due_at"); err != nil {
		return in, err
	}
	return in, nil
}

func toMilestoneInput(req milestoneReq) (MilestoneInput, error) {
	in := MilestoneInput{
		Name:        req.Name,
		Description: req.Description,
		Status:      MilestoneStatus(strings.TrimSpace(req.Status)),
		Weight:      req.Weight,
	}
	due, err := optTime(req.DueAt, "due_at")
	if err != nil {
		return in, err
	}
	in.DueAt = due
	return in, nil
}

func toTaskInput(req taskReq) (TaskInput, error) {
	in := TaskInput{
		Title:         req.Title,
		Description:   req.Description,
		Status:        TaskStatus(strings.TrimSpace(req.Status)),
		Priority:      Priority(strings.TrimSpace(req.Priority)),
		EstimateHours: req.EstimateHours,
		ActualHours:   req.ActualHours,
	}
	var err error
	if in.MilestoneID, err = optUUID(req.MilestoneID, "milestone_id"); err != nil {
		return in, err
	}
	if in.ParentID, err = optUUID(req.ParentID, "parent_id"); err != nil {
		return in, err
	}
	if in.AssigneePersonID, err = optUUID(req.AssigneePersonID, "assignee_person_id"); err != nil {
		return in, err
	}
	if in.DueAt, err = optTime(req.DueAt, "due_at"); err != nil {
		return in, err
	}
	return in, nil
}

func optUUID(p *string, field string) (*uuid.UUID, error) {
	if p == nil || strings.TrimSpace(*p) == "" {
		return nil, nil
	}
	id, err := uuid.Parse(strings.TrimSpace(*p))
	if err != nil {
		return nil, errInvalidField(field)
	}
	return &id, nil
}

func optTime(p *string, field string) (*time.Time, error) {
	if p == nil || strings.TrimSpace(*p) == "" {
		return nil, nil
	}
	ts, err := time.Parse(time.RFC3339, strings.TrimSpace(*p))
	if err != nil {
		return nil, errInvalidField(field)
	}
	ts = ts.UTC()
	return &ts, nil
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

func projectOf(p Project) projectView {
	v := projectView{
		ID:          p.ID.String(),
		Name:        p.Name,
		Description: p.Description,
		ProjectType: p.ProjectType,
		Status:      string(p.Status),
		StartedAt:   p.StartedAt,
		DueAt:       p.DueAt,
		CompletedAt: p.CompletedAt,
		Version:     p.Version,
		CreatedAt:   p.CreatedAt,
		UpdatedAt:   p.UpdatedAt,
	}
	if p.OwnerPersonID != nil {
		v.OwnerPersonID = p.OwnerPersonID.String()
	}
	if p.DepartmentID != nil {
		v.DepartmentID = p.DepartmentID.String()
	}
	return v
}

func milestoneOf(m Milestone) milestoneView {
	return milestoneView{
		ID:          m.ID.String(),
		ProjectID:   m.ProjectID.String(),
		Name:        m.Name,
		Description: m.Description,
		Status:      string(m.Status),
		DueAt:       m.DueAt,
		CompletedAt: m.CompletedAt,
		Weight:      m.Weight,
		Version:     m.Version,
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.UpdatedAt,
	}
}

func taskOf(t Task) taskView {
	v := taskView{
		ID:            t.ID.String(),
		ProjectID:     t.ProjectID.String(),
		Title:         t.Title,
		Description:   t.Description,
		Status:        string(t.Status),
		Priority:      string(t.Priority),
		DueAt:         t.DueAt,
		StartedAt:     t.StartedAt,
		CompletedAt:   t.CompletedAt,
		Blocked:       t.Blocked,
		BlockedReason: t.BlockedReason,
		EstimateHours: t.EstimateHours,
		ActualHours:   t.ActualHours,
		Version:       t.Version,
		CreatedAt:     t.CreatedAt,
		UpdatedAt:     t.UpdatedAt,
	}
	if t.MilestoneID != nil {
		v.MilestoneID = t.MilestoneID.String()
	}
	if t.ParentID != nil {
		v.ParentID = t.ParentID.String()
	}
	if t.AssigneePersonID != nil {
		v.AssigneePersonID = t.AssigneePersonID.String()
	}
	if t.ReporterPersonID != nil {
		v.ReporterPersonID = t.ReporterPersonID.String()
	}
	return v
}

func blockerOf(b Blocker) blockerView {
	v := blockerView{
		ID:         b.ID.String(),
		TaskID:     b.TaskID.String(),
		Reason:     b.Reason,
		ResolvedAt: b.ResolvedAt,
		CreatedAt:  b.CreatedAt,
	}
	if b.CreatedBy != nil {
		v.CreatedBy = b.CreatedBy.String()
	}
	if b.ResolvedBy != nil {
		v.ResolvedBy = b.ResolvedBy.String()
	}
	return v
}

func evidenceOf(e Evidence) evidenceView {
	v := evidenceView{
		ID:          e.ID.String(),
		FileID:      e.FileID.String(),
		UploaderID:  e.UploaderID.String(),
		Kind:        e.Kind,
		Description: e.Description,
		VerifiedAt:  e.VerifiedAt,
		CreatedAt:   e.CreatedAt,
	}
	if e.TaskID != nil {
		v.TaskID = e.TaskID.String()
	}
	if e.ProjectID != nil {
		v.ProjectID = e.ProjectID.String()
	}
	if e.VerifiedBy != nil {
		v.VerifiedBy = e.VerifiedBy.String()
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
