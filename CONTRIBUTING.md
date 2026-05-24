# Contributing to goDrive

goDrive is a self-hosted file manager with web, backend, Android, iOS, WebDAV, and CLI surfaces. Contributions should keep those surfaces coherent and avoid adding hidden platform drift.

## Before Opening a Pull Request

1. Check `todo.md`, existing issues, and the relevant docs under `docs/`.
2. Keep changes focused. Split unrelated backend, web, mobile, and workflow changes into separate PRs when practical.
3. Update documentation when behavior, configuration, APIs, workflows, or deployment expectations change.
4. Update `docs/openapi.yaml` when backend routes or response shapes change.
5. Add or update tests for bug fixes, security-sensitive changes, API changes, and user-facing workflows.

## Local Checks

Install the repository hook on a new checkout:

```sh
make install-hooks
```

Run targeted checks while developing:

```sh
make test
make web-check
make web-test
make web-build
make mobile-test
```

Run the full local gate before larger PRs:

```sh
make check
```

Hosted GitHub Actions minutes are intentionally conserved. For Linux workflow validation, prefer local `act` runs as documented in `docs/ci-local.md` and `agents.md`.

## API Contract

The OpenAPI contract is the source of truth for REST/SSE API shape:

```sh
make api-contract
make api-types
```

Run these when touching backend routes, web API types, or mobile API models.

## Security

Do not open public issues for suspected vulnerabilities. Follow `SECURITY.md` instead.

For security-sensitive code, include regression tests and explain the deployment assumptions, especially around reverse proxies, cookies, filesystem access, previews, WebDAV, webhooks, and demo mode.

## Style

- Prefer small, direct changes over broad refactors.
- Follow the existing architecture and naming in the touched area.
- Keep the filesystem as the source of truth; SQLite metadata should remain rebuildable unless explicitly documented otherwise.
- Keep frontend UI practical and dense. Avoid marketing-style layouts inside the app.
- Avoid platform-specific feature drift unless it is documented in the feature matrix.
