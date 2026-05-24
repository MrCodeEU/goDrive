# goDrive

Self-hosted file manager for families. Filesystem is the source of truth. SQLite stores metadata only — sessions, index, trash records, upload state. Everything is rebuildable from disk.

## Project Status

goDrive is still pre-release software. The core web/backend/demo flows are usable, but store publishing, wider device testing, long-running deployment testing, and some open source project polish are still in progress. Track the remaining work in `todo.md`.

Do not expose a personal deployment directly to the public internet without TLS, backups, rate limits, and a reverse proxy configuration you understand. The intended production model is a small private self-hosted instance, not a public multi-tenant SaaS.

## Features

**Web UI (Svelte/SVAR)**
- File browser with list and grid views, drag-and-drop upload, breadcrumb navigation
- Image lightbox with zoom, pan, prev/next, original/preview toggle
- In-browser video player, PDF viewer (native), text/Markdown preview, RAW/Office cached previews
- Upload queue with TUS resume, per-file progress, persistence across reloads
- Trash management, search (indexed filename/path), admin modal

**Backend**
- TUS resumable uploads with atomic finalization and conflict suffix
- Inode-based thumbnail cache — stable across renames/moves on the same filesystem
- Post-upload thumbnail generation (all warmup sizes generated asynchronously after each upload)
- Cached previews for images, RAW photos, video poster frames, PDFs, and Office documents
- Login rate limiting, CSRF protection, Argon2id password hashing
- Webhook/event API: subscribers receive `upload.complete`, `file.moved`, `file.deleted`, `file.restored` events with HMAC-SHA256 signature
- Periodic reconciliation scanner + fsnotify watcher for external changes (SMB, rsync, shell)
- Folder listing pagination (offset/limit) with load-more
- Graceful shutdown: in-flight requests drain for up to 15 s on SIGTERM
- Hourly expired-session cleanup; upload cleanup every 6 h
- SQLite WAL mode with 8 concurrent connections for read parallelism

**Mobile (Flutter)**
- Android/iOS app: file browser (list + grid), image viewer, in-app video player
- TUS upload queue with resume, wakelock during active uploads
- Android foreground-service and iOS background URLSession upload options for selected files
- Admin screen: stats, reindex/warmup jobs, user management, webhook management

**Deployment**
- Docker: multi-stage build, multi-arch (`amd64`/`arm64`), `ghcr.io` publish on version tags
- Unraid/NAS: docker-compose in `deploy/` with clear data/appdata/cache volume separation

## Quick Start

Copy `.env.example` to `.env` and set at minimum:

```sh
GODRIVE_BOOTSTRAP_ADMIN_PASSWORD=change-me
GODRIVE_DATA_ROOT=/path/to/your/files
GODRIVE_ADDR=0.0.0.0:8121
```

Then:

```sh
make run          # backend on :8121
make web-dev      # Svelte dev server on :5173 (proxies /api to :8121)
```

Open `http://127.0.0.1:5173/files`.

## Docker

```sh
# Local dev
docker compose -f deploy/docker-compose.local.yml up

# Production (edit volume paths first)
docker compose -f deploy/docker-compose.yml up -d
```

Release images are published to `ghcr.io/<owner>/godrive` on `v*` tags.

For a disposable public demo deployment, use the hardened demo compose profile in `deploy/docker-compose.demo.yml`; see `docs/demo-instance.md`.

For private production deployments, run goDrive behind a reverse proxy that terminates HTTPS, sets sane request/body limits, and forwards the expected headers. Set `GODRIVE_COOKIE_SECURE=true` and enable HSTS when the site is HTTPS-only. Keep `/data`, durable appdata, and trash on storage that is backed up.

## CI and Local Workflow Checks

Hosted GitHub Actions are kept focused on release orchestration, scheduled security checks, Docker publishing, and iOS/macOS builds. Routine Linux workflow checks should be run locally first with `make` and `act`; see [Local CI and GitHub Actions](docs/ci-local.md).

The REST/SSE/TUS API contract lives in [OpenAPI](docs/openapi.yaml). Web schema types are generated from it, and Flutter model drift is checked locally; see [API Contract](docs/api-contract.md).

For a high-level system overview, see [Architecture](docs/architecture.md).

For versioning, changelog, and tag-release steps, see [Release Process](docs/release-process.md).

## Mobile (Android)

```sh
make mobile-install   # flutter pub get
make mobile-dev       # start emulator + backend + flutter run
make mobile-run       # flutter run (emulator + backend already running)
make mobile-build-android  # debug APK
```

The emulator reaches the backend at `http://10.0.2.2:8121` (Android emulator maps `10.0.2.2` → host `127.0.0.1`).

Android package metadata, permissions, versioning, and Play listing draft are tracked in [Android Release Metadata](docs/android-release-metadata.md).

The app-store release and testing flow is tracked in [Mobile Store Release](docs/mobile-store-release.md).

## Mobile (iOS — physical device, from Linux)

iOS builds run on GitHub Actions (`macos-latest`). [xtool](https://xtool.sh) (AppImage) signs and installs on Linux without Xcode.

**One-time setup (Fedora Atomic):**

```sh
# usbmuxd — required for iPhone USB communication
rpm-ostree install usbmuxd && systemctl reboot   # skip if already installed

make xtool-setup    # downloads xtool AppImage, adds Apple USB udev rule
make xtool-auth     # Apple ID login (stored in keychain)
make ios-devices    # plug in iPhone → tap Trust → verify it appears
```

**Dev loop:**

```sh
make ios-deploy     # edit code → push to ios-dev branch → CI builds (~8-12 min) → xtool signs + installs
make ios-refresh    # re-sign last IPA when 7-day free cert expires (no rebuild)
```

`ios-deploy` force-pushes HEAD to a scratch branch `ios-dev` — `main` is never touched during iteration.

**Backend for iPhone testing** — iPhone must be on same WiFi as the laptop:

```sh
# .env
GODRIVE_ADDR=0.0.0.0:8121
```

Connect from iPhone to `http://<laptop-LAN-IP>:8121`.

## Webhooks

Register a subscriber to receive file events:

```sh
TOKEN='<admin bearer token>'

curl -X POST http://127.0.0.1:8121/api/webhooks \
  -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"url":"https://sorter.internal/events","events":["upload.complete"],"description":"HEIC sorter"}'
```

Response includes a `secret` used to verify `X-GoDrive-Signature: sha256=<hmac>` on each delivery.

Events: `upload.complete`, `file.moved`, `file.deleted`, `file.restored`.

Empty `events` array = subscribe to all events.

Webhook targets must use HTTPS and public IP ranges by default. For trusted LAN automation behind a reverse proxy, set `GODRIVE_WEBHOOK_ALLOW_HTTP=true` and/or `GODRIVE_WEBHOOK_ALLOW_PRIVATE=true` intentionally; do not enable these for a public demo instance.

Test a subscription:

```sh
curl -X POST http://127.0.0.1:8121/api/webhooks/<id>/test \
  -H "Authorization: Bearer $TOKEN"
```

## CLI Maintenance

```sh
godrive status
godrive verify
godrive reindex
godrive reindex --user alice
godrive preview-warmup
godrive preview-cache clear
godrive uploads cleanup --ttl 48h
godrive admin create --username admin --password 'change-me' --root ./var/data/admin
godrive admin reset-password --username admin --password 'new-password'
```

## Security Checks

The GitHub `Security` workflow runs weekly and on manual dispatch. It covers Go vulnerability checks, npm audit, OSV lockfile scans for npm/Pub dependencies, and an optional Anchore/Grype scan of the Docker image. Dependabot is configured for GitHub Actions, Go modules, npm, Pub, and Docker base images.

Please report suspected vulnerabilities privately. See [SECURITY.md](SECURITY.md).

Local release checks:

```sh
make security        # govulncheck, npm audit, OSV lockfile scan
make security-docker # Docker build + Grype image scan
```

## Environment Variables

| Variable | Default | Description |
|---|---|---|
| `GODRIVE_ADDR` | `127.0.0.1:8080` | HTTP listen address |
| `GODRIVE_DATA_ROOT` | `./var/data` | User files root |
| `GODRIVE_APPDATA_DIR` | `./var/appdata` | DB, trash, uploads, previews |
| `GODRIVE_DB_PATH` | `{appdata}/godrive.sqlite` | SQLite database path |
| `GODRIVE_BOOTSTRAP_ADMIN_USER` | `admin` | First-boot admin username |
| `GODRIVE_BOOTSTRAP_ADMIN_PASSWORD` | _(none)_ | First-boot admin password (ignored after admin exists) |
| `GODRIVE_BOOTSTRAP_ADMIN_ROOT` | `{data}/admin` | First-boot admin home root |
| `GODRIVE_SESSION_TTL` | `720h` | Session lifetime |
| `GODRIVE_COOKIE_SECURE` | `false` | Set `true` behind HTTPS |
| `GODRIVE_COOKIE_SAMESITE` | `strict` | Browser cookie SameSite policy: `strict`, `lax`, or `none`. `none` requires `GODRIVE_COOKIE_SECURE=true`. |
| `GODRIVE_HSTS` | `false` | Send `Strict-Transport-Security: max-age=31536000`. Automatically enabled when `GODRIVE_COOKIE_SECURE=true`; enable explicitly when HTTPS is terminated by a trusted reverse proxy and secure cookies are handled elsewhere. |
| `GODRIVE_ENABLE_WATCHER` | `true` | fsnotify watcher for external changes |
| `GODRIVE_RECONCILE_INTERVAL` | `24h` | Full reconciliation scan interval (`0` disables) |
| `GODRIVE_UPLOAD_TTL` | `48h` | Incomplete TUS upload expiry (`0` disables cleanup) |
| `GODRIVE_PREVIEW_WORKERS` | `0` | Thumbnail worker count (`0` = auto: half CPUs, 2–64) |
| `GODRIVE_PREVIEW_TIMEOUT` | `45s` | Per-file timeout for external preview tools |
| `GODRIVE_PREVIEW_DIR` | `{appdata}/previews` | Rebuildable thumbnail cache |
| `GODRIVE_UPLOAD_DIR` | `{appdata}/uploads` | TUS staging area |
| `GODRIVE_TRASH_DIR` | `{appdata}/trash` | Trash storage (durable — back up) |
| `GODRIVE_MAX_UPLOAD_BYTES` | `0` | Max declared TUS upload size in bytes (`0` = unlimited) |
| `GODRIVE_WEBHOOK_ALLOW_HTTP` | `false` | Allow non-HTTPS webhook URLs. Keep `false` for internet-facing deployments; set `true` only for trusted local networks. |
| `GODRIVE_WEBHOOK_ALLOW_PRIVATE` | `false` | Allow webhook delivery to private, loopback, link-local, and other non-public IP ranges. Useful for LAN automation, but unsafe for public demo instances. |
| `GODRIVE_DEMO_MODE` | `false` | Disable dangerous public-demo surfaces such as WebDAV, admin APIs, webhooks, uploads, trash mutations, API keys, and file writes. |
| `GODRIVE_DEV_LATENCY` | _(none)_ | Inject fake API latency, e.g. `10ms-25ms` |

## Volume Categories (Backup Guide)

Preview tools run with bounded wall-clock time, capped command output, isolated temp HOME/XDG/TMP directories, process-group cleanup, and, when the host provides `prlimit`, CPU/address-space/output-file/open-file resource limits. `prlimit` is included in the Docker runtime image through util-linux on Debian-based images; bare-metal installs should provide it for the same hard limits.

```text
/data/users/alice     → user files               BACK UP
/appdata/godrive.db   → durable app state        BACK UP
/appdata/trash/       → durable app state        BACK UP
/appdata/previews/    → rebuildable cache        safe to delete
/appdata/uploads/     → in-progress TUS uploads  safe to delete when server is down
```

## API Summary

```text
POST   /api/auth/login
POST   /api/auth/logout
GET    /api/me

GET    /api/files/list?path=&offset=&limit=
GET    /api/files/search?q=&limit=
POST   /api/files/mkdir
GET    /api/files/download?path=
GET    /api/files/raw?path=
GET    /api/files/text?path=
GET    /api/files/thumbnail?path=&size=
POST   /api/files/move
DELETE /api/files?path=
POST   /api/files/bulk/delete
POST   /api/files/bulk/move
POST   /api/files/bulk/download

ALL    /api/tus/*

GET    /api/trash
POST   /api/trash/{id}/restore
DELETE /api/trash/{id}

GET    /api/admin/users
POST   /api/admin/users
PATCH  /api/admin/users/{id}
POST   /api/admin/users/{id}/password
GET    /api/admin/stats
GET    /api/admin/jobs/current
POST   /api/admin/jobs/reindex
POST   /api/admin/jobs/preview-warmup
DELETE /api/admin/preview-cache

GET    /api/webhooks
POST   /api/webhooks
DELETE /api/webhooks/{id}
POST   /api/webhooks/{id}/test
```

Bearer token auth skips CSRF. Cookie auth uses `SameSite=Strict` by default and requires `X-CSRF-Token` on mutating requests. The session cookie is `HttpOnly`; the CSRF cookie remains readable by the browser client so it can echo the token in the request header.

```text
GET/PUT/DELETE/PROPFIND/...  /dav/{path}   WebDAV mount (per-user home root)
```

WebDAV supports Basic Auth for native clients such as Finder, iOS Files, and rclone. Bearer tokens also work. Browser cookie auth is accepted for WebDAV reads, but mutating WebDAV methods require `X-CSRF-Token`; Basic/Bearer auth is recommended for WebDAV clients. Repeated failed password and token authentication attempts are rate-limited per client IP.

## Quality Gate

```sh
make check    # fmt-check + vet + golangci-lint + test + web-test + web-build
```

Individual targets:

```sh
make test           # Go unit tests
make test-race      # with race detector
GOCACHE=/tmp/godrive-gocache go test ./internal/store -bench BenchmarkFileIndex400k -run '^$'
make web-test       # Vitest frontend tests
make web-check      # svelte-check type checking
make mobile-test    # Flutter tests
```

Before putting real data behind the server, run the release gates:

- [Feature support matrix](docs/feature-support.md)
- [Security audit plan](docs/security-audit.md)
- [Manual test plan](docs/manual-test-plan.md)

## Contributing

Contributions are welcome once the repository is public. Start with [CONTRIBUTING.md](CONTRIBUTING.md), use the issue and pull request templates, and report vulnerabilities privately through [SECURITY.md](SECURITY.md).

## License

goDrive is released under the [MIT License](LICENSE).
