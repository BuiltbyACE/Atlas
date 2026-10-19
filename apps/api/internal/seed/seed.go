package seed

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/safaristack/atlas/apps/api/internal/settings"
)

// Report summarises what a seed run wrote.
type Report struct {
	Roles           int
	Permissions     int
	RolePermissions int
	SettingsAdded   int
	CRMConfigAdded  int
}

// Validate checks the catalogue and mapping are internally consistent. It runs
// before any database work so a typo fails fast and loudly rather than writing a
// partial catalogue.
func Validate() error {
	known := make(map[string]struct{}, len(Permissions))
	for _, p := range Permissions {
		if p.Key == "" {
			return fmt.Errorf("seed: permission with empty key")
		}
		if _, dup := known[p.Key]; dup {
			return fmt.Errorf("seed: duplicate permission %q", p.Key)
		}
		known[p.Key] = struct{}{}
	}

	roleSet := make(map[string]struct{}, len(Roles))
	for _, r := range Roles {
		roleSet[r.Key] = struct{}{}
	}

	for role, perms := range RolePermissions {
		if _, ok := roleSet[role]; !ok {
			return fmt.Errorf("seed: mapping for unknown role %q", role)
		}
		for _, key := range perms {
			if _, ok := known[key]; !ok {
				return fmt.Errorf("seed: role %q references unknown permission %q", role, key)
			}
		}
	}

	for _, r := range Roles {
		if _, ok := RolePermissions[r.Key]; !ok {
			return fmt.Errorf("seed: role %q has no permission mapping", r.Key)
		}
	}

	// Seeded settings must be settings the API knows: a typo here would write a
	// row the settings engine cannot type or validate (§6.7).
	for key := range SettingDefaults {
		if _, ok := settings.DefinitionFor(key); !ok {
			return fmt.Errorf("seed: setting default %q has no definition in the settings catalogue", key)
		}
	}
	return nil
}

// Run upserts the configuration catalogue in a single transaction. It is
// idempotent: roles and permissions are upserted, each role's permission set is
// replaced, and setting defaults are only inserted when absent (so an operator's
// later change is preserved).
func Run(ctx context.Context, pool *pgxpool.Pool) (Report, error) {
	var rep Report

	if err := Validate(); err != nil {
		return rep, err
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return rep, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	for _, r := range Roles {
		if _, err := tx.Exec(ctx, `
			INSERT INTO core.roles (key, name, description)
			VALUES ($1, $2, $3)
			ON CONFLICT (key) DO UPDATE
			  SET name = EXCLUDED.name, description = EXCLUDED.description`,
			r.Key, r.Name, r.Description); err != nil {
			return rep, fmt.Errorf("upsert role %s: %w", r.Key, err)
		}
	}
	rep.Roles = len(Roles)

	for _, p := range Permissions {
		if _, err := tx.Exec(ctx, `
			INSERT INTO core.permissions (key, description, sensitive)
			VALUES ($1, $2, $3)
			ON CONFLICT (key) DO UPDATE
			  SET description = EXCLUDED.description, sensitive = EXCLUDED.sensitive`,
			p.Key, p.Description, p.Sensitive); err != nil {
			return rep, fmt.Errorf("upsert permission %s: %w", p.Key, err)
		}
	}
	rep.Permissions = len(Permissions)

	for role, perms := range RolePermissions {
		if _, err := tx.Exec(ctx, `DELETE FROM core.role_permissions WHERE role_key = $1`, role); err != nil {
			return rep, fmt.Errorf("clear role %s permissions: %w", role, err)
		}
		for _, key := range perms {
			if _, err := tx.Exec(ctx,
				`INSERT INTO core.role_permissions (role_key, permission_key) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
				role, key); err != nil {
				return rep, fmt.Errorf("grant %s to %s: %w", key, role, err)
			}
			rep.RolePermissions++
		}
	}

	for key, value := range SettingDefaults {
		tag, err := tx.Exec(ctx,
			`INSERT INTO core.settings (key, value) VALUES ($1, $2::jsonb) ON CONFLICT (key) DO NOTHING`,
			key, value)
		if err != nil {
			return rep, fmt.Errorf("seed setting %s: %w", key, err)
		}
		rep.SettingsAdded += int(tag.RowsAffected())
	}

	// CRM config (lead sources, pipeline stages) is seeded only when absent, so
	// an operator's edit is preserved (§8.1 [CONFIG]).
	added, err := seedCRMConfig(ctx, tx)
	if err != nil {
		return rep, err
	}
	rep.CRMConfigAdded = added

	if err := tx.Commit(ctx); err != nil {
		return rep, fmt.Errorf("commit seed: %w", err)
	}
	return rep, nil
}
