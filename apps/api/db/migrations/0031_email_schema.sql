-- +goose Up
-- +goose StatementBegin
-- Phase 6: Two-way email pipeline (§8.4).
--
-- Stores inbound and outbound email as durable records so every message
-- sent or received through Atlas is auditable and viewable in the UI.
-- Threads group messages via In-Reply-To / References headers.
CREATE TABLE IF NOT EXISTS core.email_threads (
	id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	subject     text NOT NULL DEFAULT '',
	created_at  timestamptz NOT NULL DEFAULT now(),
	updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_email_threads_updated_at
	ON core.email_threads(updated_at DESC);

CREATE TABLE IF NOT EXISTS core.email_messages (
	id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	thread_id       uuid NOT NULL REFERENCES core.email_threads(id) ON DELETE CASCADE,
	direction       text NOT NULL CHECK (direction IN ('inbound', 'outbound')),
	from_address    text NOT NULL,
	to_addresses    text[] NOT NULL DEFAULT '{}',
	cc_addresses    text[] NOT NULL DEFAULT '{}',
	bcc_addresses   text[] NOT NULL DEFAULT '{}',
	subject         text NOT NULL DEFAULT '',
	body_text       text NOT NULL DEFAULT '',
	body_html       text NOT NULL DEFAULT '',
	message_id      text NOT NULL UNIQUE,
	in_reply_to     text,
	references      text,
	received_at     timestamptz,
	sent_at         timestamptz,
	status          text NOT NULL DEFAULT 'received' CHECK (status IN ('received', 'sent', 'delivered', 'failed')),
	provider_ref    text,
	error_message   text,
	created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_email_messages_thread
	ON core.email_messages(thread_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_email_messages_direction
	ON core.email_messages(direction, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_email_messages_message_id
	ON core.email_messages(message_id);

CREATE TABLE IF NOT EXISTS core.email_attachments (
	id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	message_id      uuid NOT NULL REFERENCES core.email_messages(id) ON DELETE CASCADE,
	filename        text NOT NULL,
	content_type    text NOT NULL,
	size_bytes      bigint NOT NULL DEFAULT 0,
	storage_key     text NOT NULL,
	created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_email_attachments_message
	ON core.email_attachments(message_id);

CREATE TABLE IF NOT EXISTS core.email_addresses (
	id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	user_id         uuid NOT NULL REFERENCES core.user_accounts(id) ON DELETE CASCADE,
	address         text NOT NULL,
	verified        boolean NOT NULL DEFAULT false,
	is_primary      boolean NOT NULL DEFAULT false,
	created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_email_addresses_address
	ON core.email_addresses(address);
CREATE INDEX IF NOT EXISTS idx_email_addresses_user
	ON core.email_addresses(user_id);

-- Trigger to keep updated_at current on threads.
CREATE TRIGGER core_email_threads_touch BEFORE UPDATE ON core.email_threads
	FOR EACH ROW EXECUTE FUNCTION core.touch_updated_at();

-- Grants for runtime roles.
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA core TO atlas_api;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA core TO atlas_worker;
GRANT SELECT ON ALL TABLES IN SCHEMA core TO atlas_readonly;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA core TO atlas_api;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA core TO atlas_worker;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA core TO atlas_readonly;

-- +goose StatementEnd
-- +goose Down
-- +goose StatementBegin
DROP TRIGGER IF EXISTS core_email_threads_touch ON core.email_threads;
DROP TABLE IF EXISTS core.email_attachments;
DROP TABLE IF EXISTS core.email_messages;
DROP TABLE IF EXISTS core.email_threads;
DROP TABLE IF EXISTS core.email_addresses;
-- +goose StatementEnd
