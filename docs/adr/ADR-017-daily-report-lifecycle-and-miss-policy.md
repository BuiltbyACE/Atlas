# ADR-017 — Daily-report lifecycle, prompt/deadline times and repeated-miss policy (D5)

| | |
|---|---|
| **Status** | Accepted |
| **Date** | 2026-10-17 |
| **Deciders** | Internal CEO (D5 of §21) |
| **Phase** | Needed by Phase 2 (§21); implemented in Phase 2 |
| **Supersedes / Superseded by** | Decides D5 of §21 and replaces the illustrative `[CONFIG]` values of §8.5 |

## Context

Decision **D5** (§21, owned by the Internal CEO) covers the daily-report rhythm:
when a report is **prompted**, when it falls **due**, which days are working
days, and what happens when a report is **not filed**. `docs/ATLAS.md` §8.5
already carried illustrative values as `[CONFIG]` — 16:30 EAT prompt, 09:00 the
next working day, three misses in ten days → L2 — and §9.2 sketches the
sequence, but none of it was enforced:

1. The lifecycle existed in name only. `people.daily_reports.status` allowed
   `draft`, `submitted`, `reviewed` (migration `00011`); there was no `due`,
   `late`, `missed` or `excused`, and no timestamp for when a report became due
   or fell missed. Nothing opened a report, decided lateness, or counted misses.
2. §9.2 shows *Elixir* (`DailyRhythm`) driving the rhythm with Oban timers and
   calling Go internal commands (`GET people due today`, `mark missing reports as
   missed`) that do not exist. This is the same gap ADR-016 records for the
   escalation ladder: the accepted deviation (`docs/PHASE2.md` §9) puts
   time-driven work in the **Go worker sweep**, mirroring the approvals expiry
   sweeper.
3. Notification fan-out (§6.4) is a Phase 7 item (ADR-016, `docs/PHASE2.md` §5),
   yet D5's own rule — "line manager notified" — is a notification.

D5 blocks the daily-report compliance metric (§8.5), the escalation of repeated
misses, and the §17 exit criterion (≥ 90% daily-report compliance), so the
rhythm had to be decided and enforced for Phase 2 to be usable.

## Decision

**Rhythm (implemented).**

| Setting | Key | Default | Meaning |
|---|---|---|---|
| Prompt | `people.daily_report.prompt_minute` | 990 (16:30 EAT) | a working day's report is opened and its prompt fires |
| Deadline | `people.daily_report.deadline_minute` | 540 (09:00 EAT) | the next working day; later filings are **late**, an unfiled report is **missed** |
| Working days | `people.daily_report.working_days` | `1,2,3,4,5` | ISO weekdays (Mon–Fri) that count |
| Miss threshold | `people.daily_report.missed_threshold` | 3 | misses inside the window that escalate a person |
| Miss window | `people.daily_report.missed_window_days` | 10 | days the miss threshold counts over |

- A report is **prompted and opened** for everyone **active** on a working day
  who is **not on approved leave** covering that day (§8.5). The prompt instant
  is the report's `due_at`.
- The report falls **due at 09:00 the next working day** — Friday's report is due
  Monday. `missed_at` is that instant.
- Three misses inside ten days escalate the person to **L2** — the
  `escalation.l2_role` holder (default `ceo_internal`), the same rung and the
  same `work.escalations` table as ADR-016, keyed by `entity_type = 'person'` so
  one queue serves both. The escalation resolves automatically when the person's
  count falls back below the threshold (misses filed late, or aged out of the
  window).

**Where it is enforced.** The **atlas-worker daily-report sweep** is the only
place the policy is applied (`docs/PHASE2.md` §5). The submit path never decides
whether a filing was on time: any date is still accepted, and the sweep makes
that call. It borrows only the *day boundary* — the calendar date
`people.daily_reports.report_date` uses is the policy zone's date
(`company.timezone`), computed by the one `policy.Today`, so a report filed at
01:00 local lands on the day the sweep will look for rather than the previous
UTC day. (`dateOnly` normalises to UTC first and so is deliberately not used for
this: doing so put the boundary at 00:00 UTC and let a near-midnight rhythm open
the wrong day.) Each pass is idempotent and bounded by the miss window, so a
worker that was down catches up the days it slept through and a second pass in
the same minute does nothing:

1. **Open** — on a working day, once the prompt minute is reached, open today's
   report for everyone who owes one (`due`, `due_at` = prompt instant) and emit
   `people.daily_report.due.v1`.
2. **Excuse** — a due or missed report for someone on approved leave covering
   the day becomes `excused` and never counts as a miss.
3. **Miss** — a report still `due` after the deadline becomes `missed`
   (`missed_at` = deadline), emitting `people.daily_report.missed.v1` (the event
   carries the line manager id) and, best-effort after commit, a line-manager
   notice through the notifications engine (§6.4).
4. **Late** — a `submitted` report whose `submitted_at` is past the deadline
   becomes `late`. A late filing still counts as filed: it never becomes a miss,
   and if the day had already been marked `missed` the row is recovered when the
   report is submitted (the upsert clears `missed_at`).
5. **Escalate / resolve** — count misses per person since the window opened;
   raise one L2 per breaching person, and resolve the open L2 of anyone no
   longer breaching.

- The times, the working week and the miss rule are **typed settings** (§6.7),
  read on every sweep pass, so the CEO retunes the rhythm through the audited
  settings API without a deploy. No new table: five values do not justify one,
  and `core.settings` already carries the change history.
- "Working day" is defined by the `working_days` set, not by a holiday calendar;
  public holidays are not modelled yet (see Consequences).
- The lifecycle is a **state machine** on `status`: `due → missed` (deadline, no
  filing), `due → excused` (leave), `submitted → late` (filed after the
  deadline), `* → submitted` (filing), `* → excused` (leave). Migration `00017`
  widens the check constraint, adds `due_at`/`missed_at` and the two indexes the
  sweep reads (`(status, report_date)` and a partial index on
  `(reporter_person_id, report_date) WHERE status = 'missed'`).

## Consequences

### Positive
- D5 is decided; the daily-report rhythm is enforced rather than illustrative, so
  the compliance metric, the repeated-miss escalation and the §17 exit criterion
  have a real basis.
- Retuning the prompt, the deadline, the working week or the miss rule is an
  audited settings write, not a deploy. Normalisation keeps a misconfigured
  policy sane (a bad time falls back to the default, an empty working week to
  Mon–Fri).
- The submit path stays fast and free of the clock (§4.1), and the sweep is
  idempotent and catches up after downtime — one clock, one set of state
  transitions, audited as the system.
- The repeated-miss escalation reuses the work ladder and the one escalations
  table instead of a second mechanism, so the CEO's shared queue stays singular.

### Negative
- **Public holidays are not modelled.** A public holiday is a working day to the
  sweep, so reports are expected and a miss is recorded. The mitigation is
  operational: approved leave excuses the day, and the `working_days` setting can
  be narrowed. A holiday calendar is a follow-up.
- **The line-manager notice is on by default** while the rest of notification
  fan-out waits for Phase 7 (ADR-016). It is best-effort and idempotent per
  report (`people.daily_report.missed:<report_id>`), and the durable record is
  the missed row plus its event, so a notification failure never fails the
  sweep.
- A person with **no line manager** has no one to notify; the miss still counts
  toward escalation. The org chart (§10.1) grows into this.
- The reporter's own 16:30 **prompt** is a notification too, and it is *not* sent
  by this work: the sweep opens the report and emits `people.daily_report.due.v1`
  for the Phase 7 fan-out (or a client) to turn into a prompt. Until then the
  web app's `GET /api/v1/daily-reports/today` is how a person sees the day's
  report.

### Neutral
- The cut-over reinterprets the old `draft`/`submitted` rows honestly: the sweep
  never rewrites an already-filed day, so historical reports are untouched.
- The day boundary follows the policy zone (`company.timezone`), as `policy.Today`
  always claimed but did not do: it had reused `dateOnly`, which normalises to UTC
  first, so the boundary was midnight UTC and a rhythm set near midnight would
  have opened the wrong day. Both the API's `/today` answer and the sweep now use
  the same `policy.Today`.
- The daily-report sweep is a third time-driven job in `atlas-worker`, configured
  like the others (`ATLAS_DAILY_REPORT_SWEEP_INTERVAL`/`_BATCH`).
- `_ "time/tzdata"` is linked into both commands: the distroless image ships no
  zoneinfo, so without it the rhythm and notification quiet hours silently fall
  back to UTC. (This fixes a latent bug beyond D5.)

## Alternatives considered

| Option | Trade-offs |
|---|---|
| **Elixir Oban timers + Go internal commands** (§9.2 literal) | Faithful to the sequence diagram, but the internal command API does not exist and the Go worker sweep is already accepted for approvals and the ladder (ADR-016). Moving the clock later changes no data or events. |
| **Enforce the deadline in the submit path** | Rejects or relabels a late submission at write time, but needs the policy and the timezone in the request path, makes the write path slower and clock-dependent (§4.1), and cannot mark a report missed — nothing runs when nobody submits. The sweep is the only place that can see silence. |
| **A `people.daily_report_policies` table** | Faithful to a policy-table reading, but adds a table, CRUD and a change workflow for five values. Typed settings already provide versioning, validation and auditing (§6.7). |
| **A public-holiday calendar now** | More correct than a weekday set, but there is no holiday source yet and it grows the decision; the working-day set plus leave covers the common case, and the gap is recorded here. |
| **Escalate per missed report (L1 each)** | Every miss visibly escalates, but a person who is briefly away floods the CEO queue; a threshold over a window is what §8.5 asks and what keeps the queue meaningful. |
| **Treat a late filing as missed** | Simpler counting, but it punishes filing at 09:05 the same as not filing, which does not match "compliance" and would make the metric unfair. Late is recorded distinctly. |

## References

- `docs/ATLAS.md` §3.4 (escalation ladder), §4.1 (write path), §6.4
  (notifications), §6.6 (concurrency/audit), §6.7 (configuration), §8.5 (people
  module rules), §9.2 (daily report cycle), §11 (SLOs), §17 (Phase 2 exit), §21
  (D5)
- `docs/PHASE2.md` §4 (people module), §5 (worker sweeps), §9 (deviations)
- ADR-002 (Go writes, Elixir orchestrates), ADR-009 (signals, not data), ADR-010
  (UTC storage, Nairobi display), ADR-016 (escalation ladder and L2 role)
- `docs/TODO.md` — reporter prompt fan-out and the holiday calendar deferred
