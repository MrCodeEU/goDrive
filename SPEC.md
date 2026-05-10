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
- 3D preview: later feature, likely web-side Three.js for formats such as `glb`, `gltf`, `stl`, and `obj`.
- Deployment target: Docker on a local machine or NAS, especially Unraid.
- Storage target: local disk only.

## Current Implementation Status

Implemented:

- Go HTTP server with `.env`-loaded configuration.
- SQLite schema and migrations.
- Admin bootstrap.
- Users, sessions, Argon2id password hashing, cookies, bearer-token auth, and CSRF checks.
- Safe per-user filesystem path resolution.
- List, mkdir, download, rename/move, delete-to-trash, trash listing, restore, and permanent delete.
- TUS-compatible upload create/head/patch/delete flow with final conflict suffixing.
- Cached thumbnails for images, video poster frames, and PDFs.
- Text preview endpoint with bounded reads.
- File index table and admin full reindex.
- Preview warmup admin job with bounded goroutine worker pool.
- Reindex batching for SQLite writes.
- Recursive fsnotify watcher logging external changes.
- Embedded web UI.
- Experimental Svelte/SVAR UI with file browser, thumbnails, topbar upload, trash modal, admin modal, `/files/...` URL path sync, persisted view mode, and image lightbox.
- Svelte upload queue with per-file progress and retry for failed uploads.

Partially implemented:

- Admin UI exists but is still minimal.
- Uploads work at the API level and the Svelte UI has a page-session upload queue. Queue persistence across reloads is not implemented.
- Preview warmup is parallelized, but worker count is only environment-configured.
- Watcher logs changes but does not yet fully reconcile index state from every event.

Not implemented yet:

- WebDAV.
- Flutter apps.
- Server-backed search.
- Large-folder pagination/cursor listing.
- Full upload queue with retry/resume UI.
- User management in the Svelte/SVAR UI.
- Full text search.

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

The first mobile version can use foreground uploads. Full native background upload support is deferred.

Mobile requirements:

- User selects files/images manually.
- Upload queue persists locally.
- TUS resume URL persists locally.
- Failed uploads can retry.
- Per-file status is visible.
- The app can keep the screen awake during active uploads.
- Uploads should tolerate app restarts where the platform allows it.

Native background workers can be added later if foreground uploads are too unreliable for large manual batches.

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
- Plain text.
- Markdown.
- PDF first page or browser view.

Later preview support:

- Office documents through LibreOffice headless or Apache Tika plus renderer.
- RAW photo formats if a reliable local toolchain is selected.
- 3D models through Three.js in the web client.

The preview system should be internally modular:

```text
CanHandle(file)
GeneratePreview(file, variant)
ExtractMetadata(file)
```

Preview generation must be bounded so huge or malformed files cannot exhaust memory, CPU, or disk.

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
- Folder navigation.

Full-text document search is deferred. Later options include Apache Tika, SQLite FTS, Meilisearch, or Elasticsearch/OpenSearch.

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

Large folders should not require loading every item into the browser at once. The API should support pagination or cursor-based listing.

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
GODRIVE_PREVIEW_WORKERS
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

Start with server-side pagination or limit/offset so large folders do not overload the web UI.

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

### 12. Reconciliation Scanner

Status: Next.

Add periodic reconciliation scans.

This protects against missed events, watcher overflow, downtime, and external bulk changes.

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
- Server-side pagination is still required before this is safe for very large single folders.

### 19. Web Upload Queue

Status: Active.

Add TUS uploads in the web UI with progress, pause/resume where supported, retry, and final path display.

### 20. Web Trash And Admin UI

Status: Active.

Add trash restore/permanent delete and basic admin user management.

Admin UI can stay minimal.

### 21. Docker Deployment

Status: Later.

Add Dockerfile, compose example, Unraid-friendly volume layout, and reverse proxy notes.

The deployment must make clear which paths are user data, durable app data, and rebuildable cache.

### 22. WebDAV Compatibility

Status: Later.

Add WebDAV after the core browser and TUS flow works.

It should reuse auth, path resolution, trash/delete policy, and index update hooks.

### 23. Flutter App Skeleton

Status: Later.

Create Flutter app structure, login, session storage, file browser, and preview/open behavior.

This should start after the backend API stabilizes.

### 24. Flutter Upload Queue

Status: Later.

Implement manual file/image selection, persistent upload queue, TUS resume support, retry behavior, and screen-awake option.

Native background upload is deferred unless real-device testing proves foreground uploads are not acceptable.

### 25. Extended Search

Status: Next.

Add better search after the core file manager is stable.

Start with SQLite filename/path search. Later evaluate SQLite FTS, Apache Tika, Meilisearch, or Elasticsearch/OpenSearch for full-text search.

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

### 28. Operational Tooling

Add admin/maintenance commands:

- Create/reset admin.
- Rescan user.
- Rebuild preview cache.
- Clear temporary uploads.
- Show scan status.
- Verify config paths.

These commands are important for a self-hosted system where manual recovery should be straightforward.

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
