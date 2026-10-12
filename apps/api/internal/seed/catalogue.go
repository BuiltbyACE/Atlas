// Package seed writes Atlas configuration data: the role catalogue (§3.2), the
// permission catalogue (§6.2) and the role→permission mapping. It contains no
// business records and no people — those arrive through the app, from real
// users. `atlas-cli seed` runs this; it is idempotent.
//
// Everything here is derived from docs/ATLAS.md:
//
//   - roles:             §3.2 "Roles (initial)"
//   - permissions:       §6.2 ("module.resource.action") and the §10.1 schemas
//   - role→permission:   the one-line description of each role in §3.2
//   - setting defaults:  §6.6 (UTC storage, Africa/Nairobi display, KES)
//
// The blueprint does not enumerate every permission, so this catalogue is the
// initial, explicit starting point. It is code, version-controlled and
// reviewable; adding a permission is a deliberate act.
package seed

// Role is a row in core.roles (§3.2).
type Role struct {
	Key         string
	Name        string
	Description string
}

// Permission is a row in core.permissions (§6.2). A sensitive permission gates
// access to fields that the API otherwise omits (§6.2, field-level sensitivity).
type Permission struct {
	Key         string
	Description string
	Sensitive   bool
}

// Roles is the initial role catalogue, verbatim from §3.2.
var Roles = []Role{
	{"ceo_commercial", "Commercial CEO", "Full CRM and contracts; finance read; approvals per the matrix."},
	{"ceo_internal", "Internal & Technical CEO", "Full work, people and systems; finance read; approvals per the matrix."},
	{"finance_officer", "Finance Officer", "Records expenses, invoices and payments; cannot approve own entries."},
	{"sales_rep", "Sales Representative", "Leads and interactions that they own or are assigned."},
	{"project_lead", "Project Lead", "Manages assigned projects, tasks, reviews and project-level expenses."},
	{"engineer", "Engineer", "Assigned tasks, reviews, daily reports and own expenses."},
	{"staff", "Staff", "Assigned tasks, daily reports, leave and own expenses."},
	{"unit_manager", "Business Unit Manager", "Unit-scoped views only (e.g. a school)."},
	{"auditor", "Auditor", "Read-only access to the audit log and finance (external accountant)."},
	{"system_admin", "System Administrator", "Technical administration (users, integrations, settings). Not a business-approval role."},
}

// Permissions is the initial permission catalogue. Keys follow
// `module.resource.action` (§6.2).
var Permissions = []Permission{
	// core
	{"core.user.read", "View user accounts and their status", false},
	{"core.user.manage", "Invite, suspend and deactivate user accounts", false},
	{"core.role.read", "View roles and permission mappings", false},
	{"core.role.manage", "Create roles and change role-permission mappings", false},
	{"core.settings.read", "View company settings", false},
	{"core.settings.manage", "Change company settings", false},
	{"core.audit.read", "View the audit log", true},
	{"core.notification.read", "View own notifications", false},
	{"core.notification.manage", "Manage notification templates and delivery", false},
	{"core.file.read", "View file metadata and download files", false},
	{"core.file.write", "Upload files and attach evidence", false},
	{"core.service_identity.manage", "Manage service identities and credentials", false},
	{"core.approval.read", "View approval requests", false},
	{"core.approval.decide", "Approve or reject approval requests", false},
	{"core.approval_policy.read", "View approval policies", false},
	{"core.approval_policy.manage", "Change approval policies", false},

	// people
	{"people.person.read", "View person records", false},
	{"people.person.manage", "Create and edit person records", false},
	{"people.person.sensitive.read", "View sensitive person fields (e.g. salary)", true},
	{"people.department.read", "View departments", false},
	{"people.department.manage", "Create and edit departments", false},
	{"people.daily_report.read", "View daily reports", false},
	{"people.daily_report.submit", "Submit a daily report", false},
	{"people.leave.read", "View leave records", false},
	{"people.leave.request", "Request leave", false},
	{"people.leave.manage", "Approve and manage leave", false},
	{"people.review.read", "View performance reviews", false},
	{"people.review.manage", "Create and manage performance reviews", false},

	// crm
	{"crm.organization.read", "View organizations", false},
	{"crm.organization.write", "Create and edit organizations", false},
	{"crm.contact.read", "View contacts", false},
	{"crm.contact.write", "Create and edit contacts", false},
	{"crm.lead.read", "View leads", false},
	{"crm.lead.write", "Create and edit leads", false},
	{"crm.lead.assign", "Assign leads to owners", false},
	{"crm.interaction.read", "View interactions (calls, messages)", false},
	{"crm.interaction.write", "Record interactions", false},
	{"crm.opportunity.read", "View opportunities", false},
	{"crm.opportunity.write", "Create and edit opportunities", false},
	{"crm.proposal.read", "View proposals", false},
	{"crm.proposal.write", "Create and edit proposals", false},
	{"crm.proposal.discount_approve", "Approve proposal discounts", false},
	{"crm.pipeline.read", "View the sales pipeline and commercial metrics", false},

	// contracts
	{"contracts.contract.read", "View contracts", false},
	{"contracts.contract.write", "Create and edit contracts", false},
	{"contracts.contract.sign", "Sign contracts", false},
	{"contracts.contract_value.sensitive.read", "View contract values", true},
	{"contracts.milestone.read", "View contract milestones", false},
	{"contracts.milestone.manage", "Manage contract milestones", false},
	{"contracts.deliverable.read", "View contract deliverables", false},
	{"contracts.deliverable.manage", "Manage contract deliverables", false},
	{"contracts.change_request.read", "View change requests", false},
	{"contracts.change_request.approve", "Approve change requests", false},

	// work
	{"work.project.read", "View projects", false},
	{"work.project.manage", "Create and manage projects", false},
	{"work.task.read", "View tasks", false},
	{"work.task.write", "Create and edit tasks", false},
	{"work.task.assign", "Assign tasks", false},
	{"work.task.review", "Review tasks and request changes", false},
	{"work.evidence.read", "View task evidence", false},
	{"work.evidence.write", "Submit task evidence", false},
	{"work.blocker.read", "View blockers", false},
	{"work.blocker.manage", "Raise and resolve blockers", false},
	{"work.handover.read", "View contract handovers", false},
	{"work.handover.accept", "Accept contract handovers", false},
	{"work.time_entry.read", "View time entries", false},
	{"work.time_entry.write", "Record time entries", false},

	// finance
	{"finance.budget.read", "View budgets", false},
	{"finance.budget.manage", "Create and edit budgets", false},
	{"finance.expense.read", "View expenses", false},
	{"finance.expense.write", "Record expenses", false},
	{"finance.expense.approve", "Approve expenses", false},
	{"finance.invoice.read", "View invoices", false},
	{"finance.invoice.write", "Create and edit invoices", false},
	{"finance.invoice.issue", "Issue invoices to clients", false},
	{"finance.payment.read", "View payments", false},
	{"finance.payment.record", "Record payments", false},
	{"finance.report.read", "View financial reports", false},
	{"finance.amount.sensitive.read", "View sensitive financial amounts", true},

	// systems
	{"systems.system.read", "View registered systems", false},
	{"systems.system.manage", "Register and manage systems", false},
	{"systems.check.read", "View health checks and results", false},
	{"systems.incident.read", "View incidents", false},
	{"systems.incident.manage", "Manage incidents", false},
	{"systems.domain.manage", "Manage domains and certificates", false},

	// units
	{"units.unit.read", "View business units", false},
	{"units.unit.manage", "Manage business units and their integrations", false},
	{"units.indicator.read", "View business unit indicators", false},

	// intel
	{"intel.decision.read", "View decisions", false},
	{"intel.decision.manage", "Record and manage decisions", false},
	{"intel.risk.read", "View risks", false},
	{"intel.risk.manage", "Record and manage risks", false},
	{"intel.brief.read", "View briefs", false},
	{"intel.brief.manage", "Create and manage briefs", false},
	{"intel.metric.read", "View metric snapshots", false},

	// integrations
	{"integrations.registry.read", "View the integration registry", false},
	{"integrations.registry.manage", "Manage integrations", false},
	{"integrations.webhook.manage", "Manage webhook endpoints and secrets", false},
}

// SettingDefaults are the runtime defaults the blueprint fixes in §6.6. They are
// configuration, not business records. Idempotent: only inserted if absent, so an
// operator's later change is never overwritten.
var SettingDefaults = map[string]string{
	"company.timezone": `"Africa/Nairobi"`,
	"company.currency": `"KES"`,
}
