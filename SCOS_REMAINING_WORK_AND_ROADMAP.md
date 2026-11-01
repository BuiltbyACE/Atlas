# SCOS Remaining Work and Roadmap

Source: Repository at commit e93f419, branch feat/web-redesign.
Priorities: P0 = critical blocker to company operations; P1 = essential operational capability; P2 = important improvement; P3 = advanced capability / future expansion.

---

## P0 — Critical Blockers to Company Operations

### P0-1: CRM frontend (leads, opportunities, pipeline)

**What is missing:** The CRM backend (`internal/crm/`) is complete with store, service, HTTP handlers, and tests. However, `apps/web/app/(app)/crm/page.tsx` is a single-file shell with no forms, no pipeline board, no lead create/edit, no opportunity tracking, and no interaction recording.

**Why it matters:** The External CEO cannot register a lead, manage a pipeline, track opportunities, or record customer interactions through the UI. The entire commercial lifecycle (§9.1 lead-to-cash) cannot start in SCOS.

**Dependencies:** None — backend is done.

**Estimated effort:** Large (2–4 weeks for a full CRM UI: lead forms, pipeline board, opportunity pages, interaction logging, follow-up scheduling).

**Acceptance criteria:** A sales rep can create a lead, assign an owner and next action, record an interaction, convert to an opportunity, and view the pipeline board — all through the web UI.

---

### P0-2: Contracts frontend

**What is missing:** The contracts backend (`internal/contracts/`) is complete (contract lifecycle, versions, milestones, deliverables, obligations, SLA terms, change requests). However, `apps/web/app/(app)/contracts/page.tsx` is a single-file shell with no contract forms, no signing flow, and no change-request UI.

**Why it matters:** After a won opportunity, the External CEO cannot draft, approve, sign, or track a contract in SCOS. The lead-to-cash workflow breaks at the contract stage.

**Dependencies:** CRM frontend (P0-1) — contracts reference organizations/opportunities.

**Estimated effort:** Large (2–3 weeks: contract forms, signing workflow, change requests, milestone tracking).

**Acceptance criteria:** A contract can be drafted from an accepted proposal, submitted for internal review, approved, signed, and linked to a handover — through the web UI.

---

### P0-3: External CEO commercial dashboard

**What is missing:** The single `/dashboard` serves both CEOs with overlapping tiles (people, approvals, escalations, units, intel). There is no commercial-first dashboard showing pipeline value, conversion rates, win/loss, receivables ageing, or client health.

**Why it matters:** Success criterion #1 (ATLAS.md §1.3) requires both CEOs to answer "what is happening, what needs attention, who is responsible, are we on track?" in under five minutes. The External CEO cannot do this.

**Dependencies:** CRM frontend (P0-1) — dashboard needs pipeline data.

**Estimated effort:** Medium (1–2 weeks: commercial dashboard page with pipeline, receivables, recent deals tiles).

**Acceptance criteria:** The External CEO has a dedicated route showing commercial KPIs and actionable priorities.

---

### P0-4: Approvals frontend

**What is missing:** The approvals engine backend is complete (policy lookup, four-eyes/two-person enforcement, notification fan-out, worker expiry sweep). However, there is no frontend route for `GET /api/v1/approvals/pending`, `GET /api/v1/approvals/mine`, or approve/decline/escalate/cancel actions.

**Why it matters:** Both CEOs cannot approve expenses, purchases, or proposals through the UI. The approval matrix (§3.3) is unenforceable from the browser.

**Dependencies:** None — backend is done.

**Estimated effort:** Medium (1–2 weeks: approvals list page, approve/decline forms, escalation actions).

**Acceptance criteria:** A CEO can view pending approvals for their role and approve/decline an expense through the web UI.

---

## P1 — Essential Operational Capabilities

### P1-1: Role-based navigation

**What is missing:** No role-based navigation differentiation. All users potentially see all module navigation entries; Go backend refuses unauthorized calls, but the UI is confusing.

**Why it matters:** Employees see modules they cannot use; CEOs cannot quickly find their role-specific tools.

**Dependencies:** Auth session roles (already returned by `GET /api/v1/auth/me`).

**Estimated effort:** Small (3–5 days: nav component that filters by role/permissions).

**Acceptance criteria:** Navigation entries shown to a user match the permissions in their session.

---

### P1-2: Employee dashboard

**What is missing:** No role-specific employee dashboard. Employees see either the shared executive dashboard (if they have CEO permissions) or nothing role-specific.

**Why it matters:** Success criterion for employees (ATLAS.md §5 Part 5F) — employees must understand what to do today, what is overdue, what requires attention.

**Dependencies:** Work tasks backend (done), notifications backend (done).

**Estimated effort:** Medium (1–2 weeks: employee dashboard page with today's tasks, overdue items, leave status).

**Acceptance criteria:** An employee can log in and see their assigned tasks, overdue items, and pending approvals.

---

### P1-3: Onboarding and offboarding UI

**What is missing:** Invite/accept flow exists in backend (`/api/v1/auth/invites`, `/api/v1/auth/invites/accept`), but there is no onboarding checklist UI. Offboard endpoint exists but no UI form.

**Why it matters:** Employee lifecycle (Workflow 1) cannot be completed through SCOS.

**Dependencies:** People backend (done).

**Estimated effort:** Medium (1 week: onboarding checklist page, offboard form).

**Acceptance criteria:** An admin can invite a new employee, track onboarding checklist completion, and offboard an employee through the UI.

---

### P1-4: Permission list from auth/me

**What is missing:** `GET /api/v1/auth/me` returns roles only. Finance/permissions.ts hardcodes role→capability mappings. When Go exposes a real permission list, the frontend must consume it.

**Why it matters:** Capability-based UI gating is currently hardcoded and can drift from backend permissions.

**Dependencies:** Auth service changes.

**Estimated effort:** Small (2–3 days: extend /me response with permissions; update finance/permissions.ts to consume it).

**Acceptance criteria:** `financeCapabilities()` derives from the permissions list returned by `/api/v1/auth/me`, not from a hardcoded role map.

---

## P2 — Important Improvements

### P2-1: Daily executive brief (07:30 EAT)

**What is missing:** Not implemented. ATLAS.md §8.8 requires a daily brief with yesterday's results, needs-attention items, today's follow-ups, and decisions awaiting each CEO.

**Dependencies:** Dashboard backend (done), intel backend (done).

**Estimated effort:** Medium (1–2 weeks: brief generation endpoint + delivery UI).

**Acceptance criteria:** Both CEOs receive a daily brief summarizing company status and decisions awaiting them.

---

### P2-2: Company scorecard (§18)

**What is missing:** The 18 metrics across growth, sales discipline, delivery, quality, commitments, cash, profit, people, reliability, management are not computed or surfaced.

**Dependencies:** All module backends (mostly done).

**Estimated effort:** Medium (1–2 weeks: scorecard endpoint + UI).

**Acceptance criteria:** The weekly CEO meeting can be run from Atlas scorecard data.

---

### P2-2b: Performance review UI

**What is missing:** `people.reviews` table exists but no UI for performance reviews.

**Estimated effort:** Small (3–5 days).

**Acceptance criteria:** A manager can create and complete a performance review for a report.

---

### P2-2c: Department management UI

**What is missing:** `people.departments` table exists but no UI to manage departments or view department structure.

**Estimated effort:** Small (3–5 days).

**Acceptance criteria:** An admin can create departments, assign leads, and view department rosters.

---

## P3 — Advanced Capabilities and Future Expansion

| Task | What | Effort | Dependencies |
|------|------|--------|--------------|
| E2E browser tests (Playwright) | Login+MFA, create task→escalation, submit daily report, lead→contract→handover, expense approval | Large | None |
| GitHub PR/CI developer view | View linked PRs, review feedback, CI/CD outcomes | Medium | GitHub evidence backend (done) |
| Daily executive brief delivery | In-app + email + WhatsApp summary | Medium | Brief generation (P2-1) |
| WhatsApp Business adapter | Lead conversations, reminders, daily report prompts | Large | ADR-011 |
| Telephony adapter | Click-to-call, call logging | Large | ADR-012 |
| E-invoicing (KRA eTIMS) | Compliant invoices | Large | ADR-013 |
| Payments (M-Pesa/bank) | Payment received notifications, reconciliation | Large | Not started |
| Client portal (D14) | View projects, approve milestones, pay invoices | Large | D14 open decision |
| Productisation assessment | Jenga Systems path | Large | D15 open decision |

---

## Estimated Remaining Development Effort

| Priority | Effort |
|--------|--------|
| P0 (critical blockers) | ~8–12 weeks |
| P1 (essential) | ~4–6 weeks |
| P2 (important) | ~4–6 weeks |
| P3 (advanced) | ~20–30 weeks |
| **Total** | **~36–54 weeks** |

This assumes a small team (2–4 engineers) per ATLAS.md §17. The P0 items alone represent roughly 2–3 months of work before SCOS can support daily business for all three roles.

---

## Recommended Implementation Order

1. **CRM frontend** (P0-1) — unblocks External CEO commercial operations
2. **Contracts frontend** (P0-2) — completes lead-to-cash
3. **External CEO commercial dashboard** (P0-3) — gives External CEO a role-appropriate view
4. **Approvals frontend** (P0-4) — enables both-CEO approval workflow
5. **Role-based navigation** (P1-1) — fixes UI clarity for all roles
6. **Employee dashboard** (P1-2) — gives employees a workspace
7. **Onboarding/offboarding UI** (P1-3) — completes employee lifecycle
8. **Permission list from /auth/me** (P1-4) — prevents capability drift
9. **Daily executive brief + scorecard** (P2-1, P2-2) — enables management rhythm
10. **E2E browser tests** (P3) — verifies all the above
