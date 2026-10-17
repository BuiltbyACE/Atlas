-- +goose Up
-- +goose StatementBegin
-- Phase 2: time entries (decision D7, docs/adr/ADR-019).
--
-- §8.2 models a time entry as an optional record — person, task, date, minutes,
-- note — that is "enabled per project type [CONFIG]". Migration 00011 shipped
-- the work schema without it, so this adds it. Whether timesheets are required,
-- and for which project types, is a typed setting
-- (work.timesheet.required_project_types) read through the settings API; the
-- table just records the hours people log.
CREATE TABLE work.time_entries (
  id uuid PRIMARY KEY,
  person_id uuid NOT NULL REFERENCES people.persons(id),
  task_id uuid NOT NULL REFERENCES work.tasks(id) ON DELETE CASCADE,
  project_id uuid NOT NULL REFERENCES work.projects(id) ON DELETE CASCADE,
  entry_date date NOT NULL,
  minutes integer NOT NULL CHECK (minutes > 0 AND minutes <= 1440),
  note text,
  version integer NOT NULL DEFAULT 1,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

-- A person's week, a task's logged effort and a project's timesheet coverage are
-- the three ways entries are read (workload, margin, compliance).
CREATE INDEX idx_work_time_entries_person_date ON work.time_entries (person_id, entry_date);
CREATE INDEX idx_work_time_entries_task ON work.time_entries (task_id);
CREATE INDEX idx_work_time_entries_project_date ON work.time_entries (project_id, entry_date);

CREATE TRIGGER time_entries_touch_updated_at BEFORE UPDATE ON work.time_entries
  FOR EACH ROW EXECUTE FUNCTION work.touch_updated_at();
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS work.time_entries;
-- +goose StatementEnd
