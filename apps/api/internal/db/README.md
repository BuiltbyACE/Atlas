# internal/db

Typed Go generated from `db/queries/*.sql` by [sqlc](https://sqlc.dev), using
`schema: db/migrations` and `sql_package: pgx/v5` (see `../../sqlc.yaml`).

**This directory is intentionally empty of Go code in Phase 0.** Phase 0's only
database access is the hand-written outbox writer in `internal/outbox`, which
keeps the hello-event demo free of a code-generation step. Generated code lands
in Phase 1 alongside the first real module queries.

Generate with:

```
task generate          # sqlc generate
```

Generated files are committed (the CI generation-diff check requires the
working tree to be clean after regeneration — docs/ATLAS.md §16.2).
