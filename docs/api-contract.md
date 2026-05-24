# API Contract

`docs/openapi.yaml` is the source-of-truth API contract for the JSON REST, SSE, and TUS endpoints used by the web and mobile clients.

The current contract step is intentionally pragmatic:

- Document every `/api` route implemented in `internal/server/server.go`.
- Keep common request/response shapes in OpenAPI components.
- Run a route coverage check locally and in CI so new routes cannot silently drift away from the spec.
- Generate web TypeScript schema types into `web/src/lib/api-types.ts`.
- Validate hand-written Flutter models against required OpenAPI response fields.

Run the check locally:

```sh
make api-contract
```

or directly:

```sh
ruby scripts/check-openapi-routes.rb
```

Regenerate web TypeScript types after editing the spec:

```sh
make api-types
```

The checker compares `mux.HandleFunc("METHOD /api/...")` registrations in `internal/server/server.go` with `docs/openapi.yaml`. It ignores TUS `OPTIONS` routes because they are protocol preflight/capability routes rather than client model operations.

## Client Generation Path

Current state:

- Web: TypeScript schema types are generated from OpenAPI and used by `web/src/lib/api.ts`.
- Mobile: Dart model classes remain hand-written, but `scripts/check-dart-openapi-models.rb` validates required response fields.

Remaining work:

- Generate or validate endpoint request/response wrapper types, not only component schemas.
- Decide whether Flutter should eventually use generated Dart models.
- Native upload integrations: use the same contract for Kotlin and Swift upload-related types.

Until generation is added, update `docs/openapi.yaml` in the same change as backend route or response-shape changes.
