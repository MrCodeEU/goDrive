# API Contract

`docs/openapi.yaml` is the source-of-truth API contract for the JSON REST, SSE, and TUS endpoints used by the web and mobile clients.

The current contract step is intentionally pragmatic:

- Document every `/api` route implemented in `internal/server/server.go`.
- Keep common request/response shapes in OpenAPI components.
- Run a route coverage check locally and in CI so new routes cannot silently drift away from the spec.

Run the check locally:

```sh
make api-contract
```

or directly:

```sh
ruby scripts/check-openapi-routes.rb
```

The checker compares `mux.HandleFunc("METHOD /api/...")` registrations in `internal/server/server.go` with `docs/openapi.yaml`. It ignores TUS `OPTIONS` routes because they are protocol preflight/capability routes rather than client model operations.

## Client Generation Path

The next step is to validate or generate client models from `docs/openapi.yaml`:

- Web: generate TypeScript request/response types, then migrate `web/src/lib/api.ts` to use them.
- Mobile: generate or validate Dart models in `mobile/lib/api/models.dart`.
- Native upload integrations: use the same contract for Kotlin and Swift upload-related types.

Until generation is added, update `docs/openapi.yaml` in the same change as backend route or response-shape changes.
