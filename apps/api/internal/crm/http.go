package crm

import (
	"crypto/subtle"
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

// Handler serves the CRM HTTP surface (§8.1).
type Handler struct {
	service       *Service
	logger        *slog.Logger
	webhookSecret string
}

// NewHandler builds a CRM handler. webhookSecret guards the public
// website-form webhook; when empty the route is refused.
func NewHandler(service *Service, webhookSecret string, logger *slog.Logger) *Handler {
	return &Handler{service: service, logger: logger, webhookSecret: webhookSecret}
}

// ── views ───────────────────────────────────────────────────────────────────

type organizationView struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Industry      string `json:"industry,omitempty"`
	Size          string `json:"size,omitempty"`
	Location      string `json:"location,omitempty"`
	KRAPin        string `json:"kra_pin,omitempty"`
	Status        string `json:"status"`
	OwnerPersonID string `json:"owner_person_id,omitempty"`
	Version       int    `json:"version"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

type contactView struct {
	ID                     string   `json:"id"`
	OrganizationID         string   `json:"organization_id,omitempty"`
	Name                   string   `json:"name"`
	Role                   string   `json:"role,omitempty"`
	Phones                 []string `json:"phones"`
	Emails                 []string `json:"emails"`
	WhatsAppNumber         string   `json:"whatsapp_number,omitempty"`
	ConsentMarketing       bool     `json:"consent_marketing"`
	ConsentMarketingAt     string   `json:"consent_marketing_at,omitempty"`
	ConsentMarketingSource string   `json:"consent_marketing_source,omitempty"`
	ConsentWhatsApp        bool     `json:"consent_whatsapp"`
	ConsentWhatsAppAt      string   `json:"consent_whatsapp_at,omitempty"`
	ConsentRecording       bool     `json:"consent_call_recording"`
	ConsentRecordingAt     string   `json:"consent_call_recording_at,omitempty"`
	Version                int      `json:"version"`
	CreatedAt              string   `json:"created_at"`
	UpdatedAt              string   `json:"updated_at"`
}

type leadSourceView struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

type pipelineStageView struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	IsWon  bool   `json:"is_won"`
	IsLost bool   `json:"is_lost"`
}

type leadView struct {
	ID                  string `json:"id"`
	Ref                 string `json:"ref"`
	SourceKey           string `json:"source_key"`
	SourceDetail        string `json:"source_detail,omitempty"`
	OrganizationID      string `json:"organization_id,omitempty"`
	ContactID           string `json:"contact_id,omitempty"`
	ContactName         string `json:"contact_name,omitempty"`
	ContactPhone        string `json:"contact_phone,omitempty"`
	ContactEmail        string `json:"contact_email,omitempty"`
	NeedSummary         string `json:"need_summary,omitempty"`
	EstimatedValueMinor *int64 `json:"estimated_value_minor,omitempty"`
	Currency            string `json:"currency"`
	Priority            string `json:"priority"`
	OwnerPersonID       string `json:"owner_person_id,omitempty"`
	Status              string `json:"status"`
	NextAction          string `json:"next_action,omitempty"`
	NextActionDueAt     string `json:"next_action_due_at,omitempty"`
	QualificationNotes  string `json:"qualification_notes,omitempty"`
	LostReason          string `json:"lost_reason,omitempty"`
	OpportunityID       string `json:"converted_opportunity_id,omitempty"`
	Version             int    `json:"version"`
	CreatedAt           string `json:"created_at"`
	UpdatedAt           string `json:"updated_at"`
}

type opportunityView struct {
	ID              string `json:"id"`
	Ref             string `json:"ref"`
	OrganizationID  string `json:"organization_id,omitempty"`
	Title           string `json:"title"`
	ValueMinor      *int64 `json:"value_minor,omitempty"`
	Currency        string `json:"currency"`
	Probability     *int   `json:"probability,omitempty"`
	StageKey        string `json:"stage_key"`
	ExpectedCloseAt string `json:"expected_close_at,omitempty"`
	OwnerPersonID   string `json:"owner_person_id,omitempty"`
	LostReason      string `json:"lost_reason,omitempty"`
	Version         int    `json:"version"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
}

type interactionView struct {
	ID              string `json:"id"`
	Type            string `json:"interaction_type"`
	Direction       string `json:"direction"`
	ContactID       string `json:"contact_id,omitempty"`
	LeadID          string `json:"lead_id,omitempty"`
	OpportunityID   string `json:"opportunity_id,omitempty"`
	OccurredAt      string `json:"occurred_at"`
	DurationMinutes *int   `json:"duration_minutes,omitempty"`
	Outcome         string `json:"outcome"`
	Summary         string `json:"summary,omitempty"`
	NextAction      string `json:"next_action,omitempty"`
	NextActionDueAt string `json:"next_action_due_at,omitempty"`
	RecordingFileID string `json:"recording_file_id,omitempty"`
	RecordedBy      string `json:"recorded_by"`
	Version         int    `json:"version"`
	CreatedAt       string `json:"created_at"`
}

type followupView struct {
	ID            string `json:"id"`
	LeadID        string `json:"lead_id,omitempty"`
	OpportunityID string `json:"opportunity_id,omitempty"`
	OwnerPersonID string `json:"owner_person_id"`
	DueAt         string `json:"due_at"`
	Channel       string `json:"channel"`
	Reason        string `json:"reason"`
	Status        string `json:"status"`
	Outcome       string `json:"outcome,omitempty"`
	CompletedAt   string `json:"completed_at,omitempty"`
	Version       int    `json:"version"`
	CreatedAt     string `json:"created_at"`
}

type proposalView struct {
	ID             string          `json:"id"`
	Ref            string          `json:"ref"`
	OpportunityID  string          `json:"opportunity_id"`
	VersionNo      int             `json:"version_no"`
	ScopeSummary   string          `json:"scope_summary,omitempty"`
	LineItems      json.RawMessage `json:"line_items"`
	PriceMinor     int64           `json:"price_minor"`
	Currency       string          `json:"currency"`
	DiscountMinor  int64           `json:"discount_minor"`
	ValidUntil     string          `json:"valid_until,omitempty"`
	Status         string          `json:"status"`
	DocumentFileID string          `json:"document_file_id,omitempty"`
	Version        int             `json:"version"`
	CreatedAt      string          `json:"created_at"`
	UpdatedAt      string          `json:"updated_at"`
}

type pipelineTotalView struct {
	StageKey   string `json:"stage_key"`
	Label      string `json:"label"`
	Count      int    `json:"count"`
	ValueMinor int64  `json:"value_minor"`
}

// ── converters ──────────────────────────────────────────────────────────────

func rfc(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func optRFC(t *time.Time) string {
	if t == nil {
		return ""
	}
	return rfc(*t)
}

func idStr(id *uuid.UUID) string {
	if id == nil {
		return ""
	}
	return id.String()
}

func strOpt(id *uuid.UUID) string { return idStr(id) }

func organizationOf(o Organization) organizationView {
	return organizationView{
		ID: o.ID.String(), Name: o.Name, Industry: o.Industry, Size: o.Size,
		Location: o.Location, KRAPin: o.KRAPin, Status: string(o.Status),
		OwnerPersonID: idStr(o.OwnerPersonID), Version: o.Version,
		CreatedAt: rfc(o.CreatedAt), UpdatedAt: rfc(o.UpdatedAt),
	}
}

func contactOf(c Contact) contactView {
	return contactView{
		ID: c.ID.String(), OrganizationID: idStr(c.OrganizationID), Name: c.Name, Role: c.Role,
		Phones: c.Phones, Emails: c.Emails, WhatsAppNumber: c.WhatsAppNumber,
		ConsentMarketing: c.ConsentMarketing, ConsentMarketingAt: optRFC(c.ConsentMarketingAt),
		ConsentMarketingSource: c.ConsentMarketingSource,
		ConsentWhatsApp:        c.ConsentWhatsApp, ConsentWhatsAppAt: optRFC(c.ConsentWhatsAppAt),
		ConsentRecording: c.ConsentRecording, ConsentRecordingAt: optRFC(c.ConsentRecordingAt),
		Version: c.Version, CreatedAt: rfc(c.CreatedAt), UpdatedAt: rfc(c.UpdatedAt),
	}
}

func leadOf(l Lead) leadView {
	v := leadView{
		ID: l.ID.String(), Ref: l.Ref, SourceKey: l.SourceKey, SourceDetail: l.SourceDetail,
		OrganizationID: idStr(l.OrganizationID), ContactID: idStr(l.ContactID),
		ContactName: l.ContactName, ContactPhone: l.ContactPhone, ContactEmail: l.ContactEmail,
		NeedSummary: l.NeedSummary, EstimatedValueMinor: l.EstimatedValueMinor, Currency: l.Currency,
		Priority: string(l.Priority), OwnerPersonID: idStr(l.OwnerPersonID), Status: string(l.Status),
		NextAction: l.NextAction, NextActionDueAt: optRFC(l.NextActionDueAt),
		QualificationNotes: l.QualificationNotes, OpportunityID: idStr(l.ConvertedOpportunityID),
		Version: l.Version, CreatedAt: rfc(l.CreatedAt), UpdatedAt: rfc(l.UpdatedAt),
	}
	if l.LostReason != nil {
		v.LostReason = string(*l.LostReason)
	}
	return v
}

func opportunityOf(o Opportunity) opportunityView {
	v := opportunityView{
		ID: o.ID.String(), Ref: o.Ref, OrganizationID: idStr(o.OrganizationID), Title: o.Title,
		ValueMinor: o.ValueMinor, Currency: o.Currency, Probability: o.Probability,
		StageKey: o.StageKey, ExpectedCloseAt: optRFC(o.ExpectedCloseAt),
		OwnerPersonID: idStr(o.OwnerPersonID), Version: o.Version,
		CreatedAt: rfc(o.CreatedAt), UpdatedAt: rfc(o.UpdatedAt),
	}
	if o.LostReason != nil {
		v.LostReason = string(*o.LostReason)
	}
	return v
}

func interactionOf(i Interaction) interactionView {
	return interactionView{
		ID: i.ID.String(), Type: string(i.Type), Direction: string(i.Direction),
		ContactID: idStr(i.ContactID), LeadID: idStr(i.LeadID), OpportunityID: idStr(i.OpportunityID),
		OccurredAt: rfc(i.OccurredAt), DurationMinutes: i.DurationMinutes, Outcome: i.Outcome,
		Summary: i.Summary, NextAction: i.NextAction, NextActionDueAt: optRFC(i.NextActionDueAt),
		RecordingFileID: idStr(i.RecordingFileID), RecordedBy: i.RecordedBy.String(),
		Version: i.Version, CreatedAt: rfc(i.CreatedAt),
	}
}

func followupOf(f Followup) followupView {
	return followupView{
		ID: f.ID.String(), LeadID: idStr(f.LeadID), OpportunityID: idStr(f.OpportunityID),
		OwnerPersonID: f.OwnerPersonID.String(), DueAt: rfc(f.DueAt), Channel: string(f.Channel),
		Reason: f.Reason, Status: string(f.Status), Outcome: f.Outcome,
		CompletedAt: optRFC(f.CompletedAt), Version: f.Version, CreatedAt: rfc(f.CreatedAt),
	}
}

func proposalOf(p Proposal) proposalView {
	items := json.RawMessage(p.LineItems)
	if len(items) == 0 {
		items = json.RawMessage("[]")
	}
	return proposalView{
		ID: p.ID.String(), Ref: p.Ref, OpportunityID: p.OpportunityID.String(), VersionNo: p.VersionNo,
		ScopeSummary: p.ScopeSummary, LineItems: items, PriceMinor: p.PriceMinor, Currency: p.Currency,
		DiscountMinor: p.DiscountMinor, ValidUntil: optRFC(p.ValidUntil), Status: string(p.Status),
		DocumentFileID: idStr(p.DocumentFileID), Version: p.Version,
		CreatedAt: rfc(p.CreatedAt), UpdatedAt: rfc(p.UpdatedAt),
	}
}

// ── organizations ───────────────────────────────────────────────────────────

func (h *Handler) ListOrganizations(w http.ResponseWriter, r *http.Request) {
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
	items, err := h.service.ListOrganizations(r.Context(), r.URL.Query().Get("q"), limit+1, before)
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
	views := make([]organizationView, 0, len(items))
	for _, o := range items {
		views = append(views, organizationOf(o))
	}
	httpx.WriteJSON(w, http.StatusOK, httpx.NewPage(views, next, limit))
}

func (h *Handler) CreateOrganization(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	var req organizationReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	in, err := toOrganizationInput(req)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	o, err := h.service.CreateOrganization(r.Context(), principal.PersonID, in)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(o.Version))
	httpx.WriteJSON(w, http.StatusCreated, organizationOf(o))
}

func (h *Handler) GetOrganization(w http.ResponseWriter, r *http.Request) {
	id, ok := h.idParam(w, r, "id", "organization")
	if !ok {
		return
	}
	o, err := h.service.GetOrganization(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(o.Version))
	httpx.WriteJSON(w, http.StatusOK, organizationOf(o))
}

func (h *Handler) UpdateOrganization(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "organization")
	if !ok {
		return
	}
	current, err := h.service.GetOrganization(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	if err := httpx.RequireIfMatch(r, int64(current.Version)); err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	var req organizationReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	in, err := toOrganizationInput(req)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	o, err := h.service.UpdateOrganization(r.Context(), principal.PersonID, id, current.Version, in)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(o.Version))
	httpx.WriteJSON(w, http.StatusOK, organizationOf(o))
}

// ── contacts ────────────────────────────────────────────────────────────────

func (h *Handler) ListContacts(w http.ResponseWriter, r *http.Request) {
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
	var orgID *uuid.UUID
	if raw := strings.TrimSpace(r.URL.Query().Get("organization_id")); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			httpx.WriteProblem(w, r, httpx.BadRequest("organization_id is not a valid id."))
			return
		}
		orgID = &id
	}
	items, err := h.service.ListContacts(r.Context(), orgID, limit+1, before)
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
	views := make([]contactView, 0, len(items))
	for _, c := range items {
		views = append(views, contactOf(c))
	}
	httpx.WriteJSON(w, http.StatusOK, httpx.NewPage(views, next, limit))
}

func (h *Handler) CreateContact(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	var req contactReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	in, err := toContactInput(req)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	c, err := h.service.CreateContact(r.Context(), principal.PersonID, in)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(c.Version))
	httpx.WriteJSON(w, http.StatusCreated, contactOf(c))
}

func (h *Handler) GetContact(w http.ResponseWriter, r *http.Request) {
	id, ok := h.idParam(w, r, "id", "contact")
	if !ok {
		return
	}
	c, err := h.service.GetContact(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(c.Version))
	httpx.WriteJSON(w, http.StatusOK, contactOf(c))
}

func (h *Handler) UpdateContact(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "contact")
	if !ok {
		return
	}
	current, err := h.service.GetContact(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	if err := httpx.RequireIfMatch(r, int64(current.Version)); err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	var req contactReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	in, err := toContactInput(req)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	c, err := h.service.UpdateContact(r.Context(), principal.PersonID, id, current.Version, in)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(c.Version))
	httpx.WriteJSON(w, http.StatusOK, contactOf(c))
}

// ── config ──────────────────────────────────────────────────────────────────

func (h *Handler) ListLeadSources(w http.ResponseWriter, r *http.Request) {
	sources, err := h.service.ListLeadSources(r.Context())
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	views := make([]leadSourceView, 0, len(sources))
	for _, s := range sources {
		views = append(views, leadSourceView{Key: s.Key, Label: s.Label})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": views})
}

func (h *Handler) ListPipelineStages(w http.ResponseWriter, r *http.Request) {
	stages, err := h.service.ListPipelineStages(r.Context())
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	views := make([]pipelineStageView, 0, len(stages))
	for _, s := range stages {
		views = append(views, pipelineStageView{Key: s.Key, Label: s.Label, IsWon: s.IsWon, IsLost: s.IsLost})
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": views})
}

// ── leads ───────────────────────────────────────────────────────────────────

func (h *Handler) ListLeads(w http.ResponseWriter, r *http.Request) {
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
	q := ListLeadsQuery{}
	if raw := strings.TrimSpace(r.URL.Query().Get("status")); raw != "" {
		s := LeadStatus(raw)
		q.Status = &s
	}
	q.SourceKey = strings.TrimSpace(r.URL.Query().Get("source_key"))
	for _, f := range []struct {
		name string
		dst  **uuid.UUID
	}{{"owner_person_id", &q.OwnerID}, {"organization_id", &q.OrganizationID}} {
		if raw := strings.TrimSpace(r.URL.Query().Get(f.name)); raw != "" {
			id, err := uuid.Parse(raw)
			if err != nil {
				httpx.WriteProblem(w, r, httpx.BadRequest(f.name+" is not a valid id."))
				return
			}
			*f.dst = &id
		}
	}
	items, err := h.service.ListLeads(r.Context(), q, limit+1, before)
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
	views := make([]leadView, 0, len(items))
	for _, l := range items {
		views = append(views, leadOf(l))
	}
	httpx.WriteJSON(w, http.StatusOK, httpx.NewPage(views, next, limit))
}

func (h *Handler) CreateLead(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	var req leadReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	in, err := toLeadInput(req)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	l, err := h.service.CreateLead(r.Context(), principal.PersonID, in)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(l.Version))
	httpx.WriteJSON(w, http.StatusCreated, leadOf(l))
}

func (h *Handler) GetLead(w http.ResponseWriter, r *http.Request) {
	id, ok := h.idParam(w, r, "id", "lead")
	if !ok {
		return
	}
	l, err := h.service.GetLead(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(l.Version))
	httpx.WriteJSON(w, http.StatusOK, leadOf(l))
}

func (h *Handler) UpdateLead(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "lead")
	if !ok {
		return
	}
	current, err := h.service.GetLead(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	if err := httpx.RequireIfMatch(r, int64(current.Version)); err != nil {
		httpx.WriteError(w, r, h.logger, err)
		return
	}
	var req leadReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	in, err := toLeadInput(req)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	l, err := h.service.UpdateLead(r.Context(), principal.PersonID, id, current.Version, in)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(l.Version))
	httpx.WriteJSON(w, http.StatusOK, leadOf(l))
}

func (h *Handler) AssignLead(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "lead")
	if !ok {
		return
	}
	var req assignLeadReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	owner, err := uuid.Parse(strings.TrimSpace(req.OwnerPersonID))
	if err != nil {
		httpx.WriteProblem(w, r, httpx.BadRequest("owner_person_id is not a valid id."))
		return
	}
	due, err := optTime(req.NextActionDueAt, "next_action_due_at")
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	var dueAt time.Time
	if due != nil {
		dueAt = *due
	}
	l, err := h.service.AssignLead(r.Context(), principal.PersonID, id, owner, req.NextAction, dueAt)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(l.Version))
	httpx.WriteJSON(w, http.StatusOK, leadOf(l))
}

func (h *Handler) SetLeadStatus(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "lead")
	if !ok {
		return
	}
	var req leadStatusReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	var reason *LostReason
	if req.LostReason != nil && strings.TrimSpace(*req.LostReason) != "" {
		r2 := LostReason(strings.TrimSpace(*req.LostReason))
		reason = &r2
	}
	l, err := h.service.SetLeadStatus(r.Context(), principal.PersonID, id, LeadStatus(strings.TrimSpace(req.Status)), reason)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(l.Version))
	httpx.WriteJSON(w, http.StatusOK, leadOf(l))
}

func (h *Handler) ConvertLead(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "lead")
	if !ok {
		return
	}
	var req convertReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	in, err := toConvertInput(req)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	opp, lead, err := h.service.ConvertLead(r.Context(), principal.PersonID, id, in)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(opp.Version))
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"opportunity": opportunityOf(opp),
		"lead":        leadOf(lead),
	})
}

// ── opportunities ───────────────────────────────────────────────────────────

func (h *Handler) ListOpportunities(w http.ResponseWriter, r *http.Request) {
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
	var stageKey *string
	if raw := strings.TrimSpace(r.URL.Query().Get("stage_key")); raw != "" {
		stageKey = &raw
	}
	var ownerID *uuid.UUID
	if raw := strings.TrimSpace(r.URL.Query().Get("owner_person_id")); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			httpx.WriteProblem(w, r, httpx.BadRequest("owner_person_id is not a valid id."))
			return
		}
		ownerID = &id
	}
	items, err := h.service.ListOpportunities(r.Context(), stageKey, ownerID, limit+1, before)
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
	views := make([]opportunityView, 0, len(items))
	for _, o := range items {
		views = append(views, opportunityOf(o))
	}
	httpx.WriteJSON(w, http.StatusOK, httpx.NewPage(views, next, limit))
}

func (h *Handler) GetOpportunity(w http.ResponseWriter, r *http.Request) {
	id, ok := h.idParam(w, r, "id", "opportunity")
	if !ok {
		return
	}
	o, err := h.service.GetOpportunity(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(o.Version))
	httpx.WriteJSON(w, http.StatusOK, opportunityOf(o))
}

func (h *Handler) MoveOpportunityStage(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "opportunity")
	if !ok {
		return
	}
	var req stageReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	o, err := h.service.MoveOpportunityStage(r.Context(), principal.PersonID, id, req.StageKey)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(o.Version))
	httpx.WriteJSON(w, http.StatusOK, opportunityOf(o))
}

func (h *Handler) WinOpportunity(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "opportunity")
	if !ok {
		return
	}
	o, err := h.service.WinOpportunity(r.Context(), principal.PersonID, id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(o.Version))
	httpx.WriteJSON(w, http.StatusOK, opportunityOf(o))
}

func (h *Handler) LoseOpportunity(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "opportunity")
	if !ok {
		return
	}
	var req loseReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	o, err := h.service.LoseOpportunity(r.Context(), principal.PersonID, id, LostReason(strings.TrimSpace(req.LostReason)))
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(o.Version))
	httpx.WriteJSON(w, http.StatusOK, opportunityOf(o))
}

// ── interactions ────────────────────────────────────────────────────────────

func (h *Handler) ListInteractions(w http.ResponseWriter, r *http.Request) {
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
	q := ListInteractionsQuery{}
	for _, f := range []struct {
		name string
		dst  **uuid.UUID
	}{{"lead_id", &q.LeadID}, {"opportunity_id", &q.OpportunityID}, {"contact_id", &q.ContactID}} {
		if raw := strings.TrimSpace(r.URL.Query().Get(f.name)); raw != "" {
			id, err := uuid.Parse(raw)
			if err != nil {
				httpx.WriteProblem(w, r, httpx.BadRequest(f.name+" is not a valid id."))
				return
			}
			*f.dst = &id
		}
	}
	items, err := h.service.ListInteractions(r.Context(), q, limit+1, before)
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
	views := make([]interactionView, 0, len(items))
	for _, i := range items {
		views = append(views, interactionOf(i))
	}
	httpx.WriteJSON(w, http.StatusOK, httpx.NewPage(views, next, limit))
}

func (h *Handler) CreateInteraction(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	var req interactionReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	in, err := toInteractionInput(req)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	i, err := h.service.RecordInteraction(r.Context(), principal.PersonID, in)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(i.Version))
	httpx.WriteJSON(w, http.StatusCreated, interactionOf(i))
}

func (h *Handler) GetInteraction(w http.ResponseWriter, r *http.Request) {
	id, ok := h.idParam(w, r, "id", "interaction")
	if !ok {
		return
	}
	i, err := h.service.GetInteraction(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(i.Version))
	httpx.WriteJSON(w, http.StatusOK, interactionOf(i))
}

// ── follow-ups ──────────────────────────────────────────────────────────────

func (h *Handler) ListFollowups(w http.ResponseWriter, r *http.Request) {
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
	q := ListFollowupsQuery{}
	if raw := strings.TrimSpace(r.URL.Query().Get("status")); raw != "" {
		s := FollowupStatus(raw)
		q.Status = &s
	}
	for _, f := range []struct {
		name string
		dst  **uuid.UUID
	}{{"owner_person_id", &q.OwnerID}, {"lead_id", &q.LeadID}} {
		if raw := strings.TrimSpace(r.URL.Query().Get(f.name)); raw != "" {
			id, err := uuid.Parse(raw)
			if err != nil {
				httpx.WriteProblem(w, r, httpx.BadRequest(f.name+" is not a valid id."))
				return
			}
			*f.dst = &id
		}
	}
	items, err := h.service.ListFollowups(r.Context(), q, limit+1, before)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	next := ""
	if len(items) > limit {
		items = items[:limit]
		last := items[len(items)-1]
		next = encodeCursor(last.DueAt, last.ID)
	}
	views := make([]followupView, 0, len(items))
	for _, f := range items {
		views = append(views, followupOf(f))
	}
	httpx.WriteJSON(w, http.StatusOK, httpx.NewPage(views, next, limit))
}

func (h *Handler) CreateFollowup(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	var req followupReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	in, err := toFollowupInput(req)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	f, err := h.service.ScheduleFollowup(r.Context(), principal.PersonID, in)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(f.Version))
	httpx.WriteJSON(w, http.StatusCreated, followupOf(f))
}

func (h *Handler) GetFollowup(w http.ResponseWriter, r *http.Request) {
	id, ok := h.idParam(w, r, "id", "follow-up")
	if !ok {
		return
	}
	f, err := h.service.GetFollowup(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(f.Version))
	httpx.WriteJSON(w, http.StatusOK, followupOf(f))
}

func (h *Handler) CompleteFollowup(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "follow-up")
	if !ok {
		return
	}
	var req outcomeReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	f, err := h.service.CompleteFollowup(r.Context(), principal.PersonID, id, req.Outcome)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(f.Version))
	httpx.WriteJSON(w, http.StatusOK, followupOf(f))
}

func (h *Handler) CancelFollowup(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "follow-up")
	if !ok {
		return
	}
	var req outcomeReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	f, err := h.service.CancelFollowup(r.Context(), principal.PersonID, id, req.Outcome)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(f.Version))
	httpx.WriteJSON(w, http.StatusOK, followupOf(f))
}

// ── proposals ───────────────────────────────────────────────────────────────

func (h *Handler) ListProposals(w http.ResponseWriter, r *http.Request) {
	id, ok := h.idParam(w, r, "id", "opportunity")
	if !ok {
		return
	}
	items, err := h.service.ListProposals(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	views := make([]proposalView, 0, len(items))
	for _, p := range items {
		views = append(views, proposalOf(p))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": views})
}

func (h *Handler) CreateProposal(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "opportunity")
	if !ok {
		return
	}
	var req proposalReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	in, err := toProposalInput(id, req)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	p, err := h.service.CreateProposal(r.Context(), principal.PersonID, principal.Can("crm.proposal.discount_approve"), in)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(p.Version))
	httpx.WriteJSON(w, http.StatusCreated, proposalOf(p))
}

func (h *Handler) GetProposal(w http.ResponseWriter, r *http.Request) {
	id, ok := h.idParam(w, r, "id", "proposal")
	if !ok {
		return
	}
	p, err := h.service.GetProposal(r.Context(), id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(p.Version))
	httpx.WriteJSON(w, http.StatusOK, proposalOf(p))
}

func (h *Handler) SendProposal(w http.ResponseWriter, r *http.Request) {
	principal, ok := h.principal(w, r)
	if !ok {
		return
	}
	id, ok := h.idParam(w, r, "id", "proposal")
	if !ok {
		return
	}
	p, err := h.service.SendProposal(r.Context(), principal.PersonID, id)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.SetETag(w, int64(p.Version))
	httpx.WriteJSON(w, http.StatusOK, proposalOf(p))
}

// ── pipeline ────────────────────────────────────────────────────────────────

func (h *Handler) Pipeline(w http.ResponseWriter, r *http.Request) {
	totals, err := h.service.Pipeline(r.Context())
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	views := make([]pipelineTotalView, 0, len(totals))
	var total int64
	var count int
	for _, t := range totals {
		views = append(views, pipelineTotalView{StageKey: t.StageKey, Label: t.Label, Count: t.Count, ValueMinor: t.ValueMinor})
		total += t.ValueMinor
		count += t.Count
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"stages": views, "open_value_minor": total, "open_opportunities": count,
	})
}

// ── public webhook ──────────────────────────────────────────────────────────

type websiteFormReq struct {
	Name         string `json:"name"`
	Email        string `json:"email"`
	Phone        string `json:"phone"`
	Company      string `json:"company"`
	Need         string `json:"need"`
	SourceDetail string `json:"source_detail"`
	SourceKey    string `json:"source_key"`
}

// WebsiteForm handles POST /webhooks/website-forms. It is public: the website
// authenticates with a shared secret header, not a principal. It creates a
// website-form lead and returns its ref (§8.1).
func (h *Handler) WebsiteForm(w http.ResponseWriter, r *http.Request) {
	if h.webhookSecret == "" {
		httpx.WriteProblem(w, r, httpx.NotFound("No such endpoint."))
		return
	}
	got := r.Header.Get("X-Atlas-Webhook-Secret")
	if subtle.ConstantTimeCompare([]byte(got), []byte(h.webhookSecret)) != 1 {
		httpx.WriteProblem(w, r, httpx.Unauthenticated("Invalid webhook secret."))
		return
	}
	var req websiteFormReq
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		h.writeError(w, r, err)
		return
	}
	if strings.TrimSpace(req.Name) == "" && strings.TrimSpace(req.Email) == "" {
		httpx.WriteProblem(w, r, httpx.BadRequest("A name or email is required."))
		return
	}
	source := strings.TrimSpace(req.SourceKey)
	if source == "" {
		source = "website_form"
	}
	in := LeadInput{
		SourceKey: source, SourceDetail: req.SourceDetail,
		ContactName: req.Name, ContactEmail: req.Email, ContactPhone: req.Phone,
		NeedSummary: req.Need, Priority: PriorityNormal,
	}
	if in.SourceDetail == "" {
		in.SourceDetail = strings.TrimSpace(req.Company)
	}
	// The webhook has no principal; the lead is unowned and enters as new, to be
	// triaged by the commercial team (§8.1).
	l, err := h.service.CreateLead(r.Context(), uuid.Nil, in)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusAccepted, map[string]any{"lead_id": l.ID.String(), "ref": l.Ref})
}

// ── request bodies ──────────────────────────────────────────────────────────

type organizationReq struct {
	Name          string  `json:"name"`
	Industry      string  `json:"industry"`
	Size          string  `json:"size"`
	Location      string  `json:"location"`
	KRAPin        string  `json:"kra_pin"`
	Status        string  `json:"status"`
	OwnerPersonID *string `json:"owner_person_id"`
}

func toOrganizationInput(req organizationReq) (OrganizationInput, error) {
	in := OrganizationInput{
		Name: req.Name, Industry: req.Industry, Size: req.Size, Location: req.Location,
		KRAPin: req.KRAPin, Status: OrganizationStatus(strings.TrimSpace(req.Status)),
	}
	owner, err := optUUID(req.OwnerPersonID, "owner_person_id")
	if err != nil {
		return in, err
	}
	in.OwnerPersonID = owner
	return in, nil
}

type contactReq struct {
	OrganizationID   *string  `json:"organization_id"`
	Name             string   `json:"name"`
	Role             string   `json:"role"`
	Phones           []string `json:"phones"`
	Emails           []string `json:"emails"`
	WhatsAppNumber   string   `json:"whatsapp_number"`
	ConsentMarketing bool     `json:"consent_marketing"`
	ConsentWhatsApp  bool     `json:"consent_whatsapp"`
	ConsentRecording bool     `json:"consent_call_recording"`
	ConsentSource    string   `json:"consent_source"`
}

func toContactInput(req contactReq) (ContactInput, error) {
	in := ContactInput{
		Name: req.Name, Role: req.Role, Phones: req.Phones, Emails: req.Emails,
		WhatsAppNumber: req.WhatsAppNumber, ConsentMarketing: req.ConsentMarketing,
		ConsentWhatsApp: req.ConsentWhatsApp, ConsentRecording: req.ConsentRecording,
		ConsentSource: req.ConsentSource,
	}
	org, err := optUUID(req.OrganizationID, "organization_id")
	if err != nil {
		return in, err
	}
	in.OrganizationID = org
	return in, nil
}

type leadReq struct {
	SourceKey           string  `json:"source_key"`
	SourceDetail        string  `json:"source_detail"`
	OrganizationID      *string `json:"organization_id"`
	ContactID           *string `json:"contact_id"`
	ContactName         string  `json:"contact_name"`
	ContactPhone        string  `json:"contact_phone"`
	ContactEmail        string  `json:"contact_email"`
	NeedSummary         string  `json:"need_summary"`
	EstimatedValueMinor *int64  `json:"estimated_value_minor"`
	Currency            string  `json:"currency"`
	Priority            string  `json:"priority"`
	OwnerPersonID       *string `json:"owner_person_id"`
	Status              string  `json:"status"`
	NextAction          string  `json:"next_action"`
	NextActionDueAt     *string `json:"next_action_due_at"`
	QualificationNotes  string  `json:"qualification_notes"`
	LostReason          *string `json:"lost_reason"`
}

func toLeadInput(req leadReq) (LeadInput, error) {
	in := LeadInput{
		SourceKey: req.SourceKey, SourceDetail: req.SourceDetail, ContactName: req.ContactName,
		ContactPhone: req.ContactPhone, ContactEmail: req.ContactEmail, NeedSummary: req.NeedSummary,
		EstimatedValueMinor: req.EstimatedValueMinor, Currency: req.Currency,
		Priority: Priority(strings.TrimSpace(req.Priority)), Status: LeadStatus(strings.TrimSpace(req.Status)),
		NextAction: req.NextAction, QualificationNotes: req.QualificationNotes,
	}
	var err error
	if in.OrganizationID, err = optUUID(req.OrganizationID, "organization_id"); err != nil {
		return in, err
	}
	if in.ContactID, err = optUUID(req.ContactID, "contact_id"); err != nil {
		return in, err
	}
	if in.OwnerPersonID, err = optUUID(req.OwnerPersonID, "owner_person_id"); err != nil {
		return in, err
	}
	if in.NextActionDueAt, err = optTime(req.NextActionDueAt, "next_action_due_at"); err != nil {
		return in, err
	}
	if req.LostReason != nil && strings.TrimSpace(*req.LostReason) != "" {
		lr := LostReason(strings.TrimSpace(*req.LostReason))
		in.LostReason = &lr
	}
	return in, nil
}

type assignLeadReq struct {
	OwnerPersonID   string  `json:"owner_person_id"`
	NextAction      string  `json:"next_action"`
	NextActionDueAt *string `json:"next_action_due_at"`
}

type leadStatusReq struct {
	Status     string  `json:"status"`
	LostReason *string `json:"lost_reason"`
}

type convertReq struct {
	Title           string  `json:"title"`
	OrganizationID  *string `json:"organization_id"`
	ValueMinor      *int64  `json:"value_minor"`
	Currency        string  `json:"currency"`
	StageKey        string  `json:"stage_key"`
	Probability     *int    `json:"probability"`
	ExpectedCloseAt *string `json:"expected_close_at"`
	OwnerPersonID   *string `json:"owner_person_id"`
}

func toConvertInput(req convertReq) (ConvertInput, error) {
	in := ConvertInput{
		Title: req.Title, ValueMinor: req.ValueMinor, Currency: req.Currency,
		StageKey: req.StageKey, Probability: req.Probability,
	}
	var err error
	if in.OrganizationID, err = optUUID(req.OrganizationID, "organization_id"); err != nil {
		return in, err
	}
	if in.OwnerPersonID, err = optUUID(req.OwnerPersonID, "owner_person_id"); err != nil {
		return in, err
	}
	if in.ExpectedCloseAt, err = optTime(req.ExpectedCloseAt, "expected_close_at"); err != nil {
		return in, err
	}
	return in, nil
}

type stageReq struct {
	StageKey string `json:"stage_key"`
}

type loseReq struct {
	LostReason string `json:"lost_reason"`
}

type interactionReq struct {
	Type            string  `json:"interaction_type"`
	Direction       string  `json:"direction"`
	ContactID       *string `json:"contact_id"`
	LeadID          *string `json:"lead_id"`
	OpportunityID   *string `json:"opportunity_id"`
	OccurredAt      *string `json:"occurred_at"`
	DurationMinutes *int    `json:"duration_minutes"`
	Outcome         string  `json:"outcome"`
	Summary         string  `json:"summary"`
	NextAction      string  `json:"next_action"`
	NextActionDueAt *string `json:"next_action_due_at"`
	RecordingFileID *string `json:"recording_file_id"`
	SkipAdvance     bool    `json:"skip_advance"`
}

func toInteractionInput(req interactionReq) (InteractionInput, error) {
	in := InteractionInput{
		Type: InteractionType(strings.TrimSpace(req.Type)), Direction: Direction(strings.TrimSpace(req.Direction)),
		DurationMinutes: req.DurationMinutes, Outcome: req.Outcome, Summary: req.Summary,
		NextAction: req.NextAction, SkipAdvance: req.SkipAdvance,
	}
	var err error
	if in.ContactID, err = optUUID(req.ContactID, "contact_id"); err != nil {
		return in, err
	}
	if in.LeadID, err = optUUID(req.LeadID, "lead_id"); err != nil {
		return in, err
	}
	if in.OpportunityID, err = optUUID(req.OpportunityID, "opportunity_id"); err != nil {
		return in, err
	}
	if in.RecordingFileID, err = optUUID(req.RecordingFileID, "recording_file_id"); err != nil {
		return in, err
	}
	if in.OccurredAt, err = optTime(req.OccurredAt, "occurred_at"); err != nil {
		return in, err
	}
	if in.NextActionDueAt, err = optTime(req.NextActionDueAt, "next_action_due_at"); err != nil {
		return in, err
	}
	return in, nil
}

type followupReq struct {
	LeadID        *string `json:"lead_id"`
	OpportunityID *string `json:"opportunity_id"`
	OwnerPersonID *string `json:"owner_person_id"`
	DueAt         string  `json:"due_at"`
	Channel       string  `json:"channel"`
	Reason        string  `json:"reason"`
}

func toFollowupInput(req followupReq) (FollowupInput, error) {
	in := FollowupInput{
		Channel: FollowupChannel(strings.TrimSpace(req.Channel)), Reason: req.Reason,
	}
	var err error
	if in.LeadID, err = optUUID(req.LeadID, "lead_id"); err != nil {
		return in, err
	}
	if in.OpportunityID, err = optUUID(req.OpportunityID, "opportunity_id"); err != nil {
		return in, err
	}
	if in.OwnerPersonID, err = optUUID(req.OwnerPersonID, "owner_person_id"); err != nil {
		return in, err
	}
	due, err := time.Parse(time.RFC3339, strings.TrimSpace(req.DueAt))
	if err != nil {
		return in, errInvalidField("due_at")
	}
	in.DueAt = due.UTC()
	return in, nil
}

type outcomeReq struct {
	Outcome string `json:"outcome"`
}

type proposalReq struct {
	ScopeSummary   string          `json:"scope_summary"`
	LineItems      json.RawMessage `json:"line_items"`
	PriceMinor     int64           `json:"price_minor"`
	Currency       string          `json:"currency"`
	DiscountMinor  int64           `json:"discount_minor"`
	ValidUntil     *string         `json:"valid_until"`
	DocumentFileID *string         `json:"document_file_id"`
}

func toProposalInput(opportunityID uuid.UUID, req proposalReq) (ProposalInput, error) {
	in := ProposalInput{
		OpportunityID: opportunityID, ScopeSummary: req.ScopeSummary, LineItems: []byte(req.LineItems),
		PriceMinor: req.PriceMinor, Currency: req.Currency, DiscountMinor: req.DiscountMinor,
	}
	var err error
	if in.DocumentFileID, err = optUUID(req.DocumentFileID, "document_file_id"); err != nil {
		return in, err
	}
	if in.ValidUntil, err = optTime(req.ValidUntil, "valid_until"); err != nil {
		return in, err
	}
	return in, nil
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
		httpx.WriteProblem(w, r, httpx.NotFound("No such CRM record."))
	case errors.Is(err, ErrInvalid):
		httpx.WriteProblem(w, r, httpx.BadRequest(err.Error()))
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
		// Tolerate a bare date for date-only fields.
		if d, derr := time.Parse("2006-01-02", strings.TrimSpace(*p)); derr == nil {
			return &d, nil
		}
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

func encodeCursor(at time.Time, id uuid.UUID) string {
	return strconv.FormatInt(at.UTC().UnixNano(), 10) + ":" + id.String()
}

func decodeCursor(encoded string) (*Cursor, error) {
	parts := strings.SplitN(encoded, ":", 2)
	if len(parts) != 2 {
		return nil, errors.New("crm: malformed cursor")
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
