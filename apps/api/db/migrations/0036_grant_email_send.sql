-- +goose Up
-- +goose StatementBegin
-- Compose is gated by core.email.send, but no role was ever granted it, so every
-- signed-in user got FORBIDDEN when sending. Grant it to every working role
-- (the read-only auditor is left out). Additive and idempotent: it never removes
-- a grant, so edits made to roles in the app are preserved.
INSERT INTO core.permissions (key, description, sensitive)
VALUES ('core.email.send', 'Compose and send email from Atlas', false)
ON CONFLICT (key) DO NOTHING;

INSERT INTO core.role_permissions (role_key, permission_key)
SELECT r.key, 'core.email.send'
FROM core.roles r
WHERE r.key IN ('ceo_commercial', 'ceo_internal', 'finance_officer', 'sales_rep',
                'project_lead', 'engineer', 'staff', 'unit_manager', 'system_admin')
ON CONFLICT DO NOTHING;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM core.role_permissions WHERE permission_key = 'core.email.send';
-- +goose StatementEnd
