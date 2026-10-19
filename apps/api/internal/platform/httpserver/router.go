// Package httpserver wires the Atlas HTTP surface: middleware, routing, and
// the health endpoints (docs/ATLAS.md §11, §14).
//
// Every route is declared in a routing.Registry, together with its access
// rule. `build` returns both the handler and the registry so a test can assert
// that the two agree — a route that is not in the registry fails the build
// (§6.2).
package httpserver

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/safaristack/atlas/apps/api/internal/approvals"
	"github.com/safaristack/atlas/apps/api/internal/audit"
	"github.com/safaristack/atlas/apps/api/internal/auth"
	"github.com/safaristack/atlas/apps/api/internal/contracts"
	"github.com/safaristack/atlas/apps/api/internal/crm"
	"github.com/safaristack/atlas/apps/api/internal/dashboard"
	"github.com/safaristack/atlas/apps/api/internal/files"
	"github.com/safaristack/atlas/apps/api/internal/finance"
	"github.com/safaristack/atlas/apps/api/internal/integrations"
	"github.com/safaristack/atlas/apps/api/internal/notifications"
	"github.com/safaristack/atlas/apps/api/internal/people"
	"github.com/safaristack/atlas/apps/api/internal/platform/config"
	"github.com/safaristack/atlas/apps/api/internal/platform/health"
	"github.com/safaristack/atlas/apps/api/internal/platform/httpx"
	"github.com/safaristack/atlas/apps/api/internal/platform/routing"
	"github.com/safaristack/atlas/apps/api/internal/settings"
	"github.com/safaristack/atlas/apps/api/internal/work"
)

// Deps are the collaborators the router needs. Passing them explicitly keeps
// the router testable without a global container.
type Deps struct {
	Cfg     *config.Config
	Logger  *slog.Logger
	DB      *pgxpool.Pool
	Version string
	// Auth is optional. When it is nil the authentication routes are not
	// mounted, which keeps the health-only router simple to test.
	Auth *auth.Service
	// Notifications is optional, and only mounted alongside Auth (its routes are
	// authenticated). When nil the notification routes are absent.
	Notifications *notifications.Handler
	Approvals     *approvals.Handler
	Settings      *settings.Handler
	// Audit is the read side of the audit trail (§6.3); mounted when present.
	Audit *audit.Handler
	// Dashboard is the executive dashboard read model (§8.8); mounted when present.
	Dashboard *dashboard.Handler
	// Files is optional, and only mounted when object storage is configured
	// (local development may run without a bucket).
	Files *files.Handler
	// Work is the Phase 2 work-orchestration surface (projects, milestones,
	// tasks, dependencies, blockers); mounted when present.
	Work *work.Handler
	// People is the people & accountability surface (daily reports, leave);
	// mounted when present.
	People *people.Handler
	// Integrations is the inbound provider webhook surface (D6 GitHub);
	// mounted when present. Its route is public: the provider authenticates
	// with an HMAC signature, not a principal.
	Integrations *integrations.Handler
	// CRM is the Phase 3 client-acquisition surface (organizations, contacts,
	// leads, opportunities, interactions, follow-ups, proposals) plus the
	// public website-form webhook; mounted when present.
	CRM *crm.Handler
	// Contracts is the Phase 3 contracts surface (contracts, milestones,
	// deliverables, obligations, SLA terms, change requests); mounted when
	// present.
	Contracts *contracts.Handler
	// Finance is the Phase 4 finance & commercial-control read surface (cash,
	// receivables, project profitability, budgets and spending); mounted when
	// present.
	Finance *finance.Handler
}

// NewRouter builds the root http.Handler for atlas-api.
func NewRouter(d Deps) http.Handler {
	handler, _ := build(d)
	return handler
}

// build assembles the router and returns its route registry.
func build(d Deps) (http.Handler, *routing.Registry) {
	r := chi.NewRouter()

	// Order matters: a request id must exist before anything logs or recovers.
	r.Use(chimw.RealIP)
	r.Use(httpx.RequestID)
	r.Use(httpx.Recoverer(d.Logger))
	r.Use(httpx.RequestLogger(d.Logger))
	r.Use(httpx.SecureHeaders)
	r.Use(chimw.Timeout(30 * time.Second))

	// Router-level error responses are Problem Details too (§11).
	r.NotFound(httpx.NotFoundHandler)
	r.MethodNotAllowed(httpx.MethodNotAllowedHandler)

	reg := routing.New()
	registerBase(reg, d)

	var gate routing.Gate
	if d.Auth != nil {
		handler := auth.NewHandler(d.Auth, auth.HTTPConfig{
			CookieName:    d.Cfg.SessionCookieName,
			SecureCookies: d.Cfg.Env != config.EnvLocal,
		}, d.Logger)
		registerAuth(reg, handler)
		gate = auth.NewGate(d.Auth, d.Cfg.SessionCookieName, d.Logger)
	}

	if d.Auth != nil && d.Notifications != nil {
		registerNotifications(reg, d.Notifications)
	}

	if d.Approvals != nil {
		registerApprovals(reg, d.Approvals)
	}

	if d.Settings != nil {
		registerSettings(reg, d.Settings)
	}

	if d.Audit != nil {
		registerAudit(reg, d.Audit)
	}

	if d.Dashboard != nil {
		registerDashboard(reg, d.Dashboard)
	}

	if d.Files != nil {
		registerFiles(reg, d.Files)
	}

	if d.Work != nil {
		registerWork(reg, d.Work)
	}

	if d.People != nil {
		registerPeople(reg, d.People)
	}

	if d.Integrations != nil {
		registerIntegrations(reg, d.Integrations)
	}

	if d.CRM != nil {
		registerCRM(reg, d.CRM)
	}

	if d.Contracts != nil {
		registerContracts(reg, d.Contracts)
	}

	if d.Finance != nil {
		registerFinance(reg, d.Finance)
	}

	reg.Mount(r, gate)
	return r, reg
}

// registerBase declares the always-present routes.
func registerBase(reg *routing.Registry, d Deps) {
	checker := health.NewChecker(d.DB)

	reg.Public(http.MethodGet, "/api/v1/health", "Liveness: is the process up?",
		health.LiveHandler(checker))
	reg.Public(http.MethodGet, "/api/v1/health/ready", "Readiness: can it serve traffic?",
		health.ReadyHandler(checker))
	reg.Public(http.MethodGet, "/api/v1/", "Service name and version",
		func(w http.ResponseWriter, r *http.Request) {
			httpx.WriteJSON(w, http.StatusOK, map[string]string{"service": "atlas-api", "version": d.Version})
		})
}

// registerAuth declares the §6.1 identity routes.
func registerAuth(reg *routing.Registry, h *auth.Handler) {
	reg.Public(http.MethodPost, "/api/v1/auth/login", "Sign in with email/phone and password", h.Login)
	reg.Public(http.MethodPost, "/api/v1/auth/mfa/verify", "Complete sign-in with a TOTP or recovery code", h.MFAVerify)
	reg.Public(http.MethodPost, "/api/v1/auth/mfa/enroll", "Begin MFA enrolment (challenge token)", h.MFAEnroll)
	reg.Public(http.MethodPost, "/api/v1/auth/mfa/activate", "Confirm MFA enrolment (challenge token)", h.MFAActivate)
	reg.Public(http.MethodPost, "/api/v1/auth/invites/accept", "Accept an invitation and set a password", h.AcceptInvite)

	reg.Authenticated(http.MethodPost, "/api/v1/auth/logout", "Revoke the current session", h.Logout)
	reg.Authenticated(http.MethodGet, "/api/v1/auth/me", "The current principal", h.Me)
	reg.Authenticated(http.MethodPost, "/api/v1/auth/heartbeat", "Refresh the current session's activity", h.Heartbeat)
	reg.Authenticated(http.MethodGet, "/api/v1/auth/sessions", "List your active sessions", h.Sessions)
	reg.Authenticated(http.MethodDelete, "/api/v1/auth/sessions/{id}", "Revoke one of your sessions", h.RevokeSession)
	reg.Authenticated(http.MethodPost, "/api/v1/auth/sessions/revoke-all", "Revoke every session you hold", h.RevokeAllSessions)
	reg.Authenticated(http.MethodPost, "/api/v1/auth/realtime-token", "Mint a short-lived realtime token", h.RealtimeToken)

	reg.Gated("core.user.manage", http.MethodPost, "/api/v1/auth/invites",
		"Invite a person to create an account", h.CreateInvite)

	reg.Public(http.MethodGet, "/internal/v1/.well-known/jwks.json",
		"Public signing keys for realtime token verification", h.JWKS)

	reg.Gated("core.authz.check", http.MethodGet, "/internal/v1/authz/check",
		"Check whether a user may join a realtime topic (service-to-service)", h.Check)
}

// registerNotifications declares the notification surface (§6.4). The user
// routes are authenticated (you only ever see your own); the internal command is
// service-to-service and gated by a catalogue permission.
func registerNotifications(reg *routing.Registry, h *notifications.Handler) {
	reg.Authenticated(http.MethodGet, "/api/v1/notifications",
		"List your in-app notifications", h.List)
	reg.Authenticated(http.MethodGet, "/api/v1/notifications/unread-count",
		"Count your unread notifications", h.UnreadCount)
	reg.Authenticated(http.MethodPost, "/api/v1/notifications/{id}/read",
		"Mark one notification read", h.MarkRead)
	reg.Authenticated(http.MethodPost, "/api/v1/notifications/read-all",
		"Mark every notification read", h.MarkAllRead)
	reg.Authenticated(http.MethodGet, "/api/v1/notifications/preferences",
		"Your notification preferences", h.Preferences)
	reg.Authenticated(http.MethodPut, "/api/v1/notifications/preferences",
		"Update your notification preferences", h.SavePreferences)

	reg.Gated("core.notification.manage", http.MethodPost, "/internal/v1/commands/notifications.send",
		"Ask Go to notify a person (service-to-service)", h.Send)
}

func registerApprovals(reg *routing.Registry, h *approvals.Handler) {
	reg.Authenticated(http.MethodPost, "/api/v1/approvals", "Request an approval", h.RequestApproval)
	reg.Authenticated(http.MethodGet, "/api/v1/approvals/{id}", "Get an approval", h.Get)
	reg.Authenticated(http.MethodGet, "/api/v1/approvals/pending", "List pending approvals for a role", h.PendingForRole)
	reg.Authenticated(http.MethodGet, "/api/v1/approvals/mine", "List approvals requested by you", h.MyRequested)
	reg.Authenticated(http.MethodPost, "/api/v1/approvals/{id}/approve", "Approve an approval", h.Approve)
	reg.Authenticated(http.MethodPost, "/api/v1/approvals/{id}/decline", "Decline an approval", h.Decline)
	reg.Authenticated(http.MethodPost, "/api/v1/approvals/{id}/escalate", "Escalate an approval", h.Escalate)
	reg.Authenticated(http.MethodPost, "/api/v1/approvals/{id}/cancel", "Cancel an approval", h.Cancel)
	reg.Gated("core.approval_policy.read", http.MethodGet, "/api/v1/approval-policies",
		"List approval policies", h.ListPolicies)
}

// registerSettings declares the configuration store (§6.7). Reading needs
// core.settings.read; writing needs core.settings.manage and carries the
// version it read in If-Match.
func registerSettings(reg *routing.Registry, h *settings.Handler) {
	reg.Gated("core.settings.read", http.MethodGet, "/api/v1/settings",
		"List company settings", h.List)
	reg.Gated("core.settings.read", http.MethodGet, "/api/v1/settings/{key}",
		"Get one setting", h.Get)
	reg.Gated("core.settings.read", http.MethodGet, "/api/v1/settings/{key}/history",
		"List a setting's changes", h.History)
	reg.Gated("core.settings.manage", http.MethodPut, "/api/v1/settings/{key}",
		"Change a setting", h.Set)
}

// registerFiles declares the files surface (§6.5). Reading is core.file.read;
// starting and confirming an upload, and deleting your own file, is
// core.file.write. Bytes never pass through these routes: they carry metadata
// and pre-signed URLs only.
func registerFiles(reg *routing.Registry, h *files.Handler) {
	reg.Gated("core.file.read", http.MethodGet, "/api/v1/files",
		"List files", h.List)
	reg.Gated("core.file.read", http.MethodGet, "/api/v1/files/{id}",
		"Get file metadata", h.Get)
	reg.Gated("core.file.read", http.MethodGet, "/api/v1/files/{id}/download",
		"Issue a download URL", h.Download)
	reg.Gated("core.file.write", http.MethodPost, "/api/v1/files",
		"Start an upload", h.RequestUpload)
	reg.Gated("core.file.write", http.MethodPost, "/api/v1/files/{id}/complete",
		"Confirm an upload", h.Complete)
	reg.Gated("core.file.write", http.MethodDelete, "/api/v1/files/{id}",
		"Delete your file", h.Archive)
}

// registerAudit declares the audit read surface (§6.3). Both routes need
// core.audit.read, which the auditor role and both CEOs hold: the trail is
// global for exactly those principals, and viewable per record for them too.
func registerAudit(reg *routing.Registry, h *audit.Handler) {
	reg.Gated("core.audit.read", http.MethodGet, "/api/v1/audit",
		"List audit events", h.List)
	reg.Gated("core.audit.read", http.MethodGet, "/api/v1/audit/{entity_type}/{entity_id}",
		"Get a record's audit history", h.History)
}

// registerDashboard declares the executive dashboard (§8.8). The tiles span
// people, approvals and the audit trail, so the single read route is gated by
// intel.dashboard.read, held by both CEOs.
func registerDashboard(reg *routing.Registry, h *dashboard.Handler) {
	reg.Gated("intel.dashboard.read", http.MethodGet, "/api/v1/dashboard/executive",
		"Executive dashboard tiles", h.Summary)
}

// registerWork declares the Phase 2 work-orchestration surface. Reads need
// work.project.read / work.task.read / work.blocker.read; writes need the
// matching manage/write permission. Every write carries the version it read in
// If-Match (§6.6).
// registerCRM declares the client-acquisition surface (§8.1). The website-form
// webhook is public: the website authenticates with a shared secret, not a
// principal.
func registerCRM(reg *routing.Registry, h *crm.Handler) {
	// Organizations.
	reg.Gated("crm.organization.read", http.MethodGet, "/api/v1/crm/organizations",
		"List organizations", h.ListOrganizations)
	reg.Gated("crm.organization.read", http.MethodGet, "/api/v1/crm/organizations/{id}",
		"Get an organization", h.GetOrganization)
	reg.Gated("crm.organization.write", http.MethodPost, "/api/v1/crm/organizations",
		"Create an organization", h.CreateOrganization)
	reg.Gated("crm.organization.write", http.MethodPut, "/api/v1/crm/organizations/{id}",
		"Update an organization", h.UpdateOrganization)

	// Contacts.
	reg.Gated("crm.contact.read", http.MethodGet, "/api/v1/crm/contacts",
		"List contacts", h.ListContacts)
	reg.Gated("crm.contact.read", http.MethodGet, "/api/v1/crm/contacts/{id}",
		"Get a contact", h.GetContact)
	reg.Gated("crm.contact.write", http.MethodPost, "/api/v1/crm/contacts",
		"Create a contact", h.CreateContact)
	reg.Gated("crm.contact.write", http.MethodPut, "/api/v1/crm/contacts/{id}",
		"Update a contact", h.UpdateContact)

	// Config.
	reg.Gated("crm.lead.read", http.MethodGet, "/api/v1/crm/lead-sources",
		"List lead sources", h.ListLeadSources)
	reg.Gated("crm.pipeline.read", http.MethodGet, "/api/v1/crm/pipeline-stages",
		"List pipeline stages", h.ListPipelineStages)

	// Leads.
	reg.Gated("crm.lead.read", http.MethodGet, "/api/v1/crm/leads",
		"List leads", h.ListLeads)
	reg.Gated("crm.lead.read", http.MethodGet, "/api/v1/crm/leads/{id}",
		"Get a lead", h.GetLead)
	reg.Gated("crm.lead.write", http.MethodPost, "/api/v1/crm/leads",
		"Create a lead", h.CreateLead)
	reg.Gated("crm.lead.write", http.MethodPut, "/api/v1/crm/leads/{id}",
		"Update a lead", h.UpdateLead)
	reg.Gated("crm.lead.assign", http.MethodPost, "/api/v1/crm/leads/{id}/assign",
		"Assign a lead to an owner", h.AssignLead)
	reg.Gated("crm.lead.write", http.MethodPost, "/api/v1/crm/leads/{id}/status",
		"Change a lead's status", h.SetLeadStatus)
	reg.Gated("crm.lead.write", http.MethodPost, "/api/v1/crm/leads/{id}/convert",
		"Convert a qualified lead to an opportunity", h.ConvertLead)

	// Opportunities.
	reg.Gated("crm.opportunity.read", http.MethodGet, "/api/v1/crm/opportunities",
		"List opportunities", h.ListOpportunities)
	reg.Gated("crm.opportunity.read", http.MethodGet, "/api/v1/crm/opportunities/{id}",
		"Get an opportunity", h.GetOpportunity)
	reg.Gated("crm.opportunity.write", http.MethodPost, "/api/v1/crm/opportunities/{id}/stage",
		"Move an opportunity to another stage", h.MoveOpportunityStage)
	reg.Gated("crm.opportunity.write", http.MethodPost, "/api/v1/crm/opportunities/{id}/win",
		"Win an opportunity", h.WinOpportunity)
	reg.Gated("crm.opportunity.write", http.MethodPost, "/api/v1/crm/opportunities/{id}/lose",
		"Lose an opportunity", h.LoseOpportunity)

	// Interactions.
	reg.Gated("crm.interaction.read", http.MethodGet, "/api/v1/crm/interactions",
		"List interactions", h.ListInteractions)
	reg.Gated("crm.interaction.read", http.MethodGet, "/api/v1/crm/interactions/{id}",
		"Get an interaction", h.GetInteraction)
	reg.Gated("crm.interaction.write", http.MethodPost, "/api/v1/crm/interactions",
		"Record a call, message, email, meeting or note", h.CreateInteraction)

	// Follow-ups.
	reg.Gated("crm.lead.read", http.MethodGet, "/api/v1/crm/followups",
		"List follow-ups", h.ListFollowups)
	reg.Gated("crm.lead.read", http.MethodGet, "/api/v1/crm/followups/{id}",
		"Get a follow-up", h.GetFollowup)
	reg.Gated("crm.lead.write", http.MethodPost, "/api/v1/crm/followups",
		"Schedule a follow-up", h.CreateFollowup)
	reg.Gated("crm.lead.write", http.MethodPost, "/api/v1/crm/followups/{id}/complete",
		"Complete a follow-up", h.CompleteFollowup)
	reg.Gated("crm.lead.write", http.MethodPost, "/api/v1/crm/followups/{id}/cancel",
		"Cancel a follow-up", h.CancelFollowup)

	// Proposals.
	reg.Gated("crm.proposal.read", http.MethodGet, "/api/v1/crm/opportunities/{id}/proposals",
		"List an opportunity's proposals", h.ListProposals)
	reg.Gated("crm.proposal.read", http.MethodGet, "/api/v1/crm/proposals/{id}",
		"Get a proposal", h.GetProposal)
	reg.Gated("crm.proposal.write", http.MethodPost, "/api/v1/crm/opportunities/{id}/proposals",
		"Create a proposal", h.CreateProposal)
	reg.Gated("crm.proposal.write", http.MethodPost, "/api/v1/crm/proposals/{id}/send",
		"Send a proposal", h.SendProposal)

	// Pipeline.
	reg.Gated("crm.pipeline.read", http.MethodGet, "/api/v1/crm/pipeline",
		"Commercial pipeline summary", h.Pipeline)

	// Public website-form webhook (shared secret, not a principal).
	reg.Public(http.MethodPost, "/webhooks/website-forms",
		"Inbound website enquiry that creates a lead", h.WebsiteForm)
}

// registerContracts declares the contracts surface (§8.3).
func registerContracts(reg *routing.Registry, h *contracts.Handler) {
	reg.Gated("contracts.contract.read", http.MethodGet, "/api/v1/contracts",
		"List contracts", h.List)
	reg.Gated("contracts.contract.read", http.MethodGet, "/api/v1/contracts/{id}",
		"Get a contract", h.Get)
	reg.Gated("contracts.contract.write", http.MethodPost, "/api/v1/contracts",
		"Create a contract", h.Create)
	reg.Gated("contracts.contract.write", http.MethodPut, "/api/v1/contracts/{id}",
		"Update a contract", h.Update)
	reg.Gated("contracts.contract.write", http.MethodPost, "/api/v1/contracts/{id}/submit",
		"Submit a contract for internal review", h.Submit)
	reg.Gated("contracts.contract.write", http.MethodPost, "/api/v1/contracts/{id}/approve",
		"Approve a contract internally", h.Approve)
	reg.Gated("contracts.contract.write", http.MethodPost, "/api/v1/contracts/{id}/send",
		"Send an approved contract to the client", h.Send)
	reg.Gated("contracts.contract.sign", http.MethodPost, "/api/v1/contracts/{id}/sign",
		"Record a signature on a contract", h.Sign)
	reg.Gated("contracts.contract.write", http.MethodPost, "/api/v1/contracts/{id}/activate",
		"Activate a signed contract", h.Activate)
	reg.Gated("contracts.contract.write", http.MethodPost, "/api/v1/contracts/{id}/terminate",
		"Terminate a contract", h.Terminate)

	// Milestones.
	reg.Gated("contracts.milestone.read", http.MethodGet, "/api/v1/contracts/{id}/milestones",
		"List contract milestones", h.ListMilestones)
	reg.Gated("contracts.milestone.manage", http.MethodPost, "/api/v1/contracts/{id}/milestones",
		"Create a contract milestone", h.CreateMilestone)

	// Deliverables.
	reg.Gated("contracts.deliverable.read", http.MethodGet, "/api/v1/contracts/{id}/deliverables",
		"List contract deliverables", h.ListDeliverables)
	reg.Gated("contracts.deliverable.manage", http.MethodPost, "/api/v1/contracts/{id}/deliverables",
		"Create a contract deliverable", h.CreateDeliverable)

	// Obligations and SLA terms (contract read/write).
	reg.Gated("contracts.contract.read", http.MethodGet, "/api/v1/contracts/{id}/obligations",
		"List contract obligations", h.ListObligations)
	reg.Gated("contracts.contract.write", http.MethodPost, "/api/v1/contracts/{id}/obligations",
		"Create a contract obligation", h.CreateObligation)
	reg.Gated("contracts.contract.read", http.MethodGet, "/api/v1/contracts/{id}/sla-terms",
		"List contract SLA terms", h.ListSLATerms)
	reg.Gated("contracts.contract.write", http.MethodPost, "/api/v1/contracts/{id}/sla-terms",
		"Create a contract SLA term", h.CreateSLATerm)

	// Change requests.
	reg.Gated("contracts.change_request.read", http.MethodGet, "/api/v1/contracts/{id}/change-requests",
		"List a contract's change requests", h.ListChangeRequests)
	reg.Gated("contracts.contract.write", http.MethodPost, "/api/v1/contracts/{id}/change-requests",
		"Raise a change request", h.CreateChangeRequest)
	reg.Gated("contracts.change_request.approve", http.MethodPost, "/api/v1/change-requests/{id}/approve",
		"Approve a change request", h.ApproveChangeRequest)
	reg.Gated("contracts.change_request.approve", http.MethodPost, "/api/v1/change-requests/{id}/reject",
		"Reject a change request", h.RejectChangeRequest)
}

// registerFinance declares the finance & commercial-control surface (§8.4).
// This pass is read-only: every route is a GET, and the money-bearing reads are
// gated by the module's read permissions, with amount visibility decided
// separately per request by finance.amount.sensitive.read.
func registerFinance(reg *routing.Registry, h *finance.Handler) {
	// Headline, cash, receivables and profitability are all financial reports.
	reg.Gated("finance.report.read", http.MethodGet, "/api/v1/finance/overview",
		"Finance headline: cash, receivables and spending at a glance", h.Overview)
	reg.Gated("finance.report.read", http.MethodGet, "/api/v1/finance/accounts",
		"List cash accounts with their derived balances", h.ListAccounts)
	reg.Gated("finance.report.read", http.MethodGet, "/api/v1/finance/cash-position",
		"Cash position across accounts with 30-day flows", h.CashPosition)
	reg.Gated("finance.report.read", http.MethodGet, "/api/v1/finance/receivables",
		"Receivables ageing by days past due", h.Receivables)
	reg.Gated("finance.report.read", http.MethodGet, "/api/v1/finance/recurring-costs",
		"List standing vendor costs by next renewal", h.RecurringCosts)
	reg.Gated("finance.report.read", http.MethodGet, "/api/v1/finance/projects/{id}/profitability",
		"A project's revenue, cost and margin", h.ProjectProfitability)

	// Expenses.
	reg.Gated("finance.expense.read", http.MethodGet, "/api/v1/finance/expense-categories",
		"List configured expense categories", h.ListExpenseCategories)
	reg.Gated("finance.expense.read", http.MethodGet, "/api/v1/finance/expenses",
		"List expenses", h.ListExpenses)
	reg.Gated("finance.expense.read", http.MethodGet, "/api/v1/finance/expenses/{id}",
		"Get an expense", h.GetExpense)

	// Invoices.
	reg.Gated("finance.invoice.read", http.MethodGet, "/api/v1/finance/invoices",
		"List invoices", h.ListInvoices)
	reg.Gated("finance.invoice.read", http.MethodGet, "/api/v1/finance/invoices/{id}",
		"Get an invoice with its lines", h.GetInvoice)

	// Payments.
	reg.Gated("finance.payment.read", http.MethodGet, "/api/v1/finance/payments",
		"List payments", h.ListPayments)

	// Budgets.
	reg.Gated("finance.budget.read", http.MethodGet, "/api/v1/finance/budgets",
		"List budgets with their actuals", h.ListBudgets)
	reg.Gated("finance.budget.read", http.MethodGet, "/api/v1/finance/budgets/{id}",
		"Get a budget", h.GetBudget)
}

// registerWork declares the Phase 2 work-orchestration surface (§8.2).
func registerWork(reg *routing.Registry, h *work.Handler) {
	// Projects.
	reg.Gated("work.project.read", http.MethodGet, "/api/v1/work/projects",
		"List projects", h.ListProjects)
	reg.Gated("work.project.read", http.MethodGet, "/api/v1/work/projects/{id}",
		"Get a project", h.GetProject)
	reg.Gated("work.project.manage", http.MethodPost, "/api/v1/work/projects",
		"Create a project", h.CreateProject)
	reg.Gated("work.project.manage", http.MethodPut, "/api/v1/work/projects/{id}",
		"Update a project", h.UpdateProject)
	reg.Gated("work.project.read", http.MethodGet, "/api/v1/work/projects/{id}/members",
		"List a project's members", h.ListMembers)
	reg.Gated("work.project.manage", http.MethodPost, "/api/v1/work/projects/{id}/members",
		"Add a project member", h.AddMember)
	reg.Gated("work.project.manage", http.MethodDelete, "/api/v1/work/projects/{id}/members/{person_id}",
		"Remove a project member", h.RemoveMember)

	// Milestones.
	reg.Gated("work.project.read", http.MethodGet, "/api/v1/work/projects/{id}/milestones",
		"List a project's milestones", h.ListMilestones)
	reg.Gated("work.project.read", http.MethodGet, "/api/v1/work/milestones/{id}",
		"Get a milestone", h.GetMilestone)
	reg.Gated("work.project.manage", http.MethodPost, "/api/v1/work/projects/{id}/milestones",
		"Create a milestone", h.CreateMilestone)
	reg.Gated("work.project.manage", http.MethodPut, "/api/v1/work/milestones/{id}",
		"Update a milestone", h.UpdateMilestone)

	// Tasks.
	reg.Gated("work.task.read", http.MethodGet, "/api/v1/work/tasks",
		"List tasks", h.ListTasks)
	reg.Gated("work.task.read", http.MethodGet, "/api/v1/work/tasks/{id}",
		"Get a task", h.GetTask)
	reg.Gated("work.task.write", http.MethodPost, "/api/v1/work/projects/{id}/tasks",
		"Create a task", h.CreateTask)
	reg.Gated("work.task.write", http.MethodPut, "/api/v1/work/tasks/{id}",
		"Update a task", h.UpdateTask)
	reg.Gated("work.task.write", http.MethodPost, "/api/v1/work/tasks/{id}/dependencies",
		"Add a task dependency", h.AddDependency)
	reg.Gated("work.task.write", http.MethodDelete, "/api/v1/work/tasks/{id}/dependencies/{predecessor_id}",
		"Remove a task dependency", h.RemoveDependency)

	// Blockers.
	reg.Gated("work.blocker.read", http.MethodGet, "/api/v1/work/tasks/{id}/blockers",
		"List a task's blockers", h.ListBlockers)
	reg.Gated("work.blocker.manage", http.MethodPost, "/api/v1/work/tasks/{id}/blockers",
		"Raise a blocker", h.RaiseBlocker)
	reg.Gated("work.blocker.manage", http.MethodPost, "/api/v1/work/blockers/{id}/resolve",
		"Resolve a blocker", h.ResolveBlocker)

	// Evidence.
	reg.Gated("work.evidence.read", http.MethodGet, "/api/v1/work/tasks/{id}/evidence",
		"List a task's evidence", h.ListTaskEvidence)
	reg.Gated("work.evidence.read", http.MethodGet, "/api/v1/work/projects/{id}/evidence",
		"List a project's evidence", h.ListProjectEvidence)
	reg.Gated("work.evidence.read", http.MethodGet, "/api/v1/work/evidence/{id}",
		"Get an evidence record", h.GetEvidence)
	reg.Gated("work.evidence.write", http.MethodPost, "/api/v1/work/tasks/{id}/evidence",
		"Attach evidence to a task", h.AttachTaskEvidence)
	reg.Gated("work.evidence.write", http.MethodPost, "/api/v1/work/projects/{id}/evidence",
		"Attach evidence to a project", h.AttachProjectEvidence)
	reg.Gated("work.evidence.write", http.MethodPost, "/api/v1/work/evidence/{id}/verify",
		"Verify an evidence record", h.VerifyEvidence)
	reg.Gated("work.evidence.write", http.MethodDelete, "/api/v1/work/evidence/{id}",
		"Remove an evidence record", h.DeleteEvidence)

	// Escalations (overdue work).
	reg.Gated("work.task.read", http.MethodGet, "/api/v1/work/escalations",
		"List escalations", h.ListEscalations)
	reg.Gated("work.task.read", http.MethodGet, "/api/v1/work/escalations/{id}",
		"Get an escalation", h.GetEscalation)
	reg.Gated("work.task.review", http.MethodPost, "/api/v1/work/escalations/{id}/resolve",
		"Resolve an escalation", h.ResolveEscalation)

	// Time entries (D7): hours a person logged against a task. Reads need
	// work.time_entry.read; writes need work.time_entry.write. Every update
	// carries the version it read in If-Match (§6.6).
	reg.Gated("work.time_entry.read", http.MethodGet, "/api/v1/work/time-entries",
		"List time entries", h.ListTimeEntries)
	reg.Gated("work.time_entry.read", http.MethodGet, "/api/v1/work/time-entries/{id}",
		"Get a time entry", h.GetTimeEntry)
	reg.Gated("work.time_entry.write", http.MethodPost, "/api/v1/work/time-entries",
		"Record a time entry", h.RecordTimeEntry)
	reg.Gated("work.time_entry.write", http.MethodPut, "/api/v1/work/time-entries/{id}",
		"Update a time entry", h.UpdateTimeEntry)
	reg.Gated("work.time_entry.write", http.MethodDelete, "/api/v1/work/time-entries/{id}",
		"Delete a time entry", h.DeleteTimeEntry)
}

// registerPeople declares the people & accountability surface (§8.5): daily
// reports, leave, the availability roster and off-boarding. Daily reports live
// at the top level per §8.5; the rest is nested under /people. Reads need
// people.daily_report.read / people.leave.read / people.person.read; writes need
// the submit/request/manage permission.
func registerPeople(reg *routing.Registry, h *people.Handler) {
	// Daily reports (one per person per day).
	reg.Gated("people.daily_report.read", http.MethodGet, "/api/v1/daily-reports",
		"List daily reports", h.ListDailyReports)
	reg.Gated("people.daily_report.read", http.MethodGet, "/api/v1/daily-reports/today",
		"Get the caller's daily report for today", h.GetTodayDailyReport)
	reg.Gated("people.daily_report.read", http.MethodGet, "/api/v1/daily-reports/{id}",
		"Get a daily report", h.GetDailyReport)
	reg.Gated("people.daily_report.submit", http.MethodPost, "/api/v1/daily-reports",
		"Submit a daily report", h.SubmitDailyReport)

	// Leave requests.
	reg.Gated("people.leave.read", http.MethodGet, "/api/v1/people/leave-requests",
		"List leave requests", h.ListLeaveRequests)
	reg.Gated("people.leave.read", http.MethodGet, "/api/v1/people/leave-requests/{id}",
		"Get a leave request", h.GetLeaveRequest)
	reg.Gated("people.leave.request", http.MethodPost, "/api/v1/people/leave-requests",
		"Request leave", h.RequestLeave)
	reg.Gated("people.leave.request", http.MethodPost, "/api/v1/people/leave-requests/{id}/cancel",
		"Cancel your leave request", h.CancelLeaveRequest)
	reg.Gated("people.leave.manage", http.MethodPost, "/api/v1/people/leave-requests/{id}/decide",
		"Approve or decline leave", h.DecideLeaveRequest)

	// Availability and offboarding.
	reg.Gated("people.person.read", http.MethodGet, "/api/v1/people/availability",
		"See who is available on a date", h.GetAvailability)
	reg.Gated("people.person.manage", http.MethodPost, "/api/v1/people/{id}/offboard",
		"Offboard a person and revoke their access", h.OffboardPerson)

	// Session activity: who is signed in now, and how long they have been. This
	// is derived read-only from the auth session rows (§8.5).
	reg.Gated("people.session.read", http.MethodGet, "/api/v1/people/presence",
		"See who is signed in and active now", h.GetPresence)
	reg.Gated("people.session.read", http.MethodGet, "/api/v1/people/sessions",
		"List sign-in sessions and logged activity", h.ListSessions)
}

// registerIntegrations declares the inbound provider webhook surface (section
// 11, section 12). The GitHub route is deliberately NOT under /api/v1: it is
// GitHub's endpoint, not Atlas's API, and it is unauthenticated by principal.
// The HMAC signature in the body is the gate, so it is a Public route.
func registerIntegrations(reg *routing.Registry, h *integrations.Handler) {
	reg.Public(http.MethodPost, "/webhooks/github",
		"Receive GitHub webhook deliveries (signed, de-duplicated)", h.ReceiveGitHub)
}
