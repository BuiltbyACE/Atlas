package auth

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ServiceIdentityInput describes a service identity (§6.1) to create or update.
type ServiceIdentityInput struct {
	// ID is the subject the identity presents, e.g. "svc:orchestrator".
	ID string
	// Description records what the identity is for.
	Description string
	// Permissions are the §6.2 permission keys the identity may use.
	Permissions []string
}

// EnsureService is the operator-facing way to create or update a service
// identity (backed by `atlas-cli service create`). It is idempotent: running it
// again replaces the permission set, so provisioning the orchestrator is one
// repeatable command. Every permission must exist in the catalogue — a typo
// fails loudly rather than minting a token that can do nothing.
//
// credential_hash is required by the schema but is not consulted when a token
// is verified: service tokens are signed JWTs, so possession of the credential
// is never checked (ADR-008). A fresh random hash is stored anyway so the
// column reflects a real rotation.
func EnsureService(ctx context.Context, pool *pgxpool.Pool, in ServiceIdentityInput) error {
	if in.ID == "" {
		return fmt.Errorf("auth: service identity id is required")
	}
	if len(in.Permissions) == 0 {
		return fmt.Errorf("auth: service %q needs at least one permission", in.ID)
	}

	for _, permission := range in.Permissions {
		var exists bool
		if err := pool.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM core.permissions WHERE key = $1)`, permission,
		).Scan(&exists); err != nil {
			return fmt.Errorf("auth: check permission %q: %w", permission, err)
		}
		if !exists {
			return fmt.Errorf("auth: unknown permission %q (run `atlas-cli seed`)", permission)
		}
	}

	credential, err := newOpaqueToken()
	if err != nil {
		return err
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO core.service_identities (id, description, permissions, credential_hash, rotated_at)
		VALUES ($1, $2, $3, $4, now())
		ON CONFLICT (id) DO UPDATE SET
			description     = EXCLUDED.description,
			permissions     = EXCLUDED.permissions,
			credential_hash = EXCLUDED.credential_hash,
			rotated_at      = now(),
			disabled_at     = NULL`,
		in.ID, in.Description, in.Permissions, HashToken(credential),
	); err != nil {
		return fmt.Errorf("auth: upsert service %q: %w", in.ID, err)
	}
	return nil
}
