# goDrive

goDrive is a small self-hosted file manager for local disks. The filesystem is the source of truth; SQLite stores users, sessions, trash records, resumable upload state, and rebuildable operational metadata.

## Current State

Implemented foundation:

- Go HTTP API.
- SQLite migrations.
- Argon2id password hashing.
- Cookie and bearer-token sessions.
- CSRF checks for cookie-authenticated writes.
- Per-user home roots.
- Safe logical path resolution under each user root.
- Folder listing, mkdir, download, move/rename, delete-to-trash, restore, and permanent trash delete.
- Minimal TUS 1.0-compatible upload creation, resume, patch, and termination endpoints.
- Preview type classification for images, video, PDF, text, Markdown, and 3D files.
- Cached thumbnails for image, video, and PDF preview candidates.
- Admin reindex and preview warmup jobs with progress.
- Batched reindex writes and parallel preview warmup workers.
- Recursive fsnotify watcher that logs external filesystem changes.
- Embedded vanilla web UI.
- Experimental Svelte/Vite/SVAR file-manager UI under `web/`.
- Svelte upload queue with per-file progress and retry for failed uploads.

Flutter clients, WebDAV, and full-text search are not implemented yet.

## Run Locally

First boot requires an admin password:

```sh
GODRIVE_BOOTSTRAP_ADMIN_PASSWORD=change-me go run ./cmd/godrive
```

Or create `.env` from `.env.example` and use:

```sh
make run
```

Defaults:

- API address: `127.0.0.1:8121` when using `.env.example`; built-in fallback is `127.0.0.1:8080`
- Data root: `./var/data`
- App data: `./var/appdata`
- Admin username: `admin`
- Admin home root: `./var/data/admin`

The web UI is served at `/` on the configured address.

After the first admin exists, `GODRIVE_BOOTSTRAP_ADMIN_PASSWORD` is ignored.

## Svelte/SVAR UI

The SVAR spike runs separately during development:

```sh
make run
make web-dev
```

Open:

```text
http://127.0.0.1:5173/files
```

Vite proxies `/api` and `/health` to `http://127.0.0.1:8121`.

Folder navigation is stored in the browser URL:

```text
/files/Photos/2026/Trip
```

The older `?path=/Photos/2026/Trip` form is still accepted and normalized on the next navigation. UI preferences such as SVAR view mode are stored in `localStorage`, not cookies.

Useful frontend commands:

```sh
make web-install
make web-check
make web-build
```

## Admin Jobs

Admin jobs currently support:

- Full reindex.
- Preview warmup.

Preview warmup generates cached `240`, `420`, and `1024` thumbnails. Worker count is controlled by:

```text
GODRIVE_PREVIEW_WORKERS=0
```

`0` means automatic sizing: half the CPU count, clamped between 2 and 64 workers. On small NAS hardware, start with `2` to `4`. On a stronger local server, `8` to `16` is a reasonable first test.

The preview cache is rebuildable. User data and trash are not.

## Current Roadmap

Near-term:

- Verify larger multi-file upload batches against real phone/camera exports.
- Persist the upload queue across page reloads where browser `File` handles allow it, or add clear recovery messaging.
- Add current-folder and indexed filename search.
- Add server-backed pagination/cursor listing before testing very large single folders.
- Improve image viewer with next/previous navigation, zoom/pan, and higher-resolution preview/original viewing.
- Make admin settings more complete: worker count, preview sizes, cache stats, and reindex status.

Later:

- WebDAV compatibility.
- Flutter app skeleton.
- Flutter upload queue.
- Extended previews for Office, RAW, and 3D files.
- Full-text search with SQLite FTS/Tika or an external engine.

## Useful API Calls

Login:

```sh
curl -c /tmp/godrive.cookies \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"change-me"}' \
  http://127.0.0.1:8080/api/auth/login
```

List files:

```sh
curl -b /tmp/godrive.cookies 'http://127.0.0.1:8080/api/files/list?path=/'
```

TUS upload flow:

```sh
TOKEN='<token returned by /api/auth/login>'

LOCATION=$(curl -i \
  -H "Authorization: Bearer ${TOKEN}" \
  -H 'Tus-Resumable: 1.0.0' \
  -H 'Upload-Length: 11' \
  -H 'Upload-Metadata: filename aGVsbG8udHh0' \
  -X POST 'http://127.0.0.1:8080/api/tus?path=/' \
  | awk '/^Location:/ {print $2}' | tr -d '\r')

printf 'hello world' | curl -i \
  -H "Authorization: Bearer ${TOKEN}" \
  -H 'Tus-Resumable: 1.0.0' \
  -H 'Content-Type: application/offset+octet-stream' \
  -H 'Upload-Offset: 0' \
  --data-binary @- \
  -X PATCH "http://127.0.0.1:8080${LOCATION}"
```

For cookie-authenticated state-changing API requests, send `X-CSRF-Token` using the value returned by login. Bearer-token clients can send `Authorization: Bearer <token>` instead.

## Verify

```sh
make test
make web-check
make web-build
```
