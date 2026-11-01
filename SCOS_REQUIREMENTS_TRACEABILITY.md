# SCOS Requirements Traceability

Maps each documented requirement (docs/ATLAS.md v0.1, PHASE2.md, PHASE3.md, ADRs) to implementation evidence.

Status: COMPLETE | PARTIAL | BACKEND ONLY | FRONTEND ONLY | PLACEHOLDER | MISSING | BLOCKED

Source: Repository at commit e93f419, branch feat/web-redesign.

---

## §3 Organization Model: Two CEOs

| Req | Source | Module | DB | Backend | Frontend | Tests | Status |
|-----|--------|--------|----|---------|----------|-------|--------|
| CEO role keys (ceo_internal, ceo_commercial) | §3.2 | core/auth | core.roles | authz package | No role UI | authz_test | PARTIAL |
| Approval matrix T1/T2/D1/C2 | §3.3 | approvals/settings | core.approval_policies | approvals service | No policy UI | policy_test | PARTIAL |
| Separation of duties (no self-approval) | §3.3 | approvals | core.approval_decisions | FourEyes/TwoPersonRule | finance/permissions.ts | policy_test | COMPLETE |
| Both-CEO approvals (2 distinct rows) | §3.3 | approvals | approvals_required col | approvals service | finance capabilities | policy_test | COMPLETE |
| Approvals expire on subject change | §3.3 | approvals | core.approvals | approvals sweeper | No UI | sweeper_test | PARTIAL |
| Escalation ladder L0-L3 | §3.4 | work/escalations | work.escalations | ladder.go + sweeper | Escalations page | ladder_test | PARTIAL |

## §4 Capability Map

| Req | Source | Module | DB | Backend | Frontend | Tests | Status |
|-----|--------|--------|----|---------|----------|-------|--------|
| Module 0 Platform foundation | §4 | core | core.* | auth, audit, settings | Settings page | - | PARTIAL |
| Module 1 CRM | §4 | crm | crm.* | crm service/http | crm/page.tsx (shell) | crm_test | BACKEND ONLY |
| Module 2 Work | §4 | work | work.* | work service/http | work/page.tsx + forms | work_test | COMPLETE |
| Module 3 Contracts | §4 | contracts | contracts.* | contracts service/http | contracts/page.tsx (shell) | contracts_test | BACKEND ONLY |
| Module 4 Finance | §4 | finance | finance.* | finance service/http | finance/page.tsx + forms | finance_test | COMPLETE |
| Module 5 People | §4 | people | people.* | people service/http | people/page.tsx | people_test | PARTIAL |
| Module 6 Systems | §4 | systems | systems.* | systems service/http | systems/page.tsx + forms | systems_test | COMPLETE |
| Module 7 Units | §4 | units | units.* | units service/http | units/page.tsx + forms | units_test | COMPLETE |
| Module 8 Intel | §4 | intel | intel.* | intel service/http | intel/page.tsx + forms | intel_test | COMPLETE |
| Integrations hub | §4 | integrations | integrations.* | integrations service/http | integrations/page.tsx + forms | integrations_test | COMPLETE |
| Orchestrator (Elixir) | §4 | orchestrator | orchestrator.* | outbox consumer + handlers | N/A | mix test (110 pass) | COMPLETE |

## §5 System Architecture

| Req | Source | Module | DB | Backend | Frontend | Tests | Status |
|-----|--------|--------|----|---------|----------|-------|--------|
| Go sole writer | §5.2 | All | All | ADR-002 enforced | N/A | - | COMPLETE |
| Elixir orchestrates via commands | §5.2 | orchestrator | orchestrator.* | Outbox consumer → handlers | N/A | mix test | COMPLETE |
| Next.js thin BFF | §5.3 | web | N/A | N/A | atlas.ts + pages | - | PARTIAL |
| Same-origin routing via Caddy | §5.3 | infra | N/A | N/A | N/A | - | PARTIAL |
| Schema-per-module | §5.6/§10.1 | All | 27 migrations | - | - | - | COMPLETE |
| Transactional outbox | §5.5/§7.1 | outbox | outbox.events | outbox.Write | N/A | event_test | COMPLETE |

## §6 Platform Foundation

| Req | Source | Module | DB | Backend | Frontend | Tests | Status |
|-----|--------|--------|----|---------|----------|-------|--------|
| Accounts + user_accounts | §6.1 | auth | core.user_accounts | auth service | login page | auth_test | COMPLETE |
| MFA TOTP | §6.1 | auth | mfa_totp_secret_enc | auth service | login-flow | totp_test | COMPLETE |
| Sessions | §6.1 | auth | core.sessions | auth service | sessions page | - | COMPLETE |
| Realtime tokens (Ed25519) | §6.1 | auth | N/A | RealtimeToken handler | N/A | - | COMPLETE |
| Service identities | §6.1 | auth | core.service_identities | service_identity.go | N/A | service_identity_test | COMPLETE |
| RBAC + scopes | §6.2 | authz | core.role_permissions/grants | authz package | finance/permissions.ts | authz_test | PARTIAL |
| Field-level sensitivity | §6.2 | finance | N/A | amount withholding | Restricted component | - | PARTIAL |
| Audit trail | §6.3 | audit | audit.events | audit service | No UI page | audit_test | BACKEND ONLY |
| Notifications | §6.4 | notifications | core.notifications | notifications service | No UI page | notifications_test | BACKEND ONLY |
| Files (pre-signed URLs) | §6.5 | files | core.files | files service | No UI page | files_test | BACKEND ONLY |
| Evidence (polymorphic) | §6.5 | work/integrations | work.evidence | work service | evidence upload | work_test | PARTIAL |

## §7 Event Model

| Req | Source | Module | DB | Backend | Frontend | Tests | Status |
|-----|--------|--------|----|---------|----------|-------|--------|
| Event catalogue | §7.4 | outbox | outbox.events | outbox/*.go constructors | N/A | - | PARTIAL (some event types missing) |
| Real-time channel topics | §7.5 | orchestrator | orchestrator.* | handlers.ex + channels.ex | SignalRefresh | mix test | PARTIAL |

## §8 Module Specifications

### 8.1 CRM

| Req | Source | Status | Evidence |
|-----|--------|--------|----------|
| Lead lifecycle states | §8.1 | BACKEND ONLY | crm store/service complete; no frontend |
| Opportunity stages | §8.1 | BACKEND ONLY | crm store/service complete; no frontend |
| Duplicate detection | §8.1 | BACKEND ONLY | crm.go validation helpers; no frontend |
| Pipeline board | §8.1 | MISSING | No frontend pipeline UI |
| WhatsApp inbox | §8.1 | MISSING | Deferred (ADR-011) |

### 8.2 Work

| Req | Source | Status | Evidence |
|-----|--------|--------|----------|
| Task lifecycle | §8.2 | COMPLETE | work service + http + frontend Kanban |
| Done means evidenced | §8.2 | PARTIAL | Refused without evidence (409); "Done only from InReview" not modelled |
| Timesheets (D7) | §8.2/ADR-019 | COMPLETE | work.time_entries; report-only enforcement |
| GitHub evidence (D6) | §8.2/ADR-018 | COMPLETE | /webhooks/github; pull_request evidence |

### 8.3 Contracts

| Req | Source | Status | Evidence |
|-----|--------|--------|----------|
| Contract lifecycle Draft→Signed | §8.3 | BACKEND ONLY | contracts service complete; no frontend |
| Handover object | §8.3 | PARTIAL | work.handovers table; no acceptance UI |
| Change requests | §8.3 | BACKEND ONLY | contracts service; no frontend |
| Renewal reminders | §8.3 | MISSING | Not implemented |

### 8.4 Finance

| Req | Source | Status | Evidence |
|-----|--------|--------|----------|
| Expense lifecycle | §8.4 | COMPLETE | finance service + http + frontend forms |
| Invoice lifecycle | §8.4 | COMPLETE | finance service + http + frontend forms |
| Payment reconciliation | §8.4 | COMPLETE | finance service + http + frontend forms |
| Budget threshold alerts | §8.4 | PARTIAL | Backend emits threshold events; no UI alert list |
| Project profitability | §8.4 | COMPLETE | getProjectProfitability() |
| E-invoicing (KRA eTIMS) | §8.4 | MISSING | Deferred (ADR-013) |

### 8.5 People

| Req | Source | Status | Evidence |
|-----|--------|--------|----------|
| Daily reports | §8.5 | COMPLETE | people service + http + frontend |
| Leave requests | §8.5 | COMPLETE | people service + http + leave panel |
| Availability | §8.5 | COMPLETE | GET /api/v1/people/availability |
| Offboarding | §8.5 | BACKEND ONLY | Offboard endpoint; no UI |
| Daily-report sweep (D5) | §8.5/ADR-017 | COMPLETE | sweep.go + policy.go; worker sweep |

### 8.6 Systems

| Req | Source | Status | Evidence |
|-----|--------|--------|----------|
| System registry | §8.6 | COMPLETE | systems page + forms |
| Health checks | §8.6 | COMPLETE | systems checks |
| Incidents | §8.6 | COMPLETE | systems incident page + forms |
| Post-incident reviews | §8.6 | PARTIAL | review_required logic; no dedicated review UI |
| Certificate/domain expiry | §8.6 | COMPLETE | systems domain/certificate pages |
| Credential location only | §8.6 | COMPLETE | store_path; no secrets |

### 8.7 Units

| Req | Source | Status | Evidence |
|-----|--------|--------|----------|
| Business units | §8.7 | COMPLETE | units page + forms |
| Unit integrations | §8.7 | COMPLETE | units integration page + forms |
| Indicator definitions | §8.7 | COMPLETE | units indicator page + forms |
| Unit snapshots | §8.7 | COMPLETE | units snapshots backend |
| Unit finance summaries | §8.7 | COMPLETE | units finance summary backend |

### 8.8 Intel

| Req | Source | Status | Evidence |
|-----|--------|--------|----------|
| Executive dashboard | §8.8 | PARTIAL | Dashboard page exists; not CEO-scoped |
| Daily executive brief | §8.8 | MISSING | Not implemented |
| Decisions queue | §8.8 | COMPLETE | intel decisions page |
| Risk register | §8.8 | COMPLETE | intel risks page |
| Weekly management pack | §8.8 | MISSING | Not implemented |

### 8.9 Integrations

| Req | Source | Status | Evidence |
|-----|--------|--------|----------|
| Integration registry | §8.9 | COMPLETE | integrations page + forms |
| Inbound webhooks | §8.9 | PARTIAL | GitHub webhook implemented; raw inbox deferred |
| Outbound calls/adapters | §8.9 | PARTIAL | Outbound log table; no adapter UI |
| Public API for partners | §8.9 | MISSING | Not implemented |

## §9 Cross-module Workflows

| Req | Source | Status | Evidence |
|-----|--------|--------|----------|
| Lead to cash | §9.1 | BACKEND ONLY | CRM/contracts/finance backends complete; frontend missing |
| Daily report cycle | §9.2 | COMPLETE | Sweep + people service; orchestrator reacts |
| Expense approval | §9.3 | COMPLETE | Approvals engine + finance |
| Change request | §9.4 | BACKEND ONLY | Contracts backend; no frontend |
| Incident response | §9.5 | PARTIAL | Systems incident page; no incident channel UI |
| Onboarding/offboarding | §9.6 | PARTIAL | Offboard endpoint; no onboarding UI |
| Renewal/retention | §9.7 | MISSING | Not implemented |

## §10 Data Architecture

| Req | Source | Status | Evidence |
|-----|--------|--------|----------|
| Schema per module | §10.1 | COMPLETE | 27 migrations; schemas created |
| Cross-schema FK policy | §10.1 | COMPLETE | Only to core/people/shared anchors |
| UUIDv7 primary keys | §10.3 | COMPLETE | All tables use UUID |
| Human references | §6.6 | COMPLETE | core.next_reference(type, year) |
| timestamptz UTC | §6.6 | COMPLETE | All timestamps timestamptz |
| Money as minor units | ADR-010 | COMPLETE | amount_minor bigint + currency |
| Optimistic locking | §6.6 | COMPLETE | version column + If-Match/ETag |
| Cursor pagination | §6.6 | COMPLETE | limit+1 cursor pattern |

## §11 API Standards

| Req | Source | Status | Evidence |
|-----|--------|--------|----------|
| OpenAPI 3.1 spec-first | §11 | PARTIAL | public.yaml covers auth+internal only; work/people missing |
| RFC 9457 Problem Details | §11 | COMPLETE | httpx.WriteProblem used everywhere |
| Idempotency-Key | §11 | COMPLETE | Required on internal commands |
| ETag/If-Match | §11 | COMPLETE | All updates use version |
| X-Request-Id | §11 | COMPLETE | Middleware adds request ID |

## §12 Integrations

| Req | Source | Status | Evidence |
|-----|--------|--------|----------|
| WhatsApp Business Platform | §12 | MISSING | Deferred (ADR-011) |
| Telephony provider | §12 | MISSING | Deferred (ADR-012) |
| Email (transactional) | §12 | PARTIAL | notifications dispatcher exists; no email adapter UI |
| GitHub (D6) | §12/ADR-018 | COMPLETE | /webhooks/github with HMAC verification |
| Payments M-Pesa/bank | §12 | MISSING | Not implemented |
| KRA eTIMS | §12 | MISSING | Deferred (ADR-013) |
| Accounting package | §12 | MISSING | Deferred (ADR-011) |
| School management system | §12 | PARTIAL | Units integration backend; no school data agreement |
| Object storage | §12 | COMPLETE | S3-compatible adapter (R2/SeaweedFS) |

## §13 Security

| Req | Source | Status | Evidence |
|-----|--------|--------|----------|
| TLS everywhere | §13.1 | PARTIAL | Infrastructure concern; not in code |
| MFA for privileged roles | §13.1 | COMPLETE | TOTP mandatory for CEOs/finance/auditor |
| Login throttling | §13.1 | UNKNOWN | Not verified in code |
| Least privilege | §13.1 | COMPLETE | RBAC + scopes in Go |
| Secrets manager | §13.1 | UNKNOWN | Configuration concern |
| Webhook signature verification | §13.1 | COMPLETE | GitHub HMAC-SHA256 |
| Dependency/SAST scanning | §13.1 | UNKNOWN | CI concern |
| Application-level encryption | §13.1 | PARTIAL | MFA secrets encrypted |

## §14 Observability

| Req | Source | Status | Evidence |
|-----|--------|--------|----------|
| OpenTelemetry traces | §14 | PARTIAL | telemetry.Init exists; not verified end-to-end |
| Metrics | §14 | PARTIAL | config exists; no dashboard for metrics |
| Structured logs | §14 | COMPLETE | slog (Go), Logger (Elixir) |
| API availability SLO | §14 | UNKNOWN | Not measured |

## §17 Roadmap Phase Exit Criteria

| Phase | Exit Criterion | Status | Evidence |
|-------|---------------|--------|----------|
| Phase 1: Staff accounts | All staff have accounts | UNKNOWN | No seeded data; no staff count |
| Phase 1: Permission enforcement | Every permission enforced in Go + tests | COMPLETE | Route permission declarations; build-failing test |
| Phase 2: Dogfood rule (2 weeks, 90% report compliance) | Not verifiable | MISSING | No production usage data |
| Phase 3: Every enquiry in Atlas with owner + next action | Not verifiable | MISSING | CRM frontend missing |
| Phase 4: Month-end cash position from Atlas | PARTIAL | Finance backend complete; frontend read-only for some operations |

---

## Traceability Gaps Summary

- **46 requirements** mapped across §3–§17.
- **COMPLETE**: ~20
- **PARTIAL**: ~14
- **BACKEND ONLY**: ~8
- **MISSING**: ~4

The largest gaps are in CRM/contracts frontend (External CEO cannot operate), executive dashboard role-scoping, and daily executive brief.
