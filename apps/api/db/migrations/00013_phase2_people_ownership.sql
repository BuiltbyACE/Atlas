-- +goose Up
-- +goose StatementBegin
-- Phase 2 correction: daily reports and leave are the *people* module's data,
-- not work's. §7.4 catalogues people.daily_report.submitted.v1 / people.leave.*,
-- §8.5 (People & accountability) lists daily_report and leave_request as people
-- entities, and §10.1 maps those tables to the `people` schema. Migration 00011
-- created them under `work`; this moves them home, renames the entity to match
-- the blueprint, and gives them a people-owned updated_at trigger.
--
-- No data is lost: rows, ids and foreign keys are untouched; only the schema,
-- the leave table name and the leave_type column name change.

ALTER TABLE work.daily_reports SET SCHEMA people;
ALTER TABLE work.absences SET SCHEMA people;

ALTER TABLE people.absences RENAME TO leave_requests;
ALTER TABLE people.leave_requests RENAME COLUMN absence_type TO leave_type;

-- Indexes and constraints move with their table but keep their old names; rename
-- the indexes so `idx_work_*` does not linger on people tables.
ALTER INDEX people.idx_work_daily_reports_date RENAME TO idx_people_daily_reports_date;
ALTER INDEX people.idx_work_daily_reports_reporter RENAME TO idx_people_daily_reports_reporter;
ALTER INDEX people.idx_work_absences_person RENAME TO idx_people_leave_requests_person;
ALTER INDEX people.idx_work_absences_status RENAME TO idx_people_leave_requests_status;
ALTER INDEX people.idx_work_absences_dates RENAME TO idx_people_leave_requests_dates;

-- Leave types are [CONFIG] in the blueprint; add the missing `unpaid` while we
-- are renaming the check.
ALTER TABLE people.leave_requests DROP CONSTRAINT absences_absence_type_check;
ALTER TABLE people.leave_requests ADD CONSTRAINT leave_requests_leave_type_check
  CHECK (leave_type IN ('annual','sick','compassionate','maternity','paternity','unpaid','other'));

-- A people-owned touch trigger, so people tables do not depend on the work schema.
CREATE FUNCTION people.touch_updated_at() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  NEW.updated_at := now();
  RETURN NEW;
END;
$$;

DROP TRIGGER daily_reports_touch_updated_at ON people.daily_reports;
DROP TRIGGER absences_touch_updated_at ON people.leave_requests;

CREATE TRIGGER daily_reports_touch_updated_at BEFORE UPDATE ON people.daily_reports FOR EACH ROW EXECUTE FUNCTION people.touch_updated_at();
CREATE TRIGGER leave_requests_touch_updated_at BEFORE UPDATE ON people.leave_requests FOR EACH ROW EXECUTE FUNCTION people.touch_updated_at();
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TRIGGER IF EXISTS leave_requests_touch_updated_at ON people.leave_requests;
DROP TRIGGER IF EXISTS daily_reports_touch_updated_at ON people.daily_reports;

ALTER TABLE people.leave_requests DROP CONSTRAINT leave_requests_leave_type_check;
ALTER TABLE people.leave_requests ADD CONSTRAINT absences_absence_type_check
  CHECK (leave_type IN ('annual','sick','compassionate','maternity','paternity','other'));

ALTER INDEX people.idx_people_leave_requests_dates RENAME TO idx_work_absences_dates;
ALTER INDEX people.idx_people_leave_requests_status RENAME TO idx_work_absences_status;
ALTER INDEX people.idx_people_leave_requests_person RENAME TO idx_work_absences_person;
ALTER INDEX people.idx_people_daily_reports_reporter RENAME TO idx_work_daily_reports_reporter;
ALTER INDEX people.idx_people_daily_reports_date RENAME TO idx_work_daily_reports_date;

ALTER TABLE people.leave_requests RENAME COLUMN leave_type TO absence_type;
ALTER TABLE people.leave_requests RENAME TO absences;
ALTER TABLE people.daily_reports SET SCHEMA work;
ALTER TABLE people.absences SET SCHEMA work;

DROP FUNCTION IF EXISTS people.touch_updated_at();
CREATE TRIGGER daily_reports_touch_updated_at BEFORE UPDATE ON work.daily_reports FOR EACH ROW EXECUTE FUNCTION work.touch_updated_at();
CREATE TRIGGER absences_touch_updated_at BEFORE UPDATE ON work.absences FOR EACH ROW EXECUTE FUNCTION work.touch_updated_at();
-- +goose StatementEnd
