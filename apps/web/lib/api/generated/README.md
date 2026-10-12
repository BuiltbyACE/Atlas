# Generated API client (do not edit)

This directory holds the **generated** TypeScript client for the Go API's
OpenAPI 3.1 contract at `contracts/openapi/public.yaml`.

- **Everything here is generated output.** Do not hand-edit files in this
  directory; your changes will be overwritten on the next codegen run.
- The generated files are **gitignored** (only `.gitkeep` and this `README.md`
  are committed). The directory is empty until codegen runs; `public.yaml` does
  not exist yet (Phase 0).
- The stable, hand-written layer in front of the generated code lives one level
  up in [`../client.ts`](../client.ts) (`apiFetch`, `ApiError`, `ProblemDetails`).

## Generating

Once `contracts/openapi/public.yaml` exists, generate into this folder with
[`openapi-typescript`](https://openapi-ts.dev/):

```bash
# from apps/web
npx openapi-typescript ../../contracts/openapi/public.yaml -o lib/api/generated/schema.d.ts
```

`npm run generate:api` is the intended entry point; it is currently a
placeholder that prints these instructions. Wire it to the command above (and to
`openapi-fetch` if we adopt a runtime client) when the spec lands.

CI runs a **generation diff check**: it regenerates the client and fails the
build if the committed generated output differs (blueprint section 16.2).
