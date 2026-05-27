# WebDAV

goDrive exposes a WebDAV endpoint at `/dav/` for native filesystem clients.

## Supported clients

| Client | Platform | Notes |
|---|---|---|
| Finder | macOS | Connect via `⌘K` → `http(s)://host/dav/` |
| Files app | iOS / iPadOS | Add server under Browse → ... → Connect to Server |
| rclone | Any | Use `webdav` remote type |
| Cyberduck | macOS / Windows | WebDAV (HTTP/HTTPS) connection |
| WinSCP | Windows | WebDAV protocol |
| Any WebDAV client | — | Standard RFC 4918 |

## Authentication

WebDAV tries Basic Auth first, then falls back to bearer/cookie auth.

**Basic Auth** (recommended for native clients like Finder and iOS Files):

```
Username: your goDrive username
Password: your goDrive password
```

Basic Auth attempts share the same rate limiter as web login — too many failed attempts from the same IP are blocked temporarily.

**Bearer token**: pass an API key in the `Authorization: Bearer <token>` header. Preferred for rclone and scripted access.

**Cookie auth**: accepted for read operations. Mutating requests (`PUT`, `DELETE`, `MKCOL`, `MOVE`, `COPY`, `LOCK`) via cookie require a valid `X-CSRF-Token` header. In practice use Basic or Bearer for any client that issues writes.

## Connecting with rclone

```bash
rclone config
# Type: webdav
# URL: https://your-server/dav/
# Vendor: other
# User: your-username
# Password: your-password (or use --webdav-bearer-token for an API key)
```

## Scope and limitations

WebDAV is a file-access surface — it does not expose goDrive-specific features:

- No search
- No trash management (deleted files via WebDAV bypass the goDrive trash)
- No preview generation or thumbnail access
- No admin operations, webhooks, or API key management

Index updates after WebDAV writes rely on the filesystem watcher and periodic reconciliation scanner. There may be a short delay (seconds to minutes) before changes appear in the web/mobile UI.

!!! warning
    WebDAV is disabled in demo mode.

## macOS Finder tips

- Use HTTPS in production — Basic Auth over plain HTTP sends credentials in the clear.
- Finder caches credentials in Keychain. If you change your password, remove the old entry from Keychain Access.
- For large transfers, rclone or Cyberduck are more reliable than Finder's built-in WebDAV client.
