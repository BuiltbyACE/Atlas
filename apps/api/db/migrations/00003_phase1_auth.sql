-- +goose Up
-- +goose StatementBegin
-- Phase 1: authentication, lockout and idempotency (docs/ATLAS.md §6.1, §11, §13.1).
--
-- Additions only: this migration must not disturb the §10.3 foundation.

-- ── Login identifiers ────────────────────────────────────────
-- Email is already required and unique. Phone is an optional second
-- identifier, stored in E.164 and unique when present.
ALTER TABLE core.user_accounts ADD COLUMN phone_e164 text;
CREATE UNIQUE INDEX user_accounts_phone_key
  ON core.user_accounts (phone_e164) WHERE phone_e164 IS NOT NULL;

-- MFA lifecycle: the sealed secret lives in mfa_totp_secret_enc (dropped in by
-- the app), and this timestamp records when the user first proved possession.
ALTER TABLE core.user_accounts ADD COLUMN mfa_totp_activated_at timestamptz;

-- ── Account invites (§6.1 lifecycle) ─────────────────────────
CREATE TABLE core.account_invites (
  id              uuid PRIMARY KEY,
  email           citext NOT NULL,
  phone_e164      text,
  full_name       text,
  employment_type text NOT NULL DEFAULT 'full_time'
                  CHECK (employment_type IN ('full_time','part_time','contractor','intern','external')),
  role_grants     jsonb NOT NULL DEFAULT '[]'::jsonb,
  token_hash      bytea NOT NULL UNIQUE,
  invited_by      uuid REFERENCES core.user_accounts(id),
  expires_at      timestamptz NOT NULL,
  accepted_at     timestamptz,
  revoked_at      timestamptz,
  created_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX account_invites_email_idx ON core.account_invites (email);

-- ── MFA recovery codes (§6.1) ────────────────────────────────
CREATE TABLE core.recovery_codes (
  id         uuid PRIMARY KEY,
  user_id    uuid NOT NULL REFERENCES core.user_accounts(id) ON DELETE CASCADE,
  code_hash  bytea NOT NULL,
  used_at    timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX recovery_codes_user_idx ON core.recovery_codes (user_id);

-- ── Sign-in attempts, for lockout and throttling (§13.1) ─────
CREATE TABLE core.login_attempts (
  id           bigserial PRIMARY KEY,
  identifier   text NOT NULL,
  succeeded    boolean NOT NULL,
  ip           inet,
  user_agent   text,
  attempted_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX login_attempts_identifier_idx ON core.login_attempts (identifier, attempted_at DESC);

-- ── Idempotency records (§11) ────────────────────────────────
-- Non-GET internal commands must carry an Idempotency-Key; the original
-- response is replayed within the retention window.
CREATE TABLE core.idempotency_records (
  id           uuid PRIMARY KEY,
  scope        text NOT NULL,
  actor        text NOT NULL,
  idem_key     text NOT NULL,
  request_hash text NOT NULL,
  status       int NOT NULL,
  response     jsonb NOT NULL,
  created_at   timestamptz NOT NULL DEFAULT now(),
  expires_at   timestamptz NOT NULL,
  UNIQUE (scope, actor, idem_key)
);
CREATE INDEX idempotency_records_expires_idx ON core.idempotency_records (expires_at);

-- ── Grants ───────────────────────────────────────────────────
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'atlas_api') THEN
    GRANT SELECT, INSERT, UPDATE, DELETE ON core.account_invites, core.recovery_codes TO atlas_api;
    GRANT SELECT, INSERT, UPDATE, DELETE ON core.idempotency_records TO atlas_api;
    GRANT SELECT, INSERT ON core.login_attempts TO atlas_api;
    GRANT USAGE, SELECT ON SEQUENCE core.login_attempts_id_seq TO atlas_api;
  END IF;
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'atlas_worker') THEN
    GRANT SELECT, INSERT, UPDATE, DELETE ON core.account_invites, core.recovery_codes TO atlas_worker;
    GRANT SELECT, INSERT, UPDATE, DELETE ON core.idempotency_records TO atlas_worker;
    GRANT SELECT, INSERT ON core.login_attempts TO atlas_worker;
    GRANT USAGE, SELECT ON SEQUENCE core.login_attempts_id_seq TO atlas_worker;
  END IF;
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'atlas_readonly') THEN
    GRANT SELECT ON core.account_invites, core.recovery_codes, core.idempotency_records, core.login_attempts TO atlas_readonly;
  END IF;
END
$$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS core.idempotency_records;
DROP INDEX IF EXISTS core.login_attempts_identifier_idx;
DROP TABLE IF EXISTS core.login_attempts;
DROP TABLE IF EXISTS core.recovery_codes;
DROP TABLE IF EXISTS core.account_invites;
DROP INDEX IF EXISTS core.user_accounts_phone_key;
ALTER TABLE core.user_accounts DROP COLUMN IF EXISTS phone_e164;
ALTER TABLE core.user_accounts DROP COLUMN IF EXISTS mfa_totp_activated_at;
-- +goose StatementEnd
