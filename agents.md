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
```

Run a single Go test:
```bash
go test ./internal/server/... -run TestName
```

### Frontend (`web/`)
```bash
make web-dev      # Vite dev server on port 5173 (proxies /api to :8121)
make web-check    # svelte-check type checking
make web-build    # production build
make web-install  # npm install
```

Dev requires **both** `make run` and `make web-dev` running simultaneously.

## Environment

Copy `.env.example` to `.env`. Required for first boot:
- `GODRIVE_BOOTSTRAP_ADMIN_PASSWORD` — sets admin password once, ignored on subsequent starts
- `GODRIVE_DATA_ROOT` — filesystem root for user files
- `GODRIVE_DB_PATH` — SQLite path (auto-migrated on startup)

Notable dev variables:
- `GODRIVE_DEV_LATENCY=10ms-25ms` — injects random API latency (used by `make run`)
- `GODRIVE_ENABLE_WATCHER=true` — watches filesystem for external changes

## Architecture

### Backend (`internal/`)

**Source of truth is the filesystem.** SQLite stores metadata only — users, sessions, trash records, TUS upload state, and a **rebuildable file index** (`file_index` table). If the index is lost, `POST /api/admin/jobs/reindex` rebuilds it by walking the data root.

| Package | Responsibility |
|---|---|
| `cmd/godrive/main.go` | Config load, DB open, migrations, bootstrap admin, graceful shutdown |
| `internal/server/` | HTTP mux, middleware, JSON encoding, route handlers |
| `internal/files/service.go` | All filesystem ops — list, mkdir, move, trash, restore, upload finalization |
| `internal/store/` | SQLite schema + migrations; all DB queries |
| `internal/auth/` | Argon2id password hashing, session tokens, CSRF tokens |
| `internal/preview/` | Preview kind classification (image/video/pdf/text) |
| `internal/config/` | Env-based config loading |
| `internal/watch/` | fsnotify watcher that invalidates file index on external changes |

**Auth flow:** `POST /api/auth/login` returns a session cookie plus `X-CSRF-Token` response header. Cookie auth requires `X-CSRF-Token` on all mutating requests. Bearer token auth (stored in `localStorage`) skips CSRF. Both paths go through `withUser()` middleware.

**TUS upload flow:** `POST /api/tus` creates upload record + temp file. `PATCH /api/tus/{id}` streams chunks. On final chunk, `files.Service.FinalizeUpload()` moves temp to target, applying `_01`/`_02` numeric suffixes on conflicts.

**Trash:** Physical files move to `{APPDATA}/trash/{user_id}/{trash_id}/file`. Metadata (original path, size, deleted_at) stored in SQLite. Restore moves back; permanent delete calls `os.RemoveAll`.

**SQLite pragmas:** WAL journal mode, foreign keys on, 5s busy timeout.

### Frontend (`web/src/`)

Single Svelte 5 component (`App.svelte`) — no router. UI states: loading → login form → file manager.

| File | Role |
|---|---|
| `App.svelte` | All UI: topbar, file manager shell, modals (trash, admin, viewer), upload queue |
| `lib/api.ts` | Typed fetch wrappers for every backend endpoint; TUS client |
| `lib/svar.ts` | Converts `FileEntry` (backend shape) to `SvarFile` (SVAR component format) |

**File manager component:** `@svar-ui/svelte-filemanager` wrapped in `<Willow>` (loads Willow icon font + Open Sans from `cdn.svar.dev`). Custom `icons` prop generates inline SVG data URIs for folders and file types. Custom `previews` prop returns thumbnail URLs. Custom `menuOptions` filters out unsupported actions (copy, paste, add-file).

**File manager API intercepts** in `initFilemanager()` — all return `false` to suppress default SVAR behavior and handle operations against the backend instead:
- `request-data` → `GET /api/files/list` then `provide-data`
- `create-file` → TUS upload or `POST /api/files/mkdir`
- `rename-file` → `POST /api/files/move`
- `delete-files` → `POST /api/files/bulk/delete` (moves to trash, not permanent)
- `move-files` → `POST /api/files/bulk/move`
- `download-file` → `GET /api/files/download` (blob save to disk)
- `open-file` → lightbox viewer (images) or `show-preview` panel (video/pdf/text)

**URL sync:** Current folder path is kept in `?path=` query param. `popstate` events navigate the file manager on browser back/forward.

**Upload queue:** Independent of the SVAR component. Files selected via hidden `<input type="file">` are queued in `uploadQueue[]` state and uploaded sequentially via `uploadTus()`. Progress tracked per-item; retry is supported.
