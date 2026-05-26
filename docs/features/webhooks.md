# Webhooks

goDrive emits HMAC-signed events for file lifecycle changes.

## Events

| Event | Trigger |
|---|---|
| `upload.complete` | File upload finalized |
| `file.moved` | File renamed or moved |
| `file.deleted` | File moved to trash |
| `file.restored` | File restored from trash |

## Delivery

Each webhook subscription gets:

- `POST` request to your configured URL
- JSON body with event type and file metadata
- `X-GoDrive-Signature` header — HMAC-SHA256 of the body, keyed by your subscription secret

```go
// Verify signature (Go example)
mac := hmac.New(sha256.New, []byte(secret))
mac.Write(body)
expected := hex.EncodeToString(mac.Sum(nil))
```

## Managing subscriptions

Subscriptions are managed in the admin panel (Web UI, Android, iOS) or via the API.

Per-subscription settings:

- Target URL
- Secret key for HMAC signing
- Event filter (subscribe to specific events only)

## Configuration

```bash
GODRIVE_WEBHOOK_WORKERS=4            # concurrent delivery workers
GODRIVE_WEBHOOK_ALLOW_HTTP=false     # allow non-HTTPS targets
GODRIVE_WEBHOOK_ALLOW_PRIVATE=false  # allow RFC-1918 / localhost targets
```

!!! warning
    `GODRIVE_WEBHOOK_ALLOW_PRIVATE=true` enables SSRF to internal network addresses. Only enable in controlled environments.

## Retry behavior

Failed deliveries are retried with backoff. Delivery state is visible in the admin panel.
