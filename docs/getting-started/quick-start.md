# Quick Start

Get goDrive running in under 5 minutes with Docker.

## Prerequisites

- Docker + Docker Compose
- A directory of files you want to manage

## 1. Clone and configure

```bash
git clone https://github.com/MrCodeEU/goDrive.git
cd goDrive
cp .env.example .env
```

Edit `.env` and set at minimum:

```bash
GODRIVE_BOOTSTRAP_ADMIN_PASSWORD=change-me   # (1)
GODRIVE_DATA_ROOT=/path/to/your/files        # (2)
GODRIVE_ADDR=0.0.0.0:8121
```

1. Change this before first run. The bootstrap admin is created on first startup.
2. Absolute path to the folder containing your files.

## 2. Start

```bash
docker compose -f deploy/docker-compose.yml up -d
```

Open [http://localhost:8121](http://localhost:8121) and log in with the admin credentials you set.

## 3. Local development (no Docker)

```bash
make run        # Go backend on :8121
make web-dev    # Svelte dev server on :5173 (proxies /api to :8121)
```

Open [http://127.0.0.1:5173/files](http://127.0.0.1:5173/files).

## Next steps

- [Configuration](configuration.md) — all environment variables
- [Deployment](deployment.md) — production setup with reverse proxy + HTTPS
- [Mobile Apps](../features/mobile.md) — Android and iOS setup
