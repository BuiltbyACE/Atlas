-- +goose Up
-- +goose StatementBegin
-- Phase 2: Work orchestration (projects, milestones, tasks, dependencies, blockers, evidence, reviews, handovers, daily reports, absences)
CREATE SCHEMA IF NOT EXISTS work;

-- Projects
CREATE TABLE work.projects (
  id uuid PRIMARY KEY,
  name text NOT NULL,
  description text,
  project_type text,
  status text NOT NULL CHECK (status IN ('planning','active','paused','completed','cancelled')) DEFAULT 'planning',
  owner_person_id uuid REFERENCES people.persons(id),
  department_id uuid REFERENCES people.departments(id),
  started_at timestamptz,
  due_at timestamptz,
  completed_at timestamptz,
  version integer NOT NULL DEFAULT 1,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_work_projects_status ON work.projects(status);
CREATE INDEX idx_work_projects_owner ON work.projects(owner_person_id);
CREATE INDEX idx_work_projects_department ON work.projects(department_id);
CREATE INDEX idx_work_projects_due_at ON work.projects(due_at);

-- Project members
CREATE TABLE work.project_members (
  project_id uuid NOT NULL REFERENCES work.projects(id) ON DELETE CASCADE,
  person_id uuid NOT NULL REFERENCES people.persons(id) ON DELETE CASCADE,
  role text NOT NULL,
  joined_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (project_id, person_id)
);

CREATE INDEX idx_work_project_members_person ON work.project_members(person_id);

-- Milestones
CREATE TABLE work.milestones (
  id uuid PRIMARY KEY,
  project_id uuid NOT NULL REFERENCES work.projects(id) ON DELETE CASCADE,
  name text NOT NULL,
  description text,
  status text NOT NULL CHECK (status IN ('not_started','in_progress','blocked','completed','cancelled')) DEFAULT 'not_started',
  due_at timestamptz,
  completed_at timestamptz,
  weight integer DEFAULT 0,
  version integer NOT NULL DEFAULT 1,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_work_milestones_project ON work.milestones(project_id);
CREATE INDEX idx_work_milestones_status ON work.milestones(status);
CREATE INDEX idx_work_milestones_due_at ON work.milestones(due_at);

-- Tasks
CREATE TABLE work.tasks (
  id uuid PRIMARY KEY,
  project_id uuid NOT NULL REFERENCES work.projects(id) ON DELETE CASCADE,
  milestone_id uuid REFERENCES work.milestones(id) ON DELETE SET NULL,
  parent_id uuid REFERENCES work.tasks(id) ON DELETE SET NULL,
  title text NOT NULL,
  description text,
  assignee_person_id uuid REFERENCES people.persons(id),
  reporter_person_id uuid REFERENCES people.persons(id),
  status text NOT NULL CHECK (status IN ('todo','in_progress','blocked','review','done','cancelled')) DEFAULT 'todo',
  priority text NOT NULL CHECK (priority IN ('low','medium','high','urgent')) DEFAULT 'medium',
  due_at timestamptz,
  started_at timestamptz,
  completed_at timestamptz,
  blocked bool NOT NULL DEFAULT false,
  blocked_reason text,
  estimate_hours numeric(6,2),
  actual_hours numeric(6,2),
  version integer NOT NULL DEFAULT 1,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_work_tasks_project ON work.tasks(project_id);
CREATE INDEX idx_work_tasks_milestone ON work.tasks(milestone_id);
CREATE INDEX idx_work_tasks_assignee ON work.tasks(assignee_person_id);
CREATE INDEX idx_work_tasks_status ON work.tasks(status);
CREATE INDEX idx_work_tasks_due_at ON work.tasks(due_at);
CREATE INDEX idx_work_tasks_priority ON work.tasks(priority);

-- Task dependencies
CREATE TABLE work.task_dependencies (
  predecessor_id uuid NOT NULL REFERENCES work.tasks(id) ON DELETE CASCADE,
  successor_id uuid NOT NULL REFERENCES work.tasks(id) ON DELETE CASCADE,
  dependency_type text NOT NULL CHECK (dependency_type IN ('fs','ss','ff','sf')) DEFAULT 'fs',
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (predecessor_id, successor_id),
  CHECK (predecessor_id <> successor_id)
);

CREATE INDEX idx_work_task_dep_successor ON work.task_dependencies(successor_id);

-- Blockers
CREATE TABLE work.blockers (
  id uuid PRIMARY KEY,
  task_id uuid NOT NULL REFERENCES work.tasks(id) ON DELETE CASCADE,
  reason text NOT NULL,
  created_by_person_id uuid REFERENCES people.persons(id),
  resolved_by_person_id uuid REFERENCES people.persons(id),
  resolved_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_work_blockers_task ON work.blockers(task_id);
CREATE INDEX idx_work_blockers_resolved ON work.blockers(resolved_at);

-- Evidence (link to files)
CREATE TABLE work.evidence (
  id uuid PRIMARY KEY,
  task_id uuid REFERENCES work.tasks(id) ON DELETE CASCADE,
  project_id uuid REFERENCES work.projects(id) ON DELETE CASCADE,
  file_id uuid NOT NULL REFERENCES core.files(id) ON DELETE RESTRICT,
  uploader_person_id uuid NOT NULL REFERENCES people.persons(id),
  kind text,
  description text,
  verified_by_person_id uuid REFERENCES people.persons(id),
  verified_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  CHECK ((task_id IS NOT NULL) OR (project_id IS NOT NULL))
);

CREATE INDEX idx_work_evidence_task ON work.evidence(task_id);
CREATE INDEX idx_work_evidence_project ON work.evidence(project_id);
CREATE INDEX idx_work_evidence_file ON work.evidence(file_id);

-- Reviews
CREATE TABLE work.reviews (
  id uuid PRIMARY KEY,
  task_id uuid REFERENCES work.tasks(id) ON DELETE CASCADE,
  milestone_id uuid REFERENCES work.milestones(id) ON DELETE CASCADE,
  project_id uuid REFERENCES work.projects(id) ON DELETE CASCADE,
  reviewer_person_id uuid NOT NULL REFERENCES people.persons(id),
  outcome text NOT NULL CHECK (outcome IN ('approved','changes_requested','rejected')),
  notes text,
  reviewed_at timestamptz NOT NULL DEFAULT now(),
  CHECK ((task_id IS NOT NULL) OR (milestone_id IS NOT NULL) OR (project_id IS NOT NULL))
);

CREATE INDEX idx_work_reviews_task ON work.reviews(task_id);
CREATE INDEX idx_work_reviews_project ON work.reviews(project_id);

-- Handovers
CREATE TABLE work.handovers (
  id uuid PRIMARY KEY,
  from_person_id uuid REFERENCES people.persons(id),
  to_person_id uuid NOT NULL REFERENCES people.persons(id),
  project_id uuid REFERENCES work.projects(id) ON DELETE SET NULL,
  task_id uuid REFERENCES work.tasks(id) ON DELETE SET NULL,
  scope text NOT NULL,
  checklist jsonb,
  accepted_by_person_id uuid REFERENCES people.persons(id),
  accepted_at timestamptz,
  version integer NOT NULL DEFAULT 1,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_work_handovers_to ON work.handovers(to_person_id);
CREATE INDEX idx_work_handovers_project ON work.handovers(project_id);

-- Daily reports
CREATE TABLE work.daily_reports (
  id uuid PRIMARY KEY,
  reporter_person_id uuid NOT NULL REFERENCES people.persons(id),
  report_date date NOT NULL,
  status text NOT NULL CHECK (status IN ('draft','submitted','reviewed')) DEFAULT 'draft',
  work_done text,
  blockers text,
  risks text,
  next_day text,
  hours_worked numeric(4,2),
  submitted_at timestamptz,
  reviewed_by_person_id uuid REFERENCES people.persons(id),
  reviewed_at timestamptz,
  notes text,
  version integer NOT NULL DEFAULT 1,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (reporter_person_id, report_date)
);

CREATE INDEX idx_work_daily_reports_date ON work.daily_reports(report_date);
CREATE INDEX idx_work_daily_reports_reporter ON work.daily_reports(reporter_person_id);

-- Absences
CREATE TABLE work.absences (
  id uuid PRIMARY KEY,
  person_id uuid NOT NULL REFERENCES people.persons(id),
  absence_type text NOT NULL CHECK (absence_type IN ('annual','sick','compassionate','maternity','paternity','other')),
  from_date date NOT NULL,
  to_date date NOT NULL,
  reason text,
  status text NOT NULL CHECK (status IN ('requested','approved','declined','cancelled')) DEFAULT 'requested',
  approver_person_id uuid REFERENCES people.persons(id),
  approved_at timestamptz,
  declined_at timestamptz,
  cancel_reason text,
  version integer NOT NULL DEFAULT 1,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CHECK (to_date >= from_date)
);

CREATE INDEX idx_work_absences_person ON work.absences(person_id);
CREATE INDEX idx_work_absences_status ON work.absences(status);
CREATE INDEX idx_work_absences_dates ON work.absences(from_date, to_date);

-- Touch triggers
CREATE FUNCTION work.touch_updated_at() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  NEW.updated_at := now();
  RETURN NEW;
END;
$$;

CREATE TRIGGER projects_touch_updated_at BEFORE UPDATE ON work.projects FOR EACH ROW EXECUTE FUNCTION work.touch_updated_at();
CREATE TRIGGER milestones_touch_updated_at BEFORE UPDATE ON work.milestones FOR EACH ROW EXECUTE FUNCTION work.touch_updated_at();
CREATE TRIGGER tasks_touch_updated_at BEFORE UPDATE ON work.tasks FOR EACH ROW EXECUTE FUNCTION work.touch_updated_at();
CREATE TRIGGER handovers_touch_updated_at BEFORE UPDATE ON work.handovers FOR EACH ROW EXECUTE FUNCTION work.touch_updated_at();
CREATE TRIGGER daily_reports_touch_updated_at BEFORE UPDATE ON work.daily_reports FOR EACH ROW EXECUTE FUNCTION work.touch_updated_at();
CREATE TRIGGER absences_touch_updated_at BEFORE UPDATE ON work.absences FOR EACH ROW EXECUTE FUNCTION work.touch_updated_at();
-- +goose StatementEnd

-- +goose Down
DROP SCHEMA IF EXISTS work CASCADE;
