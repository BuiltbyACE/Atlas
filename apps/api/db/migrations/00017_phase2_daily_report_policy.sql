-- +goose Up
-- +goose StatementBegin
-- Phase 2: daily-report lifecycle (decision D5, docs/adr/ADR-017).
--
-- §8.5 models a daily report as due -> submitted/late/missed (or excused), but
-- migration 00011 shipped only draft/submitted/reviewed. This widens the status
-- set and records when a report became due and when it was missed, so the
-- atlas-worker sweep can walk the lifecycle at the prompt and deadline times.
--
-- The policy values themselves (16:30 prompt, 09:00 next-working-day deadline,
-- Monday-Friday, three misses in ten days -> L2) are typed settings in the
-- settings catalogue, so they are retuned through the audited settings API and
-- never by a migration.
ALTER TABLE people.daily_reports DROP CONSTRAINT daily_reports_status_check;
ALTER TABLE people.daily_reports ADD CONSTRAINT daily_reports_status_check
  CHECK (status IN ('due','draft','submitted','late','missed','excused','reviewed'));

ALTER TABLE people.daily_reports
  ADD COLUMN due_at timestamptz,
  ADD COLUMN missed_at timestamptz;

-- The sweep opens a day's due rows and, later, counts a person's misses in the
-- window: both are keyed on report_date (+ status) and reporter.
CREATE INDEX idx_people_daily_reports_status_date ON people.daily_reports (status, report_date);
CREATE INDEX idx_people_daily_reports_missed ON people.daily_reports (reporter_person_id, report_date) WHERE status = 'missed';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS people.idx_people_daily_reports_missed;
DROP INDEX IF EXISTS people.idx_people_daily_reports_status_date;
ALTER TABLE people.daily_reports DROP COLUMN IF EXISTS missed_at;
ALTER TABLE people.daily_reports DROP COLUMN IF EXISTS due_at;
ALTER TABLE people.daily_reports DROP CONSTRAINT daily_reports_status_check;
ALTER TABLE people.daily_reports ADD CONSTRAINT daily_reports_status_check
  CHECK (status IN ('draft','submitted','reviewed'));
-- +goose StatementEnd
