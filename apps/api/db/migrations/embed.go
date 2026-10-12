// Package migrations embeds the goose SQL migrations so the atlas-cli binary
// can migrate a database without the source tree present (docs/ATLAS.md §10).
//
// Migrations are forward-only in practice: every Up must be safe to run against
// a database that already has data. Down migrations exist for local resets.
package migrations

import "embed"

// FS holds every *.sql migration, applied in lexical filename order by goose.
//
//go:embed *.sql
var FS embed.FS
