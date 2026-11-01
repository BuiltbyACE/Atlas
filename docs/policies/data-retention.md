# Data Retention Policy

**Source:** ATLAS.md §10.4 — Data lifecycle [CONFIG / DECIDE with legal advice]  
**Owner:** Both CEOs (with legal advice)  
**Status:** Active  
**Last reviewed:** 2026-11-01  
**Next review:** 2027-11-01 (annual, or upon legal advice change)

---

## Purpose

This policy defines how long Atlas retains each category of data, balancing operational needs, statutory requirements, and privacy obligations under the Kenya Data Protection Act, 2019.

---

## Default Retention Schedule

| Data Category | Default Retention | Notes |
|---|---|---|
| **Audit events** | 7 years | Financial relevance; append-only, no UPDATE/DELETE |
| **Financial records** | Per statutory requirement | Confirm with accountant; tax invoices, payments, budgets |
| **Check results** | Raw: 90 days; daily aggregates: indefinitely | Partitioned by month; raw results aged out |
| **WhatsApp message content** | Business-relevant only; content purged after [CONFIG] | Metadata kept; only messages with consent retained |
| **Call recordings** | Only with consent; shortest practical period | Requires announced consent; not stored by default |
| **Daily reports** | Duration of employment + [CONFIG] | Submitted reports tied to employment record |
| **Ex-staff personal data** | Minimised after exit per policy | Access revoked, personal data reduced to minimum |
| **Incident records** | 7 years | SEV1/SEV2 post-incident reviews retained for reference |
| **Contracts** | Duration of contract + 7 years | Signed contracts and change requests |
| **Invoices** | Per statutory requirement | Tax compliance; confirm with accountant |
| **Expense records** | Per statutory requirement | Reimbursement records; confirm with accountant |
| **Time entries** | Duration of employment + [CONFIG] | Timesheet data for client_delivery projects |
| **Outbox events** | 90 days active; archive to `outbox.events_archive` | Older events archived, not deleted |
| **Notifications** | 1 year | Delivery attempts and status |
| **Sessions** | Session absolute TTL (configurable) | Revoked on logout, deactivation, or expiry |
| **Approval records** | 7 years | Approval history and decisions retained |

[CONFIG] values are stored in `core.settings` and may only be changed jointly by both CEOs.

---

## Data Minimisation Principles

| Principle | Application in Atlas |
|---|---|
| **Collect only what is necessary** | No screen recording, keylogging, or personal browsing data |
| **Store only for as long as needed** | Retention schedule above; automated cleanup jobs |
| **Aggregate where possible** | School indicators stored as aggregates, not student-level data |
| **Purge on exit** | Ex-staff personal data minimised after exit |
| **Consent-based retention** | WhatsApp content and call recordings only with consent |

---

## Special Categories

### WhatsApp and Communications
- Only business-relevant messages are retained
- Content purged after [CONFIG] days; metadata (sender, timestamp, direction) kept longer
- Free-form replies limited to the customer-service window after contact last messaged
- Opt-in/consent recorded per contact

### School Management System Data
- Aggregates and indicators only — not student-level personal data
- Unless a specific, approved need exists (documented data-sharing agreement)
- Unit managers see their unit; CEOs see all

### Financial and Tax Records
- Retained per statutory requirement — confirm retention periods with the accountant
- Export to statutory accounting package may be used instead of long-term Atlas storage [DECIDE]

### Audit Events
- **Append-only** — database triggers reject UPDATE and DELETE
- Only `atlas_api`/`atlas_worker` may INSERT
- 7-year retention minimum for financial relevance

---

## Backup and Recovery

| Backup Type | Frequency | Retention | Location |
|---|---|---|---|
| Full backup | Nightly | 30 days | Off-site, encrypted |
| WAL archiving | Continuous | 7 days | Point-in-time recovery |
| Restore drill | Monthly | N/A | Recorded in Atlas as task with evidence |

Backups are encrypted and stored off-site. Restore drills are conducted monthly and recorded as tasks in Atlas with evidence.

---

## Data Subject Rights (Kenya DPA 2019)

| Right | Atlas Process |
|---|---|
| **Access** | Export personal data upon request |
| **Correction** | Update inaccurate personal data |
| **Deletion (erasure)** | Delete personal data where no longer necessary |
| **Objection** | Object to processing; assessment by both CEOs |
| **Portability** | Provide data in machine-readable format |

Data-subject requests are handled through a documented export/erasure procedure. Requests are logged and tracked to completion.

---

## Change Control

Changes to this policy require:
1. Joint approval by both CEOs
2. Legal advice review (Kenya Data Protection Act compliance)
3. Effective date range and [CONFIG] value updates in `core.settings`
4. An audit entry recording old vs new values
5. Staff communication before implementation changes affect them

---

## Related Documents

- `docs/policies/approval-matrix.md` — approval thresholds
- `docs/policies/accountability-policy.md` — staff accountability boundaries
- `docs/ATLAS.md §10.4` — architectural data lifecycle definition
- `docs/ATLAS.md §13.2` — privacy and Kenya Data Protection Act compliance
- `docs/ATLAS.md §13.3` — threat model highlights
