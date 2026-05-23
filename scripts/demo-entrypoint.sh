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

mkdir -p \
	/data/demo/Documents/Reports \
	/data/demo/Documents/Notes \
	/data/demo/Documents/Office \
	/data/demo/Photos/Travel \
	/data/demo/Photos/Product \
	/data/demo/Projects/Launch \
	/data/demo/Projects/Engineering \
	/data/demo/Design \
	/data/demo/Data
cat > /data/demo/README.md <<'EOF'
# goDrive Demo

This is a disposable demo account. Data is reset whenever the demo container restarts.

Uploads, file edits, WebDAV, webhooks, trash mutations, API keys, and admin actions are disabled in demo mode.
EOF

cat > /data/demo/Documents/notes.txt <<'EOF'
Welcome to goDrive.

This demo shows browsing, search, previews, downloads, and the mobile/web file experience without allowing persistent writes.
EOF

cat > /data/demo/Documents/Notes/meeting-notes.md <<'EOF'
# Product Review Notes

- Improve mobile layout before public launch.
- Keep demo data disposable and resettable.
- Use the feature matrix as the source of truth for parity decisions.
- Test WebDAV through Finder, iOS Files, and rclone before release.
EOF

cat > /data/demo/Documents/Reports/storage-summary.csv <<'EOF'
area,files,size_mb,status
photos,128,432,healthy
documents,52,36,healthy
design,18,74,healthy
archives,4,290,review
EOF

cat > /data/demo/Documents/Reports/release-readiness.md <<'EOF'
# Release Readiness

## Done

- Demo deployment is isolated.
- Dangerous mutations are disabled in demo mode.
- Docker images are built by GitHub Actions.

## Remaining

- Store signing and listing setup.
- Responsive web UI pass.
- Public open source documentation.
EOF

cat > /data/demo/Documents/Office/demo-brief.rtf <<'EOF'
{\rtf1\ansi\deff0
{\fonttbl{\f0 Arial;}}
\fs36\b goDrive Demo Brief\b0\par
\fs24 This RTF document exercises the office preview pipeline through LibreOffice.\par
\par
\b Preview surfaces\b0\par
- Image thumbnails\par
- PDF first-page previews\par
- Office document conversion\par
- Text and Markdown previews\par
}
EOF

cat > /data/demo/Documents/Office/storage-plan.doc <<'EOF'
{\rtf1\ansi\deff0
{\fonttbl{\f0 Arial;}}
\fs32\b Storage Plan\b0\par
\fs24 This legacy .doc fixture contains RTF content so the demo can show an office-like document without embedding private or binary assets.\par
\par
The production preview stack converts office documents to PDF and then renders the first page thumbnail.\par
}
EOF

cat > /data/demo/Data/sample-customers.json <<'EOF'
[
  {"id":"cus_1001","name":"Northwind Studio","plan":"team","storage_gb":82},
  {"id":"cus_1002","name":"Blue Harbor Labs","plan":"business","storage_gb":238},
  {"id":"cus_1003","name":"Pine Valley Design","plan":"starter","storage_gb":24}
]
EOF

cat > /data/demo/Data/import-log.csv <<'EOF'
timestamp,source,files,result
2026-05-22T08:00:00Z,camera-roll,42,ok
2026-05-22T08:15:00Z,shared-drive,19,ok
2026-05-22T08:30:00Z,archive,3,skipped
EOF

cat > /data/demo/Projects/Launch/demo-plan.md <<'EOF'
# Demo Plan

- Browse folders
- Search indexed files
- Preview text and Markdown
- Download sample files
- Inspect image thumbnails
- Open file details and metadata
EOF

cat > /data/demo/Projects/Launch/checklist.txt <<'EOF'
Public demo checklist

[x] Disposable account
[x] Reset on container restart
[x] Demo credentials documented
[ ] Scheduled reset
[ ] Monitoring
[ ] Mobile responsive web UI
EOF

cat > /data/demo/Projects/Engineering/api-notes.md <<'EOF'
# API Notes

The demo account can read files, search, preview, and download content.
Uploads, edits, admin actions, WebDAV, webhooks, and API key creation are blocked by demo mode.
EOF

cat > /data/demo/Design/brand-palette.svg <<'EOF'
<svg xmlns="http://www.w3.org/2000/svg" width="1200" height="800" viewBox="0 0 1200 800">
  <rect width="1200" height="800" fill="#f7f8fb"/>
  <text x="80" y="110" font-family="Arial, sans-serif" font-size="48" font-weight="700" fill="#172033">goDrive Palette</text>
  <rect x="80" y="180" width="220" height="360" rx="18" fill="#2563eb"/>
  <rect x="340" y="180" width="220" height="360" rx="18" fill="#14b8a6"/>
  <rect x="600" y="180" width="220" height="360" rx="18" fill="#f59e0b"/>
  <rect x="860" y="180" width="220" height="360" rx="18" fill="#111827"/>
  <text x="80" y="610" font-family="Arial, sans-serif" font-size="28" fill="#172033">Primary</text>
  <text x="340" y="610" font-family="Arial, sans-serif" font-size="28" fill="#172033">Sync</text>
  <text x="600" y="610" font-family="Arial, sans-serif" font-size="28" fill="#172033">Warning</text>
  <text x="860" y="610" font-family="Arial, sans-serif" font-size="28" fill="#172033">Ink</text>
</svg>
EOF

cat > /data/demo/Design/dashboard-wireframe.svg <<'EOF'
<svg xmlns="http://www.w3.org/2000/svg" width="1400" height="900" viewBox="0 0 1400 900">
  <rect width="1400" height="900" fill="#eef2f7"/>
  <rect x="70" y="60" width="1260" height="780" rx="24" fill="#ffffff"/>
  <rect x="110" y="110" width="250" height="680" rx="16" fill="#172033"/>
  <rect x="400" y="110" width="880" height="90" rx="16" fill="#f8fafc"/>
  <rect x="400" y="240" width="260" height="180" rx="16" fill="#dbeafe"/>
  <rect x="700" y="240" width="260" height="180" rx="16" fill="#ccfbf1"/>
  <rect x="1000" y="240" width="280" height="180" rx="16" fill="#fef3c7"/>
  <rect x="400" y="460" width="880" height="330" rx="16" fill="#f8fafc"/>
  <g fill="#94a3b8">
    <rect x="440" y="510" width="760" height="20" rx="10"/>
    <rect x="440" y="570" width="690" height="20" rx="10"/>
    <rect x="440" y="630" width="720" height="20" rx="10"/>
    <rect x="440" y="690" width="620" height="20" rx="10"/>
  </g>
</svg>
EOF

cat > /data/demo/Photos/Travel/mountain-lake.svg <<'EOF'
<svg xmlns="http://www.w3.org/2000/svg" width="1600" height="1000" viewBox="0 0 1600 1000">
  <rect width="1600" height="1000" fill="#bfe3ff"/>
  <circle cx="1240" cy="180" r="95" fill="#ffd166"/>
  <path d="M0 650 L360 260 L650 650 Z" fill="#334155"/>
  <path d="M260 370 L360 260 L445 650 Z" fill="#e2e8f0"/>
  <path d="M420 650 L820 180 L1230 650 Z" fill="#475569"/>
  <path d="M700 320 L820 180 L930 650 Z" fill="#f8fafc"/>
  <rect y="650" width="1600" height="350" fill="#2563eb"/>
  <path d="M0 770 C260 720 390 840 650 790 S1130 700 1600 770 L1600 1000 L0 1000 Z" fill="#1d4ed8"/>
  <path d="M0 650 C220 610 390 690 600 650 S1070 610 1600 650 L1600 720 L0 720 Z" fill="#0f766e"/>
</svg>
EOF

cat > /data/demo/Photos/Travel/harbor-evening.svg <<'EOF'
<svg xmlns="http://www.w3.org/2000/svg" width="1600" height="1000" viewBox="0 0 1600 1000">
  <defs>
    <linearGradient id="sky" x1="0" x2="0" y1="0" y2="1">
      <stop offset="0" stop-color="#fb7185"/>
      <stop offset="1" stop-color="#fde68a"/>
    </linearGradient>
  </defs>
  <rect width="1600" height="1000" fill="url(#sky)"/>
  <circle cx="1180" cy="310" r="120" fill="#fef3c7"/>
  <rect y="570" width="1600" height="430" fill="#0e7490"/>
  <path d="M0 620 C200 570 420 680 640 620 S1090 560 1600 630 L1600 1000 L0 1000 Z" fill="#155e75"/>
  <g fill="#172033">
    <rect x="180" y="430" width="120" height="160"/>
    <rect x="330" y="390" width="150" height="200"/>
    <rect x="520" y="450" width="90" height="140"/>
    <rect x="680" y="410" width="130" height="180"/>
  </g>
  <path d="M980 720 L1140 720 L1080 790 L1020 790 Z" fill="#f8fafc"/>
  <path d="M1060 500 L1060 720 L1180 720 Z" fill="#ffffff"/>
</svg>
EOF

cat > /data/demo/Photos/Product/file-preview-card.svg <<'EOF'
<svg xmlns="http://www.w3.org/2000/svg" width="1200" height="900" viewBox="0 0 1200 900">
  <rect width="1200" height="900" fill="#f1f5f9"/>
  <rect x="160" y="110" width="880" height="680" rx="28" fill="#ffffff"/>
  <rect x="220" y="180" width="300" height="220" rx="20" fill="#dbeafe"/>
  <rect x="560" y="190" width="360" height="30" rx="15" fill="#172033"/>
  <rect x="560" y="250" width="300" height="22" rx="11" fill="#94a3b8"/>
  <rect x="560" y="300" width="250" height="22" rx="11" fill="#94a3b8"/>
  <rect x="220" y="470" width="700" height="24" rx="12" fill="#cbd5e1"/>
  <rect x="220" y="530" width="640" height="24" rx="12" fill="#cbd5e1"/>
  <rect x="220" y="590" width="690" height="24" rx="12" fill="#cbd5e1"/>
  <rect x="220" y="660" width="170" height="54" rx="12" fill="#2563eb"/>
</svg>
EOF

/usr/local/bin/generate-demo-data.sh /data/demo

/usr/local/bin/godrive admin create \
	--username "$GODRIVE_DEMO_USER" \
	--password "$GODRIVE_DEMO_PASSWORD" \
	--root /data/demo \
	--admin=true >/dev/null

/usr/local/bin/godrive reindex --user "$GODRIVE_DEMO_USER" >/dev/null
/usr/local/bin/godrive preview-warmup >/dev/null

exec /usr/local/bin/godrive serve
