-- +goose Up
-- +goose StatementBegin
-- Phase 5: system availability history (§8.6).
--
-- The scorecard rolls up availability over a rolling 90-day window from
-- systems.system_availability (availability_bp in basis points, measured_at
-- timestamptz). The table never existed, so the scorecard 500'd on every
-- request. Create it here and grant the runtime roles access.
CREATE TABLE IF NOT EXISTS systems.system_availability (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  system_id     uuid NOT NULL REFERENCES systems.systems(id) ON DELETE CASCADE,
  measured_at   timestamptz NOT NULL,
  availability_bp integer NOT NULL CHECK (availability_bp >= 0 AND availability_bp <= 10000),
  created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_system_availability_system_measured
  ON systems.system_availability(system_id, measured_at DESC);
CREATE INDEX IF NOT EXISTS idx_system_availability_measured
  ON systems.system_availability(measured_at);

GRANT SELECT ON ALL TABLES IN SCHEMA systems TO atlas_api;
GRANT SELECT ON ALL TABLES IN SCHEMA systems TO atlas_worker;
GRANT SELECT ON ALL TABLES IN SCHEMA systems TO atlas_readonly;

-- +goose StatementEnd
-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS systems.system_availability;
-- +goose StatementEnd
