package systems

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

// Handler serves the systems HTTP surface (§8.6).
type Handler struct {
	service *Service
	logger  *slog.Logger
}

// NewHandler builds a systems handler.
func NewHandler(service *Service, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{service: service, logger: logger}
}

// ── views ───────────────────────────────────────────────────────────────────

// Every optional field is a pointer with omitempty: a check that has never run
// has no status, and the JSON says so by omitting it rather than sending a
// zero that reads as "pass".

type overviewView struct {
	AsOf                string              `json:"as_of"`
	SystemsTotal        int                 `json:"systems_total"`
	SystemsTier1        int                 `json:"systems_tier1"`
	SystemsTier2        int                 `json:"systems_tier2"`
	SystemsTier3        int                 `json:"systems_tier3"`
	ChecksTotal         int                 `json:"checks_total"`
	ChecksFailing       int                 `json:"checks_failing"`
	ChecksWarning       int                 `json:"checks_warning"`
	ChecksUnprobed      int                 `json:"checks_unprobed"`
	OpenIncidents       int                 `json:"open_incidents"`
	OpenSev1            int                 `json:"open_sev1"`
	OpenSev2            int                 `json:"open_sev2"`
	OpenIncidentsBySev  []severityCountView `json:"open_incidents_by_severity"`
	Incidents30D        int                 `json:"incidents_30d"`
	DetectedByAtlas30D  int                 `json:"detected_by_atlas_30d"`
	DetectedByClient30D int                 `json:"detected_by_client_30d"`
	MTTASeconds         *int64              `json:"mtta_seconds,omitempty"`
	MTTRSeconds         *int64              `json:"mttr_seconds,omitempty"`
	ReviewsOutstanding  int                 `json:"reviews_outstanding"`
	ExpiringCount       int                 `json:"expiring_count"`
	RotationsDueCount   int                 `json:"rotations_due_count"`
	Availability        []availabilityView  `json:"availability"`
	ExpiryWarningDays   []int               `json:"expiry_warning_days"`
}

type severityCountView struct {
	Severity string `json:"severity"`
	Label    string `json:"label"`
	Count    int    `json:"count"`
}

type availabilityView struct {
	SystemID       string `json:"system_id"`
	Ref            string `json:"ref"`
	Name           string `json:"name"`
	Criticality    string `json:"criticality"`
	Passed         int    `json:"passed"`
	Total          int    `json:"total"`
	AvailabilityBP int    `json:"availability_bp"`
}

type systemView struct {
	ID             string           `json:"id"`
	Ref            string           `json:"ref"`
	Name           string           `json:"name"`
	SystemType     string           `json:"system_type"`
	Owner          *personRefView   `json:"owner,omitempty"`
	Client         *orgRefView      `json:"client,omitempty"`
	Environment    string           `json:"environment"`
	Criticality    string           `json:"criticality"`
	ContractRef    string           `json:"contract_ref,omitempty"`
	RunbookURL     string           `json:"runbook_url,omitempty"`
	RepositoryURL  string           `json:"repository_url,omitempty"`
	Description    string           `json:"description,omitempty"`
	Status         string           `json:"status"`
	ChecksTotal    int              `json:"checks_total"`
	ChecksFailing  int              `json:"checks_failing"`
	ChecksWarning  int              `json:"checks_warning"`
	OpenIncidents  int              `json:"open_incidents"`
	AvailabilityBP *int             `json:"availability_bp,omitempty"`
	Credentials    []credentialView `json:"credentials,omitempty"`
	Version        int              `json:"version"`
	CreatedAt      string           `json:"created_at"`
	UpdatedAt      string           `json:"updated_at"`
}

type personRefView struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type orgRefView struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type checkView struct {
	ID                     string   `json:"id"`
	SystemID               string   `json:"system_id"`
	SystemName             string   `json:"system_name"`
	SystemCriticality      string   `json:"system_criticality"`
	Name                   string   `json:"name"`
	CheckType              string   `json:"check_type"`
	Target                 string   `json:"target"`
	Expect                 string   `json:"expect,omitempty"`
	IntervalSeconds        int      `json:"interval_seconds"`
	TimeoutSeconds         int      `json:"timeout_seconds"`
	Regions                []string `json:"regions"`
	WarnMS                 *int     `json:"warn_ms,omitempty"`
	FailMS                 *int     `json:"fail_ms,omitempty"`
	FailureThreshold       int      `json:"failure_threshold"`
	HeartbeatWindowSeconds *int     `json:"heartbeat_window_seconds,omitempty"`
	Active                 bool     `json:"active"`
	LastStatus             *string  `json:"last_status,omitempty"`
	LastAt                 string   `json:"last_at,omitempty"`
	LastLatencyMS          *int     `json:"last_latency_ms,omitempty"`
	ConsecutiveFailures    int      `json:"consecutive_failures"`
	Version                int      `json:"version"`
	CreatedAt              string   `json:"created_at"`
	UpdatedAt              string   `json:"updated_at"`
}

type checkResultView struct {
	ID        string `json:"id"`
	CheckID   string `json:"check_id"`
	At        string `json:"at"`
	Status    string `json:"status"`
	LatencyMS *int   `json:"latency_ms,omitempty"`
	Detail    string `json:"detail,omitempty"`
	Source    string `json:"source"`
	Region    string `json:"region,omitempty"`
}

type incidentView struct {
	ID             string               `json:"id"`
	Ref            string               `json:"ref"`
	Title          string               `json:"title"`
	Severity       string               `json:"severity"`
	SeverityLabel  string               `json:"severity_label"`
	Status         string               `json:"status"`
	DetectedAt     string               `json:"detected_at"`
	DetectedSource string               `json:"detected_source"`
	AcknowledgedAt string               `json:"acknowledged_at,omitempty"`
	MitigatedAt    string               `json:"mitigated_at,omitempty"`
	ResolvedAt     string               `json:"resolved_at,omitempty"`
	ClosedAt       string               `json:"closed_at,omitempty"`
	Commander      *personRefView       `json:"commander,omitempty"`
	ClientImpact   string               `json:"client_impact,omitempty"`
	RootCause      string               `json:"root_cause,omitempty"`
	Summary        string               `json:"summary,omitempty"`
	ReviewRequired bool                 `json:"review_required"`
	ReviewAt       string               `json:"review_at,omitempty"`
	SystemNames    []string             `json:"system_names"`
	Systems        []incidentSystemView `json:"systems,omitempty"`
	Timeline       []timelineEntryView  `json:"timeline,omitempty"`
	Actions        []incidentActionView `json:"actions,omitempty"`
	MTTASeconds    *int64               `json:"mtta_seconds,omitempty"`
	MTTRSeconds    *int64               `json:"mttr_seconds,omitempty"`
	Version        int                  `json:"version"`
	CreatedAt      string               `json:"created_at"`
	UpdatedAt      string               `json:"updated_at"`
}

type incidentSystemView struct {
	SystemID    string `json:"system_id"`
	Ref         string `json:"ref"`
	Name        string `json:"name"`
	Criticality string `json:"criticality"`
}

type timelineEntryView struct {
	ID        string         `json:"id"`
	At        string         `json:"at"`
	Actor     *personRefView `json:"actor,omitempty"`
	EntryType string         `json:"entry_type"`
	Note      string         `json:"note"`
}

type incidentActionView struct {
	ID        string         `json:"id"`
	Title     string         `json:"title"`
	Owner     *personRefView `json:"owner,omitempty"`
	DueOn     string         `json:"due_on,omitempty"`
	Status    string         `json:"status"`
	TaskID    string         `json:"task_id,omitempty"`
	Version   int            `json:"version"`
	CreatedAt string         `json:"created_at"`
	UpdatedAt string         `json:"updated_at"`
}

// credentialView is a pointer, never a value: it carries where a credential is
// kept, and there is no field for the secret itself (§8.6).
type credentialView struct {
	ID             string         `json:"id"`
	Label          string         `json:"label"`
	CredentialKind string         `json:"credential_kind"`
	StorePath      string         `json:"store_path"`
	Owner          *personRefView `json:"owner,omitempty"`
	RotatedOn      string         `json:"rotated_on,omitempty"`
	NextRotationOn string         `json:"next_rotation_on,omitempty"`
	Notes          string         `json:"notes,omitempty"`
	Active         bool           `json:"active"`
	Version        int            `json:"version"`
}

type expiryView struct {
	Kind        string         `json:"kind"`
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	ExpiresOn   string         `json:"expires_on"`
	DaysOut     int            `json:"days_out"`
	AutoRenew   bool           `json:"auto_renew"`
	WarningBand int            `json:"warning_band"`
	System      *systemRefView `json:"system,omitempty"`
	Owner       *personRefView `json:"owner,omitempty"`
}

type systemRefView struct {
	ID   string `json:"id"`
	Name string `json:"name"`
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

func systemRef(id *uuid.UUID, name string) *systemRefView {
	if id == nil {
		return nil
	}
	return &systemRefView{ID: id.String(), Name: name}
}

func overviewOf(o Overview) overviewView {
	bySev := make([]severityCountView, 0, len(o.OpenIncidentsBySev))
	for _, s := range o.OpenIncidentsBySev {
		bySev = append(bySev, severityCountView{Severity: s.Severity, Label: s.Label, Count: s.Count})
	}
	avail := make([]availabilityView, 0, len(o.Availability))
	for _, a := range o.Availability {
		avail = append(avail, availabilityView{
			SystemID: a.SystemID.String(), Ref: a.Ref, Name: a.Name,
			Criticality: string(a.Criticality), Passed: a.Passed, Total: a.Total,
			AvailabilityBP: a.AvailabilityBP,
		})
	}
	return overviewView{
		AsOf:                rfc(o.AsOf),
		SystemsTotal:        o.SystemsTotal,
		SystemsTier1:        o.SystemsTier1,
		SystemsTier2:        o.SystemsTier2,
		SystemsTier3:        o.SystemsTier3,
		ChecksTotal:         o.ChecksTotal,
		ChecksFailing:       o.ChecksFailing,
		ChecksWarning:       o.ChecksWarning,
		ChecksUnprobed:      o.ChecksUnprobed,
		OpenIncidents:       o.OpenIncidents,
		OpenSev1:            o.OpenSev1,
		OpenSev2:            o.OpenSev2,
		OpenIncidentsBySev:  bySev,
		Incidents30D:        o.Incidents30D,
		DetectedByAtlas30D:  o.DetectedByAtlas30D,
		DetectedByClient30D: o.DetectedByClient30D,
		MTTASeconds:         o.MTTASeconds,
		MTTRSeconds:         o.MTTRSeconds,
		ReviewsOutstanding:  o.ReviewsOutstanding,
		ExpiringCount:       o.ExpiringCount,
		RotationsDueCount:   o.RotationsDueCount,
		Availability:        avail,
		ExpiryWarningDays:   ExpiryWarningDays,
	}
}

func systemOf(s System) systemView {
	v := systemView{
		ID:             s.ID.String(),
		Ref:            s.Ref,
		Name:           s.Name,
		SystemType:     string(s.SystemType),
		Owner:          personRef(s.OwnerPersonID, s.OwnerName),
		Environment:    string(s.Environment),
		Criticality:    string(s.Criticality),
		ContractRef:    s.ContractRef,
		RunbookURL:     s.RunbookURL,
		RepositoryURL:  s.RepositoryURL,
		Description:    s.Description,
		Status:         string(s.Status),
		ChecksTotal:    s.ChecksTotal,
		ChecksFailing:  s.ChecksFailing,
		ChecksWarning:  s.ChecksWarning,
		OpenIncidents:  s.OpenIncidents,
		AvailabilityBP: s.AvailabilityBP,
		Version:        s.Version,
		CreatedAt:      rfc(s.CreatedAt),
		UpdatedAt:      rfc(s.UpdatedAt),
	}
	if s.OrganizationID != nil {
		v.Client = &orgRefView{ID: s.OrganizationID.String(), Name: s.OrganizationName}
	}
	return v
}

func credentialOf(c CredentialRecord) credentialView {
	return credentialView{
		ID:             c.ID.String(),
		Label:          c.Label,
		CredentialKind: c.CredentialKind,
		StorePath:      c.StorePath,
		Owner:          personRef(c.OwnerPersonID, c.OwnerName),
		RotatedOn:      dayStr(c.RotatedOn),
		NextRotationOn: dayStr(c.NextRotationOn),
		Notes:          c.Notes,
		Active:         c.Active,
		Version:        c.Version,
	}
}

func checkOf(c Check) checkView {
	v := checkView{
		ID:                     c.ID.String(),
		SystemID:               c.SystemID.String(),
		SystemName:             c.SystemName,
		SystemCriticality:      string(c.SystemCriticality),
		Name:                   c.Name,
		CheckType:              string(c.CheckType),
		Target:                 c.Target,
		Expect:                 c.Expect,
		IntervalSeconds:        c.IntervalSeconds,
		TimeoutSeconds:         c.TimeoutSeconds,
		Regions:                c.Regions,
		WarnMS:                 c.WarnMS,
		FailMS:                 c.FailMS,
		FailureThreshold:       c.FailureThreshold,
		HeartbeatWindowSeconds: c.HeartbeatWindowSeconds,
		Active:                 c.Active,
		LastAt:                 rfcPtr(c.LastAt),
		LastLatencyMS:          c.LastLatencyMS,
		ConsecutiveFailures:    c.ConsecutiveFailures,
		Version:                c.Version,
		CreatedAt:              rfc(c.CreatedAt),
		UpdatedAt:              rfc(c.UpdatedAt),
	}
	if v.Regions == nil {
		v.Regions = []string{}
	}
	if c.LastStatus != nil {
		s := string(*c.LastStatus)
		v.LastStatus = &s
	}
	return v
}

func checkResultOf(r CheckResult) checkResultView {
	return checkResultView{
		ID:        r.ID.String(),
		CheckID:   r.CheckID.String(),
		At:        rfc(r.At),
		Status:    string(r.Status),
		LatencyMS: r.LatencyMS,
		Detail:    r.Detail,
		Source:    string(r.Source),
		Region:    r.Region,
	}
}

func incidentOf(in Incident) incidentView {
	names := in.SystemNames
	if names == nil {
		names = []string{}
	}
	v := incidentView{
		ID:             in.ID.String(),
		Ref:            in.Ref,
		Title:          in.Title,
		Severity:       string(in.Severity),
		SeverityLabel:  severityLabel(in.Severity),
		Status:         string(in.Status),
		DetectedAt:     rfc(in.DetectedAt),
		DetectedSource: string(in.DetectedSource),
		AcknowledgedAt: rfcPtr(in.AcknowledgedAt),
		MitigatedAt:    rfcPtr(in.MitigatedAt),
		ResolvedAt:     rfcPtr(in.ResolvedAt),
		ClosedAt:       rfcPtr(in.ClosedAt),
		Commander:      personRef(in.CommanderPersonID, in.CommanderName),
		ClientImpact:   in.ClientImpact,
		RootCause:      in.RootCause,
		Summary:        in.Summary,
		ReviewRequired: in.ReviewRequired,
		ReviewAt:       rfcPtr(in.ReviewAt),
		SystemNames:    names,
		MTTASeconds:    elapsedSeconds(in.DetectedAt, in.AcknowledgedAt),
		MTTRSeconds:    elapsedSeconds(in.DetectedAt, in.ResolvedAt),
		Version:        in.Version,
		CreatedAt:      rfc(in.CreatedAt),
		UpdatedAt:      rfc(in.UpdatedAt),
	}
	if in.Systems != nil {
		v.Systems = make([]incidentSystemView, 0, len(in.Systems))
		for _, s := range in.Systems {
			v.Systems = append(v.Systems, incidentSystemView{
				SystemID: s.SystemID.String(), Ref: s.Ref, Name: s.Name, Criticality: string(s.Criticality),
			})
		}
	}
	if in.Timeline != nil {
		v.Timeline = make([]timelineEntryView, 0, len(in.Timeline))
		for _, e := range in.Timeline {
			v.Timeline = append(v.Timeline, timelineEntryView{
				ID: e.ID.String(), At: rfc(e.At), Actor: personRef(e.ActorPersonID, e.ActorName),
				EntryType: string(e.EntryType), Note: e.Note,
			})
		}
	}
	if in.Actions != nil {
		v.Actions = make([]incidentActionView, 0, len(in.Actions))
		for _, a := range in.Actions {
			v.Actions = append(v.Actions, incidentActionView{
				ID: a.ID.String(), Title: a.Title, Owner: personRef(a.OwnerPersonID, a.OwnerName),
				DueOn: dayStr(a.DueOn), Status: string(a.Status), TaskID: idStr(a.TaskID),
				Version: a.Version, CreatedAt: rfc(a.CreatedAt), UpdatedAt: rfc(a.UpdatedAt),
			})
		}
	}
	return v
}

// elapsedSeconds is how long a phase took, or nil while it is still running.
func elapsedSeconds(from time.Time, to *time.Time) *int64 {
	if to == nil {
		return nil
	}
	n := int64(to.Sub(from).Seconds())
	return &n
}

func expiryOf(e Expiry) expiryView {
	return expiryView{
		Kind:        string(e.Kind),
		ID:          e.ID.String(),
		Name:        e.Name,
		ExpiresOn:   e.ExpiresOn.UTC().Format("2006-01-02"),
		DaysOut:     e.DaysOut,
		AutoRenew:   e.AutoRenew,
		WarningBand: e.WarningBand,
		System:      systemRef(e.SystemID, e.SystemName),
		Owner:       &personRefView{Name: e.OwnerName},
	}
}

func idStr(id *uuid.UUID) string {
	if id == nil {
		return ""
	}
	return id.String()
}

// ── handlers ────────────────────────────────────────────────────────────────

// Overview handles GET /api/v1/systems/overview.
func (h *Handler) Overview(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.principal(w, r); !ok {
		return
	}
	o, err := h.service.Overview(r.Context())
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, overviewOf(o))
}

// ListSystems handles GET /api/v1/systems.
func (h *Handler) ListSystems(w http.ResponseWriter, r *http.Request) {
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
	var q SystemQuery
	var ok bool
	if raw := trim(r.URL.Query().Get("type")); raw != "" {
		t := SystemType(raw)
		q.Type = &t
	}
	if raw := trim(r.URL.Query().Get("environment")); raw != "" {
		e := Environment(raw)
		q.Environment = &e
	}
	if raw := trim(r.URL.Query().Get("criticality")); raw != "" {
		c := Criticality(raw)
		q.Criticality = &c
	}
	if raw := trim(r.URL.Query().Get("status")); raw != "" {
		s := SystemStatus(raw)
		q.Status = &s
	}
	if q.OwnerID, ok = h.uuidParam(w, r, "owner_person_id"); !ok {
		return
	}
	if q.OrganizationID, ok = h.uuidParam(w, r, "organization_id"); !ok {
		return
	}
	if q.UnhealthyOnly, ok = h.boolParam(w, r, "unhealthy"); !ok {
		return
	}
	q.Search = trim(r.URL.Query().Get("q"))

	items, err := h.service.Systems(r.Context(), q, limit+1, before)
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
	views := make([]systemView, 0, len(items))
	for _, s := range items {
		views = append(views, systemOf(s))
	}
	httpx.WriteJSON(w, http.StatusOK, httpx.NewPage(views, next, limit))
}

// GetSystem handles GET /api/v1/systems/{id}. It carries the system's
// credential records — locations only, never secrets (§8.6).
func (h *Handler) GetSystem(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.principal(w, r); !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "system")
	if !ok {
		return
	}
	sys, err := h.service.System(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	view := systemOf(sys)
	creds, err := h.service.Credentials(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	view.Credentials = make([]credentialView, 0, len(creds))
	for _, c := range creds {
		view.Credentials = append(view.Credentials, credentialOf(c))
	}
	httpx.SetETag(w, int64(sys.Version))
	httpx.WriteJSON(w, http.StatusOK, view)
}

// ListChecks handles GET /api/v1/systems/checks.
func (h *Handler) ListChecks(w http.ResponseWriter, r *http.Request) {
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
	var q CheckQuery
	var ok bool
	if q.SystemID, ok = h.uuidParam(w, r, "system_id"); !ok {
		return
	}
	if raw := trim(r.URL.Query().Get("type")); raw != "" {
		t := CheckType(raw)
		q.Type = &t
	}
	if raw := trim(r.URL.Query().Get("status")); raw != "" {
		s := CheckStatus(raw)
		q.Status = &s
	}
	if active, ok := h.boolParam(w, r, "active"); !ok {
		return
	} else if trim(r.URL.Query().Get("active")) != "" {
		q.Active = &active
	}
	q.Search = trim(r.URL.Query().Get("q"))

	items, err := h.service.Checks(r.Context(), q, limit+1, before)
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
	views := make([]checkView, 0, len(items))
	for _, c := range items {
		views = append(views, checkOf(c))
	}
	httpx.WriteJSON(w, http.StatusOK, httpx.NewPage(views, next, limit))
}

// ListCheckResults handles GET /api/v1/systems/checks/{id}/results.
func (h *Handler) ListCheckResults(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.principal(w, r); !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "check")
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
	items, err := h.service.CheckResults(r.Context(), id, limit+1, before)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	next := ""
	if len(items) > limit {
		items = items[:limit]
		last := items[len(items)-1]
		next = encodeCursor(last.At, last.ID)
	}
	views := make([]checkResultView, 0, len(items))
	for _, res := range items {
		views = append(views, checkResultOf(res))
	}
	httpx.WriteJSON(w, http.StatusOK, httpx.NewPage(views, next, limit))
}

// ListIncidents handles GET /api/v1/systems/incidents.
func (h *Handler) ListIncidents(w http.ResponseWriter, r *http.Request) {
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
	var q IncidentQuery
	var ok bool
	if raw := trim(r.URL.Query().Get("severity")); raw != "" {
		s := IncidentSeverity(raw)
		q.Severity = &s
	}
	if raw := trim(r.URL.Query().Get("status")); raw != "" {
		s := IncidentStatus(raw)
		q.Status = &s
	}
	if q.SystemID, ok = h.uuidParam(w, r, "system_id"); !ok {
		return
	}
	if q.CommanderID, ok = h.uuidParam(w, r, "commander_person_id"); !ok {
		return
	}
	if q.From, ok = h.timeParam(w, r, "from"); !ok {
		return
	}
	if q.To, ok = h.timeParam(w, r, "to"); !ok {
		return
	}
	if open, ok := h.boolParam(w, r, "open"); !ok {
		return
	} else if trim(r.URL.Query().Get("open")) != "" {
		q.OpenOnly = open
	}
	if review, ok := h.boolParam(w, r, "review_required"); !ok {
		return
	} else if trim(r.URL.Query().Get("review_required")) != "" {
		q.ReviewRequired = &review
	}
	q.Search = trim(r.URL.Query().Get("q"))

	items, err := h.service.Incidents(r.Context(), q, limit+1, before)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	next := ""
	if len(items) > limit {
		items = items[:limit]
		last := items[len(items)-1]
		next = encodeCursor(last.DetectedAt, last.ID)
	}
	views := make([]incidentView, 0, len(items))
	for _, in := range items {
		views = append(views, incidentOf(in))
	}
	httpx.WriteJSON(w, http.StatusOK, httpx.NewPage(views, next, limit))
}

// GetIncident handles GET /api/v1/systems/incidents/{id}.
func (h *Handler) GetIncident(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.principal(w, r); !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "incident")
	if !ok {
		return
	}
	in, err := h.service.Incident(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(in.Version))
	httpx.WriteJSON(w, http.StatusOK, incidentOf(in))
}

// Expiring handles GET /api/v1/systems/expiring: domains, certificates and
// credential rotations lapsing within the requested window (§8.6 warns at
// 30/14/7 days).
func (h *Handler) Expiring(w http.ResponseWriter, r *http.Request) {
	if _, ok := h.principal(w, r); !ok {
		return
	}
	within := ExpiryWarningDays[0]
	if raw := trim(r.URL.Query().Get("within_days")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			h.writeError(w, r, errInvalidField("within_days"))
			return
		}
		within = n
	}
	items, err := h.service.Expiring(r.Context(), within)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	views := make([]expiryView, 0, len(items))
	for _, e := range items {
		views = append(views, expiryOf(e))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"items":               views,
		"within_days":         within,
		"expiry_warning_days": ExpiryWarningDays,
	})
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

// uuidParam reads an optional uuid query parameter. An absent or empty value is
// nil (no filter); a present but malformed one is a validation error.
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

// boolParam reads an optional boolean query parameter.
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
		httpx.WriteProblem(w, r, httpx.NotFound("No such systems record."))
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
		return nil, errors.New("systems: malformed cursor")
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
