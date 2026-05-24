# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## First-time setup on a new checkout

```bash
make install-hooks   # symlinks scripts/pre-commit.sh → .git/hooks/pre-commit
```

The pre-commit hook runs on staged files only — fast for typical changes:
- Go staged: `gofmt` check + `go vet` + `go test`
- `web/` staged: `svelte-check`
- `mobile/` staged: `flutter analyze`

CI re-runs the main backend/frontend checks as a second gate for code changes. Documentation-only changes are ignored by hosted CI to conserve GitHub Actions minutes.

## CI and workflow validation

Use local checks first. Do not trigger GitHub Actions just to validate routine Linux jobs when `act` can run the same workflow locally.

```bash
make test
make web-check
make web-test
make web-build
act -l
act -W .github/workflows/ci.yml -j backend --artifact-server-addr 127.0.0.1 --cache-server-addr 127.0.0.1
act -W .github/workflows/ci.yml -j frontend --artifact-server-addr 127.0.0.1 --cache-server-addr 127.0.0.1
```

For heavier local workflow checks:

```bash
act -W .github/workflows/ci.yml -j docker --input docker_build=true --artifact-server-addr 127.0.0.1 --cache-server-addr 127.0.0.1
act -W .github/workflows/security.yml -j web-audit --artifact-server-addr 127.0.0.1 --cache-server-addr 127.0.0.1
act -W .github/workflows/security.yml -j docker-image --input docker_image_scan=true --artifact-server-addr 127.0.0.1 --cache-server-addr 127.0.0.1
```

Use `-n` for a dry-run when checking workflow shape without executing commands. Use GitHub-hosted runners for release publishing and iOS/macOS builds. iOS cannot be realistically validated through `act` on Linux. Before pushing workflow changes, run `act -l` and the relevant Linux jobs locally when practical.

## Commands

### Backend
```bash
make run          # start backend (port 8121) with dev latency simulation
make test         # run all unit tests (isolated GOCACHE)
make test-cover   # tests with coverage report
make test-race    # tests with race detector
make tidy         # sync go.mod/go.sum
make check        # full quality gate: fmt-check + vet + lint + test + web-test + web-build
```

Run a single Go test:
```bash
go test ./internal/server/... -run TestName
```

### Frontend (`web/`)
```bash
make web-dev      # Vite dev server on port 5173 (proxies /api to :8121)
make web-check    # svelte-check type checking
make web-build    # production build (outputs to internal/server/static/)
make web-install  # npm install
make web-test     # Vitest frontend unit tests
```

Dev requires **both** `make run` and `make web-dev` running simultaneously.

### Mobile (`mobile/`)
```bash
make mobile-install          # flutter pub get
make mobile-test             # flutter test
make mobile-build-android    # debug APK → build/app/outputs/flutter-apk/app-debug.apk
make mobile-run              # flutter run on emulator-5554
make mobile-dev              # start emulator + backend (no latency) + flutter run
```

iOS platform scaffold already committed (`mobile/ios/`). Android scaffold is in `mobile/android/`. No `flutter create` needed on fresh checkout.

### iOS on physical iPhone (from Linux / Fedora Atomic)

iOS builds run on GitHub Actions (`macos-latest`). xtool (AppImage) handles signing + install on Linux without Xcode.

**Repo:** `https://github.com/MrCodeEU/goDrive` (private)

**One-time machine setup:**
```bash
# 1. usbmuxd — socket-activated, needed for iPhone USB
systemctl is-active usbmuxd || rpm-ostree install usbmuxd && systemctl reboot

# 2. xtool AppImage + Apple USB udev rule
make xtool-setup

# 3. Apple ID login (stored in system keychain)
make xtool-auth

# 4. Verify iPhone is seen
make ios-devices    # plug in iPhone, tap Trust when prompted
```

**Dev loop:**
```bash
make ios-deploy     # push → CI build (~8-12 min) → download IPA → sign + install
make ios-refresh    # re-sign last IPA when 7-day free cert expires (skips rebuild)
```

`ios-deploy` force-pushes HEAD to a scratch branch `ios-dev` — main is never touched. CI trigger: `.github/workflows/ios.yml` (`workflow_dispatch` only).

**Connectivity for testing:**
```bash
# .env
GODRIVE_ADDR=0.0.0.0:8121   # bind to LAN, not just localhost
```
iPhone connects to `http://<laptop-LAN-IP>:8121` — same WiFi required.

## Environment

Copy `.env.example` to `.env`. Required for first boot:
- `GODRIVE_BOOTSTRAP_ADMIN_PASSWORD` — sets admin password once, ignored on subsequent starts
- `GODRIVE_DATA_ROOT` — filesystem root for user files
- `GODRIVE_ADDR` — set to `0.0.0.0:8121` when testing with Android emulator
- `GODRIVE_DB_PATH` — SQLite path (auto-migrated on startup)

Notable dev variables:
- `GODRIVE_DEV_LATENCY=10ms-25ms` — injects random API latency (used by `make run`; clear for mobile testing)
- `GODRIVE_ENABLE_WATCHER=true` — watches filesystem for external changes
- `GODRIVE_RECONCILE_INTERVAL=24h` — periodic full reindex interval
- `GODRIVE_UPLOAD_TTL=48h` — incomplete TUS upload cleanup TTL
- `GODRIVE_MAX_UPLOAD_BYTES=0` — max declared upload size in bytes; 0 = unlimited (useful for limiting storage per deployment)

Android emulator reaches host backend at `http://10.0.2.2:8121`.

## Architecture

### Backend (`internal/`)

**Source of truth is the filesystem.** SQLite stores metadata only — users, sessions, trash records, TUS upload state, webhook subscriptions, and a **rebuildable file index** (`file_index` table). If the index is lost, `POST /api/admin/jobs/reindex` rebuilds it by walking the data root.

| Package | Responsibility |
|---|---|
| `cmd/godrive/main.go` | Config load, DB open, migrations, bootstrap admin, graceful shutdown (15 s drain on SIGTERM), periodic session/upload cleanup |
| `internal/server/` | HTTP mux, middleware, JSON encoding, route handlers |
| `internal/server/webhooks.go` | Async webhook event dispatch with HMAC-SHA256, retry, per-delivery context |
| `internal/files/service.go` | All filesystem ops — list, mkdir, move, trash, restore, upload finalization |
| `internal/store/` | SQLite schema + migrations; all DB queries including webhooks |
| `internal/auth/` | Argon2id password hashing, session tokens, CSRF tokens |
| `internal/preview/` | Preview kind classification (image/video/pdf/text/markdown) |
| `internal/config/` | Env-based config loading |
| `internal/watch/` | fsnotify watcher that invalidates file index on external changes |

**Auth flow:** `POST /api/auth/login` returns a session cookie plus bearer token. Cookie auth requires `X-CSRF-Token` on all mutating requests. Bearer token auth skips CSRF. Both go through `withUser()` middleware. Login rate limited: 10 failures/IP/5min → 15min block. Sessions expire per `GODRIVE_SESSION_TTL`; expired+revoked sessions pruned hourly by `StartSessionCleanup`.

**TUS upload flow:** `POST /api/tus` creates upload record + temp file, rejects declared `Upload-Length` > `GODRIVE_MAX_UPLOAD_BYTES` (default 0 = unlimited). `PATCH /api/tus/{id}` streams chunks. On final chunk, `files.Service.FinalizeUpload()` moves temp to target (conflict suffix), then `generateThumbnailsAsync()` generates all warmup sizes in the background for image/raw/video/pdf/office, then `fireEvent("upload.complete", ...)` notifies webhook subscribers.

**Thumbnail cache key:** `hash(version, userID, inode, device, size, mtime, thumbSize)` — inode-based, stable across renames/moves on same filesystem. Falls back to path-based key when inode unavailable.

**Trash:** Physical files move to `{APPDATA}/trash/{user_id}/{trash_id}/file`. Metadata (original path, size, deleted_at) stored in SQLite. Restore moves back; permanent delete calls `os.RemoveAll`.

**Webhooks:** `server.fireEvent(user, event, data)` dispatches asynchronously. Each hook gets its own `context.WithTimeout(context.Background(), 2min)` — independent of the query context that found the hooks (bug-prone otherwise: shared cancel = premature abort). HTTP POST with `X-GoDrive-Signature: sha256=<hmac>`, 3 attempts, backoff 0s/5s/30s.

**WebDAV:** mounted at `/dav/` per authenticated user, backed by `webdav.Dir(user.HomeRoot)` from `golang.org/x/net/webdav`. Per-user in-memory lock system. Mount in macOS Finder: `http://host:8121/dav/` with bearer token or cookie auth. No file-index integration — operates directly on filesystem.

**SQLite pragmas:** WAL journal mode, `synchronous = NORMAL`, foreign keys on, 5s busy timeout. `MaxOpenConns(8)` — WAL allows concurrent reads; writes still serialize at the SQLite layer.

**Auth per-request cost:** `UserByValidSession` is a single JOIN (sessions + users) — one DB round-trip per authenticated request.

### Frontend (`web/src/`)

Single Svelte 5 component (`App.svelte`) — no router. UI states: loading → login form → file manager. SPA routing: server serves `index.html` for `/`, `/files`, and `/files/*`; direct navigation and browser refresh work. Static assets served with `Cache-Control: public, max-age=31536000, immutable` (Vite uses content-hash filenames).

| File | Role |
|---|---|
| `App.svelte` | All UI: topbar, file manager shell, modals (trash, admin, viewer), upload queue |
| `lib/api.ts` | Typed fetch wrappers for every backend endpoint; TUS client with resume |
| `lib/svar.ts` | Converts `FileEntry` (backend shape) to `SvarFile` (SVAR component format) |

**File manager API intercepts** in `initFilemanager()` — all return `false` to suppress default SVAR behavior:
- `request-data` → `GET /api/files/list?offset=&limit=` then `provide-data`
- `create-file` → TUS upload or `POST /api/files/mkdir`
- `rename-file` → `POST /api/files/move`
- `delete-files` → `POST /api/files/bulk/delete` (trash, not permanent)
- `move-files` → `POST /api/files/bulk/move`
- `download-file` → `GET /api/files/download`
- `open-file` → custom viewer (image lightbox / video player / PDF iframe / text panel)

**Upload queue:** Persisted to localStorage (metadata only — File handles are ephemeral). Active uploads set wakelock. TUS resume: `onUploadCreated` callback stores the TUS URL; retry does HEAD → PATCH from stored offset; falls back to fresh upload on 404.

**Folder pagination:** Default 500 entries/page. `has_more=true` shows a "Load more" banner that fetches the next page and appends to SVAR's `provide-data`.

### iOS sideload from Linux (Fedora Atomic)

xtool is an AppImage — no rpm-ostree needed. Runs with `APPIMAGE_EXTRACT_AND_RUN=1` to avoid FUSE dependency.

**One-time setup:**
```sh
rpm-ostree install usbmuxd && systemctl reboot   # if not already installed
make xtool-setup                                  # downloads AppImage, adds udev rule
make xtool-auth                                   # Apple ID login (stored in keychain)
```

**Dev iteration loop:**
```sh
make ios-deploy   # force-pushes to ios-dev → triggers GH Actions → downloads IPA → xtool signs + installs
make ios-refresh  # re-signs last IPA without rebuilding (7-day cert refresh)
```

`ios-dev` branch is force-pushed every run — no history junk, main stays clean. Build: ~8 min warm, ~12 min cold.

### Mobile (`mobile/`)

Flutter app with Provider state management. Targets Android (primary) and iOS.

| File | Role |
|---|---|
| `lib/main.dart` | App root, MultiProvider, theme |
| `lib/api/client.dart` | Typed HTTP client, all REST endpoints including webhooks |
| `lib/api/tus.dart` | TUS client: create → HEAD-resume → streaming PATCH |
| `lib/state/auth_state.dart` | Login/logout/session-restore, try/catch around all init to avoid crash on keystore failure |
| `lib/state/upload_queue.dart` | Upload queue, 3 concurrent workers, wakelock_plus, SharedPreferences persistence |
| `lib/screens/files_screen.dart` | File browser, grid/list toggle, pagination, search, upload picker (file_picker + image_picker) |
| `lib/screens/image_viewer_screen.dart` | PhotoViewGallery, prev/next, metadata overlay |
| `lib/screens/video_player_screen.dart` | VideoPlayerController.networkUrl with auth headers + chewie |
| `lib/screens/admin_screen.dart` | System stats, job management, user CRUD, webhook management |

**Key gotcha:** `AuthState.init()` and `UploadQueue.init()` are called as `..init()` (cascade, future discarded). Unhandled async errors crash the Dart VM. Both must wrap ALL async ops in try/catch with a `finally { _loading = false; notifyListeners(); }`.

**Emulator quirk:** Impeller rendering crashes on x86_64 emulators. Disabled via `AndroidManifest.xml` meta-data `io.flutter.embedding.android.EnableImpeller = false`. Gradle 9.0 required for Java 25 compatibility.
