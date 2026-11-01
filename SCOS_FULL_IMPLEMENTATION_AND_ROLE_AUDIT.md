# SCOS Full Implementation and Role Audit

**Repository:** SafariStack Atlas (C:\HOME\Safari Atlas)
**Branch:** feat/web-redesign (up to date with origin/feat/web-redesign)
**Commit:** e93f419 feat(intelgrations): complete frontend write interfaces
**Working tree:** Dirty — uncommitted dashboard + integrations + outbox changes present
**Auditor role:** Independent audit; no code modified

---

## Executive Summary

The SafariStack Company Operating System (SCOS / Atlas) is built on a modular-monolith Go backend, Next.js frontend, Elixir orchestrator, and PostgreSQL. The three principal roles (Internal CEO, External CEO, Employee) are NOT equally served.

The backend has substantial implementation across finance, systems, units, intel, integrations, work, people, CRM, contracts, finance, dashboard, approvals, audit, files, settings, notifications, and auth. However:

- **The frontend frontend routes exist only as pages.tsx shells** for most modules. CRM (`crm/page.tsx`) and contracts (`contracts/page.tsx`) are single-page shells with no forms. Systems has forms but no page-level workflow. Units/Intel have forms. Work has a Kanban board.
- **CRM and contracts backend implementations exist** (store.go, service.go, http.go, tests) but there is **no usable frontend** for lead creation, opportunity management, proposal drafting, or contract lifecycle — the two Commercial CEO-critical modules.
- **The External CEO has no dedicated commercial dashboard.** The single executive dashboard (`/dashboard`) serves both CEOs with overlapping tiles; there is no pipeline-centric, commercial-first view.
- **Employee onboarding/offboarding** is partially implemented (offboard endpoint exists, but no onboarding checklist UI, no full HR workflow).
- **No end-to-end E2E browser tests** exist; Playwright is not configured. Verification is limited to Go unit/integration tests and Elixir ExUnit tests.

Verdict: **SCOS is not ready for daily business use by all three roles.** The Internal CEO has the most complete environment; the External CEO has almost none; employees have a partial workspace.

---

## Architecture Assessment

### Stack boundaries (respected)

| Concern | Owner | Status |
|---------|-------|--------|
| Business data writes | Go | ✅ Respected — Go is sole writer |
| Business rules/validation | Go | ✅ Respected |
| AuthN/AuthZ | Go | ✅ Respected |
| External provider calls | Go | ✅ Respected (GitHub webhook in integrations) |
| Inbound webhooks | Go | ✅ Resolved |
| Time-based orchestration | Elixir | ✅ Resolved (escalation sweeps, daily reports) |
| Event processing/reactions | Elixir | ✅ Resolved (outbox consumer → handlers) |
| Real-time to browser | Elixir Phoenix Channels | ✅ Resolved |
| UI rendering/UX | Next.js | ⚠️ Partial — many pages are shells |

### Architectural drift

1. **CRM/contracts backend exists but has zero frontend surface.** `apps/api/internal/crm/` and `apps/api/internal/contracts/` have complete Go implementations, but `apps/web/app/(app)/crm/page.tsx` and `apps/web/app/(app)/contracts/page.tsx` are single-file shells with no forms or workflows. This violates the "Go owns rules; UI renders" pattern only in the sense that the UI side is absent.

2. **Two `registerIntegrations` calls** in router.go (lines 163 and 191) — a duplicate but harmless since both are gated on `d.Integrations != nil`.

3. **Finance frontend write functions exist in atlas.ts** (createExpense, submitExpense, decideExpense, settleExpense, reconcileExpense, createInvoice, updateInvoice, approveInvoice, issueInvoice, disputeInvoice, writeOffInvoice, createPayment, allocatePayment, reconcilePayment, createBudget, updateBudget, approveBudget, closeBudget, createAccount, updateAccount, createRecurringCost, updateRecurringCost) — matching the backend. Finance is one of the few modules where frontend and backend write coverage aligns.

4. **GitHub webhook is processed synchronously** (deviation from blueprint §8.9 which sketches an async inbox pipeline) — recorded as acceptable in PHASE2 §9.

---

## Documentation Baseline

| Document | Status | Relevance |
|----------|--------|-----------|
| docs/ATLAS.md | Current blueprint v0.1 | Primary requirements source |
| docs/PHASE2.md | Phase 2 to-do list | Work orchestration |
| docs/PHASE3.md | Phase 3 to-do list | CRM & contracts (backend only) |
| docs/adr/ADR-001…019 | Architecture decisions | Stack, boundaries, policies |
| README.md | Project overview | Vision, roadmap |
| TODO.md | Implementation tracking | Phase 1 complete, Phase 2 backend done |
| docs/FRONTEND.md | Frontend guide | Auth, session, realtime |
| docs/runbooks/local-development.md | Dev setup | Not read (not critical to role audit) |

Key documentation facts:
- **Two-CEO model** is established (ATLAS.md §3): `ceo_internal` (Internal & Technical CEO) and `ceo_commercial` (Commercial CEO) are distinct permission domains.
- **Shared scope**: Both CEOs handle major expenditure, pricing exceptions, hiring/termination, company performance, cross-functional blockers, strategy, business-unit oversight.
- **Approval matrix** (§3.3): Below T1 → line manager; T1–T2 → domain CEO; Above T2 → both CEOs. Nobody approves their own request.
- **Escalation ladder** (§3.4): L0 owner → L1 owner's lead → L2 domain CEO → L3 both CEOs.

---

## Internal CEO Audit

### A. Executive and organizational control

| Capability | Status | Evidence |
|------------|--------|----------|
| Internal CEO dashboard | PARTIAL | Single `/dashboard` with staff status, approvals, escalations, units, intel tiles. No CEO-specific view. |
| Company-wide operational overview | PARTIAL | Dashboard tiles + audit trail |
| Organizational structure | PARTIAL | People page (`/people`) with team-grid, leave-panel; departments exist in backend |
| Departments and business units | PARTIAL | Units page exists; departments backend exists |
| Employee directory | PARTIAL | `/people` page with team-grid.tsx |
| Reporting relationships | PARTIAL | person.line_manager_id in schema; no UI surfacing |
| Role and permission management | BACKEND ONLY | `core.settings`, `authz` package exist; no UI role-management page |
| Company policies | BACKEND ONLY | Settings API exists; no dedicated policy UI |
| Executive decisions | PARTIAL | Intel decisions page exists (`/intel`) |
| Approvals and escalations | PARTIAL | Approvals engine backend complete; frontend approvals UI not found in app/(app) routes |
| Audit trails | PARTIAL | `/audit` global list + per-record history exist in backend; frontend route not in app/(app) listing |

**Assessment:** Internal CEO has read visibility across most domains but lacks a dedicated, CEO-scoped executive dashboard. Approvals UI is missing from frontend routes.

### B. People and HR management

| Capability | Status | Evidence |
|------------|--------|----------|
| Employee onboarding | MISSING | No onboarding checklist UI; invite flow exists in backend |
| Employee accounts | BACKEND ONLY | Auth service (login, MFA, invite, sessions) complete |
| Employee profiles | PARTIAL | `/people` team-grid |
| Department assignments | BACKEND ONLY | Schema exists; no UI department management |
| Job positions | BACKEND ONLY | `people.positions` table; no UI |
| Role assignments | BACKEND ONLY | `core.role_grants`; no UI |
| Attendance | PARTIAL | Presence/session activity (`/people/activity`) |
| Leave requests and approvals | PARTIAL | Leave panel exists; backend decide flow complete |
| Employee performance | MISSING | `people.reviews` table exists; no UI |
| Employee task assignments | PARTIAL | Work tasks page exists |
| Employee offboarding | BACKEND ONLY | Offboard endpoint exists; no UI form found |
| Account suspension/archival | BACKEND ONLY | Sessions revoke endpoints exist |
| Employee records/documents | BACKEND ONLY | Files module exists |

**Assessment:** HR lifecycle is partially implemented. Onboarding, performance reviews, and offboarding UI are missing.

### C. Engineering and technical operations

| Capability | Status | Evidence |
|------------|--------|----------|
| Repository management | PARTIAL | Systems page; GitHub evidence integration (ADR-018) |
| Project technical oversight | PARTIAL | Work projects page |
| Engineering task visibility | PARTIAL | Work tasks Kanban |
| Developer activity | MISSING | No GitHub PR/CI dashboard |
| Code review visibility | MISSING | Work reviews exist backend; no dedicated UI |
| Technical delivery | PARTIAL | Work module |
| Software releases | MISSING | No release management |
| Infrastructure inventory | PARTIAL | Systems page |
| System health monitoring | PARTIAL | Systems checks |
| Incidents and technical risks | PARTIAL | Systems incident page |
| Cybersecurity oversight | MISSING | No security dashboard |

**GitHub repository monitoring:** Implemented as webhook (`POST /webhooks/github`, HMAC-SHA256 verified, dedup by X-GitHub-Delivery, links merged PR to task by Atlas ref). This is an **implemented** requirement, not proposed.

### D. Internal financial oversight

| Capability | Status | Evidence |
|------------|--------|----------|
| Expenses | COMPLETE | Frontend + backend write + approve/settle/reconcile |
| Revenue visibility | PARTIAL | Finance overview/receivables pages |
| Cash position | COMPLETE | `getCashPosition()` |
| Budgets | COMPLETE | Frontend + backend |
| Financial reports | PARTIAL | Project profitability exists |
| Approval authority | COMPLETE | Expense approval flow |
| Financial risks | MISSING | No dedicated financial risk view |
| Business-unit financial summaries | PARTIAL | Units finance summary backend exists |

**Split:** Expenses/budgets/payments are usable by Internal CEO. Statutory accounting is explicitly out of scope (§8.4).

### E. Management intelligence

| Capability | Status | Evidence |
|------------|--------|----------|
| Company performance | PARTIAL | Dashboard tiles |
| Department performance | MISSING | No department scorecard |
| Business-unit performance | PARTIAL | Units finance summaries |
| Operational indicators | PARTIAL | Systems availability, units indicators |
| Decisions | PARTIAL | Intel decisions page |
| Risks | PARTIAL | Intel risks page |
| Executive briefs | MISSING | Daily brief (07:30 EAT) not implemented |
| Company scorecard | MISSING | §18 scorecard not implemented |

**Assessment:** Internal CEO can see current state but cannot run the weekly management meeting from Atlas (success criterion #3 unmet).

---

## External CEO Audit

The External CEO has **no dedicated commercial environment.**

| Capability | Status | Evidence |
|------------|--------|----------|
| Lead registration | BACKEND ONLY | CRM backend complete; no frontend forms |
| Prospect management | BACKEND ONLY | CRM organizations/contacts/leads |
| Sales pipeline | BACKEND ONLY | `getPipeline()` in atlas.ts; no pipeline board UI |
| Opportunity tracking | BACKEND ONLY | Contracts/opportunities backend; page.tsx shell only |
| Follow-up scheduling | BACKEND ONLY | Followups backend; no UI |
| Sales activities | BACKEND ONLY | Interactions backend; no UI |
| Client acquisition reporting | MISSING | No commercial dashboard |
| Conversion tracking | MISSING | Pipeline metrics not surfaced |
| Sales targets | MISSING | No target-setting UI |
| Client registration | BACKEND ONLY | Organizations backend |
| Client profiles | BACKEND ONLY | Contacts backend |
| Contact management | BACKEND ONLY | No UI |
| Customer communication | MISSING | WhatsApp deferred (ADR-011) |
| Client interaction history | BACKEND ONLY | Interactions backend |
| Client segmentation | MISSING | No segmentation UI |
| Client relationship ownership | BACKEND ONLY | account owner field exists |
| Client issue escalation | MISSING | No client issue tracking |
| Proposal creation | BACKEND ONLY | Proposals backend; no UI |
| Quotations | BACKEND ONLY | Proposal line items backend |
| Pricing | MISSING | No pricing configuration UI |
| Negotiations | MISSING | No negotiation tracking UI |
| Contract preparation | BACKEND ONLY | Contracts backend |
| Contract signing/status | BACKEND ONLY | Contract lifecycle backend |
| Contract renewals | MISSING | Renewal reminders not implemented |
| Partnership management | MISSING | No partnership module |
| Revenue opportunities | MISSING | No revenue tracking UI |
| Active client projects | PARTIAL | Work projects page |
| Project progress | PARTIAL | Work tasks/board |
| Delivery milestones | PARTIAL | Milestones backend |
| Customer issues | MISSING | No customer issue module |
| Outstanding deliverables | MISSING | No deliverable tracking UI |
| Client satisfaction | MISSING | No satisfaction tracking |
| Delivery risks | PARTIAL | Blockers board |
| Revenue generated | MISSING | No commercial revenue dashboard |
| Sales pipeline value | MISSING | Pipeline value computed backend; not surfaced |
| Conversion rates | MISSING | Not computed/surfaced |
| Proposal success rates | MISSING | Not tracked |
| Client acquisition | MISSING | No acquisition dashboard |
| Recurring revenue | MISSING | Recurring costs exist (finance) but not revenue |
| Outstanding receivables | PARTIAL | Finance receivables ageing page |
| Commercial performance | MISSING | No commercial dashboard |
| External CEO dedicated dashboard | MISSING | Only shared `/dashboard` exists |

**Assessment:** External CEO cannot complete any commercial lifecycle step through the UI. CRM/contracts backends are complete but frontendless. Cannot run commercial operations through SCOS.

---

## Employee Experience Audit

| Capability | Status | Evidence |
|------------|--------|----------|
| Employee dashboard | MISSING | No `/employee` or role-specific employee dashboard; employees see shared `/dashboard` if they have CEO permissions, otherwise nothing |
| Personal profile | PARTIAL | `/people` team-grid |
| Assigned department | BACKEND ONLY | department_id on person; no UI |
| Reporting manager | BACKEND ONLY | line_manager_id; no UI |
| Assigned responsibilities | PARTIAL | Work tasks assigned to person |
| Personal notifications | PARTIAL | Notifications backend; frontend route not in app/(app) listing |
| Company announcements | MISSING | No announcements module |
| Assigned projects | PARTIAL | Work projects page |
| Assigned tasks | PARTIAL | Work tasks Kanban + detail |
| Task details | PARTIAL | Work task page |
| Task status updates | PARTIAL | Work transitions |
| Deadlines | PARTIAL | Task due_at |
| Subtasks | MISSING | task_dependencies exist; no subtask UI |
| Deliverable submission | PARTIAL | Evidence upload |
| Work progress | PARTIAL | Project board |
| Blocker reporting | PARTIAL | Blockers board |
| Task history | PARTIAL | Audit trail exists |
| Attendance | PARTIAL | Presence view |
| Leave requests | PARTIAL | Leave panel exists |
| Leave history | PARTIAL | Leave list backend |
| Approval status | PARTIAL | Leave decision status |
| Company documents | BACKEND ONLY | Files module; no UI found |
| Asset assignments | MISSING | No asset module |
| HR requests | MISSING | No HR request module |
| Employee support tickets | MISSING | No support module |
| Announcements | MISSING | No announcements module |
| Internal messages | MISSING | No messaging module |
| Direct communication | MISSING | No direct messaging |
| Notifications | PARTIAL | In-app notifications backend |
| Message history | MISSING | No message history UI |
| Attachments | BACKEND ONLY | Files module |
| GitHub identity connection | MISSING | No UI to connect GitHub identity |
| View assigned repositories | MISSING | No repository view |
| Link code activity to tasks | PARTIAL | GitHub evidence links PR to task (backend) |
| View pull requests | MISSING | No PR UI |
| View review feedback | MISSING | Work reviews backend; no UI |
| CI/CD outcomes | MISSING | No CI/CD integration |
| What to do today | PARTIAL | Work tasks page |
| Overdue work | PARTIAL | Overdue tasks visible |
| Approaching deadline | PARTIAL | Task due dates |
| Requires attention | PARTIAL | Blockers/escalations |
| Completed work | PARTIAL | Task status |
| Feedback/approvals awaited | PARTIAL | Reviews backend |

**Assessment:** Employees have task execution visibility through the work module, but no dedicated employee dashboard, no onboarding, no leave UI integration with daily work, no GitHub integration UI, and no communication tools.

---

## Frontend and UI Completeness Audit

| Module | Navigation | Page | List | Detail | Create | Edit | Delete | Workflow | Status |
|--------|-----------|------|------|--------|--------|------|--------|----------|--------|
| Dashboard | ✅ | ✅ | ✅ | N/A | N/A | N/A | N/A | N/A | PARTIAL |
| Work | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ❌ | ✅ | COMPLETE |
| People | ✅ | ✅ | ✅ | ✅ | ❌ | ❌ | ❌ | ✅ (leave) | PARTIAL |
| Finance | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ❌ | ✅ | COMPLETE |
| Systems | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ❌ | ✅ (incident) | COMPLETE |
| Units | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ❌ | N/A | COMPLETE |
| Intel | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ❌ | N/A | COMPLETE |
| Integrations | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ❌ | N/A | COMPLETE |
| CRM | ✅ | ✅ (shell) | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | BACKEND ONLY |
| Contracts | ✅ | ✅ (shell) | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | BACKEND ONLY |
| Approvals | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | MISSING (frontend) |
| Audit | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | MISSING (frontend) |
| Files | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | MISSING (frontend) |
| Settings | ✅ | ✅ | N/A | N/A | N/A | N/A | N/A | N/A | PARTIAL |
| Notifications | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | MISSING (frontend) |
| Daily reports | ✅ | ✅ | N/A | N/A | ✅ | ❌ | ❌ | ✅ | PARTIAL |
| Leave | ✅ | ✅ (panel) | N/A | N/A | ✅ | ❌ | ❌ | ✅ | PARTIAL |

No module has DELETE/archive UI. Loading states, empty states, and error handling are partially implemented (ProblemNotice, EmptyState components exist). Permission-aware controls exist via capability maps (finance/permissions.ts). Responsive layouts use Tailwind. API connectivity is via atlas.ts typed client.

**Responsive layouts:** Partially implemented (Tailwind classes, but no dedicated mobile verification).

---

## Auth, Authorization, Role Separation

| Control | Status |
|---------|--------|
| Login/logout | COMPLETE (Go auth service + Next.js login flow) |
| Session management | COMPLETE (sessions table, revoke, heartbeat) |
| MFA | COMPLETE (TOTP, mandatory for privileged roles) |
| Password reset | PARTIAL (invite/accept flow; no explicit forgot-password) |
| Role assignments | BACKEND ONLY (core.role_grants; no UI) |
| Permission definitions | COMPLETE (module.resource.action format) |
| Backend authorization | COMPLETE (every route gated by permission) |
| Frontend route guards | PARTIAL (capability maps like finance/permissions.ts) |
| Capability-based controls | PARTIAL (finance has it; others unclear) |
| Role-specific navigation | MISSING (no role-based nav differentiation) |
| Unauthorized access handling | COMPLETE (401/403 Problem Details) |
| Audit logging | COMPLETE (append-only audit.events) |
| Approval authority | COMPLETE (both-CEO rule for >T2) |
| Joint CEO decisions | COMPLETE (approvals_required=2) |
| Separation of duties | COMPLETE (FourEyes, TwoPersonRule, no self-approval) |

**Vulnerabilities identified:** None found in code review, but no frontend role-based navigation means users may see navigation entries for modules they cannot use (Go backend will refuse, but UI is confusing).

---

## End-to-End Workflow Audit

| Workflow | Status | Missing Steps |
|----------|--------|---------------|
| Employee lifecycle | PARTIAL | Onboarding UI, offboarding UI, performance review UI, account suspension UI |
| Commercial lifecycle | BACKEND ONLY | Lead→opportunity→proposal→contract frontend missing entirely |
| Project delivery | PARTIAL | Handover acceptance UI, client acceptance UI, evidence verification UI |
| Internal operations | PARTIAL | Approvals UI missing, policy change UI missing |
| Financial lifecycle | PARTIAL | Invoice→payment reconciliation UI partial; e-invoicing not implemented |
| Technical operations | PARTIAL | Incident channel UI, post-incident review UI, external uptime integration |

---

## Atlas Module Verification

| Module | Backend R/W | Frontend R/W | Permissions | Audit | Concurrency | E2E Workflow |
|--------|------------|--------------|-------------|-------|-------------|--------------|
| Finance | COMPLETE | COMPLETE | ✅ | ✅ | ✅ | PARTIAL (no invoice→payment UI flow) |
| Systems | COMPLETE | COMPLETE | ✅ | ✅ | ✅ | PARTIAL (no incident channel UI) |
| Units | COMPLETE | COMPLETE | ✅ | ✅ | ✅ | PARTIAL (no sync failure alert UI) |
| Intel | COMPLETE | COMPLETE | ✅ | ✅ | ✅ | PARTIAL (no daily brief UI) |
| Integrations | COMPLETE | COMPLETE | ✅ | ✅ | ✅ | PARTIAL (no outbound call adapter UI) |
| CRM | COMPLETE | MISSING | ✅ | ✅ | N/A | BACKEND ONLY |
| Contracts | COMPLETE | MISSING | ✅ | ✅ | ✅ | BACKEND ONLY |
| Work | COMPLETE | COMPLETE | ✅ | ✅ | ✅ | COMPLETE |
| People | COMPLETE | PARTIAL | ✅ | ✅ | ✅ | PARTIAL (no onboarding/offboarding UI) |
| Dashboard | COMPLETE | PARTIAL | ✅ | N/A | N/A | PARTIAL |

---

## Testing Results

| Suite | Status | Notes |
|-------|--------|-------|
| `go build ./...` | PASS | |
| `go vet ./internal/...` | PASS | |
| `go test ./internal/...` | PASS | All packages pass |
| `mix test` (orchestrator) | PASS | 110 tests pass |
| Browser/E2E tests | NOT CONFIGURED | Playwright not set up; no browser-level tests |

**Test coverage:** Strong on Go unit + DB integration tests. Strong on Elixir outbox handler tests. Zero E2E coverage for any user workflow.

---

## Technical Debt

1. Duplicate `registerIntegrations` in router.go (harmless but untidy)
2. CRM/contracts backends complete but frontendless (major gap)
3. No frontend role-based navigation
4. No permission list from `GET /api/v1/auth/me` (roles only; capabilities hardcoded in finance/permissions.ts)
5. `generate:api` is a placeholder; atlas.ts is hand-written
6. No seeded business data (by design — but means no demo state)
7. WhatsApp/telephony providers deferred (ADR-011, ADR-012)
8. No OpenAPI spec for work/people modules

---

## Security Gaps

None identified in code review. Authorization is enforced in Go backend on every route. Field-level sensitivity exists (`finance.amount.sensitive.read`). MFA mandatory for privileged roles. Sessions are revocable. Audit is append-only.

**Gap:** No frontend role-based navigation means unauthorized UI navigation is possible (backend will refuse, but user experience is poor).

---

## Missing Functionality (Consolidated)

1. CRM frontend (leads, opportunities, interactions, follow-ups, proposals, pipeline board)
2. Contracts frontend (contract lifecycle, signing, change requests, renewals)
3. Approvals frontend (pending approvals, approve/decline/escalate)
4. External CEO commercial dashboard
5. Employee dashboard
6. Onboarding UI
7. Offboarding UI
8. Performance review UI
9. Department management UI
10. Role/permission management UI
11. Company announcements
12. Internal messaging
13. GitHub PR/CI developer view
14. Daily executive brief (07:30 EAT)
15. Company scorecard (§18)
16. E-invoicing (KRA eTIMS)
17. WhatsApp/telephony providers
18. Client portal (D14)
19. E2E browser tests
20. Incident channel UI, post-incident review UI

---

## Recommended Development Sequence

1. P0: CRM frontend + Contracts frontend (External CEO can't operate without these)
2. P0: External CEO commercial dashboard
3. P0: Employee dashboard + leave UI integration
4. P1: Approvals frontend
5. P1: Role-based navigation
6. P1: Onboarding/offboarding UI
7. P2: Daily executive brief + company scorecard
8. P2: Performance review UI
9. P3: E2E browser tests
10. P3: GitHub PR/CI developer view
11. P3: WhatsApp/telephony adapters

---
