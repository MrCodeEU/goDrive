# Configuration

All configuration is via environment variables, typically set in `.env`.

## Required

| Variable | Default | Description |
|---|---|---|
| `GODRIVE_DATA_ROOT` | `./var/data` | Root directory for user files |
| `GODRIVE_BOOTSTRAP_ADMIN_PASSWORD` | `change-me` | Admin password set on first startup |

## Server

| Variable | Default | Description |
|---|---|---|
| `GODRIVE_ADDR` | `127.0.0.1:8121` | Listen address and port |
| `GODRIVE_APPDATA_DIR` | `./var/appdata` | Directory for database, trash, upload staging |
| `GODRIVE_DB_PATH` | `./var/appdata/godrive.sqlite` | SQLite database path |

## Authentication & Security

| Variable | Default | Description |
|---|---|---|
| `GODRIVE_BOOTSTRAP_ADMIN_USER` | `admin` | Admin username set on first startup |
| `GODRIVE_BOOTSTRAP_ADMIN_ROOT` | `./var/data/admin` | Admin user home directory |
| `GODRIVE_COOKIE_SECURE` | `false` | Set `true` when serving over HTTPS |
| `GODRIVE_COOKIE_SAMESITE` | `strict` | Cookie SameSite policy |
| `GODRIVE_HSTS` | `false` | Send HSTS header (requires HTTPS) |
| `GODRIVE_SESSION_TTL` | `720h` | Session expiry duration |

## Uploads

| Variable | Default | Description |
|---|---|---|
| `GODRIVE_UPLOAD_TTL` | `48h` | How long to keep incomplete uploads before cleanup |

## File Watching & Sync

| Variable | Default | Description |
|---|---|---|
| `GODRIVE_ENABLE_WATCHER` | `true` | Watch filesystem with fsnotify for external changes |
| `GODRIVE_RECONCILE_INTERVAL` | `24h` | How often to run the full reconciliation scanner |

## Previews

| Variable | Default | Description |
|---|---|---|
| `GODRIVE_PREVIEW_WORKERS` | `0` (auto) | Number of thumbnail generation workers. 0 = CPU count |
| `GODRIVE_PREVIEW_TIMEOUT` | `45s` | Per-file preview generation timeout |

## Webhooks

| Variable | Default | Description |
|---|---|---|
| `GODRIVE_WEBHOOK_WORKERS` | `4` | Concurrent webhook delivery workers |
| `GODRIVE_WEBHOOK_ALLOW_HTTP` | `false` | Allow non-HTTPS webhook targets (not recommended in production) |
| `GODRIVE_WEBHOOK_ALLOW_PRIVATE` | `false` | Allow webhook delivery to private/RFC-1918 addresses |

## Production checklist

```bash
GODRIVE_COOKIE_SECURE=true    # HTTPS only
GODRIVE_HSTS=true             # after TLS confirmed working
GODRIVE_BOOTSTRAP_ADMIN_PASSWORD=<strong-password>
GODRIVE_WEBHOOK_ALLOW_HTTP=false
```

!!! warning
    Never expose goDrive directly to the public internet without TLS, a reverse proxy, rate limiting, and backups.
    See [Deployment](deployment.md) for the full production setup guide.
