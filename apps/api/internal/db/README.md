# internal/db

Typed Go generated from `db/queries/*.sql` by [sqlc](https://sqlc.dev), using
`schema: db/migrations` and `sql_package: pgx/v5` (see `../../sqlc.yaml`).

The Go files here (`db.go`, `models.go`, `outbox.sql.go`, `querier.go`) are
generated output. Do not edit them by hand: change the query or schema under
`db/` and regenerate. sqlc owns their contents.

Generate with:

```
task generate          # sqlc generate
```

Generated files are committed (the CI generation-diff check requires the
working tree to be clean after regeneration — docs/ATLAS.md §16.2).
