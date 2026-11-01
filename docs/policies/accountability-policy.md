# Accountability Policy

**Source:** ATLAS.md §8.5 (People & Accountability), §1.3 (Success definition), §20 (Risk: staff resistance)  
**Owner:** Internal & Technical CEO  
**Status:** Active  
**Last reviewed:** 2026-11-01  
**Next review:** 2027-05-01

---

## Purpose

Atlas measures **assigned outputs and evidence, not keystrokes or screen time**. This policy makes that principle explicit so staff understand what Atlas records about their work and why, preventing the system from being perceived as workplace surveillance.

---

## Core Principle: Accountability, Not Surveillance

| We Measure | We Do NOT Measure |
|---|---|
| Assigned tasks and their status | Keystrokes or screen time |
| Evidence of completion (PR, document, test result, client approval) | Active/inactive window focus |
| Daily report submissions (work done, blockers, next actions) | Webcam, microphone, or screen recordings |
| Time logged against tasks (for client_delivery projects) | Mouse movement or click patterns |
| Leave requests and approved absences | Personal communications or browsing |
| Escalations raised and resolved | Location tracking outside work hours |

---

## Daily Report Cycle

**Prompt:** 16:30 EAT on working days, for everyone on a working day who is not on approved leave  
**Deadline:** 09:00 EAT the next working day  
**Working week:** Monday–Friday

| Event | Timing | Consequence |
|---|---|---|
| Prompt sent (in-app + WhatsApp) | 16:30 EAT | Report is due |
| Late filing recorded | After 09:00 next working day | Status = `late`, line manager notified |
| Missed (not filed by deadline) | After 09:00 next working day | Status = `missed`, line manager notified |
| Three misses in ten days | Rolling 10-day window | Escalates to L2 (Domain CEO) |

A report collected via WhatsApp is a **draft** until confirmed (one-tap confirmation link or "Reply YES to confirm"), then stored as submitted.

Report items are **pre-filled** from the person's active tasks — the person updates rather than writes from scratch. Expenses reported in a daily report create **draft expenses** in Finance (one entry point, one record).

---

## Leave and Absence

- Leave requests require `people.leave.request` permission (employee initiates, manager decides)
- Approved leave excuses daily reports for those dates
- Off-boarding deactivates accounts, revokes sessions/tokens, reassigns open tasks/leads/follow-ups, and lists company assets/credentials to recover
- A person is not marked "exited" until access revocation items are checked (blocking off-boarding)

---

## Performance Reviews

| Field | Purpose |
|---|---|
| Person (reviewee) | Who is being reviewed |
| Reviewer | Who is conducting the review |
| Period start/end | Review period boundaries |
| Rating (1–5) | Performance rating |
| Comments | Free-form performance feedback |
| Status (draft/submitted/approved/rejected) | Review lifecycle |

Reviews are created and managed by `ceo_internal` only (holds `people.review.read` and `people.review.manage`). This is a management tool for performance conversations, not a surveillance mechanism.

---

## Staff Rights and Transparency

1. **Right to see their own data** — Every staff member can view their own tasks, reports, leave, time entries, and reviews at any time
2. **Right to contest** — A staff member may dispute a task assignment, blocker, or review through their line manager
3. **Right to privacy** — Atlas does not record personal communications, browsing, or location. Work-related data only
4. **Right to know why** — Every escalation, overdue flag, or missed report has a recorded reason visible to the staff member

---

## Manager Responsibilities

Managers (project leads, CEOs) must:

- Assign clear owners and next actions for every task
- Review and act on escalations within the escalation window (L1: 24h, L2: 48h, L3: both CEOs)
- Provide timely feedback on performance reviews
- Not use Atlas data for surveillance outside work-related outputs
- Escalate blockers flagged "needs management" directly to L2

---

## Change Control

Changes to this policy require:
1. Joint approval by both CEOs
2. Staff communication before implementation
3. An audit entry recording the change

---

## Related Documents

- `docs/policies/approval-matrix.md` — approval thresholds and four-eyes rules
- `docs/policies/data-retention.md` — how long records are kept
- `docs/ATLAS.md §8.5` — people & accountability module specification
- `docs/ATLAS.md §1.3` — success definition
- `docs/ATLAS.md §20` — risk register (staff resistance to surveillance)
