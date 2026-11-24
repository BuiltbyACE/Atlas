-- +goose Up
-- +goose StatementBegin
-- Phase 6: Document storage (§8.6).
--
-- Metadata layer on top of the existing R2/S3 object storage (files package,
-- pre-signed URLs). Documents are categorized and access-controlled, and can
-- be attached to chat messages and outgoing email.
CREATE TABLE IF NOT EXISTS core.documents (
	id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	key             text NOT NULL UNIQUE,
	filename        text NOT NULL,
	content_type    text NOT NULL,
	size_bytes      bigint NOT NULL DEFAULT 0,
	category        text NOT NULL DEFAULT 'other' CHECK (category IN ('invoice', 'system_doc', 'contract', 'policy', 'other')),
	description     text NOT NULL DEFAULT '',
	uploaded_by     uuid NOT NULL REFERENCES core.user_accounts(id) ON DELETE CASCADE,
	uploaded_at     timestamptz NOT NULL DEFAULT now(),
	updated_at      timestamptz NOT NULL DEFAULT now(),
	is_public       boolean NOT NULL DEFAULT false,
	version         integer NOT NULL DEFAULT 1
);

CREATE INDEX IF NOT EXISTS idx_documents_category
	ON core.documents(category);
CREATE INDEX IF NOT EXISTS idx_documents_uploaded_by
	ON core.documents(uploaded_by);
CREATE INDEX IF NOT EXISTS idx_documents_uploaded_at
	ON core.documents(uploaded_at DESC);

CREATE TABLE IF NOT EXISTS core.document_shares (
	document_id  uuid NOT NULL REFERENCES core.documents(id) ON DELETE CASCADE,
	user_id      uuid NOT NULL REFERENCES core.user_accounts(id) ON DELETE CASCADE,
	permission   text NOT NULL DEFAULT 'read' CHECK (permission IN ('read', 'write')),
	shared_by    uuid NOT NULL REFERENCES core.user_accounts(id) ON DELETE CASCADE,
	created_at   timestamptz NOT NULL DEFAULT now(),
	PRIMARY KEY (document_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_document_shares_user
	ON core.document_shares(user_id);

CREATE TABLE IF NOT EXISTS core.document_access_log (
	id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
	document_id  uuid NOT NULL REFERENCES core.documents(id) ON DELETE CASCADE,
	user_id      uuid NOT NULL REFERENCES core.user_accounts(id) ON DELETE CASCADE,
	action       text NOT NULL CHECK (action IN ('view', 'download')),
	ip           text,
	user_agent   text,
	created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_document_access_log_document
	ON core.document_access_log(document_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_document_access_log_user
	ON core.document_access_log(user_id);

-- Trigger to keep updated_at current on documents.
CREATE TRIGGER core_documents_touch BEFORE UPDATE ON core.documents
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
DROP TRIGGER IF EXISTS core_documents_touch ON core.documents;
DROP TABLE IF EXISTS core.document_access_log;
DROP TABLE IF EXISTS core.document_shares;
DROP TABLE IF EXISTS core.documents;
-- +goose StatementEnd
