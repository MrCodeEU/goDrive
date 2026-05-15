# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

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

First time on a new checkout, generate the Flutter platform scaffold:
```bash
flutter create --project-name godrive --org com.example --platforms android,ios mobile
make mobile-install
```

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

Android emulator reaches host backend at `http://10.0.2.2:8121`.

## Architecture

### Backend (`internal/`)

**Source of truth is the filesystem.** SQLite stores metadata only — users, sessions, trash records, TUS upload state, webhook subscriptions, and a **rebuildable file index** (`file_index` table). If the index is lost, `POST /api/admin/jobs/reindex` rebuilds it by walking the data root.

| Package | Responsibility |
|---|---|
| `cmd/godrive/main.go` | Config load, DB open, migrations, bootstrap admin, graceful shutdown |
| `internal/server/` | HTTP mux, middleware, JSON encoding, route handlers |
| `internal/server/webhooks.go` | Async webhook event dispatch with HMAC-SHA256, retry, per-delivery context |
| `internal/files/service.go` | All filesystem ops — list, mkdir, move, trash, restore, upload finalization |
| `internal/store/` | SQLite schema + migrations; all DB queries including webhooks |
| `internal/auth/` | Argon2id password hashing, session tokens, CSRF tokens |
| `internal/preview/` | Preview kind classification (image/video/pdf/text/markdown) |
| `internal/config/` | Env-based config loading |
| `internal/watch/` | fsnotify watcher that invalidates file index on external changes |

**Auth flow:** `POST /api/auth/login` returns a session cookie plus bearer token. Cookie auth requires `X-CSRF-Token` on all mutating requests. Bearer token auth skips CSRF. Both go through `withUser()` middleware. Login rate limited: 10 failures/IP/5min → 15min block.

**TUS upload flow:** `POST /api/tus` creates upload record + temp file. `PATCH /api/tus/{id}` streams chunks. On final chunk, `files.Service.FinalizeUpload()` moves temp to target (conflict suffix), then `generateThumbnailsAsync()` generates all warmup sizes in the background, then `fireEvent("upload.complete", ...)` notifies webhook subscribers.

**Thumbnail cache key:** `hash(version, userID, inode, device, size, mtime, thumbSize)` — inode-based, stable across renames/moves on same filesystem. Falls back to path-based key when inode unavailable.

**Trash:** Physical files move to `{APPDATA}/trash/{user_id}/{trash_id}/file`. Metadata (original path, size, deleted_at) stored in SQLite. Restore moves back; permanent delete calls `os.RemoveAll`.

**Webhooks:** `server.fireEvent(user, event, data)` dispatches asynchronously. Each hook gets its own `context.WithTimeout(context.Background(), 2min)` — independent of the query context that found the hooks (bug-prone otherwise: shared cancel = premature abort). HTTP POST with `X-GoDrive-Signature: sha256=<hmac>`, 3 attempts, backoff 0s/5s/30s.

**SQLite pragmas:** WAL journal mode, foreign keys on, 5s busy timeout.

### Frontend (`web/src/`)

Single Svelte 5 component (`App.svelte`) — no router. UI states: loading → login form → file manager.

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
