# Security Audit Plan

This plan is the release gate for goDrive before real family data is placed behind it. The target deployment is a small private server behind HTTPS, usually on a NAS or home server.

## Scope

Audit these surfaces:

- Go HTTP API: auth, sessions, CSRF, authorization, path handling, uploads, trash, admin routes, webhooks, preview endpoints.
- Filesystem boundary: per-user roots, symlinks, rename/move/delete/restore, archive/download behavior, watcher/reconciliation updates.
- SQLite state: migrations, session storage, upload records, file index, trash metadata, webhook secrets.
- Preview pipeline: image/video/PDF/text/Markdown rendering, external tools, cache paths, timeout behavior.
- Web UI: authenticated workflows, XSS risks, CSRF behavior, download/open flows.
- Flutter app: token storage, upload queue persistence, Android foreground service, iOS background URLSession, file picker paths.
- Deployment: Docker volumes, TLS/reverse proxy expectations, backup categories, logs.

Out of scope for the first full audit:

- Public multi-tenant hardening.
- Enterprise SSO, SCIM, audit log compliance, SIEM integration.
- Full mobile binary reverse engineering.

## Threat Model

Primary assets:

- User files under each home root.
- Admin credentials and session tokens.
- Trash contents.
- Webhook secrets.
- SQLite database and upload temp files.

Primary attackers:

- Unauthenticated network user who can reach the HTTP service.
- Authenticated non-admin user attempting to access another user's files or admin APIs.
- Malicious file content uploaded by a legitimate user.
- Local process writing directly into watched folders.
- Compromised webhook endpoint or automation client.

Security goals:

- No path traversal or symlink escape outside a user's configured root.
- No cross-user file access.
- No unauthenticated writes.
- Cookie writes require CSRF protection; bearer clients do not rely on CSRF.
- Preview generation cannot write outside the preview cache or run unbounded.
- Upload temp files cannot overwrite arbitrary paths.
- Admin-only APIs reject non-admin users.
- Webhook signatures are verifiable and secrets are not leaked after creation.

## Automated Checks

Run before manual testing:

```sh
make test
make test-race
make web-test
make web-check
flutter test
flutter analyze
GOCACHE=/tmp/godrive-gocache go test ./internal/store -bench BenchmarkFileIndex400k -run '^$' -count=1
```

Run these with a clean temp deployment:

```sh
GODRIVE_DATA_ROOT=/tmp/godrive-sec-data \
GODRIVE_APPDATA_DIR=/tmp/godrive-sec-appdata \
GODRIVE_BOOTSTRAP_ADMIN_PASSWORD=change-me \
go run ./cmd/godrive verify
```

## Manual Security Tests

Authentication and sessions:

- Login succeeds with the bootstrap admin and fails with a wrong password.
- Repeated wrong logins hit the rate limit and recover after the configured window.
- Logout invalidates the session cookie.
- Expired sessions are rejected after TTL.
- A bearer token returned by login works for API calls.
- A random bearer token is rejected.

CSRF:

- Cookie-authenticated `POST`, `PATCH`, and `DELETE` requests without `X-CSRF-Token` are rejected.
- The same requests with the login CSRF token succeed.
- Bearer-authenticated writes do not require CSRF.

Authorization:

- Non-admin user cannot call `/api/admin/*`.
- User A cannot list, download, rename, delete, restore, or preview User B files by guessing paths.
- Admin user management cannot create users with missing username/password/home root.

Path confinement:

- Requests containing `..`, encoded traversal, backslashes, null bytes, absolute paths, and repeated separators are rejected or normalized safely.
- Symlinks inside a user root pointing outside the root cannot be read, downloaded, moved, previewed, or indexed.
- Rename/move cannot move a file outside the user's root.
- Restore from trash handles conflicts and does not overwrite existing files.

Uploads:

- TUS create rejects unsafe filenames and unsafe target dirs.
- PATCH with the wrong offset is rejected.
- Completed upload finalization uses conflict suffixing and never overwrites existing files.
- Orphaned `.part` cleanup only removes upload temp files in the configured upload dir.
- Interrupted upload records do not grant access across users.

Previews:

- Image/video/PDF preview requests reject paths outside the user root.
- Text preview has a bounded byte limit and reports truncation.
- Markdown/text rendering does not execute HTML/script.
- External preview commands time out on slow inputs.
- Preview cache clearing refuses empty or root-like preview paths.

Webhooks:

- Webhook create/list/delete/test are admin-only.
- Secret is returned only when created.
- Delivery includes `X-GoDrive-Signature`.
- A receiver can verify the HMAC over the raw body.
- Failing endpoints do not block file operations.

Deployment:

- `GODRIVE_COOKIE_SECURE=true` is used behind HTTPS.
- Data, appdata, trash, uploads, and previews are mounted as separate documented volumes.
- Logs do not include passwords, bearer tokens, session IDs, CSRF tokens, or webhook secrets.
- Docker container runs with only required mounted directories.

## Findings Format

Record each finding with:

- Title.
- Severity: critical, high, medium, low.
- Affected component and file path.
- Preconditions.
- Reproduction steps.
- Impact.
- Recommended fix.
- Verification after fix.

## Release Decision

Do not deploy real data if any critical or high finding remains open. Medium findings need an explicit accept/fix decision. Low findings can be tracked if the deployment is private and the risk is understood.
