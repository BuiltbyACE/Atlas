-- +goose Up
-- +goose StatementBegin
-- Phase 2: GitHub evidence integration (decision D6, docs/adr/ADR-018).
--
-- 1. Human refs on projects and tasks. The blueprint's work section describes a
--    `ref` on a project and a task, and its integrations note links a merged PR
--    to "the task ref in branch name or PR title (PRJ-2026-007/T-0142)", but
--    migration 00011 shipped neither. Go allocates refs through
--    work.ref_counters and never edits them, so a branch keeps naming one task
--    (ADR-010: ids are stable).
-- 2. work.evidence becomes polymorphic (6.5): a stored file, or a url /
--    git_commit / pull_request / test_run / client_approval / payment_reference
--    carrying a reference string. Every existing row is a `file`, so the D7
--    state is preserved exactly.
-- 3. integrations.github_deliveries remembers each X-GitHub-Delivery id so a
--    redelivered webhook is acknowledged but processed once (section 11:
--    "provider signature verification + replay window").
CREATE TABLE work.ref_counters (
  scope text PRIMARY KEY,
  value bigint NOT NULL
);

ALTER TABLE work.projects ADD COLUMN ref text;
ALTER TABLE work.tasks ADD COLUMN ref text;

DO $$
DECLARE
  proj RECORD;
  tsk RECORD;
  yr text;
  pseq bigint;
  tseq bigint;
  pref text;
BEGIN
  FOR proj IN SELECT id, created_at FROM work.projects ORDER BY created_at, id LOOP
    yr := to_char(proj.created_at, 'YYYY');
    INSERT INTO work.ref_counters (scope, value) VALUES ('PRJ-' || yr, 1)
      ON CONFLICT (scope) DO UPDATE SET value = work.ref_counters.value + 1
      RETURNING value INTO pseq;
    pref := 'PRJ-' || yr || '-' || lpad(pseq::text, 3, '0');
    UPDATE work.projects SET ref = pref WHERE id = proj.id;

    FOR tsk IN SELECT id FROM work.tasks WHERE project_id = proj.id ORDER BY created_at, id LOOP
      INSERT INTO work.ref_counters (scope, value) VALUES ('T', 1)
        ON CONFLICT (scope) DO UPDATE SET value = work.ref_counters.value + 1
        RETURNING value INTO tseq;
      UPDATE work.tasks SET ref = pref || '/T-' || lpad(tseq::text, 4, '0') WHERE id = tsk.id;
    END LOOP;
  END LOOP;
END
$$;

ALTER TABLE work.projects ALTER COLUMN ref SET NOT NULL;
ALTER TABLE work.tasks ALTER COLUMN ref SET NOT NULL;
CREATE UNIQUE INDEX idx_work_projects_ref ON work.projects(ref);
CREATE UNIQUE INDEX idx_work_tasks_ref ON work.tasks(ref);

-- Polymorphic evidence. The pairing rule is the 6.5 taxonomy: a `file` names a
-- stored object and nothing else; every other type names a reference and no
-- file. `reference` is a URL, a sha, an external key.
ALTER TABLE work.evidence ADD COLUMN evidence_type text NOT NULL DEFAULT 'file';
ALTER TABLE work.evidence ADD COLUMN reference text;
ALTER TABLE work.evidence ALTER COLUMN file_id DROP NOT NULL;
-- An integration (a merged PR, a payment reference) records evidence with no
-- human submitter; only the file path names the person who attached it.
ALTER TABLE work.evidence ALTER COLUMN uploader_person_id DROP NOT NULL;
ALTER TABLE work.evidence ADD CONSTRAINT work_evidence_type_valid CHECK (
  evidence_type IN ('file','url','git_commit','pull_request','test_run','client_approval','payment_reference'));
ALTER TABLE work.evidence ADD CONSTRAINT work_evidence_source_present CHECK (
  (evidence_type = 'file' AND file_id IS NOT NULL AND reference IS NULL)
  OR (evidence_type <> 'file' AND file_id IS NULL AND reference IS NOT NULL));

CREATE INDEX idx_work_evidence_type ON work.evidence(evidence_type);
CREATE INDEX idx_work_evidence_reference ON work.evidence(reference);
-- One external link (a merged PR URL) attaches once per task. A redelivered
-- webhook re-attaches the same reference; the index makes that a no-op the
-- service reuses, and the insert a race the database still rejects.
CREATE UNIQUE INDEX idx_work_evidence_task_reference
  ON work.evidence(task_id, evidence_type, reference)
  WHERE reference IS NOT NULL;

-- Inbound integration receipts (schema owned by atlas_api; see 10.1).
CREATE SCHEMA IF NOT EXISTS integrations;

CREATE TABLE integrations.github_deliveries (
  delivery_id text PRIMARY KEY,
  event_type  text NOT NULL,
  received_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_integrations_github_deliveries_received ON integrations.github_deliveries(received_at);

-- Grants mirror 00015: atlas_api writes, atlas_readonly reads. This is the
-- webhook-receiving surface; no other role needs it.
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'atlas_api') THEN
    GRANT USAGE ON SCHEMA integrations TO atlas_api;
    GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA integrations TO atlas_api;
    ALTER DEFAULT PRIVILEGES FOR ROLE atlas_migrator IN SCHEMA integrations
      GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO atlas_api;
  END IF;
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'atlas_readonly') THEN
    GRANT USAGE ON SCHEMA integrations TO atlas_readonly;
    GRANT SELECT ON ALL TABLES IN SCHEMA integrations TO atlas_readonly;
    ALTER DEFAULT PRIVILEGES FOR ROLE atlas_migrator IN SCHEMA integrations
      GRANT SELECT ON TABLES TO atlas_readonly;
  END IF;
END
$$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP SCHEMA IF EXISTS integrations CASCADE;
ALTER TABLE work.evidence
  DROP COLUMN IF EXISTS reference,
  DROP COLUMN IF EXISTS evidence_type;
ALTER TABLE work.evidence ALTER COLUMN file_id SET NOT NULL;
ALTER TABLE work.evidence ALTER COLUMN uploader_person_id SET NOT NULL;
DROP INDEX IF EXISTS work.idx_work_evidence_type;
DROP INDEX IF EXISTS work.idx_work_evidence_reference;
DROP INDEX IF EXISTS work.idx_work_evidence_task_reference;
DROP INDEX IF EXISTS work.idx_work_tasks_ref;
DROP INDEX IF EXISTS work.idx_work_projects_ref;
ALTER TABLE work.projects DROP COLUMN IF EXISTS ref;
ALTER TABLE work.tasks DROP COLUMN IF EXISTS ref;
DROP TABLE IF EXISTS work.ref_counters;
-- +goose StatementEnd
