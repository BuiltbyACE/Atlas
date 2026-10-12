package seed

// RolePermissions maps each role (§3.2) to the permissions it holds. The mapping
// is a starting point derived from each role's one-line description; scopes
// (§6.2) narrow a grant at assignment time, they do not widen the permission set.
//
// A role's permissions are replaced wholesale on each seed run, so this map is
// the source of truth for what a role can do.
var RolePermissions = map[string][]string{
	"ceo_commercial": concat(
		read(
			"core.user.read", "core.role.read", "core.settings.read", "core.audit.read",
			"core.notification.read", "core.file.read", "core.approval.read", "core.approval_policy.read",
			"people.person.read", "people.department.read", "people.daily_report.read", "people.leave.read",
			"units.unit.read", "units.indicator.read",
		),
		write("core.file.write"),
		// Full CRM and contracts.
		all(
			"core.approval.decide",
			"crm.organization.read", "crm.organization.write",
			"crm.contact.read", "crm.contact.write",
			"crm.lead.read", "crm.lead.write", "crm.lead.assign",
			"crm.interaction.read", "crm.interaction.write",
			"crm.opportunity.read", "crm.opportunity.write",
			"crm.proposal.read", "crm.proposal.write", "crm.proposal.discount_approve",
			"crm.pipeline.read",
			"contracts.contract.read", "contracts.contract.write", "contracts.contract.sign",
			"contracts.contract_value.sensitive.read",
			"contracts.milestone.read", "contracts.milestone.manage",
			"contracts.deliverable.read", "contracts.deliverable.manage",
			"contracts.change_request.read", "contracts.change_request.approve",
		),
		// Work visibility; finance read with sensitive amounts.
		all(
			"work.project.read", "work.task.read", "work.evidence.read", "work.blocker.read", "work.handover.read",
			"finance.budget.read", "finance.expense.read", "finance.invoice.read",
			"finance.payment.read", "finance.report.read", "finance.amount.sensitive.read",
			"intel.decision.read", "intel.decision.manage", "intel.risk.read", "intel.risk.manage",
			"intel.brief.read", "intel.brief.manage", "intel.metric.read",
		),
	),

	"ceo_internal": concat(
		read(
			"core.user.read", "core.role.read", "core.settings.read", "core.audit.read",
			"core.notification.read", "core.file.read", "core.approval.read", "core.approval_policy.read",
		),
		// Full work, people and systems.
		all(
			"core.file.write", "core.approval.decide",
			"people.person.read", "people.person.manage", "people.person.sensitive.read",
			"people.department.read", "people.department.manage",
			"people.daily_report.read", "people.leave.read", "people.leave.manage",
			"people.review.read", "people.review.manage",
			"work.project.read", "work.project.manage",
			"work.task.read", "work.task.write", "work.task.assign", "work.task.review",
			"work.evidence.read", "work.evidence.write",
			"work.blocker.read", "work.blocker.manage",
			"work.handover.read", "work.handover.accept",
			"work.time_entry.read",
			"systems.system.read", "systems.system.manage", "systems.check.read",
			"systems.incident.read", "systems.incident.manage", "systems.domain.manage",
			"units.unit.read", "units.unit.manage", "units.indicator.read",
		),
		// Read across contracts, CRM and finance.
		all(
			"contracts.contract.read", "contracts.contract_value.sensitive.read",
			"contracts.milestone.read", "contracts.deliverable.read", "contracts.change_request.read",
			"crm.organization.read", "crm.contact.read", "crm.lead.read",
			"crm.opportunity.read", "crm.pipeline.read",
			"finance.budget.read", "finance.expense.read", "finance.invoice.read",
			"finance.payment.read", "finance.report.read", "finance.amount.sensitive.read",
			"intel.decision.read", "intel.decision.manage", "intel.risk.read", "intel.risk.manage",
			"intel.brief.read", "intel.brief.manage", "intel.metric.read",
		),
	),

	"finance_officer": all(
		"core.notification.read", "core.file.read", "core.file.write",
		"core.settings.read", "core.approval.read",
		"people.person.read", "people.department.read",
		// Records — but does not approve; separation of duties (§3.3).
		"finance.budget.read", "finance.budget.manage",
		"finance.expense.read", "finance.expense.write",
		"finance.invoice.read", "finance.invoice.write", "finance.invoice.issue",
		"finance.payment.read", "finance.payment.record",
		"finance.report.read", "finance.amount.sensitive.read",
		"contracts.contract.read", "contracts.milestone.read", "contracts.deliverable.read",
		"work.project.read", "work.task.read",
		"crm.organization.read", "crm.contact.read",
		"units.unit.read",
	),

	"sales_rep": concat(
		write("core.notification.read", "core.file.read", "core.file.write"),
		all(
			"crm.organization.read", "crm.organization.write",
			"crm.contact.read", "crm.contact.write",
			"crm.lead.read", "crm.lead.write",
			"crm.interaction.read", "crm.interaction.write",
			"crm.opportunity.read", "crm.opportunity.write",
			"crm.proposal.read", "crm.proposal.write",
			"crm.pipeline.read",
			"contracts.contract.read",
			"units.unit.read", "units.indicator.read",
		),
	),

	"project_lead": concat(
		read("core.notification.read", "core.file.read", "people.person.read", "people.daily_report.read"),
		all(
			"core.file.write",
			"work.project.read", "work.project.manage",
			"work.task.read", "work.task.write", "work.task.assign", "work.task.review",
			"work.evidence.read", "work.evidence.write",
			"work.blocker.read", "work.blocker.manage",
			"work.handover.read", "work.time_entry.read",
			"finance.budget.read", "finance.expense.read", "finance.expense.write", "finance.expense.approve",
			"contracts.contract.read", "contracts.milestone.read", "contracts.deliverable.read",
			"contracts.change_request.read",
			"crm.organization.read", "crm.contact.read",
			"intel.risk.read",
			"units.unit.read",
		),
	),

	"engineer": concat(
		read("core.notification.read", "core.file.read"),
		all(
			"core.file.write",
			"work.project.read", "work.task.read", "work.task.write",
			"work.evidence.read", "work.evidence.write", "work.blocker.read",
			"work.time_entry.write",
			"people.daily_report.submit", "people.leave.request",
			"finance.expense.read", "finance.expense.write",
			"systems.system.read", "systems.check.read", "systems.incident.read",
			"units.unit.read",
		),
	),

	"staff": {
		"core.notification.read", "core.file.read",
		"work.project.read", "work.task.read", "work.task.write", "work.evidence.write",
		"people.daily_report.submit", "people.leave.request",
		"finance.expense.read", "finance.expense.write",
		"units.unit.read",
	},

	"unit_manager": {
		"core.notification.read", "core.file.read",
		"units.unit.read", "units.indicator.read",
		"work.project.read", "work.task.read",
		"people.person.read", "people.daily_report.read",
		"finance.budget.read", "finance.expense.read", "finance.report.read",
		"crm.organization.read", "crm.contact.read", "crm.lead.read", "crm.pipeline.read",
	},

	"auditor": {
		"core.notification.read", "core.audit.read", "core.approval.read", "core.settings.read",
		"people.person.read",
		"finance.budget.read", "finance.expense.read", "finance.invoice.read",
		"finance.payment.read", "finance.report.read", "finance.amount.sensitive.read",
		"contracts.contract.read", "contracts.contract_value.sensitive.read",
	},

	"system_admin": concat(
		read("core.file.read", "people.person.read", "people.department.read"),
		all(
			"core.user.read", "core.user.manage",
			"core.role.read", "core.role.manage",
			"core.settings.read", "core.settings.manage",
			"core.audit.read",
			"core.notification.read", "core.notification.manage",
			"core.service_identity.manage",
			"integrations.registry.read", "integrations.registry.manage", "integrations.webhook.manage",
			"systems.system.read", "systems.system.manage", "systems.check.read",
			"systems.incident.read", "systems.incident.manage", "systems.domain.manage",
		),
	),
}

// read and write are documentation helpers: they make the intent of a role's
// grant list obvious at a glance. They do not alter the keys.
func read(keys ...string) []string  { return keys }
func write(keys ...string) []string { return keys }

// all is an alias for read, kept separate so a mapping reads naturally.
func all(keys ...string) []string { return keys }

// concat joins permission slices without duplicates.
func concat(groups ...[]string) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, g := range groups {
		for _, k := range g {
			if _, ok := seen[k]; ok {
				continue
			}
			seen[k] = struct{}{}
			out = append(out, k)
		}
	}
	return out
}
