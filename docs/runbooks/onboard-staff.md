# Onboard Staff Runbook

**Source:** ATLAS.md §8.5 (People & Accountability), §9.6 (Onboarding / Off-boarding)  
**Owner:** Internal CEO (HR lead), Both CEOs (hiring approval)  
**Status:** Active  
**Last reviewed:** 2026-11-01

---

## Purpose

This runbook defines how to onboard a new person into Atlas: invite them, create their account, set up their role and permissions, assign equipment and access, and complete their onboarding checklist.

---

## Prerequisites

- [ ] **Hiring approved** — both CEOs have approved the hire (Atlas approval, `core.approvals`, status = approved)
- [ ] **Role determined** — which role key(s) the person will hold (see role list below)
- [ ] **Scope determined** — company-wide, business unit, department, project, or own scope
- [ ] **Line manager identified** — who will manage this person
- [ ] **Department assigned** — which department they belong to

---

## Role Keys (from ATLAS.md §3.2)

| Role Key | Description | Typical Users |
|---|---|---|
| `ceo_commercial` | Commercial CEO | Commercial CEO |
| `ceo_internal` | Internal & Technical CEO | Internal CEO |
| `finance_officer` | Records expenses, invoices, payments | Finance staff |
| `sales_rep` | Leads and interactions they own or are assigned | Sales team |
| `project_lead` | Manages assigned projects, tasks, reviews | Project managers |
| `engineer` | Assigned tasks, reviews, daily reports, own expenses | Engineering staff |
| `staff` | Generic staff: assigned tasks, daily reports, leave, own expenses | General staff |
| `unit_manager` | Business-unit manager | Unit (e.g. school) managers |
| `auditor` | Read-only access to audit log and finance | External accountant |
| `system_admin` | Technical administration | IT/systems staff |

A person can hold **several roles**. Roles grant permissions within a scope.

---

## Onboarding Workflow

```
1. Hire approved (both CEOs)
   ↓
2. Person record created (people module)
   ↓
3. Account invited (auth module)
   ↓
4. Person accepts invite (sets password, MFA)
   ↓
5. Roles and grants assigned (authz module)
   ↓
6. Equipment and access provisioned
   ↓
7. Onboarding checklist completed
   ↓
8. First assignments made
   ↓
9. Probation review scheduled
```

---

## Step 1: Hire Approval

Before creating anything in Atlas, the hire must be approved:

1. **Create approval request** — `POST /api/v1/approvals` with:
   - `entity_type`: `hiring`
   - `entity_id`: [person ID or placeholder]
   - `amount_minor`: 0 (or salary amount if applicable)
   - `approver_role`: `both_ceos`
   - `approvals_required`: 2
2. **Both CEOs approve** — through the Approvals page (`/approvals`)
3. **Approval recorded** — `core.approval_decisions` has two distinct approver rows

> Nobody approves their own request. If one CEO is hiring themselves, the other CEO plus a third party must approve.

---

## Step 2: Create Person Record

Create the person in the people module:

| Field | Value |
|---|---|
| `full_name` | Full legal name |
| `work_email` | Work email address |
| `phone_e164` | Phone number (E.164 format) |
| `whatsapp_e164` | WhatsApp number (if applicable) |
| `employment_type` | `full_time` \| `part_time` \| `contractor` \| `intern` \| `external` |
| `department_id` | Department UUID |
| `line_manager_id` | Line manager's person UUID |
| `status` | `onboarding` (initially) |
| `start_date` | First day of employment |

**Permission required:** `people.person.manage`

---

## Step 3: Create User Account

The person record links to a `core.user_account`:

1. **Send invitation** — `POST /api/v1/auth/invites` with:
   - `email`: person's work email
   - `full_name`: person's full name
   - `employment`: employment type
   - `grants`: array of `{role_key, scope_type, scope_id}`
   - `invited_by`: inviting person's UUID
   - `ttl`: invitation validity (default 7 days)

2. **Person receives invite email** — contains registration link
3. **Person registers** — sets password, completes MFA setup (mandatory for privileged roles: `ceo_internal`, `ceo_commercial`, `finance_officer`, `system_admin`, `auditor`)
4. **Account activated** — status changes from `invited` to `active`

**Permission required:** `core.user.manage` (both CEOs, system_admin)

---

## Step 4: Assign Roles and Grants

Assign the person's roles and scopes:

| Role | Scope Type | Scope ID | Example |
|---|---|---|---|
| `engineer` | `company` | — | All company projects |
| `project_lead` | `project` | `proj-123` | Specific project only |
| `sales_rep` | `company` | — | All leads/contacts |
| `unit_manager` | `business_unit` | `unit-456` | School unit only |

Use `core.role_grants` table:
```sql
INSERT INTO core.role_grants (id, user_id, role_key, scope_type, scope_id, granted_by)
VALUES (gen_random_uuid(), 'person-uuid', 'engineer', 'company', NULL, 'ceo-internal-uuid');
```

**Permission required:** `core.role.manage`

---

## Step 5: Equipment and Access Provision

| Item | Provisioned By | Notes |
|---|---|---|
| Laptop / workstation | IT / system_admin | Asset tag recorded |
| Email account | IT / system_admin | Work email configured |
| Atlas account | Created in Step 3 | MFA enabled |
| GitHub / repo access | IT / system_admin | Based on role (engineer needs write access) |
| Finance system access | finance_officer | If role involves expenses |
| CRM access | Commercial CEO | If role is sales_rep |
| Systems access | Internal CEO | If role involves systems monitoring |

Record equipment in the person's onboarding checklist.

---

## Step 6: Onboarding Checklist

Create and track an onboarding checklist in Atlas:

| Checklist Item | Owner | Due Date | Status |
|---|---|---|---|
| Equipment received | Person | Day 1 | ☐ |
| Account access verified | Person | Day 1 | ☐ |
| MFA configured | Person | Day 1 | ☐ |
| Role and permissions verified | Line manager | Day 1 | ☐ |
| Team introductions | Line manager | Day 2 | ☐ |
| Project assignments | Line manager | Day 3 | ☐ |
| First task assigned | Line manager | Day 3 | ☐ |
| First daily report filed | Person | Day 3 | ☐ |
| Policy acknowledgment | Person | Day 5 | ☐ |
| Probation review scheduled | Line manager | Day 30 | ☐ |

Each checklist item becomes an Atlas task or is tracked in the onboarding checklist entity.

---

## Step 7: First Assignments

Assign the person's first work:

1. **Create project membership** — if they're on a project
2. **Create tasks** — initial tasks with owner = new person
3. **Set up daily reports** — they will receive prompts at 16:30 EAT on working days
4. **Configure leave availability** — they can request leave from day one

---

## Step 8: Probation Review

Schedule a probation review (typically at 30, 60, or 90 days):

1. **Create review record** — `people.reviews` with:
   - `person_id`: new person's UUID
   - `reviewer_id`: line manager's UUID
   - `period_start`: start date
   - `period_end`: start date + probation period
   - `rating`: 1–5 (filled in at review)
   - `status`: `draft`

2. **Schedule review meeting** — line manager meets with person
3. **Complete review** — fill in ratings, comments, outcome
4. **Set status to `approved`** — both CEOs can approve reviews

---

## Off-Boarding Cross-Reference

When the person eventually leaves, see the off-boarding procedure:
- Deactivate account (revoke sessions and tokens)
- Reassign open tasks, leads, and follow-ups
- List company assets/credentials to recover
- Mark person status as `exited`

See the off-board form in the people module (`/people/[id]/offboard`).

---

## Audit Trail

Every onboarding step writes to `audit.events`:
- Person created — `people.person.created`
- Account invited — `core.user.invited`
- Account activated — `core.user.activated`
- Roles granted — `core.role.manage` (audited via authz)
- Equipment assigned — tracked in onboarding checklist

---

## Related Documents

- `docs/policies/accountability-policy.md` — staff accountability boundaries
- `docs/policies/data-retention.md` — ex-staff data minimization
- `docs/ATLAS.md §3.2` — roles and permissions
- `docs/ATLAS.md §6.1` — identity and authentication (invites, sessions, MFA)
- `docs/ATLAS.md §8.5` — people & accountability module
- `docs/ATLAS.md §9.6` — onboarding/off-boarding workflow
- `docs/runbooks/offboard.md` — off-boarding procedure (to be created)
