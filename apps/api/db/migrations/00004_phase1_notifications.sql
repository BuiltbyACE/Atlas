-- +goose Up
-- +goose StatementBegin
-- Phase 1: notifications — preferences, quiet hours and the delivery engine's
-- columns (docs/ATLAS.md §6.4, §8.10, §11).
--
-- Additions only: this migration must not disturb the §10.3 foundation.
--
-- The engine has two paths, and this schema exists to keep them apart:
--
--   * The write path (notifications.Service.Request) records the notification
--     row(s) and the outbox event in ONE transaction and never calls a provider.
--     A business action is therefore never slowed by an SMTP server: the SLOs in
--     §11 (p95 read < 300 ms, outbox lag p99 < 10 s) hold because the request
--     path does no I/O beyond the database.
--
--   * The dispatcher (atlas-worker) leases due rows with FOR UPDATE SKIP LOCKED
--     and performs the send with a timeout, retries and backoff (§8.10). The
--     columns below are exactly what a durable, at-least-once queue needs:
--     deliver_after (quiet hours + retry backoff), attempts/max_attempts,
--     idempotency_key (a retried send is not a duplicate), and the claim lease
--     (claimed_at/claimed_by) so a crashed worker's rows are retried.

-- ── delivery-engine columns on core.notifications ────────────
ALTER TABLE core.notifications
  ADD COLUMN IF NOT EXISTS critical        boolean NOT NULL DEFAULT false,
  ADD COLUMN IF NOT EXISTS max_attempts    int NOT NULL DEFAULT 5,
  ADD COLUMN IF NOT EXISTS deliver_after   timestamptz,
  ADD COLUMN IF NOT EXISTS idempotency_key text,
  ADD COLUMN IF NOT EXISTS claimed_at      timestamptz,
  ADD COLUMN IF NOT EXISTS claimed_by      text;

COMMENT ON COLUMN core.notifications.critical IS
  'Escalation L2+ / critical items bypass quiet hours (§6.4).';
COMMENT ON COLUMN core.notifications.deliver_after IS
  'Do not attempt delivery before this time: quiet-hours deferral or retry backoff.';
COMMENT ON COLUMN core.notifications.idempotency_key IS
  'Caller-supplied key; a repeat request must not create a second notification (§8.10).';
COMMENT ON COLUMN core.notifications.claimed_at IS
  'Lease timestamp set by the dispatcher while it owns the row (at-least-once).';

-- A duplicate idempotency key for the same channel is the same intent: swallow
-- it rather than send twice.
CREATE UNIQUE INDEX IF NOT EXISTS notifications_idempotency_key
  ON core.notifications (idempotency_key, channel)
  WHERE idempotency_key IS NOT NULL;

-- Hot read #1: a person's unread in-app notifications. Partial, so it stays
-- tiny no matter how many notifications a person has already read.
CREATE INDEX IF NOT EXISTS notifications_unread_idx
  ON core.notifications (recipient_person_id, created_at DESC)
  WHERE channel = 'in_app' AND read_at IS NULL;

-- The in-app centre lists one channel only; keep that scan tight.
CREATE INDEX IF NOT EXISTS notifications_in_app_idx
  ON core.notifications (recipient_person_id, created_at DESC, id DESC)
  WHERE channel = 'in_app';

-- ── delivery-engine hot read ─────────────────────────────────
-- The dispatcher scans for due work: pending, non-in-app, not before
-- deliver_after. A partial index over exactly that predicate stays tiny no
-- matter how many notifications have already been sent.
CREATE INDEX IF NOT EXISTS notifications_due_idx
  ON core.notifications (COALESCE(deliver_after, created_at))
  WHERE status = 'pending' AND channel <> 'in_app';

-- ── core: notification preferences (§6.4) ────────────────────
-- One row per person who has expressed a preference. Absence means defaults
-- (in-app + email on, no quiet hours), so this table only ever holds real
-- choices and stays small.
CREATE TABLE IF NOT EXISTS core.notification_preferences (
  person_id          uuid PRIMARY KEY REFERENCES people.persons(id) ON DELETE CASCADE,
  in_app             boolean NOT NULL DEFAULT true,
  email              boolean NOT NULL DEFAULT true,
  whatsapp           boolean NOT NULL DEFAULT false,
  sms                boolean NOT NULL DEFAULT false,
  -- Quiet hours are minutes since midnight in the person's timezone. Integers
  -- beat a time column here: they compare trivially and cannot carry a date.
  quiet_start_minute smallint CHECK (quiet_start_minute BETWEEN 0 AND 1439),
  quiet_end_minute   smallint CHECK (quiet_end_minute BETWEEN 0 AND 1439),
  timezone           text NOT NULL DEFAULT 'Africa/Nairobi',
  updated_at         timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE core.notification_preferences IS
  'Per-person notification channels and quiet hours (§6.4 [CONFIG]).';

CREATE TRIGGER notification_preferences_touch
  BEFORE UPDATE ON core.notification_preferences
  FOR EACH ROW EXECUTE FUNCTION core.touch_updated_at();

-- ── grants ───────────────────────────────────────────────────
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'atlas_api') THEN
    GRANT SELECT, INSERT, UPDATE, DELETE ON core.notification_preferences TO atlas_api;
  END IF;
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'atlas_worker') THEN
    GRANT SELECT, INSERT, UPDATE, DELETE ON core.notification_preferences TO atlas_worker;
  END IF;
  IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'atlas_readonly') THEN
    GRANT SELECT ON core.notification_preferences TO atlas_readonly;
  END IF;
END
$$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TRIGGER IF EXISTS notification_preferences_touch ON core.notification_preferences;
DROP TABLE IF EXISTS core.notification_preferences;
DROP INDEX IF EXISTS core.notifications_due_idx;
DROP INDEX IF EXISTS core.notifications_unread_idx;
DROP INDEX IF EXISTS core.notifications_idempotency_key;
ALTER TABLE core.notifications
  DROP COLUMN IF EXISTS claimed_by,
  DROP COLUMN IF EXISTS claimed_at,
  DROP COLUMN IF EXISTS idempotency_key,
  DROP COLUMN IF EXISTS deliver_after,
  DROP COLUMN IF EXISTS max_attempts,
  DROP COLUMN IF EXISTS critical;
-- +goose StatementEnd
