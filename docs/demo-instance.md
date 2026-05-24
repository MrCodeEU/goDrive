# Demo Instance

The demo deployment is intentionally separate from the normal production Docker image. It is designed for a public, disposable instance that can be exposed through a reverse proxy without allowing persistent writes.

## What The Demo Allows

- Login with the demo account.
- Browse seeded files.
- Search indexed files immediately after startup.
- Preview/download safe sample files, including SVG image samples, Markdown, CSV, JSON, and project notes.
- Prefill the web login form with demo credentials.
- Open a read-only admin UI with stats, users, current job state, and API key list visible.
- Show an in-app banner that warns users the instance is public, disposable, and read-only.

Default credentials:

```text
username: demo
password: demo
```

## What The Demo Blocks

When `GODRIVE_DEMO_MODE=true`, the server rejects:

- WebDAV.
- Admin mutation APIs. Read-only admin endpoints stay enabled so the demo can show the admin UI.
- Webhook APIs.
- TUS uploads.
- Trash mutations.
- File create, move, delete, edit, and upload paths.
- API key management.

The demo container includes the same preview toolchain as the production image: LibreOffice, ffmpeg, poppler, and libvips. This keeps the public demo on the real preview path while demo mode still disables write/admin surfaces for safety.

The seed dataset is generated at container startup. It includes hundreds of deterministic SVG image samples, optional seeded Picsum JPEG photos, a deeply nested folder tree, Markdown/CSV/JSON/code/office fixtures, generated PDF/video examples when the preview tools are present, store-copy examples, and simple OBJ 3D models. The generator keeps working offline; if Picsum cannot be reached it records the skipped images and continues. The defaults can be adjusted with `GODRIVE_DEMO_IMAGE_COUNT`, `GODRIVE_DEMO_REMOTE_IMAGE_COUNT`, `GODRIVE_DEMO_REMOTE_IMAGE_SOURCE`, `GODRIVE_DEMO_NESTED_DEPTH`, and `GODRIVE_DEMO_MODEL_COUNT`.

## Run Locally

```sh
docker compose -f deploy/docker-compose.demo.yml up --build
```

Then put a reverse proxy in front of:

```text
http://127.0.0.1:8081
```

The compose file binds only to localhost. Keep it that way unless another firewall layer constrains access.

## Reset Model

The demo uses `tmpfs` for `/data`, `/appdata`, and `/tmp`.

Data resets when the container restarts:

```sh
docker compose -f deploy/docker-compose.demo.yml restart godrive-demo
```

For an auto-resetting public demo, schedule that restart from the host, for example with systemd timer or cron.

## CI/CD

The `Docker Publish` GitHub Actions workflow publishes the demo image as:

```text
ghcr.io/mrcodeeu/godrive-demo:latest
```

After a relevant `main` branch change builds and pushes the demo image, the workflow sends a `repository_dispatch` event to `MrCodeEU/homelab-automation`:

```json
{
  "event_type": "service-update",
  "client_payload": {
    "service": "godrive-demo",
    "tag": "latest",
    "environment": "production",
    "commit_sha": "<source commit>",
    "image": "ghcr.io/mrcodeeu/godrive-demo:latest"
  }
}
```

This requires a `DISPATCH_TOKEN` repository secret in this repo with permission to dispatch workflows in `MrCodeEU/homelab-automation`.

Main-branch demo publishes build only the homelab target platform to conserve hosted Actions minutes. Version tags and manual dispatch can build multi-arch images. The production Docker image is tag/manual only. The demo image intentionally includes the full preview stack, so this keeps the public demo and production preview paths aligned.

## Container Hardening

The demo compose file sets:

- Non-root container user.
- Read-only root filesystem.
- `tmpfs` for writable paths.
- `cap_drop: [ALL]`.
- `no-new-privileges:true`.
- PID, memory, and CPU limits.
- Short session lifetime.
- HTTPS/HSTS cookie assumptions for reverse-proxy deployment.

The tmpfs limits need to leave room for preview cache and LibreOffice scratch data. The checked-in compose uses a larger `/tmp` and `/appdata` than the old lightweight image because the demo now exercises the real preview stack.

## Reverse Proxy Notes

Use HTTPS at the reverse proxy. The demo expects secure cookies:

```text
GODRIVE_COOKIE_SECURE=true
GODRIVE_HSTS=true
```

Do not enable webhook private-network delivery or HTTP webhook delivery on a public demo.
