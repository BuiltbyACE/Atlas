# SCOS Role Completion Matrix

Status classifications: COMPLETE | PARTIAL | BACKEND ONLY | FRONTEND ONLY | PLACEHOLDER | MISSING | BLOCKED

Source: Repository state at commit e93f419, branch feat/web-redesign. Documentation baseline: docs/ATLAS.md v0.1, docs/PHASE2.md, docs/PHASE3.md.

## Capability Comparison

| Capability | Internal CEO | External CEO | Employee |
|------------|-------------|--------------|----------|
| Dashboard | PARTIAL | MISSING | MISSING |
| Navigation | PARTIAL | MISSING | PARTIAL |
| Core operations | PARTIAL | BACKEND ONLY | PARTIAL |
| Create/edit operations | PARTIAL | BACKEND ONLY | PARTIAL |
| Approvals | BACKEND ONLY | BACKEND ONLY | BACKEND ONLY |
| Notifications | PARTIAL | MISSING | PARTIAL |
| Reporting | PARTIAL | MISSING | MISSING |
| Permissions | COMPLETE (backend) | COMPLETE (backend) | COMPLETE (backend) |
| Full workflow | PARTIAL | MISSING | PARTIAL |

## Detailed Breakdown

### A. Executive and Organizational Control

| Capability | Internal CEO | External CEO | Employee | Evidence |
|------------|-------------|--------------|----------|----------|
| Executive dashboard | PARTIAL | MISSING | MISSING | Single shared /dashboard; no CEO-scoped commercial view |
| Company-wide operational overview | PARTIAL | MISSING | MISSING | Dashboard tiles + audit trail backend |
| Organizational structure | PARTIAL | MISSING | MISSING | People team-grid; departments backend |
| Departments and business units | PARTIAL | MISSING | MISSING | Units page exists; departments backend only |
| Employee directory | PARTIAL | MISSING | PARTIAL | /people team-grid.tsx |
| Reporting relationships | PARTIAL | MISSING | MISSING | person.line_manager_id schema; no UI |
| Role and permission management | BACKEND ONLY | MISSING | MISSING | core.role_grants; no UI page |
| Company policies | BACKEND ONLY | MISSING | MISSING | Settings API exists; no policy UI |
| Executive decisions | PARTIAL | PARTIAL | MISSING | Intel decisions page (/intel) |
| Approvals and escalations | BACKEND ONLY | BACKEND ONLY | BACKEND ONLY | Backend engine complete; no frontend UI |
| Audit trails | PARTIAL | MISSING | MISSING | Backend global list + per-record history; no frontend route |

### B. People and HR Management

| Capability | Internal CEO | External CEO | Employee | Evidence |
|------------|-------------|--------------|----------|----------|
| Employee onboarding | MISSING | MISSING | MISSING | Invite backend exists; no onboarding checklist UI |
| Employee accounts | BACKEND ONLY | MISSING | BACKEND ONLY | Auth service complete (login, MFA, sessions) |
| Employee profiles | PARTIAL | MISSING | PARTIAL | /people team-grid |
| Department assignments | BACKEND ONLY | MISSING | BACKEND ONLY | Schema exists; no UI |
| Job positions | BACKEND ONLY | MISSING | BACKEND ONLY | people.positions table; no UI |
| Role assignments | BACKEND ONLY | MISSING | MISSING | core.role_grants; no UI |
| Attendance | PARTIAL | MISSING | PARTIAL | Presence/session activity (/people/activity) |
| Leave requests and approvals | PARTIAL | MISSING | PARTIAL | Leave panel exists; backend decide flow complete |
| Employee performance | MISSING | MISSING | MISSING | people.reviews table; no UI |
| Employee task assignments | PARTIAL | MISSING | PARTIAL | Work tasks page exists |
| Employee offboarding | BACKEND ONLY | MISSING | MISSING | Offboard endpoint exists; no UI form |
| Account suspension/archival | BACKEND ONLY | MISSING | MISSING | Sessions revoke endpoints exist |
| Employee records/documents | BACKEND ONLY | MISSING | BACKEND ONLY | Files module exists |

### C. Engineering and Technical Operations

| Capability | Internal CEO | External CEO | Employee | Evidence |
|------------|-------------|--------------|----------|----------|
| Repository management | PARTIAL | MISSING | MISSING | Systems page; GitHub evidence integration |
| Project technical oversight | PARTIAL | MISSING | PARTIAL | Work projects page |
| Engineering task visibility | PARTIAL | MISSING | PARTIAL | Work tasks Kanban |
| Developer activity | MISSING | MISSING | MISSING | No GitHub PR/CI dashboard |
| Code review visibility | MISSING | MISSING | MISSING | Work reviews backend; no UI |
| Technical delivery | PARTIAL | MISSING | PARTIAL | Work module |
| Software releases | MISSING | MISSING | MISSING | No release management |
| Infrastructure inventory | PARTIAL | MISSING | MISSING | Systems page |
| System health monitoring | PARTIAL | MISSING | MISSING | Systems checks |
| Incidents and technical risks | PARTIAL | MISSING | MISSING | Systems incident page |
| Cybersecurity oversight | MISSING | MISSING | MISSING | No security dashboard |

### D. Internal Financial Oversight

| Capability | Internal CEO | External CEO | Employee | Evidence |
|------------|-------------|--------------|----------|----------|
| Expenses | COMPLETE | PARTIAL | PARTIAL | Frontend + backend write + approve/settle/reconcile |
| Revenue visibility | PARTIAL | PARTIAL | MISSING | Finance overview/receivables pages |
| Cash position | COMPLETE | PARTIAL | MISSING | getCashPosition() |
| Budgets | COMPLETE | PARTIAL | MISSING | Frontend + backend |
| Financial reports | PARTIAL | PARTIAL | MISSING | Project profitability exists |
| Approval authority | COMPLETE | PARTIAL | PARTIAL | Expense approval flow |
| Financial risks | MISSING | MISSING | MISSING | No dedicated financial risk view |
| Business-unit financial summaries | PARTIAL | MISSING | MISSING | Units finance summary backend exists |

### E. Management Intelligence

| Capability | Internal CEO | External CEO | Employee | Evidence |
|------------|-------------|--------------|----------|----------|
| Company performance | PARTIAL | MISSING | MISSING | Dashboard tiles |
| Department performance | MISSING | MISSING | MISSING | No department scorecard |
| Business-unit performance | PARTIAL | MISSING | MISSING | Units finance summaries |
| Operational indicators | PARTIAL | MISSING | MISSING | Systems availability, units indicators |
| Decisions | PARTIAL | PARTIAL | MISSING | Intel decisions page |
| Risks | PARTIAL | MISSING | MISSING | Intel risks page |
| Executive briefs | MISSING | MISSING | MISSING | Daily brief (07:30 EAT) not implemented |
| Company scorecard | MISSING | MISSING | MISSING | §18 scorecard not implemented |

### F. Sales and Customer Acquisition (External CEO Domain)

| Capability | Internal CEO | External CEO | Employee | Evidence |
|------------|-------------|--------------|----------|----------|
| Lead registration | MISSING | BACKEND ONLY | MISSING | CRM backend complete; no frontend forms |
| Prospect management | MISSING | BACKEND ONLY | MISSING | CRM organizations/contacts/leads |
| Sales pipeline | MISSING | BACKEND ONLY | MISSING | getPipeline() in atlas.ts; no pipeline board UI |
| Opportunity tracking | MISSING | BACKEND ONLY | MISSING | Contracts/opportunities backend; page.tsx shell only |
| Follow-up scheduling | MISSING | BACKEND ONLY | MISSING | Followups backend; no UI |
| Sales activities | MISSING | BACKEND ONLY | MISSING | Interactions backend; no UI |
| Client acquisition reporting | MISSING | MISSING | MISSING | No commercial dashboard |
| Conversion tracking | MISSING | MISSING | MISSING | Pipeline metrics not surfaced |
| Sales targets | MISSING | MISSING | MISSING | No target-setting UI |

### G. Client Relationship Management (External CEO Domain)

| Capability | Internal CEO | External CEO | Employee | Evidence |
|------------|-------------|--------------|----------|----------|
| Client registration | MISSING | BACKEND ONLY | MISSING | Organizations backend |
| Client profiles | MISSING | BACKEND ONLY | MISSING | Contacts backend |
| Contact management | MISSING | BACKEND ONLY | MISSING | No UI |
| Customer communication | MISSING | MISSING | MISSING | WhatsApp deferred (ADR-011) |
| Client interaction history | MISSING | BACKEND ONLY | MISSING | Interactions backend |
| Client segmentation | MISSING | MISSING | MISSING | No segmentation UI |
| Client relationship ownership | MISSING | BACKEND ONLY | MISSING | account owner field exists |
| Client issue escalation | MISSING | MISSING | MISSING | No client issue tracking |

### H. Commercial Operations (External CEO Domain)

| Capability | Internal CEO | External CEO | Employee | Evidence |
|------------|-------------|--------------|----------|----------|
| Proposal creation | MISSING | BACKEND ONLY | MISSING | Proposals backend; no UI |
| Quotations | MISSING | BACKEND ONLY | MISSING | Proposal line items backend |
| Pricing | MISSING | MISSING | MISSING | No pricing configuration UI |
| Negotiations | MISSING | MISSING | MISSING | No negotiation tracking UI |
| Contract preparation | MISSING | BACKEND ONLY | MISSING | Contracts backend |
| Contract signing/status | MISSING | BACKEND ONLY | MISSING | Contract lifecycle backend |
| Contract renewals | MISSING | MISSING | MISSING | Renewal reminders not implemented |
| Partnership management | MISSING | MISSING | MISSING | No partnership module |
| Revenue opportunities | MISSING | MISSING | MISSING | No revenue tracking UI |

### I. Customer Delivery Visibility (External CEO Domain)

| Capability | Internal CEO | External CEO | Employee | Evidence |
|------------|-------------|--------------|----------|----------|
| Active client projects | PARTIAL | PARTIAL | PARTIAL | Work projects page |
| Project progress | PARTIAL | PARTIAL | PARTIAL | Work tasks/board |
| Delivery milestones | PARTIAL | PARTIAL | PARTIAL | Milestones backend |
| Customer issues | MISSING | MISSING | MISSING | No customer issue module |
| Outstanding deliverables | MISSING | MISSING | MISSING | No deliverable tracking UI |
| Client satisfaction | MISSING | MISSING | MISSING | No satisfaction tracking |
| Delivery risks | PARTIAL | PARTIAL | PARTIAL | Blockers board |

### J. Commercial Intelligence (External CEO Domain)

| Capability | Internal CEO | External CEO | Employee | Evidence |
|------------|-------------|--------------|----------|----------|
| Revenue generated | MISSING | MISSING | MISSING | No commercial revenue dashboard |
| Sales pipeline value | MISSING | MISSING | MISSING | Computed backend; not surfaced |
| Conversion rates | MISSING | MISSING | MISSING | Not computed/surfaced |
| Proposal success rates | MISSING | MISSING | MISSING | Not tracked |
| Client acquisition | MISSING | MISSING | MISSING | No acquisition dashboard |
| Recurring revenue | MISSING | MISSING | MISSING | Recurring costs exist (finance) but not revenue |
| Outstanding receivables | PARTIAL | PARTIAL | MISSING | Finance receivables ageing page |
| Commercial performance | MISSING | MISSING | MISSING | No commercial dashboard |

### K. Employee Workspace

| Capability | Internal CEO | External CEO | Employee | Evidence |
|------------|-------------|--------------|----------|----------|
| Employee dashboard | MISSING | MISSING | MISSING | No role-specific employee dashboard |
| Personal profile | PARTIAL | MISSING | PARTIAL | /people team-grid |
| Assigned department | BACKEND ONLY | MISSING | BACKEND ONLY | department_id on person; no UI |
| Reporting manager | BACKEND ONLY | MISSING | BACKEND ONLY | line_manager_id; no UI |
| Assigned responsibilities | PARTIAL | MISSING | PARTIAL | Work tasks assigned to person |
| Personal notifications | PARTIAL | PARTIAL | PARTIAL | Notifications backend; frontend route not found |
| Company announcements | MISSING | MISSING | MISSING | No announcements module |

### L. Work Execution (Employee Domain)

| Capability | Internal CEO | External CEO | Employee | Evidence |
|------------|-------------|--------------|----------|----------|
| Assigned projects | PARTIAL | MISSING | PARTIAL | Work projects page |
| Assigned tasks | PARTIAL | MISSING | PARTIAL | Work tasks Kanban + detail |
| Task details | PARTIAL | MISSING | PARTIAL | Work task page |
| Task status updates | PARTIAL | MISSING | PARTIAL | Work transitions |
| Deadlines | PARTIAL | MISSING | PARTIAL | Task due_at |
| Subtasks | MISSING | MISSING | MISSING | task_dependencies exist; no subtask UI |
| Deliverable submission | PARTIAL | MISSING | PARTIAL | Evidence upload |
| Work progress | PARTIAL | MISSING | PARTIAL | Project board |
| Blocker reporting | PARTIAL | MISSING | PARTIAL | Blockers board |
| Task history | PARTIAL | MISSING | PARTIAL | Audit trail exists |

### M. Employee Administration

| Capability | Internal CEO | External CEO | Employee | Evidence |
|------------|-------------|--------------|----------|----------|
| Attendance | PARTIAL | MISSING | PARTIAL | Presence view |
| Leave requests | PARTIAL | MISSING | PARTIAL | Leave panel exists |
| Leave history | PARTIAL | MISSING | PARTIAL | Leave list backend |
| Approval status | PARTIAL | MISSING | PARTIAL | Leave decision status |
| Company documents | BACKEND ONLY | MISSING | BACKEND ONLY | Files module; no UI |
| Asset assignments | MISSING | MISSING | MISSING | No asset module |
| HR requests | MISSING | MISSING | MISSING | No HR request module |
| Employee support tickets | MISSING | MISSING | MISSING | No support module |

### N. Internal Communication

| Capability | Internal CEO | External CEO | Employee | Evidence |
|------------|-------------|--------------|----------|----------|
| Announcements | MISSING | MISSING | MISSING | No announcements module |
| Internal messages | MISSING | MISSING | MISSING | No messaging module |
| Direct communication | MISSING | MISSING | MISSING | No direct messaging |
| Notifications | PARTIAL | PARTIAL | PARTIAL | In-app notifications backend |
| Message history | MISSING | MISSING | MISSING | No message history UI |
| Attachments | BACKEND ONLY | BACKEND ONLY | BACKEND ONLY | Files module |

### O. Developer-specific Experience

| Capability | Internal CEO | External CEO | Employee | Evidence |
|------------|-------------|--------------|----------|----------|
| Connect GitHub identity | MISSING | MISSING | MISSING | No UI to connect GitHub identity |
| View assigned repositories | MISSING | MISSING | MISSING | No repository view |
| Link code activity to tasks | PARTIAL | MISSING | PARTIAL | GitHub evidence links PR to task (backend) |
| View pull requests | MISSING | MISSING | MISSING | No PR UI |
| View review feedback | MISSING | MISSING | MISSING | Work reviews backend; no UI |
| See CI/CD outcomes | MISSING | MISSING | MISSING | No CI/CD integration |

### P. Employee Dashboard Awareness

| Capability | Internal CEO | External CEO | Employee | Evidence |
|------------|-------------|--------------|----------|----------|
| What to do today | PARTIAL | MISSING | PARTIAL | Work tasks page |
| Overdue work | PARTIAL | MISSING | PARTIAL | Overdue tasks visible |
| Approaching deadline | PARTIAL | MISSING | PARTIAL | Task due dates |
| Requires attention | PARTIAL | MISSING | PARTIAL | Blockers/escalations |
| Completed work | PARTIAL | MISSING | PARTIAL | Task status |
| Feedback/approvals awaited | PARTIAL | MISSING | PARTIAL | Reviews backend |

---

## Summary Counts

| Status | Internal CEO | External CEO | Employee |
|--------|-------------|--------------|----------|
| COMPLETE | ~6 | ~0 | ~0 |
| PARTIAL | ~18 | ~4 | ~20 |
| BACKEND ONLY | ~10 | ~14 | ~8 |
| MISSING | ~12 | ~22 | ~18 |
| FRONTEND ONLY | 0 | 0 | 0 |
| PLACEHOLDER | 0 | 0 | 0 |
| BLOCKED | 0 | 0 | 0 |

*Note: Counts are approximate; matrix items are aggregated from the detailed tables above.*
