# Demo Instance

The demo deployment is intentionally separate from the normal production Docker image. It is designed for a public, disposable instance that can be exposed through a reverse proxy without allowing persistent writes.

## What The Demo Allows

- Login with the demo account.
- Browse seeded files.
- Search indexed files after the first index run.
- Preview/download safe sample files.

Default credentials:

```text
username: demo
password: demo
```

## What The Demo Blocks

When `GODRIVE_DEMO_MODE=true`, the server rejects:

- WebDAV.
- Admin APIs.
- Webhook APIs.
- TUS uploads.
- Trash mutations.
- File create, move, delete, edit, and upload paths.
- API key management.

The demo container also omits LibreOffice, ffmpeg, poppler, and libvips to reduce container attack surface. It should not be used to validate full preview-tool behavior.

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

After the demo image is pushed from a successful `main` CI run, the workflow sends a `repository_dispatch` event to `MrCodeEU/homelab-automation`:

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

The production Docker image is built after the demo image succeeds. This keeps the demo deployment path fast and uses the lightweight demo image as the first Docker publish quality gate before starting the heavier preview-tool image.

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

## Reverse Proxy Notes

Use HTTPS at the reverse proxy. The demo expects secure cookies:

```text
GODRIVE_COOKIE_SECURE=true
GODRIVE_HSTS=true
```

Do not enable webhook private-network delivery or HTTP webhook delivery on a public demo.
