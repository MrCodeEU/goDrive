# goDrive Specification

goDrive is a self-hosted file management system for a small family setup. It prioritizes a normal filesystem layout, reliable uploads, fast browsing, and high-quality previews over broad enterprise features.

## Goals

- Serve around 5 users.
- Manage an existing nested folder structure with roughly 400k images plus normal files.
- Use the underlying local filesystem as the source of truth.
- Keep files visible as normal paths and filenames, without content-addressed storage, opaque UUID filenames, or app-specific storage layouts.
- Support external filesystem changes made outside goDrive, such as SMB, shell, rsync, or Docker bind mounts.
- Provide fast web browsing, upload, download, rename, move, delete, restore, and preview workflows.
- Provide solid resumable uploads through TUS.
- Provide mobile apps for iOS and Android, written in Flutter.
- Keep the first version simple enough to build and maintain as a home/family service.

## Non-Goals

- No multi-tenant SaaS design.
- No enterprise sharing or ACL model.
- No file versioning.
- No built-in quota system.
- No object storage backend in the first version.
- No distributed/multi-node deployment in the first version.
- No dependency on Nextcloud/oCIS storage layout.
- No requirement that the app owns all filesystem modifications.

## Technology Decisions

- Backend: Go.
- Web UI: Svelte/Vite spike using `@svar-ui/svelte-filemanager`, with the original embedded vanilla UI kept as a fallback during evaluation.
- Mobile: Flutter.
- Database: SQLite.
- Upload protocol: TUS-compatible HTTP endpoints implemented in Go.
- Filesystem watcher: `fsnotify`, backed by inotify on Linux.
- Image preview engine: `libvips`, likely through a Go binding or controlled worker process.
- Video preview engine: `ffmpeg`.
- PDF preview: renderer to be selected later, for example Poppler, MuPDF, or browser-native PDF view for full documents.
- Text/code/Markdown preview: server reads bounded text content and the web client renders it safely.
- 3D preview: web-side Three.js viewer for common model formats such as `glb`, `gltf`, `stl`, `obj`, and `ply`.
- Deployment target: Docker on a local machine or NAS, especially Unraid.
- Storage target: local disk only.

## Current Implementation Status

Implemented:

- Go HTTP server with `.env`-loaded configuration.
- SQLite schema and migrations.
- Admin bootstrap.
- Users, sessions, Argon2id password hashing, cookies, bearer-token auth, and CSRF checks.
- Login rate limiting (10 failures per IP per 5 min → 15 min block).
- Safe per-user filesystem path resolution, symlink confinement.
- List (offset/limit paginated), mkdir, download, rename/move, bulk move/delete/download, delete-to-trash, trash listing, restore, and permanent delete.
- TUS-compatible upload create/head/patch/delete flow with final conflict suffixing and atomic rename.
- Inode-based thumbnail cache key — stable across renames and moves on the same filesystem.
- Post-upload thumbnail generation — thumbnails for all warmup sizes generated asynchronously immediately after finalization.
- Cached thumbnails for images (vips → ffmpeg fallback with EXIF auto-rotation), RAW photos where the local toolchain supports them, video poster frames, PDFs, and Office documents via LibreOffice headless.
- Text and Markdown preview endpoint with bounded reads.
- File index table, full reindex, and periodic reconciliation scanner.
- Preview warmup admin job with bounded goroutine worker pool, overlap prevention, and cancel support.
- Reindex batching for SQLite writes.
- Recursive fsnotify watcher indexing external changes; live root reload on user create/update; indexed changes are also published to connected UI clients.
- Webhook and event API: `upload.complete`, `file.created`, `file.moved`, `file.deleted`, `file.restored`, and external watcher change events delivered via HTTP POST with HMAC-SHA256 signature, 3-attempt retry, plus an authenticated SSE stream for live clients.
- Expired TUS upload cleanup (periodic, configurable TTL).
- Embedded Svelte/SVAR web UI: file browser, grid view with lazy thumbnails, drag-and-drop upload, live folder refresh via SSE, image/video/PDF/text/Markdown/RAW/Office/3D viewer, search, upload queue with persistence, admin modal with jobs/users/stats/webhooks.
- Folder listing truncated at configurable limit with load-more pagination.
- Docker deployment: multi-stage Dockerfile, docker-compose for Unraid/NAS layout.
- CI: GitHub Actions for backend tests, frontend type-check/test/build, Docker smoke build, multi-arch image publish, Android APK build.
- Flutter app (Android/iOS): file browser with list/grid view, image viewer, in-app video player, text preview, upload queue with TUS resume, admin screen, wakelock during uploads.

Partially implemented:

- Flutter app background upload needs real-device hardening. Android supports continuing selected uploads in a foreground service with a progress notification; iOS supports continuing selected uploads through native background `URLSession`. Foreground uploads include file picker, camera/photo-library image picker, TUS resume metadata persistence, retry controls, and screen-awake handling during active uploads.
- Watcher reconciliation protects against missed events but a brief gap after restart or overflow remains.
- WebDAV not implemented (not needed — Unraid exposes SMB).

Not implemented yet:

- Deep scan diagnostics and subfolder repair tooling.

## Filesystem Model

The filesystem is authoritative. The database stores metadata, index state, preview state, sessions, users, and operational records, but it must not become the only source of truth for files.

Each user has one configured home root:

```text
/data/users/alice
/data/users/bob
```

The application exposes paths relative to the user's home root:

```text
/Photos/2026/IMG_0001.HEIC
/Documents/manual.pdf
```

The app must prevent path traversal and must never allow a request to escape the configured user root.

Shared folders are not a first-class application feature in the first version. If needed, they are handled outside goDrive by Docker bind mounts, for example mounting the same host folder into multiple user homes.

Because the same underlying directory can appear in multiple user roots through bind mounts, indexing must preserve the user-visible path. The app should not assume inode identity alone is enough to identify a file in the UI.

## User Model

- One admin account.
- Normal user accounts.
- Each normal user has exactly one home root.
- No groups or ACLs in the first version.
- No app-managed sharing in the first version.
- Admin can create, disable, and reset users.
- Admin can assign or change each user's home root.

## Authentication And Sessions

- Password authentication.
- Passwords hashed with Argon2id.
- Cookie-based web sessions.
- CSRF protection for browser state-changing requests.
- API token or app session mechanism for mobile clients.
- Session revocation support.
- Rate limiting for login attempts.
- HTTPS expected at deployment, either directly or behind a reverse proxy.

## File Operations

The first version should support:

- List folder.
- Create folder.
- Download file.
- Upload file through TUS.
- Rename file or folder.
- Move file or folder.
- Delete to trash.
- Restore from trash.
- Permanently delete from trash.
- Open preview where supported.

All operations must be implemented using safe filesystem APIs. Paths from clients are always treated as logical paths relative to a user root and resolved server-side.

## Data Integrity And Concurrency

File writes should be staged and then finalized with an atomic rename where the source and destination are on the same filesystem.

Operations that target the same logical path should be serialized inside the backend where practical. This avoids races between upload finalization, rename, move, delete, restore, and preview generation.

The app must tolerate external modifications that happen concurrently through SMB, shell, rsync, or other tools. If an external change wins a race, goDrive should return a clear conflict or not-found response and let the watcher/reconciliation system repair the index.

The first version should not try to implement distributed locks or cross-process filesystem leases.

## Uploads

Uploads use TUS directly over HTTP. WebDAV is not required for TUS.

Upload behavior:

- Resumable uploads.
- Upload into a temporary app-managed staging area.
- Atomic move into the final destination after upload completion.
- Preserve the original filename when no conflict exists.
- Resolve name conflicts by appending numeric suffixes before the extension.

Conflict example:

```text
photo.jpg
photo_01.jpg
photo_02.jpg
```

If `photo_01.jpg` already exists, the server continues to the next available suffix.

The upload API should return the final resolved path to the client.

The server must handle:

- Interrupted uploads.
- Expired unfinished uploads.
- Disk write errors.
- Destination folder deleted during upload.
- Filename normalization issues.
- Duplicate names on case-insensitive clients.

## Mobile Uploads

The first mobile version can use foreground uploads. Android also supports a foreground-service upload path for manually selected files that should continue while the app is backgrounded. iOS supports a native background `URLSession` path for manually selected files.

Mobile requirements:

- User selects files/images manually.
- Upload queue persists locally.
- TUS resume URL persists locally.
- Failed uploads can retry.
- Per-file status is visible.
- The app can keep the screen awake during active uploads.
- Uploads should tolerate app restarts where the platform allows it.

Native background upload paths must be tested on real devices with large photo/video batches because simulator behavior does not fully match background scheduling on battery.

## Trash

Delete means move to trash, not immediate permanent deletion.

Trash should live outside the visible user data tree:

```text
/appdata/trash/<user-id>/<trash-id>/file
/appdata/trash/<user-id>/<trash-id>/meta.json
```

Trash metadata stores:

- Original logical path.
- Original filename.
- User id.
- Deletion time.
- File type.
- Size where known.

Restoring from trash uses the same conflict suffix behavior as uploads if the original path is occupied.

The trash directory is durable app data. Losing it means trashed files are lost.

## Preview System

Previews are generated into an app-managed cache outside the user data tree:

```text
/appdata/previews
```

Preview cache entries should be keyed by enough information to invalidate stale previews when a file changes. A practical first version can use:

- User id.
- Logical path.
- File size.
- Modified time.
- Optional inode/device information.

The preview cache is rebuildable and should not be required for backup correctness.

Preview generation should be on-demand first, with a background queue for warming common thumbnails.

Initial preview support:

- JPEG.
- PNG.
- WebP.
- HEIC, if the deployed `libvips` build supports it.
- AVIF, if the deployed `libvips` build supports it.
- GIF still preview, animation later if needed.
- Video poster frame through `ffmpeg`.
- RAW photo thumbnails where the deployed `libvips`/`ffmpeg` toolchain supports the camera format.
- Office document first-page thumbnails through LibreOffice headless plus PDF rendering.
- Plain text.
- Markdown.
- PDF first page or browser view.
- 3D models through a lazy-loaded Three.js viewer in the web client.

The preview system should be internally modular:

```text
CanHandle(file)
GeneratePreview(file, variant)
ExtractMetadata(file)
```

Preview generation must be bounded so huge or malformed files cannot exhaust memory, CPU, or disk.

External preview tool invocations must use per-file deadlines, bounded stderr/stdout capture, isolated temp HOME/XDG/TMP directories, process-group cleanup, and a child death signal. On Linux hosts with `prlimit`, preview commands also run with CPU time, address-space, output-file, and open-file limits. Default timeout is `GODRIVE_PREVIEW_TIMEOUT=45s`; `0` falls back to the built-in default rather than disabling protection.

Current preview variants:

- `240` for small thumbnails.
- `420` for card/grid thumbnails.
- `1024` for detail/lightbox previews.

Preview warmup workers:

- Controlled by `GODRIVE_PREVIEW_WORKERS`.
- `0` means automatic sizing.
- Automatic sizing uses roughly half of available CPUs, clamped between 2 and 64 workers.

## Indexing And Watchers

External filesystem modifications are a hard requirement.

The app uses two mechanisms:

- Live filesystem notifications through `fsnotify`.
- Periodic reconciliation scan.

Watchers are used for fast updates. The reconciliation scanner is used for correctness after missed events, restarts, Docker mount changes, or watcher overflow.

The index should track:

- User id.
- Logical path.
- File or directory type.
- Size.
- Modified time.
- Optional inode/device.
- Last scan time.
- Preview status.

The filename search index can start in SQLite. Search scope for the first version:

- Filename.
- Relative path.
- Bounded full-text search for text and Markdown files.
- Folder navigation.

Full-text document search starts with SQLite FTS5 over the first bounded text window of text/Markdown files. Later options include Apache Tika, Bluge, Meilisearch, or Elasticsearch/OpenSearch behind the same store/search boundary if richer ranking or binary document extraction becomes necessary.

## Web UI

The web UI is a functional file manager, not a marketing page.

Required views:

- Login.
- File browser.
- Upload queue.
- Preview modal or preview pane.
- Trash.
- Admin user management.
- Basic server/settings page.

File browser requirements:

- Fast folder loading.
- Breadcrumb navigation.
- Back/forward friendly routing.
- URL path routing uses `/files/<logical-path>` in the Svelte/SVAR UI. Query routing with `?path=` is considered backward compatibility only.
- Grid/list switch eventually, list first is acceptable.
- Sort by name, modified time, size, and type.
- Create folder.
- Upload files.
- Rename.
- Move.
- Delete.
- Restore from trash.
- Download.
- Preview.

Very large folders should not require loading every item into the browser at once. The API should eventually support cursor-based incremental loading. This is a later requirement because folders around 1k entries are acceptable for the current UI, but unusually large folders still need protection.

## Mobile Apps

The mobile apps should provide the same core user-facing file workflow as the web UI, but can be simpler in the first version.

Mobile browser requirements:

- Login.
- Browse folders.
- Search by filename/path when backend search exists.
- Open previews.
- Download/open files through the OS where practical.
- Upload selected files/images.
- Show upload queue and retry failures.
- Create folder.
- Rename.
- Move.
- Delete to trash.

The first mobile version does not need admin screens.

## WebDAV

WebDAV is optional compatibility functionality and is separate from TUS.

WebDAV goals:

- Allow generic clients to browse, download, upload, rename, and delete.
- Reuse the same auth/user-root/path resolution rules.
- Keep behavior simple and predictable.

WebDAV is not required for the first coding milestone unless explicitly selected as the next task.

## Backup And Restore

Data should be separated into three categories:

1. User files.
2. Durable app data.
3. Rebuildable cache.

User files:

```text
/data/users
```

Durable app data:

```text
/appdata/godrive.db
/appdata/trash
/appdata/config.yaml
```

Rebuildable cache:

```text
/appdata/previews
/appdata/tmp
```

The service should be recoverable if the preview cache is deleted.

If the database is lost but the config and user files remain, the app should eventually support rebuilding the file index. User accounts may need backup unless config-based bootstrap admin recovery is added.

## Security Requirements

- Strict root confinement for every file operation.
- No path traversal.
- No symlink escape from user roots unless explicitly allowed later.
- Passwords hashed with Argon2id.
- Login rate limiting.
- Secure cookies.
- CSRF protection for browser requests.
- Mobile API auth separated from browser CSRF assumptions.
- MIME sniffing must not grant dangerous behavior.
- Text and Markdown previews must be sanitized.
- Preview generation must run with resource limits.
- Archive extraction is not part of the first version.
- Quotas are intentionally omitted in the first version.

## Performance Requirements

- Folder listing should feel fast for normal folders.
- Large folders must be paginated or cursor-listed.
- Previews should be cached.
- Preview generation must not block folder listing.
- Initial scan of 400k files can take time, but progress should be visible.
- The app should remain usable while scans and preview jobs run.
- Database queries should be indexed around user id, path, parent path, name, modified time, and type.
- Avoid reading image metadata or generating thumbnails during ordinary folder listing unless already cached.
- Preview warmup should run in a bounded worker pool.
- Reindex writes should be batched so SQLite is not hit once per file.

## Configuration

Configuration should support:

- HTTP listen address.
- Public base URL.
- App data path.
- User data root defaults.
- Per-user home paths.
- Preview cache path.
- Temporary upload path.
- Max upload size optional, default unlimited or very high.
- TUS upload expiration.
- Scanner interval.
- Log level.
- Preview worker count.
- Development latency injection.

Environment variables should be supported for Docker-friendly deployment, with a config file for structured values.

Currently supported environment variables include:

```text
GODRIVE_ADDR
GODRIVE_DATA_ROOT
GODRIVE_APPDATA_DIR
GODRIVE_DB_PATH
GODRIVE_UPLOAD_DIR
GODRIVE_PREVIEW_DIR
GODRIVE_TRASH_DIR
GODRIVE_BOOTSTRAP_ADMIN_USER
GODRIVE_BOOTSTRAP_ADMIN_PASSWORD
GODRIVE_BOOTSTRAP_ADMIN_ROOT
GODRIVE_SESSION_COOKIE
GODRIVE_CSRF_COOKIE
GODRIVE_COOKIE_SECURE
GODRIVE_SESSION_TTL
GODRIVE_ENABLE_WATCHER
GODRIVE_RECONCILE_INTERVAL
GODRIVE_UPLOAD_TTL
GODRIVE_PREVIEW_WORKERS
GODRIVE_PREVIEW_TIMEOUT
GODRIVE_DEV_LATENCY
```

## API Shape

Exact endpoints can change during implementation, but the first version should roughly provide:

```text
POST   /api/auth/login
POST   /api/auth/logout
GET    /api/me

GET    /api/files?path=/Photos&cursor=...
POST   /api/files/folder
POST   /api/files/rename
POST   /api/files/move
DELETE /api/files
GET    /api/files/download?path=...

ALL    /api/uploads/tus/*

GET    /api/previews?path=...&variant=...

GET    /api/trash
POST   /api/trash/restore
DELETE /api/trash

GET    /api/admin/users
POST   /api/admin/users
PATCH  /api/admin/users/{id}
```

## Implementation Todos

Status markers:

- Done: implemented and tested at baseline level.
- Active: implemented partly or under current iteration.
- Next: recommended near-term work.
- Later: deferred.

### 1. Project Skeleton

Status: Done.

Create the repository structure for backend, web, and future mobile clients.

Suggested structure:

```text
/cmd/godrive
/internal/auth
/internal/config
/internal/db
/internal/files
/internal/indexer
/internal/preview
/internal/trash
/internal/upload
/web
/mobile
/deploy
```

This establishes boundaries before code grows around one large main package.

### 2. Backend Bootstrap

Status: Done.

Implement a Go HTTP server with config loading, structured logging, graceful shutdown, and health endpoints.

This gives every later feature a stable runtime foundation.

### 3. SQLite Schema And Migrations

Status: Done.

Add migrations for users, sessions, file index entries, trash entries, preview jobs, and upload records.

This should happen early because auth, indexing, trash, uploads, and previews all need durable state.

### 4. Config And Local Paths

Status: Done.

Implement app path configuration and validation.

The server should refuse unsafe or missing roots clearly. It should create app-owned directories such as tmp, trash, and previews where appropriate.

### 5. Auth

Status: Done.

Implement admin bootstrap, login, logout, sessions, password hashing, and middleware.

The first admin can be created from environment variables or a one-time setup command.

### 6. Filesystem Path Layer

Status: Done.

Implement one central package that converts user-visible logical paths into safe absolute filesystem paths.

This package must handle path cleaning, root confinement, symlink policy, conflict suffixes, and common file metadata.

All file operations, uploads, previews, trash, WebDAV, and scanning must use this layer.

### 7. File Browser API

Status: Active.

Implement folder listing, folder creation, download, rename, move, and delete-to-trash.

Server-side pagination or cursor loading is deferred until after core upload/search/viewer work, unless real test data shows very large single folders are common.

### 8. Trash

Status: Done.

Implement moving files into app-managed trash, storing metadata, listing trash, restore, and permanent delete.

Restore should handle conflicts with the same suffix logic used by uploads.

### 9. TUS Uploads

Status: Active.

Integrate TUS upload handling.

Uploads should write into app-managed temporary storage and finalize with an atomic move into the destination folder. Finalization should create a file index update and return the final resolved path.

### 10. Indexer

Status: Active.

Implement initial recursive scan per user root.

Track scan progress and write file index records to SQLite. The first indexer can be simple and correct before optimizing.

### 11. Filesystem Watcher

Status: Active.

Add `fsnotify` watchers for user roots.

Watcher events should enqueue index updates rather than doing expensive work directly in the event callback.

Current state:

- Active user roots are watched recursively at startup.
- Newly created directories are watched recursively.
- Create/write/chmod events upsert file index metadata.
- Remove/rename events delete the path and indexed descendants.
- Admin user create/update reloads watcher roots live, so new active home roots and changed active home roots are watched without a server restart.

### 12. Reconciliation Scanner

Status: Active.

Add periodic reconciliation scans.

This protects against missed events, watcher overflow, downtime, and external bulk changes.

Current state:

- A periodic scanner starts from `GODRIVE_RECONCILE_INTERVAL`.
- Default interval is `24h`; set it to `0` to disable.
- Reconciliation reuses the full reindex logic and skips itself when another admin job is already running. Admin-started jobs are cancelable.

### 13. Preview Job System

Status: Active.

Implement preview job records, a worker pool, cache paths, and invalidation.

This should be generic before adding many format handlers.

### 14. Image Previews

Status: Active.

Add image thumbnail generation through `libvips`.

Start with JPEG, PNG, and WebP. Add HEIC and AVIF based on the deployed library capabilities.

### 15. Video Previews

Status: Active.

Add video poster frame generation through `ffmpeg`.

The server should detect missing `ffmpeg` and degrade gracefully.

### 16. Text, Markdown, And PDF Previews

Status: Active.

Add safe bounded text previews, sanitized Markdown rendering, and PDF first-page or browser-native preview support.

This completes the practical preview baseline.

### 17. Web UI Skeleton

Status: Done.

Create the Svelte/SvelteKit app with login, authenticated layout, API client, and route structure.

Do this after backend auth and file APIs exist, so the UI can integrate against real endpoints.

### 18. Web File Browser

Status: Active.

Build folder browsing, breadcrumbs, sorting, pagination, upload button, download, rename, move, delete, and preview entry points.

List view should come first. Grid view can follow after previews are reliable.

Current state:

- SVAR supports cards, table, and panels modes.
- View mode is stored in browser `localStorage`.
- Current folder path is stored in the URL as `/files/...`.
- Image viewer supports previous/next navigation, cached high-resolution previews, zoom controls, wheel zoom, drag panning, and inline original image viewing.
- Server-side pagination/cursor loading is deferred, but still required before this is safe for unusually large single folders.

### 19. Web Upload Queue

Status: Active.

Add TUS uploads in the web UI with progress, pause/resume where supported, retry, and final path display.

Current state:

- Page-session upload queue.
- Per-file progress.
- Retry for failed files while the selected `File` handles remain available.
- Up to 3 concurrent uploads.
- `beforeunload` warning while uploads are queued or active.
- UI messaging that browser file handles are temporary and lost on reload.

### 20. Web Trash And Admin UI

Status: Active.

Add trash restore/permanent delete and basic admin user management.

Admin UI can stay minimal.

Current state:

- Trash listing, restore, and permanent delete are available.
- Admin modal shows stats, reindex/preview jobs, user creation, user editing, disable/admin toggles, home root editing, and password reset.
- Admin stats show watcher enabled state, watched root/path counts, and reconciliation interval.
- Admin stats show preview worker/sizing settings, running jobs can be canceled, and the rebuildable preview cache can be cleared from the admin modal.

### 21. Docker Deployment

Status: Done.

`Dockerfile` — three-stage build: node web assets → Go binary (CGO_ENABLED=0) → Debian slim runtime with `ffmpeg`, `libvips-tools`, `poppler-utils`.

`deploy/docker-compose.yml` — Unraid/NAS layout with `/mnt/user/godrive/{data,appdata}` volumes.
`deploy/docker-compose.local.yml` — local dev compose with localhost-only bind and `GODRIVE_COOKIE_SECURE=false`.

Volume categories:

```text
/data          → user files (back up)
/appdata       → durable state: godrive.sqlite, trash/, config (back up)
/appdata/previews  → rebuildable preview cache (safe to delete)
/appdata/uploads   → in-progress TUS uploads (safe to delete when server is down)
```

Reverse proxy: place nginx or Caddy in front, set `GODRIVE_COOKIE_SECURE=true`, and proxy `/` to the container port.

Multi-arch images (`linux/amd64`, `linux/arm64`) are built and pushed to `ghcr.io` by the `docker-publish.yml` workflow on version tags.

### 22. WebDAV Compatibility

Status: Later.

Add WebDAV after the core browser and TUS flow works.

It should reuse auth, path resolution, trash/delete policy, and index update hooks.

### 23. Flutter App Skeleton

Status: Active.

Create Flutter app structure, login, session storage, file browser, and preview/open behavior.

Current state:

- `mobile/` Flutter project with Provider state management.
- Login screen with server URL + credentials; token stored in `FlutterSecureStorage`.
- Files screen: folder listing, breadcrumb navigation, sort, search, create folder, rename, delete-to-trash, download/open via `url_launcher`.
- Image viewer: `PhotoViewGallery` with next/previous, pinch-zoom, cached network images, original/preview toggle.
- Text/Markdown preview: fetches `/api/files/text`, renders in scrollable sheet.
- Video and PDF: opens via `url_launcher` in external app (native player/viewer).
- Trash screen: list, restore, permanent delete.

### 24. Flutter Upload Queue

Status: Active.

Implement manual file/image selection, persistent upload queue, TUS resume support, retry behavior, and screen-awake option.

Current state:

- `file_picker` for multi-file selection.
- `UploadQueue` ChangeNotifier with up to 3 concurrent uploads.
- TUS client: create → PATCH with streaming progress; HEAD-based resume on retry.
- Queue metadata persisted to `SharedPreferences` (`done`/`interrupted` states survive restart).
- `UploadQueueSheet` bottom sheet with progress bars, status badges, clear-done button.
- Retry respects existing TUS URL (resumes) or falls back to fresh upload if gone.
- Android can hand selected uploads to a native foreground service. The service creates/resumes TUS uploads, streams file bytes while the app is backgrounded, shows a progress notification, and writes final status back to the persisted queue.
- iOS can hand selected uploads to a native background `URLSession`. The bridge creates/resumes TUS uploads, schedules the PATCH as a background upload task, and writes progress/final status back to the persisted queue.

Missing:

- Real-device hardening for Android foreground-service and iOS background-URLSession uploads with large photo/video batches.

### 25. Extended Search

Status: Active.

Add better search after the core file manager is stable.

Start with SQLite filename/path search. Later evaluate SQLite FTS, Apache Tika, Meilisearch, or Elasticsearch/OpenSearch for full-text search.

Current state:

- Indexed filename/path search is implemented through SQLite FTS5 with LIKE fallback for short/symbol queries.
- Text and Markdown content search is implemented through a bounded SQLite FTS5 document index populated by reindex and watcher updates.
- The SVAR search field can search the per-user index and navigate to matching folders or parent folders for files.
- Search freshness depends on watcher/indexer updates or a manual reindex after external filesystem changes.

### 26. Extended Preview Handlers

Status: Later.

Add Office, RAW, and 3D previews only after the core preview system is stable.

These features should plug into the existing preview job interface rather than changing file browser logic.

### 27. Test Coverage

Status: Active.

Add tests around the risky core:

- Path traversal prevention.
- Root confinement.
- Conflict suffix generation.
- Trash restore behavior.
- Upload finalization.
- Index reconciliation.
- Preview invalidation.
- Auth/session behavior.

End-to-end tests can be added after the web UI exists.

### 28. CI And Release Pipeline

Status: Active.

GitHub Actions workflows under `.github/workflows/`:

| File | Trigger | Purpose |
|---|---|---|
| `ci.yml` | push/PR to main | Backend tests (race), frontend type-check + test + build, Docker build smoke test |
| `docker-publish.yml` | `v*` tags | Build multi-arch image (`linux/amd64`, `linux/arm64`) and push to `ghcr.io` |
| `mobile.yml` | changes to `mobile/` | Android APK build + Flutter test; iOS placeholder |

#### Docker release

Tag a release to publish:

```sh
git tag v0.1.0
git push origin v0.1.0
```

Image published to `ghcr.io/<owner>/godrive:0.1.0` and `:latest`.

#### Android build

Requirements in CI: Ubuntu runner, `actions/setup-java` (Temurin 21), `subosito/flutter-action` (stable).

Local debug build:

```sh
cd mobile
flutter pub get
flutter test
flutter build apk --debug
```

Release build (signed):

```sh
flutter build apk --release  # requires key.jks + key.properties
flutter build appbundle       # for Play Store
```

Signing: set `ANDROID_KEYSTORE_BASE64`, `KEYSTORE_PASSWORD`, `KEY_ALIAS`, `KEY_PASSWORD` as repository secrets and decode at build time.

#### iOS build

iOS CI requires macOS runners and costs real money on GitHub-hosted runners. Options:

1. **Local**: `flutter build ios --release` in Xcode with a paid Apple Developer account.
2. **Self-hosted macOS runner**: a Mac mini on the local network registered as a GitHub Actions runner. Cheapest for private repos.
3. **Third-party CI**: Codemagic or Bitrise both have free tiers with iOS support.

Requirements regardless of platform:
- Apple Developer account ($99/year).
- Distribution certificate (`Certificates, Identifiers & Profiles`).
- Provisioning profile (Ad Hoc for direct install, App Store for distribution).
- Bundle ID registered (`com.example.godrive` or similar).

The `mobile.yml` workflow contains a commented-out iOS job skeleton. Enable it on a macOS runner once the above is in place.

TestFlight distribution is the recommended path for family testing before App Store submission.

### 29. Webhook And Event API

Status: Active.

Current state: Implemented. `webhooks` SQLite table, CRUD API (`GET/POST /api/webhooks`, `DELETE/POST /api/webhooks/{id}/test`), async delivery with independent per-delivery contexts, HMAC-SHA256 signing, 3-attempt retry. Events wired: `upload.complete`, `file.created`, `file.moved`, `file.deleted`, `file.restored`, `file.external_changed`, and `file.external_deleted`. Authenticated SSE endpoint (`GET /api/events`) streams the same event payloads to live clients.

goDrive emits structured events when files change. External services subscribe by registering a webhook endpoint. This allows autonomous post-processing pipelines (sorting, conversion, deduplication, backup) to react to file activity without polling the filesystem or fighting over inotify.

#### Design goals

- Decoupled: subscribers are separate processes, any language, any host.
- Reliable: at-least-once delivery, retry with exponential backoff.
- Verifiable: HMAC-SHA256 signature on every delivery.
- Filterable: subscribers register interest in specific event types.
- Operable: admin can list, test, and delete subscriptions via UI and API.

#### Event types

| Event | When fired |
|---|---|
| `upload.complete` | `FinalizeUpload` atomically moves temp file to destination |
| `file.created` | Folder created via API |
| `file.moved` | File or folder renamed or moved via API |
| `file.deleted` | File moved to trash or permanently deleted |
| `file.restored` | File restored from trash |
| `file.external_changed` | Watcher indexed a filesystem create/write/chmod change |
| `file.external_deleted` | Watcher removed an indexed filesystem path |
| `index.scan.done` | Full reindex or reconciliation scan completes for a user |

#### Payload schema

Every delivery is a POST to the subscriber URL with `Content-Type: application/json`:

```json
{
  "id": "evt_01jx…",
  "event": "upload.complete",
  "timestamp": "2026-05-12T09:14:00Z",
  "user_id": 1,
  "username": "alice",
  "data": {
    "path": "/uploads/IMG_0042.HEIC",
    "old_path": null,
    "size": 4200000,
    "mime_type": "image/heic",
    "preview_kind": "image"
  }
}
```

`old_path` is set for `file.moved`. All other `data` fields are present for all events where applicable.

#### Security

Each webhook subscription has a randomly generated `secret` (32 bytes, base64url). goDrive adds:

```
X-GoDrive-Signature: sha256=<hmac-sha256(secret, body)>
X-GoDrive-Event: upload.complete
X-GoDrive-Delivery: evt_01jx…
```

Subscribers verify the signature before acting. This prevents replay attacks and spoofing.

#### Delivery semantics

- Delivery is fire-and-forget from the request handler; dispatched via a bounded goroutine worker pool.
- 3 attempts, backoff: 0s → 5s → 30s.
- Timeout per attempt: 10s.
- Failed deliveries are logged; no dead-letter queue in v1.
- If a subscriber returns non-2xx, it counts as failure and triggers retry.

#### Subscription management API

```text
GET    /api/webhooks              list subscriptions (admin only)
POST   /api/webhooks              register a new subscription
DELETE /api/webhooks/{id}         remove a subscription
POST   /api/webhooks/{id}/test    send a test ping event
```

Registration body:

```json
{
  "url": "https://sorter.internal/godrive-events",
  "events": ["upload.complete", "file.moved"],
  "description": "HEIC sorter"
}
```

Response includes the generated `secret`. Secret is shown once and not retrievable later.

#### SQLite schema

```sql
CREATE TABLE webhooks (
  id          TEXT PRIMARY KEY,
  url         TEXT NOT NULL,
  secret_hash TEXT NOT NULL,
  events      TEXT NOT NULL,   -- JSON array of event type strings
  description TEXT NOT NULL DEFAULT '',
  created_at  TEXT NOT NULL,
  updated_at  TEXT NOT NULL
);
```

The raw secret is never stored; only `HMAC(secret, "verify")` for display purposes is kept. The actual secret is used as-is when signing deliveries.

Wait — more precisely: the secret is stored in plaintext in the DB (like a session token) because it is needed to sign outgoing requests. It is not a user password; it is a symmetric shared secret between goDrive and the subscriber. It should be treated as sensitive data.

#### Environment variable

`GODRIVE_WEBHOOK_WORKERS` — number of concurrent webhook delivery goroutines. Default: 4.

### 30. Integration Patterns

Status: Active.

This section describes how to build external services that integrate with goDrive via the webhook and file API.

#### Pattern: post-upload file processor

A processor receives `upload.complete`, performs work, and manipulates files via the goDrive API. This avoids direct filesystem access outside goDrive, preserving index coherence and thumbnail cache validity.

Example flow for a HEIC-to-JPEG converter:

```
1. Processor subscribes to upload.complete where mime_type=image/heic
2. Uploads arrive at /uploads/ folder
3. goDrive fires upload.complete { path: /uploads/IMG_0042.HEIC, mime_type: image/heic }
4. Processor downloads the original:
     GET /api/files/download?path=/uploads/IMG_0042.HEIC
5. Processor converts HEIC → JPEG locally using vips/ffmpeg
6. Processor uploads converted file via TUS to target folder:
     POST /api/tus?path=/Photos/2026   (goDrive applies conflict suffix if needed)
     → upload.complete fires for the new .jpg (thumbnails generated immediately)
7. Processor deletes original:
     DELETE /api/files?path=/uploads/IMG_0042.HEIC
     → file.deleted fires
```

The processor uses a standard bearer token for auth. Create a dedicated user account for automated processors — it gets its own home root and session token, and admin can revoke it independently.

#### Pattern: folder sorter / organizer

A sorter subscribes to `file.created` on a specific inbox folder and moves files to date-organised destinations using `POST /api/files/move`. Because the move goes through the goDrive API, the file index and thumbnail cache (keyed by inode) remain valid without regeneration.

```
file.created { path: /inbox/photo.jpg }
→ sorter reads EXIF date
→ POST /api/files/move { from: /inbox/photo.jpg, to: /Photos/2026/05/photo.jpg }
→ file.moved fires with old_path + new_path
→ goDrive updates index; thumbnail cache key (inode-based) remains valid
```

#### Pattern: backup / offsite sync

Subscribe to `file.created`, `file.moved`, `file.deleted`. On each event, sync the delta to an offsite store (S3, Backblaze, rsync target). Avoids full-scan polling; events give exact change sets.

#### Pattern: real-time event stream (SSE)

For browser-based dashboards or admin tooling:

```
GET /api/events    (auth required, SSE stream)
```

Returns a `text/event-stream` of the same JSON payloads as webhook deliveries. No subscription registration needed — connect and receive. Intended for live monitoring, not reliable processing (no retry, connection-lifetime only).

Status: Implemented for web UI live refresh and browser/admin clients. Mobile can use the same endpoint later, but needs lifecycle-aware reconnect/backoff.

#### Processor authentication

Processors authenticate as a normal goDrive user with a bearer token. No special processor account type. Recommended setup:

1. Admin creates a user `sorter` with home root pointing to the inbox folder.
2. Admin generates a long-lived session token (future: dedicated API token endpoint).
3. Processor stores token securely and uses it for all API calls.
4. Admin can disable the `sorter` user to instantly stop all processor activity.

### 31. Inode-Based Thumbnail Cache And Post-Upload Generation

Status: Done.

#### Problem

The current thumbnail cache key is derived from `(version, userID, logicalPath, fileSize, mtime, thumbSize)`. Moving a file changes `logicalPath`, invalidating the cache and forcing regeneration. For large photo libraries this causes visible lag when browsing reorganised folders.

Additionally, thumbnails are currently generated on first request. Uploading a batch of photos and immediately browsing them in grid view results in every thumbnail being generated on-demand, causing 300-500ms delays per cell.

#### Solution A: inode-based cache key

Change cache key to include `(inode, device)` from `syscall.Stat_t` in place of `logicalPath`. Inode is stable across renames and moves on the same filesystem. `mtime` and `size` are retained to detect content changes.

New key inputs: `(version, userID, inode, device, size, mtime, thumbSize)`

On Linux the `os.FileInfo` underlying `sys` field exposes `syscall.Stat_t` with `Ino` and `Dev`.

Falls back gracefully: if inode is unavailable (non-Linux or cross-FS copy), the old path-based key is used. A copy creates a new inode → cache miss → regenerate (correct behaviour).

#### Solution B: post-upload thumbnail generation

After `FinalizeUpload` atomically moves the temp file to its destination, launch async background goroutines to generate thumbnails for all configured warmup sizes (`previewWarmupSizes`). The upload response returns immediately; thumbnail generation happens in the background.

This means: newly uploaded files have thumbnails ready within seconds, before the user navigates to the folder. Combined with the inode-based key, subsequent moves of those files reuse the already-generated thumbnails.

Implementation:
- `FinalizeUpload` returns the resolved `Entry` including physical path.
- TUS `completeUpload` calls a new `Server.generateThumbnailsAsync(user, entry)` method.
- The method spawns at most `previewWarmupWorkerCount` goroutines, one per size.
- Progress is not tracked (fire-and-forget); errors are logged, not fatal.

#### Interaction with webhook system

`upload.complete` fires after `FinalizeUpload` but before thumbnail generation completes. The webhook payload includes `path`, `mime_type`, and `preview_kind`. Subscribers that immediately request thumbnails may get a cache miss on the first request if generation hasn't finished yet; the thumbnail endpoint handles this by generating on-demand as a fallback.

The `index.scan.done` event fires after reconciliation, signalling that subscribers can request thumbnails for newly indexed files.

### 32. Operational Tooling

Add admin/maintenance commands:

- Implemented: create user/admin, reset password, verify config paths, show index/trash/preview status, run full reindex, reindex one user, repair/reindex one file or subfolder, run preview warmup, clear preview cache, and clear expired/orphaned uploads.
- Job diagnostics include indexed count, failure count, stale index deletions, and scoped user/path metadata where applicable.

These commands are important for a self-hosted system where manual recovery should be straightforward.

### 33. Hardening And Scale Backlog

Status: Next.

Near-term implementation order:

1. Harden preview workers: per-command timeouts, bounded command output, isolated command homes, process-group cleanup, child death signaling, and opportunistic `prlimit` CPU/memory/file limits are implemented. Remaining: clearer degraded-mode errors and, if deployments need it, a stricter container/seccomp sandbox profile for preview workers.
2. Cursor pagination: stable cursor pagination is implemented, with offset/limit kept temporarily for compatibility. Folder listing now uses the SQLite file index when a folder is indexed, falling back to the filesystem when not indexed.
3. SQLite benchmark and tuning: synthetic 400k-file index benchmark is implemented, including folder-list cursor timings. Current SQLite setup already enables WAL, foreign keys, and a 5s busy timeout on open. Filename/path search uses SQLite FTS5 trigram with LIKE fallback for short/symbol searches. Remaining: benchmark-driven tuning for very broad folder cursor pages and write amplification.
4. Watcher reliability: watcher health, suspicious-error rescan flagging, live root reload, event debounce/coalescing, and automatic reconciliation after watcher rescan flags are implemented. Remaining: expose deeper watcher diagnostics if real deployments show missed-event patterns.
5. Operational CLI: verify config paths, create/reset admin, full/user/scoped reindex, preview warmup, status, clear preview cache, expired/orphaned upload cleanup, and basic job diagnostics are implemented. Remaining: only add deeper scan diagnostics if real deployments show unexplained drift.
6. Background upload polish: Android notification permission prompt, iOS background `URLSession`, and live queue refresh from native progress/completion are implemented. Remaining: real-device hardening with large uploads and interrupted network/app lifecycle cases.
7. FTS and broad previews: filename/path search, bounded text/Markdown full-text search, and browser-side 3D model previews are implemented. Search uses SQLite FTS5; the 3D viewer lazy-loads Three.js for `glb`, `gltf`, `stl`, `obj`, and `ply`. Remaining: binary document text extraction only if needed, plus real-world validation across larger preview fixtures. Extended previews should prefer proven command-line renderers (`libreoffice` headless for Office, `dcraw`/`libraw` tooling for RAW where practical, browser/Three.js viewers for 3D) behind the same bounded preview worker controls.
8. Release validation: a full security audit plan and manual end-to-end test plan are documented in `docs/security-audit.md` and `docs/manual-test-plan.md`. Remaining: execute them against a clean release candidate and fix findings.

API tokens are not required for a five-user family deployment, but scoped/revocable tokens still make sense if processors, mobile devices, or integrations need long-lived access. The concrete value is revocation and blast-radius control: disabling one automation token should not require changing a user's password or invalidating every session. Defer this until integrations become real.

## Suggested First Milestone

The first milestone should be backend-only:

- Go server starts.
- SQLite migrations run.
- Admin login works.
- One user root can be configured.
- Safe path layer exists.
- Folder listing works.
- Download works.
- Create folder works.
- Rename/move works.
- Delete-to-trash and restore work.

This milestone proves the filesystem model before uploads, previews, WebDAV, or mobile clients add complexity.

## Suggested Second Milestone

- TUS upload endpoint.
- Upload finalization into user folders.
- Conflict suffix handling.
- Initial recursive scanner.
- Filename/path index in SQLite.
- Basic fsnotify integration.
- Reconciliation scan.

This milestone proves external-change support and reliable upload behavior.

## Suggested Third Milestone

- Preview job queue.
- Image previews.
- Video poster previews.
- Text/Markdown/PDF previews.
- Svelte web UI file browser.
- Web upload queue.

This milestone makes the system useful as a daily browser.

## Suggested Fourth Milestone

- Web trash UI.
- Admin user UI.
- Docker deployment.
- WebDAV compatibility.
- Flutter app skeleton.
- Flutter upload queue.

This milestone moves from prototype to usable self-hosted service.
