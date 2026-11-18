package navigation

// Default is the workspace map every signed-in person is filtered against.
//
// Each View is the read permission the destination's primary endpoint (or
// endpoints) declares in internal/platform/httpserver/router.go; an item is
// shown only when all of them are held. Each Action is an exact key from the
// §6.2 permission catalogue (internal/seed/catalogue.go) — no invented names,
// so a capability can never drift from what Go actually enforces.
//
// The entries mirror the modules the web app has built. Destinations that do
// not exist yet are not listed: the shell must never promise a page that 404s.
var Default = Registry{
	{
		Key: "home",
		Items: []Item{
			{
				Key:      "dashboard",
				Label:    "Dashboard",
				Href:     "/dashboard",
				Icon:     "dashboard",
				Keywords: "home overview",
				// The dashboard degrades per panel: every read it makes is
				// self-scoped or already optional, so every signed-in person
				// can land here.
				Actions: []string{"intel.dashboard.read"},
			},
		},
	},
	{
		Key:   "work",
		Title: "Work",
		Items: []Item{
			{
				Key:      "approvals",
				Label:    "Approvals",
				Href:     "/approvals",
				Icon:     "shieldCheck",
				Section:  "Work",
				Keywords: "approve decline escalate",
				View:     []string{"core.approval.read"},
				Actions: []string{
					"core.approval.decide",
					"core.approval_policy.read",
					"core.approval_policy.manage",
				},
			},
			{
				Key:     "projects",
				Label:   "Projects",
				Href:    "/work",
				Icon:    "folder",
				Section: "Work",
				View:    []string{"work.project.read"},
				Actions: []string{"work.project.manage"},
			},
			{
				Key:      "tasks",
				Label:    "Task board",
				Href:     "/work/tasks",
				Icon:     "board",
				Section:  "Work",
				Keywords: "kanban tasks",
				View:     []string{"work.task.read"},
				Actions:  []string{"work.task.write", "work.task.assign", "work.task.review"},
			},
			{
				Key:      "workload",
				Label:    "Workload",
				Href:     "/work/workload",
				Icon:     "bars",
				Section:  "Work",
				Keywords: "capacity deadlines",
				View:     []string{"work.task.read"},
			},
			{
				Key:      "escalations",
				Label:    "Escalations",
				Href:     "/work/escalations",
				Icon:     "alert",
				Section:  "Work",
				Badge:    "escalations",
				Keywords: "overdue",
				View:     []string{"work.task.read"},
				Actions:  []string{"work.task.review"},
			},
			{
				Key:      "time-entries",
				Label:    "Time entries",
				Href:     "/work/time-entries",
				Icon:     "clock",
				Section:  "Work",
				Keywords: "timesheet hours logged tasks",
				View:     []string{"work.time_entry.read"},
				Actions:  []string{"work.time_entry.write"},
			},
			{
				Key:      "my-work",
				Label:    "My Work",
				Href:     "/dashboard/my-work",
				Icon:     "dashboard",
				Section:  "Work",
				Keywords: "my tasks my reports my leave my time",
				// Every read behind My Work is self-scoped or optional. This item
				// is always visible, so it also carries the global "New" actions:
				// a destination that is merely hidden must never be the reason a
				// permitted control disappears.
				Actions: []string{
					"work.task.write",
					"people.daily_report.submit",
					"people.leave.request",
					"work.time_entry.write",
					"finance.expense.write",
				},
			},
		},
	},
	{
		Key:   "people",
		Title: "People",
		Items: []Item{
			{
				Key:      "availability",
				Label:    "Availability & leave",
				Href:     "/people",
				Icon:     "calendar",
				Section:  "People",
				Keywords: "roster holiday absence",
				View:     []string{"people.person.read"},
				Actions:  []string{"people.leave.read", "people.leave.manage"},
			},
			{
				Key:      "activity",
				Label:    "Activity",
				Href:     "/people/activity",
				Icon:     "pulse",
				Section:  "People",
				Keywords: "sessions online sign-in",
				View:     []string{"people.session.read"},
			},
			{
				Key:      "daily-reports",
				Label:    "Daily reports",
				Href:     "/daily-reports",
				Icon:     "clipboard",
				Section:  "People",
				Keywords: "report standup",
				View:     []string{"people.daily_report.read"},
				Actions:  []string{"people.daily_report.submit"},
			},
			{
				Key:      "departments",
				Label:    "Departments",
				Href:     "/people/departments",
				Icon:     "building",
				Section:  "People",
				Keywords: "departments units teams",
				// The department endpoints are gated by people.person.read /
				// people.person.manage today (people.department.* exists in the
				// catalogue but is not yet enforced).
				View:    []string{"people.person.read"},
				Actions: []string{"people.person.manage"},
			},
			{
				Key:      "onboard",
				Label:    "Onboard new person",
				Href:     "/people/onboard",
				Icon:     "plus",
				Section:  "People",
				Keywords: "invite new hire register",
				View:     []string{"core.user.manage"},
			},
			{
				Key:      "reviews",
				Label:    "Reviews",
				Href:     "/people/reviews",
				Icon:     "clipboard",
				Section:  "People",
				Keywords: "performance review rating feedback",
				View:     []string{"people.review.read"},
				Actions:  []string{"people.review.manage"},
			},
		},
	},
	{
		Key:   "revenue",
		Title: "Revenue",
		Items: []Item{
			{
				Key:      "commercial-dashboard",
				Label:    "Commercial dashboard",
				Href:     "/dashboard/commercial",
				Icon:     "pie",
				Section:  "Revenue",
				Keywords: "pipeline receivables cash leads",
				View:     []string{"intel.dashboard.read"},
				Actions:  []string{"crm.pipeline.read", "finance.report.read"},
			},
			{
				Key:      "daily-brief",
				Label:    "Daily Brief",
				Href:     "/dashboard/daily",
				Icon:     "clipboard",
				Section:  "Revenue",
				Keywords: "yesterday today attention decisions",
				View:     []string{"intel.dashboard.read"},
			},
			{
				Key:      "scorecard",
				Label:    "Scorecard",
				Href:     "/dashboard/scorecard",
				Icon:     "dashboard",
				Section:  "Revenue",
				Keywords: "financial operational commercial systems people health",
				View:     []string{"intel.dashboard.read"},
			},
			{
				Key:      "crm",
				Label:    "CRM",
				Href:     "/crm",
				Icon:     "people",
				Section:  "Revenue",
				Keywords: "leads pipeline sales opportunities contacts clients",
				// The CRM landing page reads leads, opportunities and the
				// pipeline summary; all three are required for it to render.
				View:    []string{"crm.lead.read", "crm.opportunity.read", "crm.pipeline.read"},
				Actions: []string{"crm.lead.write", "crm.lead.assign", "crm.opportunity.write"},
			},
			{
				Key:      "interactions",
				Label:    "Interactions",
				Href:     "/crm/interactions",
				Icon:     "clipboard",
				Section:  "Revenue",
				Keywords: "calls emails meetings notes logged",
				View:     []string{"crm.interaction.read"},
				Actions:  []string{"crm.interaction.write"},
			},
			{
				Key:      "followups",
				Label:    "Follow-ups",
				Href:     "/crm/followups",
				Icon:     "clock",
				Section:  "Revenue",
				Keywords: "scheduled next touch follow up",
				View:     []string{"crm.lead.read"},
				Actions:  []string{"crm.lead.write"},
			},
			{
				Key:      "proposals",
				Label:    "Proposals",
				Href:     "/crm/proposals",
				Icon:     "document",
				Section:  "Revenue",
				Keywords: "priced offers accepted declined",
				View:     []string{"crm.proposal.read"},
				Actions:  []string{"crm.proposal.write", "crm.proposal.discount_approve"},
			},
			{
				Key:      "contracts",
				Label:    "Contracts",
				Href:     "/contracts",
				Icon:     "document",
				Section:  "Revenue",
				Keywords: "agreements signing milestones renewals obligations",
				View:     []string{"contracts.contract.read"},
				Actions: []string{
					"contracts.contract.write",
					"contracts.contract.sign",
					"contracts.milestone.manage",
					"contracts.deliverable.manage",
					"contracts.change_request.approve",
				},
			},
			{
				Key:      "finance",
				Label:    "Finance",
				Href:     "/finance",
				Icon:     "card",
				Section:  "Revenue",
				Keywords: "cash receivables expenses invoices budgets payments profitability recurring",
				View:     []string{"finance.report.read"},
				// Every finance control the page offers, named by the exact
				// permission its endpoint requires (§8.4). Errors here used to
				// hide every finance write control from everyone, including both
				// CEOs: invented keys never matched the catalogue.
				Actions: []string{
					"finance.expense.read",
					"finance.expense.write",
					"finance.expense.approve",
					"finance.invoice.read",
					"finance.invoice.write",
					"finance.invoice.issue",
					"finance.payment.read",
					"finance.payment.record",
					"finance.budget.read",
					"finance.budget.manage",
					"finance.account.manage",
					"finance.recurring_cost.manage",
					"finance.amount.sensitive.read",
				},
			},
			{
				Key:      "my-expenses",
				Label:    "My Expenses",
				Href:     "/finance/my-expenses",
				Icon:     "card",
				Section:  "Revenue",
				Keywords: "expense claim reimbursement receipts",
				View:     []string{"finance.expense.read"},
				Actions:  []string{"finance.expense.write"},
			},
		},
	},
	{
		Key:   "platform",
		Title: "Platform",
		Items: []Item{
			{
				Key:      "systems",
				Label:    "Systems",
				Href:     "/systems",
				Icon:     "server",
				Section:  "Platform",
				Keywords: "monitoring uptime incidents checks domains certificates credentials health",
				View:     []string{"systems.system.read"},
				Actions: []string{
					"systems.system.manage",
					"systems.check.read",
					"systems.incident.read",
					"systems.incident.manage",
					"systems.domain.manage",
				},
			},
			{
				Key:      "units",
				Label:    "Units",
				Href:     "/units",
				Icon:     "building",
				Section:  "Platform",
				Keywords: "business units integrations indicators",
				View:     []string{"units.unit.read"},
				Actions:  []string{"units.unit.manage", "units.indicator.read"},
			},
			{
				Key:      "intel",
				Label:    "Intel",
				Href:     "/intel",
				Icon:     "pie",
				Section:  "Platform",
				Keywords: "decisions risks briefs metric snapshots",
				View:     []string{"intel.decision.read"},
				Actions: []string{
					"intel.decision.manage",
					"intel.risk.read",
					"intel.risk.manage",
					"intel.brief.read",
					"intel.brief.manage",
					"intel.metric.read",
				},
			},
			{
				Key:      "github",
				Label:    "GitHub",
				Href:     "/integrations/github",
				Icon:     "github",
				Section:  "Platform",
				Keywords: "connect repository code activity",
				// The page only reads the caller's own connection status
				// (GET /api/v1/integrations/github/status is authenticated).
			},
		},
	},
	{
		Key:   "communicate",
		Title: "Communicate",
		Items: []Item{
			{
				Key:      "inbox",
				Label:    "Inbox",
				Href:     "/email/inbox",
				Icon:     "document",
				Section:  "Communicate",
				Keywords: "email messages inbox messages",
				// Thread and address reads are authenticated and self-scoped.
			},
			{
				Key:      "compose",
				Label:    "Compose",
				Href:     "/email/compose",
				Icon:     "plus",
				Section:  "Communicate",
				Keywords: "send email write message",
				// Go gates POST /api/v1/email/compose on core.email.send.
				View:    []string{"core.email.send"},
				Actions: []string{"core.email.send"},
			},
			{
				Key:      "addresses",
				Label:    "Addresses",
				Href:     "/email/addresses",
				Icon:     "card",
				Section:  "Communicate",
				Keywords: "send as addresses email identity",
				// Address management is self-scoped and authenticated.
			},
		},
	},
	{
		Key: "account",
		Items: []Item{
			{
				Key:      "settings",
				Label:    "Settings",
				Href:     "/settings",
				Icon:     "sliders",
				Keywords: "sessions devices account",
				// Settings is the caller's own account: sessions and devices.
			},
		},
	},
}
