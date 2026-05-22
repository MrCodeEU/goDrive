#!/bin/sh
set -eu

: "${GODRIVE_DEMO_USER:=demo}"
: "${GODRIVE_DEMO_PASSWORD:=demo}"

reset_dir() {
	dir="$1"
	mkdir -p "$dir"
	find "$dir" -mindepth 1 -maxdepth 1 -exec rm -rf {} +
}

reset_dir /data
reset_dir /appdata

mkdir -p /data/demo/Documents /data/demo/Photos /data/demo/Projects
cat > /data/demo/README.md <<'EOF'
# goDrive Demo

This is a disposable demo account. Data is reset whenever the demo container restarts.

Uploads, file edits, WebDAV, webhooks, trash mutations, API keys, and admin actions are disabled in demo mode.
EOF

cat > /data/demo/Documents/notes.txt <<'EOF'
Welcome to goDrive.

This demo shows browsing, search, previews, downloads, and the mobile/web file experience without allowing persistent writes.
EOF

cat > /data/demo/Documents/example.csv <<'EOF'
name,type,status
demo,environment,reset-on-restart
uploads,mutation,disabled
webhooks,egress,disabled
EOF

cat > /data/demo/Projects/demo-plan.md <<'EOF'
# Demo Plan

- Browse folders
- Search indexed files
- Preview text and Markdown
- Download sample files
EOF

/usr/local/bin/godrive admin create \
	--username "$GODRIVE_DEMO_USER" \
	--password "$GODRIVE_DEMO_PASSWORD" \
	--root /data/demo \
	--admin=false >/dev/null

exec /usr/local/bin/godrive serve
