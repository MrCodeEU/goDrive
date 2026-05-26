# Deployment

## Docker (recommended)

Production images publish to `ghcr.io/<owner>/godrive` on `v*` tags. Multi-arch: `amd64` and `arm64`.

```bash
# Edit volume paths in deploy/docker-compose.yml first
docker compose -f deploy/docker-compose.yml up -d
```

Volume layout:

| Volume | Purpose |
|---|---|
| `/data` | User files — back this up |
| `/appdata` | Database, trash, upload staging — back this up |
| `/cache` | Preview thumbnails — rebuildable, no backup needed |

## Reverse Proxy

Always run goDrive behind a reverse proxy that handles HTTPS.

**Caddy example:**

```caddyfile
files.example.com {
  reverse_proxy localhost:8121
}
```

**nginx example:**

```nginx
server {
  listen 443 ssl;
  server_name files.example.com;

  client_max_body_size 0;  # TUS handles chunking, don't limit here

  location / {
    proxy_pass http://127.0.0.1:8121;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_request_buffering off;  # required for TUS streaming
    proxy_buffering off;
  }
}
```

Set these env vars when running behind HTTPS:

```bash
GODRIVE_COOKIE_SECURE=true
GODRIVE_HSTS=true
```

## Unraid / NAS

Use the compose template in `deploy/` with clear volume separation:

```bash
deploy/docker-compose.yml        # standard production
deploy/docker-compose.demo.yml   # public demo instance
deploy/docker-compose.local.yml  # local dev
```

## Demo Instance

For a disposable public demo, use the hardened demo profile:

```bash
docker compose -f deploy/docker-compose.demo.yml up -d
```

Sets `GODRIVE_DEMO_MODE=true` — all write and admin mutation APIs are blocked. See `docs/demo-instance.md` for full details.

## Backups

At minimum back up:

- `GODRIVE_DATA_ROOT` — your files
- `GODRIVE_APPDATA_DIR` — database + trash

The preview cache (`/cache`) is fully rebuildable via admin → Preview Warmup.

## Graceful shutdown

On `SIGTERM`, in-flight requests drain for up to 15 seconds before the process exits. Docker stop/restart is safe.
