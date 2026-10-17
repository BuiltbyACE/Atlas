-- +goose Up
-- +goose StatementBegin
-- Session activity (§8.5): the people activity view reads core.sessions to show
-- who is signed in right now and how long a person has been active. No new
-- state is stored — a session already carries created_at, last_seen_at,
-- revoked_at and expires_at, and rows are retained after revocation — so this
-- migration only adds the index the live-session roster scans: active rows
-- ordered by last activity, with the presence window as a range predicate.
--
-- Without it the presence endpoint grows a seq scan over core.sessions on every
-- heartbeat-driven poll.
CREATE INDEX sessions_active_last_seen_idx
  ON core.sessions (last_seen_at DESC)
  WHERE revoked_at IS NULL;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS core.sessions_active_last_seen_idx;
-- +goose StatementEnd
