-- +goose Up
-- +goose StatementBegin
-- Phase 6 follow-up: restore the chat-attachment -> document foreign key.
--
-- 0032_chat_schema.sql creates core.chat_message_attachments, but core.documents
-- is created later by 0033_document_schema.sql. Declaring the reference inline
-- in 0032 broke every fresh database, so the column is plain uuid there and the
-- constraint is added here, after both tables exist.
--
-- The guard is name-agnostic: it looks for any existing foreign key from
-- chat_message_attachments.document_id to core.documents(id), so a database that
-- already carries this reference (for example one migrated before this fix, where
-- PostgreSQL would have named the inline constraint exactly as below) is left
-- untouched rather than gaining a duplicate constraint.
DO $$
BEGIN
	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint c
		JOIN pg_attribute a
		  ON a.attrelid = c.conrelid
		 AND a.attnum = ANY (c.conkey)
		WHERE c.contype = 'f'
		  AND c.conrelid = 'core.chat_message_attachments'::regclass
		  AND c.confrelid = 'core.documents'::regclass
		  AND a.attname = 'document_id'
	) THEN
		ALTER TABLE core.chat_message_attachments
			ADD CONSTRAINT chat_message_attachments_document_id_fkey
			FOREIGN KEY (document_id) REFERENCES core.documents(id) ON DELETE SET NULL;
	END IF;
END $$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Removes only this constraint; the column and table belong to 0032.
ALTER TABLE core.chat_message_attachments
	DROP CONSTRAINT IF EXISTS chat_message_attachments_document_id_fkey;
-- +goose StatementEnd
