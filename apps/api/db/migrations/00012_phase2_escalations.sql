-- +goose Up
-- +goose StatementBegin
-- Phase 2: overdue-task escalation ladder (docs/ATLAS.md §3.2, §5.8, §7).
--
-- An escalation is a durable record that a work item has passed its deadline and
-- needs a human to act. Go is the authority for the record (ADR-002); the
-- orchestrator reacts to core.escalation.* by nudging the assignee and the
-- executive dashboard (ADR-009).
--
-- The ladder's timing windows (E1-E3) are decision D4 and still open, so the
-- sweep raises level 1 as soon as a task is overdue. The level column already
-- exists so a later decision only needs to add timing, not schema.
CREATE TABLE work.escalations (
  id uuid PRIMARY KEY,
  entity_type text NOT NULL DEFAULT 'task',
  entity_id uuid NOT NULL,
  project_id uuid REFERENCES work.projects(id) ON DELETE CASCADE,
  level integer NOT NULL DEFAULT 1 CHECK (level >= 1),
  status text NOT NULL DEFAULT 'open' CHECK (status IN ('open','resolved')),
  reason text,
  assignee_person_id uuid REFERENCES people.persons(id),
  raised_by_person_id uuid REFERENCES people.persons(id),   -- NULL = system sweep
  resolved_by_person_id uuid REFERENCES people.persons(id),
  resolved_at timestamptz,
  version integer NOT NULL DEFAULT 1,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

-- At most one open escalation per entity per level: the sweep relies on this to
-- stay idempotent, and a manual raise cannot duplicate an active one.
CREATE UNIQUE INDEX idx_work_escalations_open
  ON work.escalations (entity_type, entity_id, level) WHERE status = 'open';
CREATE INDEX idx_work_escalations_entity ON work.escalations (entity_type, entity_id);
CREATE INDEX idx_work_escalations_status ON work.escalations (status);
CREATE INDEX idx_work_escalations_project ON work.escalations (project_id);
CREATE INDEX idx_work_escalations_assignee ON work.escalations (assignee_person_id);

CREATE TRIGGER escalations_touch_updated_at BEFORE UPDATE ON work.escalations FOR EACH ROW EXECUTE FUNCTION work.touch_updated_at();
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS work.escalations;
-- +goose StatementEnd
