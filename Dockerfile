FROM node:22-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
COPY internal/server/static/ ../internal/server/static/
RUN npm run build

FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/internal/server/static/ ./internal/server/static/
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /godrive ./cmd/godrive

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    ffmpeg \
    libvips-tools \
    libreoffice \
    poppler-utils \
    && rm -rf /var/lib/apt/lists/*

COPY --from=build /godrive /usr/local/bin/godrive

VOLUME ["/data", "/appdata"]
EXPOSE 8080

ENV GODRIVE_ADDR=0.0.0.0:8080
ENV GODRIVE_DATA_ROOT=/data
ENV GODRIVE_APPDATA_DIR=/appdata
ENV GODRIVE_DB_PATH=/appdata/godrive.sqlite
ENV GODRIVE_UPLOAD_DIR=/appdata/uploads
ENV GODRIVE_PREVIEW_DIR=/appdata/previews
ENV GODRIVE_TRASH_DIR=/appdata/trash
ENV GODRIVE_BOOTSTRAP_ADMIN_USER=admin
ENV GODRIVE_COOKIE_SECURE=true
ENV GODRIVE_ENABLE_WATCHER=true
ENV GODRIVE_RECONCILE_INTERVAL=24h
ENV GODRIVE_UPLOAD_TTL=48h
ENV GODRIVE_WEBHOOK_WORKERS=4
ENV GODRIVE_PREVIEW_WORKERS=0

ENTRYPOINT ["/usr/local/bin/godrive"]
