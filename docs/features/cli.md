# CLI Reference

The `godrive` binary doubles as a server and a maintenance CLI. All CLI commands open the database directly — the server does not need to be running (and should not be running simultaneously for write commands, to avoid SQLite contention).

## Usage

```
godrive <command> [flags]
```

Default command when none is given: `serve`.

## Commands

### `serve`

Start the HTTP server. Reads all config from environment / `.env`.

```bash
godrive serve
# or just:
godrive
```

---

### `status`

Print a quick summary of database state, index stats, trash, and preview cache size.

```bash
godrive status
```

Example output:

```
database: ./var/appdata/godrive.sqlite
users: 3
index: 412847 files, 8201 dirs, 892104832 bytes, 410201 preview candidates
trash: 14 items, 204800 bytes
preview cache: 61023 files, 4831838208 bytes
```

---

### `verify`

Check that all required directories exist and are accessible. Also reports which preview tools (`ffmpeg`, `vips`, `pdftoppm`, `soffice`) are found and their paths.

```bash
godrive verify
```

Useful after a fresh deployment or volume remount to confirm everything is wired up before starting the server.

---

### `reindex`

Rebuild the file index from disk. Safe to run against a live installation — the server handles concurrent reindex via the admin job system when triggered from the UI, but the CLI variant runs directly.

```bash
# Reindex all users
godrive reindex

# Reindex one user
godrive reindex --user alice

# Reindex a specific path for one user
godrive reindex --user alice --path /Photos/2024
```

| Flag | Description |
|---|---|
| `--user USER` | Limit reindex to this username |
| `--path /path` | Limit to this logical path (requires `--user`) |

---

### `preview-warmup`

Pre-generate thumbnails for all files that don't have cached previews yet.

```bash
godrive preview-warmup
```

Runs in the foreground. Useful after initial setup with a large existing file collection.

---

### `preview-cache`

Manage the preview cache directory.

```bash
# Delete all cached preview files
godrive preview-cache clear
```

After clearing, thumbnails are regenerated on demand as files are browsed or via `preview-warmup`.

---

### `uploads cleanup`

Delete stale incomplete upload records and their staging files older than the configured TTL.

```bash
godrive uploads cleanup

# Override the TTL
godrive uploads cleanup --ttl 24h
```

| Flag | Default | Description |
|---|---|---|
| `--ttl DURATION` | `GODRIVE_UPLOAD_TTL` | Delete uploads older than this |

The server runs this automatically every 6 hours. The CLI variant is useful for immediate cleanup or scripted maintenance.

---

### `admin create`

Create a new user directly in the database.

```bash
godrive admin create --username bob --password secret --root /data/bob
godrive admin create --username carol --password secret --root /data/carol --admin
```

| Flag | Required | Description |
|---|---|---|
| `--username` | Yes | Login username |
| `--password` | Yes | Initial password (Argon2id hashed at rest) |
| `--root` | Yes | Absolute path to user's home directory |
| `--admin` | No | Grant admin privileges |

---

### `admin reset-password`

Reset a user's password without logging in.

```bash
godrive admin reset-password --username alice --password newpassword
```

| Flag | Required | Description |
|---|---|---|
| `--username` | Yes | Target username |
| `--password` | Yes | New password |

---

## Docker usage

When running inside Docker, prefix commands with `docker exec`:

```bash
docker exec godrive godrive status
docker exec godrive godrive reindex --user alice
docker exec godrive godrive verify
```
