// Package migrations embeds the goose SQL migrations so the atlas-cli binary
// can migrate a database without the source tree present (docs/ATLAS.md §10).
//
// Migrations are forward-only in practice: every Up must be safe to run against
// a database that already has data. Down migrations exist for local resets.
package migrations

import "embed"

// FS holds every *.sql migration, applied in ascending numeric version order by
// goose (the digits before the first underscore, not lexical filename order).
//
//go:embed *.sql
var FS embed.FS
