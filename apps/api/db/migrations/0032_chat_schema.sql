-- +goose Up
-- +goose StatementBegin
-- Phase 6: Messaging / chat platform (§8.5).
--
-- Stores durable chat conversations and messages between users in the
-- system. Realtime delivery runs through the existing orchestrator
-- WebSocket (/socket); this schema is the durable read/write model.
CREATE TABLE IF NOT EXISTS core.chat_conversations (
	id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	type           text NOT NULL CHECK (type IN ('direct', 'group')),
	name           text,
	created_by     uuid NOT NULL REFERENCES core.user_accounts(id) ON DELETE CASCADE,
	created_at     timestamptz NOT NULL DEFAULT now(),
	updated_at     timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_chat_conversations_type
	ON core.chat_conversations(type);
CREATE INDEX IF NOT EXISTS idx_chat_conversations_updated_at
	ON core.chat_conversations(updated_at DESC);

CREATE TABLE IF NOT EXISTS core.chat_participants (
	conversation_id uuid NOT NULL REFERENCES core.chat_conversations(id) ON DELETE CASCADE,
	user_id         uuid NOT NULL REFERENCES core.user_accounts(id) ON DELETE CASCADE,
	joined_at       timestamptz NOT NULL DEFAULT now(),
	last_read_at    timestamptz,
	PRIMARY KEY (conversation_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_chat_participants_user
	ON core.chat_participants(user_id);

CREATE TABLE IF NOT EXISTS core.chat_messages (
	id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	conversation_id  uuid NOT NULL REFERENCES core.chat_conversations(id) ON DELETE CASCADE,
	sender_id        uuid NOT NULL REFERENCES core.user_accounts(id) ON DELETE CASCADE,
	body             text NOT NULL DEFAULT '',
	body_type        text NOT NULL DEFAULT 'text' CHECK (body_type IN ('text', 'file', 'system')),
	reply_to_message uuid REFERENCES core.chat_messages(id) ON DELETE SET NULL,
	created_at       timestamptz NOT NULL DEFAULT now(),
	edited_at        timestamptz,
	deleted_at       timestamptz
);

CREATE INDEX IF NOT EXISTS idx_chat_messages_conversation
	ON core.chat_messages(conversation_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_chat_messages_sender
	ON core.chat_messages(sender_id);

CREATE TABLE IF NOT EXISTS core.chat_message_attachments (
	id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	message_id       uuid NOT NULL REFERENCES core.chat_messages(id) ON DELETE CASCADE,
	document_id      uuid REFERENCES core.documents(id) ON DELETE SET NULL,
	filename         text NOT NULL,
	content_type     text NOT NULL,
	size_bytes       bigint NOT NULL DEFAULT 0,
	created_at       timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_chat_message_attachments_message
	ON core.chat_message_attachments(message_id);

-- Trigger to keep updated_at current on conversations.
CREATE TRIGGER core_chat_conversations_touch BEFORE UPDATE ON core.chat_conversations
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
DROP TRIGGER IF EXISTS core_chat_conversations_touch ON core.chat_conversations;
DROP TABLE IF EXISTS core.chat_message_attachments;
DROP TABLE IF EXISTS core.chat_messages;
DROP TABLE IF EXISTS core.chat_participants;
DROP TABLE IF EXISTS core.chat_conversations;
-- +goose StatementEnd
