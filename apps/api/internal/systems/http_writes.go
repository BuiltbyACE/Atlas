package systems

import (
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/safaristack/atlas/apps/api/internal/platform/httpx"
)

// The write HTTP surface (§8.6). Every route is gated by the router on a
// systems.* permission; these handlers parse, call the service and render. The
// rules live in the service, and a refusal arrives with a stable `code`.

// ── shared parsing ──────────────────────────────────────────────────────────

func optUUIDField(raw *string, field string) (*uuid.UUID, error) {
	if raw == nil || trim(*raw) == "" {
		return nil, nil
	}
	id, err := uuid.Parse(trim(*raw))
	if err != nil {
		return nil, errInvalidField(field)
	}
	return &id, nil
}

// optDateField parses a YYYY-MM-DD date as UTC midnight. An empty string is no
// date.
func optDateField(raw *string, field string) (*time.Time, error) {
	if raw == nil || trim(*raw) == "" {
		return nil, nil
	}
	t, err := time.Parse("2006-01-02", trim(*raw))
	if err != nil {
		return nil, errInvalidField(field)
	}
	t = t.UTC()
	return &t, nil
}

// optTimeField parses an RFC3339 instant (or a bare date).
func optTimeField(raw *string, field string) (*time.Time, error) {
	if raw == nil || trim(*raw) == "" {
		return nil, nil
	}
	if t, err := time.Parse(time.RFC3339, trim(*raw)); err == nil {
		t = t.UTC()
		return &t, nil
	}
	return optDateField(raw, field)
}

func strOr(s *string, fallback string) string {
	if s == nil {
		return fallback
	}
	return trim(*s)
}

// ── systems ─────────────────────────────────────────────────────────────────

type systemReq struct {
	Name           *string `json:"name"`
	SystemType     string  `json:"system_type"`
	Environment    string  `json:"environment"`
	Criticality    string  `json:"criticality"`
	OwnerPersonID  *string `json:"owner_person_id"`
	OrganizationID *string `json:"organization_id"`
	ContractID     *string `json:"contract_id"`
	RunbookURL     string  `json:"runbook_url"`
	RepositoryURL  string  `json:"repository_url"`
	Description    string  `json:"description"`
}

func (r systemReq) input() (SystemInput, error) {
	in := SystemInput{
		Name:          strOr(r.Name, ""),
		SystemType:    SystemType(trim(r.SystemType)),
		Environment:   Environment(strOr(&r.Environment, string(EnvProd))),
		Criticality:   Criticality(strOr(&r.Criticality, string(Tier3))),
		RunbookURL:    r.RunbookURL,
		RepositoryURL: r.RepositoryURL,
		Description:   r.Description,
	}
	var err error
	if in.OwnerPersonID, err = optUUIDField(r.OwnerPersonID, "owner_person_id"); err != nil {
		return in, err
	}
	if in.OrganizationID, err = optUUIDField(r.OrganizationID, "organization_id"); err != nil {
		return in, err
	}
	if in.ContractID, err = optUUIDField(r.ContractID, "contract_id"); err != nil {
		return in, err
	}
	return in, nil
}

// CreateSystem handles POST /api/v1/systems.
func (h *Handler) CreateSystem(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	var req systemReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	in, err := req.input()
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	sys, err := h.service.CreateSystem(r.Context(), principal.PersonID, in)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(sys.Version))
	httpx.WriteJSON(w, http.StatusCreated, systemOf(sys))
}

// UpdateSystem handles PUT /api/v1/systems/{id}.
func (h *Handler) UpdateSystem(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "system")
	if !ok {
		return
	}
	current, err := h.service.System(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	if err := httpx.RequireIfMatch(r, int64(current.Version)); err != nil {
		h.writeError(w, r, err)
		return
	}
	var req systemReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	in, err := req.input()
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	updated, err := h.service.UpdateSystem(r.Context(), principal.PersonID, id, current.Version, in)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(updated.Version))
	httpx.WriteJSON(w, http.StatusOK, systemOf(updated))
}

// RetireSystem handles POST /api/v1/systems/{id}/retire.
func (h *Handler) RetireSystem(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "system")
	if !ok {
		return
	}
	current, err := h.service.System(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	if err := httpx.RequireIfMatch(r, int64(current.Version)); err != nil {
		h.writeError(w, r, err)
		return
	}
	retired, err := h.service.RetireSystem(r.Context(), principal.PersonID, id, current.Version)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(retired.Version))
	httpx.WriteJSON(w, http.StatusOK, systemOf(retired))
}

// ── checks ──────────────────────────────────────────────────────────────────

type checkReq struct {
	SystemID               *string  `json:"system_id"`
	Name                   *string  `json:"name"`
	CheckType              string   `json:"check_type"`
	Target                 string   `json:"target"`
	Expect                 string   `json:"expect"`
	IntervalSeconds        *int     `json:"interval_seconds"`
	TimeoutSeconds         *int     `json:"timeout_seconds"`
	Regions                []string `json:"regions"`
	WarnMS                 *int     `json:"warn_ms"`
	FailMS                 *int     `json:"fail_ms"`
	FailureThreshold       *int     `json:"failure_threshold"`
	HeartbeatWindowSeconds *int     `json:"heartbeat_window_seconds"`
	Active                 *bool    `json:"active"`
}

func (r checkReq) input() (CheckInput, error) {
	in := CheckInput{
		Name:                   strOr(r.Name, ""),
		CheckType:              CheckType(trim(r.CheckType)),
		Target:                 r.Target,
		Expect:                 r.Expect,
		IntervalSeconds:        300,
		TimeoutSeconds:         10,
		FailureThreshold:       2,
		Regions:                r.Regions,
		WarnMS:                 r.WarnMS,
		FailMS:                 r.FailMS,
		HeartbeatWindowSeconds: r.HeartbeatWindowSeconds,
		Active:                 true,
	}
	if r.SystemID != nil {
		systemID, err := uuid.Parse(trim(*r.SystemID))
		if err != nil {
			return in, errInvalidField("system_id")
		}
		in.SystemID = systemID
	}
	if in.SystemID == uuid.Nil {
		return in, errInvalidField("system_id")
	}
	if r.IntervalSeconds != nil {
		in.IntervalSeconds = *r.IntervalSeconds
	}
	if r.TimeoutSeconds != nil {
		in.TimeoutSeconds = *r.TimeoutSeconds
	}
	if r.FailureThreshold != nil {
		in.FailureThreshold = *r.FailureThreshold
	}
	if r.Active != nil {
		in.Active = *r.Active
	}
	return in, nil
}

// CreateCheck handles POST /api/v1/systems/checks.
func (h *Handler) CreateCheck(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	var req checkReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	in, err := req.input()
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	check, err := h.service.CreateCheck(r.Context(), principal.PersonID, in)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(check.Version))
	httpx.WriteJSON(w, http.StatusCreated, checkOf(check))
}

// UpdateCheck handles PUT /api/v1/systems/checks/{id}.
func (h *Handler) UpdateCheck(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "check")
	if !ok {
		return
	}
	current, err := h.service.Check(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	if err := httpx.RequireIfMatch(r, int64(current.Version)); err != nil {
		h.writeError(w, r, err)
		return
	}
	var req checkReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	in, err := req.input()
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	// A probe belongs to the system it watches. Moving one to another system
	// would silently rewrite history, so an update may not do it.
	if in.SystemID != current.SystemID {
		h.writeError(w, r, errInvalidField("system_id"))
		return
	}
	updated, err := h.service.UpdateCheck(r.Context(), principal.PersonID, id, current.Version, in)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(updated.Version))
	httpx.WriteJSON(w, http.StatusOK, checkOf(updated))
}

// SetCheckActive handles POST /api/v1/systems/checks/{id}/pause and .../resume.
func (h *Handler) SetCheckActive(w http.ResponseWriter, r *http.Request, active bool) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "check")
	if !ok {
		return
	}
	current, err := h.service.Check(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	if err := httpx.RequireIfMatch(r, int64(current.Version)); err != nil {
		h.writeError(w, r, err)
		return
	}
	updated, err := h.service.SetCheckActive(r.Context(), principal.PersonID, id, current.Version, active)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(updated.Version))
	httpx.WriteJSON(w, http.StatusOK, checkOf(updated))
}

// PauseCheck handles POST /api/v1/systems/checks/{id}/pause.
func (h *Handler) PauseCheck(w http.ResponseWriter, r *http.Request) { h.SetCheckActive(w, r, false) }

// ResumeCheck handles POST /api/v1/systems/checks/{id}/resume.
func (h *Handler) ResumeCheck(w http.ResponseWriter, r *http.Request) { h.SetCheckActive(w, r, true) }

// ── incidents ───────────────────────────────────────────────────────────────

type incidentReq struct {
	Title             *string  `json:"title"`
	Severity          string   `json:"severity"`
	SystemIDs         []string `json:"system_ids"`
	DetectedAt        *string  `json:"detected_at"`
	DetectedSource    string   `json:"detected_source"`
	CommanderPersonID *string  `json:"commander_person_id"`
	ClientImpact      string   `json:"client_impact"`
}

func (r incidentReq) input() (IncidentInput, error) {
	in := IncidentInput{
		Title:          strOr(r.Title, ""),
		Severity:       IncidentSeverity(trim(r.Severity)),
		DetectedSource: IncidentSource(trim(r.DetectedSource)),
		ClientImpact:   r.ClientImpact,
	}
	for _, raw := range r.SystemIDs {
		id, err := uuid.Parse(trim(raw))
		if err != nil {
			return in, errInvalidField("system_ids")
		}
		in.SystemIDs = append(in.SystemIDs, id)
	}
	var err error
	if in.DetectedAt, err = optTimeField(r.DetectedAt, "detected_at"); err != nil {
		return in, err
	}
	if in.CommanderPersonID, err = optUUIDField(r.CommanderPersonID, "commander_person_id"); err != nil {
		return in, err
	}
	return in, nil
}

// OpenIncident handles POST /api/v1/systems/incidents.
func (h *Handler) OpenIncident(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	var req incidentReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	in, err := req.input()
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	incident, err := h.service.OpenIncident(r.Context(), principal.PersonID, in)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(incident.Version))
	httpx.WriteJSON(w, http.StatusCreated, incidentOf(incident))
}

// stepReq is the optional note a lifecycle step carries.
type stepReq struct {
	Note string `json:"note"`
}

// incidentStep reads the current version and returns the caller's note.
func (h *Handler) incidentStep(w http.ResponseWriter, r *http.Request) (uuid.UUID, int, string, bool) {
	id, ok := h.idParam(w, r, "id", "incident")
	if !ok {
		return uuid.Nil, 0, "", false
	}
	current, err := h.service.Incident(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return uuid.Nil, 0, "", false
	}
	if err := httpx.RequireIfMatch(r, int64(current.Version)); err != nil {
		h.writeError(w, r, err)
		return uuid.Nil, 0, "", false
	}
	// A step body is optional: an empty body means "no note".
	var req stepReq
	if r.ContentLength != 0 {
		if err := httpx.DecodeJSON(w, r, &req); err != nil {
			h.writeError(w, r, err)
			return uuid.Nil, 0, "", false
		}
	}
	return id, current.Version, req.Note, true
}

func (h *Handler) renderIncident(w http.ResponseWriter, r *http.Request, incident Incident, err error) {
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(incident.Version))
	httpx.WriteJSON(w, http.StatusOK, incidentOf(incident))
}

// AcknowledgeIncident handles POST /api/v1/systems/incidents/{id}/acknowledge.
func (h *Handler) AcknowledgeIncident(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, version, note, ok := h.incidentStep(w, r)
	if !ok {
		return
	}
	incident, err := h.service.AcknowledgeIncident(r.Context(), principal.PersonID, id, version, note)
	h.renderIncident(w, r, incident, err)
}

// MitigateIncident handles POST /api/v1/systems/incidents/{id}/mitigate.
func (h *Handler) MitigateIncident(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, version, note, ok := h.incidentStep(w, r)
	if !ok {
		return
	}
	incident, err := h.service.MitigateIncident(r.Context(), principal.PersonID, id, version, note)
	h.renderIncident(w, r, incident, err)
}

// ResolveIncident handles POST /api/v1/systems/incidents/{id}/resolve.
func (h *Handler) ResolveIncident(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, version, note, ok := h.incidentStep(w, r)
	if !ok {
		return
	}
	incident, err := h.service.ResolveIncident(r.Context(), principal.PersonID, id, version, note)
	h.renderIncident(w, r, incident, err)
}

// CloseIncident handles POST /api/v1/systems/incidents/{id}/close.
func (h *Handler) CloseIncident(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, version, note, ok := h.incidentStep(w, r)
	if !ok {
		return
	}
	incident, err := h.service.CloseIncident(r.Context(), principal.PersonID, id, version, note)
	h.renderIncident(w, r, incident, err)
}

// reviewReq is the post-incident review.
type reviewReq struct {
	RootCause string `json:"root_cause"`
	Summary   string `json:"summary"`
}

// ReviewIncident handles POST /api/v1/systems/incidents/{id}/review.
func (h *Handler) ReviewIncident(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, version, _, ok := h.incidentStep(w, r)
	if !ok {
		return
	}
	var req reviewReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	incident, err := h.service.ReviewIncident(r.Context(), principal.PersonID, id, version,
		IncidentReviewInput{RootCause: req.RootCause, Summary: req.Summary})
	h.renderIncident(w, r, incident, err)
}

// AddIncidentNote handles POST /api/v1/systems/incidents/{id}/notes.
func (h *Handler) AddIncidentNote(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "incident")
	if !ok {
		return
	}
	var req stepReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	entry, err := h.service.AddIncidentNote(r.Context(), principal.PersonID, id, IncidentNoteInput{Note: req.Note})
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, timelineEntryOf(entry))
}

type actionReq struct {
	Title         *string `json:"title"`
	OwnerPersonID *string `json:"owner_person_id"`
	DueOn         *string `json:"due_on"`
	Status        string  `json:"status"`
	TaskID        *string `json:"task_id"`
}

func (r actionReq) input() (IncidentActionInput, error) {
	in := IncidentActionInput{
		Title:  strOr(r.Title, ""),
		Status: ActionStatus(trim(r.Status)),
	}
	var err error
	if in.OwnerPersonID, err = optUUIDField(r.OwnerPersonID, "owner_person_id"); err != nil {
		return in, err
	}
	if in.DueOn, err = optDateField(r.DueOn, "due_on"); err != nil {
		return in, err
	}
	if in.TaskID, err = optUUIDField(r.TaskID, "task_id"); err != nil {
		return in, err
	}
	return in, nil
}

// CreateIncidentAction handles POST /api/v1/systems/incidents/{id}/actions.
func (h *Handler) CreateIncidentAction(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "incident")
	if !ok {
		return
	}
	var req actionReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	in, err := req.input()
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	action, err := h.service.CreateIncidentAction(r.Context(), principal.PersonID, id, in)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(action.Version))
	httpx.WriteJSON(w, http.StatusCreated, incidentActionOf(action))
}

// UpdateIncidentAction handles PUT /api/v1/systems/incidents/{id}/actions/{actionId}.
func (h *Handler) UpdateIncidentAction(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	incidentID, ok := h.idParam(w, r, "id", "incident")
	if !ok {
		return
	}
	actionID, ok := h.idParam(w, r, "actionId", "action")
	if !ok {
		return
	}
	actions, err := h.service.store.IncidentActions(r.Context(), incidentID)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	var current *IncidentAction
	for i := range actions {
		if actions[i].ID == actionID {
			current = &actions[i]
			break
		}
	}
	if current == nil {
		h.writeError(w, r, ErrNotFound)
		return
	}
	if err := httpx.RequireIfMatch(r, int64(current.Version)); err != nil {
		h.writeError(w, r, err)
		return
	}
	var req actionReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	in, err := req.input()
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	action, err := h.service.UpdateIncidentAction(r.Context(), principal.PersonID, incidentID, actionID, current.Version, in)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(action.Version))
	httpx.WriteJSON(w, http.StatusOK, incidentActionOf(action))
}

// ── domains ─────────────────────────────────────────────────────────────────

type domainReq struct {
	Name          *string `json:"name"`
	SystemID      *string `json:"system_id"`
	Registrar     string  `json:"registrar"`
	RegisteredOn  *string `json:"registered_on"`
	ExpiresOn     *string `json:"expires_on"`
	AutoRenew     *bool   `json:"auto_renew"`
	OwnerPersonID *string `json:"owner_person_id"`
	Notes         string  `json:"notes"`
}

func (r domainReq) input() (DomainInput, error) {
	in := DomainInput{Name: strOr(r.Name, ""), Registrar: r.Registrar, Notes: r.Notes}
	var err error
	if in.SystemID, err = optUUIDField(r.SystemID, "system_id"); err != nil {
		return in, err
	}
	if in.RegisteredOn, err = optDateField(r.RegisteredOn, "registered_on"); err != nil {
		return in, err
	}
	if in.ExpiresOn, err = optDateField(r.ExpiresOn, "expires_on"); err != nil {
		return in, err
	}
	if in.OwnerPersonID, err = optUUIDField(r.OwnerPersonID, "owner_person_id"); err != nil {
		return in, err
	}
	if r.AutoRenew != nil {
		in.AutoRenew = *r.AutoRenew
	}
	return in, nil
}

// ListDomains handles GET /api/v1/systems/domains.
func (h *Handler) ListDomains(w http.ResponseWriter, r *http.Request) {
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
	var q DomainQuery
	var ok bool
	if q.SystemID, ok = h.uuidParam(w, r, "system_id"); !ok {
		return
	}
	q.Search = trim(r.URL.Query().Get("q"))
	items, err := h.service.Domains(r.Context(), q, limit+1, before)
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
	views := make([]domainView, 0, len(items))
	for _, d := range items {
		views = append(views, domainOf(d))
	}
	httpx.WriteJSON(w, http.StatusOK, httpx.NewPage(views, next, limit))
}

// CreateDomain handles POST /api/v1/systems/domains.
func (h *Handler) CreateDomain(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	var req domainReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	in, err := req.input()
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	domain, err := h.service.CreateDomain(r.Context(), principal.PersonID, in)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(domain.Version))
	httpx.WriteJSON(w, http.StatusCreated, domainOf(domain))
}

// UpdateDomain handles PUT /api/v1/systems/domains/{id}.
func (h *Handler) UpdateDomain(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "domain")
	if !ok {
		return
	}
	current, err := h.service.Domain(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	if err := httpx.RequireIfMatch(r, int64(current.Version)); err != nil {
		h.writeError(w, r, err)
		return
	}
	var req domainReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	in, err := req.input()
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	domain, err := h.service.UpdateDomain(r.Context(), principal.PersonID, id, current.Version, in)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(domain.Version))
	httpx.WriteJSON(w, http.StatusOK, domainOf(domain))
}

type domainView struct {
	ID           string         `json:"id"`
	Name         string         `json:"name"`
	SystemID     string         `json:"system_id,omitempty"`
	SystemName   string         `json:"system_name,omitempty"`
	Registrar    string         `json:"registrar,omitempty"`
	RegisteredOn string         `json:"registered_on,omitempty"`
	ExpiresOn    string         `json:"expires_on,omitempty"`
	AutoRenew    bool           `json:"auto_renew"`
	Owner        *personRefView `json:"owner,omitempty"`
	Notes        string         `json:"notes,omitempty"`
	Version      int            `json:"version"`
	CreatedAt    string         `json:"created_at"`
	UpdatedAt    string         `json:"updated_at"`
}

func domainOf(d Domain) domainView {
	return domainView{
		ID: d.ID.String(), Name: d.Name, SystemID: idStr(d.SystemID), SystemName: d.SystemName,
		Registrar: d.Registrar, RegisteredOn: dayStr(d.RegisteredOn), ExpiresOn: dayStr(d.ExpiresOn),
		AutoRenew: d.AutoRenew, Owner: personRef(d.OwnerPersonID, d.OwnerName), Notes: d.Notes,
		Version: d.Version, CreatedAt: rfc(d.CreatedAt), UpdatedAt: rfc(d.UpdatedAt),
	}
}

// ── certificates ────────────────────────────────────────────────────────────

type certificateReq struct {
	Hostname      *string `json:"hostname"`
	SystemID      *string `json:"system_id"`
	Issuer        string  `json:"issuer"`
	IssuedOn      *string `json:"issued_on"`
	ExpiresOn     *string `json:"expires_on"`
	AutoRenew     *bool   `json:"auto_renew"`
	OwnerPersonID *string `json:"owner_person_id"`
	Notes         string  `json:"notes"`
}

func (r certificateReq) input() (CertificateInput, error) {
	in := CertificateInput{Hostname: strOr(r.Hostname, ""), Issuer: r.Issuer, Notes: r.Notes}
	var err error
	if in.SystemID, err = optUUIDField(r.SystemID, "system_id"); err != nil {
		return in, err
	}
	if in.IssuedOn, err = optDateField(r.IssuedOn, "issued_on"); err != nil {
		return in, err
	}
	if in.ExpiresOn, err = optDateField(r.ExpiresOn, "expires_on"); err != nil {
		return in, err
	}
	if in.OwnerPersonID, err = optUUIDField(r.OwnerPersonID, "owner_person_id"); err != nil {
		return in, err
	}
	if r.AutoRenew != nil {
		in.AutoRenew = *r.AutoRenew
	}
	return in, nil
}

// ListCertificates handles GET /api/v1/systems/certificates.
func (h *Handler) ListCertificates(w http.ResponseWriter, r *http.Request) {
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
	var q CertificateQuery
	var ok bool
	if q.SystemID, ok = h.uuidParam(w, r, "system_id"); !ok {
		return
	}
	q.Search = trim(r.URL.Query().Get("q"))
	items, err := h.service.Certificates(r.Context(), q, limit+1, before)
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
	views := make([]certificateView, 0, len(items))
	for _, c := range items {
		views = append(views, certificateOf(c))
	}
	httpx.WriteJSON(w, http.StatusOK, httpx.NewPage(views, next, limit))
}

// CreateCertificate handles POST /api/v1/systems/certificates.
func (h *Handler) CreateCertificate(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	var req certificateReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	in, err := req.input()
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	cert, err := h.service.CreateCertificate(r.Context(), principal.PersonID, in)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(cert.Version))
	httpx.WriteJSON(w, http.StatusCreated, certificateOf(cert))
}

// UpdateCertificate handles PUT /api/v1/systems/certificates/{id}.
func (h *Handler) UpdateCertificate(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "certificate")
	if !ok {
		return
	}
	current, err := h.service.Certificate(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	if err := httpx.RequireIfMatch(r, int64(current.Version)); err != nil {
		h.writeError(w, r, err)
		return
	}
	var req certificateReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	in, err := req.input()
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	cert, err := h.service.UpdateCertificate(r.Context(), principal.PersonID, id, current.Version, in)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(cert.Version))
	httpx.WriteJSON(w, http.StatusOK, certificateOf(cert))
}

type certificateView struct {
	ID          string         `json:"id"`
	Hostname    string         `json:"hostname"`
	SystemID    string         `json:"system_id,omitempty"`
	SystemName  string         `json:"system_name,omitempty"`
	Issuer      string         `json:"issuer,omitempty"`
	IssuedOn    string         `json:"issued_on,omitempty"`
	ExpiresOn   string         `json:"expires_on"`
	AutoRenew   bool           `json:"auto_renew"`
	Owner       *personRefView `json:"owner,omitempty"`
	Notes       string         `json:"notes,omitempty"`
	Version     int            `json:"version"`
	CreatedAt   string         `json:"created_at"`
	UpdatedAt   string         `json:"updated_at"`
	WarningBand int            `json:"warning_band,omitempty"`
}

func certificateOf(c Certificate) certificateView {
	daysOut := int(time.Until(c.ExpiresOn).Hours() / 24)
	return certificateView{
		ID: c.ID.String(), Hostname: c.Hostname, SystemID: idStr(c.SystemID), SystemName: c.SystemName,
		Issuer: c.Issuer, IssuedOn: dayStr(c.IssuedOn), ExpiresOn: dayStr(&c.ExpiresOn),
		AutoRenew: c.AutoRenew, Owner: personRef(c.OwnerPersonID, c.OwnerName), Notes: c.Notes,
		Version: c.Version, CreatedAt: rfc(c.CreatedAt), UpdatedAt: rfc(c.UpdatedAt),
		WarningBand: warningBand(daysOut),
	}
}

// ── credential records ──────────────────────────────────────────────────────

type credentialReq struct {
	SystemID       *string `json:"system_id"`
	Label          *string `json:"label"`
	CredentialKind string  `json:"credential_kind"`
	StorePath      string  `json:"store_path"`
	OwnerPersonID  *string `json:"owner_person_id"`
	RotatedOn      *string `json:"rotated_on"`
	NextRotationOn *string `json:"next_rotation_on"`
	Notes          string  `json:"notes"`
	Active         *bool   `json:"active"`
}

func (r credentialReq) input() (CredentialInput, error) {
	in := CredentialInput{
		Label:          strOr(r.Label, ""),
		CredentialKind: r.CredentialKind,
		StorePath:      r.StorePath,
		Notes:          r.Notes,
		Active:         true,
	}
	if r.SystemID != nil {
		systemID, err := uuid.Parse(trim(*r.SystemID))
		if err != nil {
			return in, errInvalidField("system_id")
		}
		in.SystemID = systemID
	}
	if in.SystemID == uuid.Nil {
		return in, errInvalidField("system_id")
	}
	var err error
	if in.OwnerPersonID, err = optUUIDField(r.OwnerPersonID, "owner_person_id"); err != nil {
		return in, err
	}
	if in.RotatedOn, err = optDateField(r.RotatedOn, "rotated_on"); err != nil {
		return in, err
	}
	if in.NextRotationOn, err = optDateField(r.NextRotationOn, "next_rotation_on"); err != nil {
		return in, err
	}
	if r.Active != nil {
		in.Active = *r.Active
	}
	return in, nil
}

// ListCredentials handles GET /api/v1/systems/credentials.
func (h *Handler) ListCredentials(w http.ResponseWriter, r *http.Request) {
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
	var q CredentialQuery
	var ok bool
	if q.SystemID, ok = h.uuidParam(w, r, "system_id"); !ok {
		return
	}
	// An absent `active` filter means "no filter", not "inactive".
	if raw := trim(r.URL.Query().Get("active")); raw != "" {
		active, ok := h.boolParam(w, r, "active")
		if !ok {
			return
		}
		q.Active = &active
	}
	items, err := h.service.AllCredentials(r.Context(), q, limit+1, before)
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
	views := make([]credentialView, 0, len(items))
	for _, c := range items {
		views = append(views, credentialOf(c))
	}
	httpx.WriteJSON(w, http.StatusOK, httpx.NewPage(views, next, limit))
}

// CreateCredential handles POST /api/v1/systems/credentials.
func (h *Handler) CreateCredential(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	var req credentialReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	in, err := req.input()
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	record, err := h.service.CreateCredential(r.Context(), principal.PersonID, in)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(record.Version))
	httpx.WriteJSON(w, http.StatusCreated, credentialOf(record))
}

// UpdateCredential handles PUT /api/v1/systems/credentials/{id}.
func (h *Handler) UpdateCredential(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "credential")
	if !ok {
		return
	}
	current, err := h.service.Credential(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	if err := httpx.RequireIfMatch(r, int64(current.Version)); err != nil {
		h.writeError(w, r, err)
		return
	}
	var req credentialReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	in, err := req.input()
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	record, err := h.service.UpdateCredential(r.Context(), principal.PersonID, id, current.Version, in)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(record.Version))
	httpx.WriteJSON(w, http.StatusOK, credentialOf(record))
}
