#!/bin/sh
set -eu

ROOT="${1:-/data/demo}"
IMAGE_COUNT="${GODRIVE_DEMO_IMAGE_COUNT:-420}"
NESTED_DEPTH="${GODRIVE_DEMO_NESTED_DEPTH:-10}"
MODEL_COUNT="${GODRIVE_DEMO_MODEL_COUNT:-18}"
REMOTE_IMAGE_COUNT="${GODRIVE_DEMO_REMOTE_IMAGE_COUNT:-96}"
REMOTE_IMAGE_SOURCE="${GODRIVE_DEMO_REMOTE_IMAGE_SOURCE:-https://picsum.photos/seed}"

mkdir -p "$ROOT"

write_svg_image() {
	file="$1"
	idx="$2"
	width="$3"
	height="$4"
	hue=$((idx * 37 % 360))
	hue2=$(((hue + 48) % 360))
	hue3=$(((hue + 142) % 360))
	r=$((width / 5))
	if [ "$height" -lt "$width" ]; then
		r=$((height / 5))
	fi
	cat > "$file" <<EOF
<svg xmlns="http://www.w3.org/2000/svg" width="$width" height="$height" viewBox="0 0 $width $height">
  <rect width="$width" height="$height" fill="hsl($hue 76% 88%)"/>
  <rect x="0" y="$((height * 3 / 5))" width="$width" height="$((height * 2 / 5))" fill="hsl($hue2 58% 46%)"/>
  <circle cx="$((width * 4 / 5))" cy="$((height / 5))" r="$r" fill="hsl($hue3 92% 62%)"/>
  <path d="M0 $((height * 3 / 5)) C$((width / 4)) $((height / 2)) $((width / 3)) $((height * 7 / 10)) $((width / 2)) $((height * 3 / 5)) S$((width * 3 / 4)) $((height / 2)) $width $((height * 3 / 5)) L$width $height L0 $height Z" fill="hsl($hue2 62% 36%)"/>
  <path d="M0 $((height * 7 / 10)) L$((width / 5)) $((height / 3)) L$((width * 2 / 5)) $((height * 7 / 10)) Z" fill="hsl($hue 42% 28%)"/>
  <path d="M$((width / 4)) $((height * 7 / 10)) L$((width / 2)) $((height / 4)) L$((width * 4 / 5)) $((height * 7 / 10)) Z" fill="hsl($hue3 36% 33%)"/>
  <text x="32" y="$((height - 34))" font-family="Arial, sans-serif" font-size="28" font-weight="700" fill="rgba(17,24,39,.74)">Demo image $idx</text>
</svg>
EOF
}

write_obj_model() {
	file="$1"
	idx="$2"
	scale=$((idx % 5 + 1))
	cat > "$file" <<EOF
# goDrive demo model $idx
o DemoPrism$idx
v 0 0 0
v $scale 0 0
v $scale $scale 0
v 0 $scale 0
v 0 0 $scale
v $scale 0 $scale
v $scale $scale $scale
v 0 $scale $scale
f 1 2 3 4
f 5 8 7 6
f 1 5 6 2
f 2 6 7 3
f 3 7 8 4
f 5 1 4 8
EOF
}

download_remote_image() {
	file="$1"
	seed="$2"
	width="$3"
	height="$4"
	if ! command -v curl >/dev/null 2>&1; then
		return 1
	fi
	curl -fsSL --retry 1 --connect-timeout 3 --max-time 8 "$REMOTE_IMAGE_SOURCE/$seed/$width/$height" -o "$file"
}

mkdir -p \
	"$ROOT/Photos/Large Gallery" \
	"$ROOT/Photos/Picsum" \
	"$ROOT/Photos/Portraits" \
	"$ROOT/Photos/Landscape" \
	"$ROOT/Photos/Square" \
	"$ROOT/Models/OBJ" \
	"$ROOT/Code/Web" \
	"$ROOT/Code/Backend" \
	"$ROOT/Code/Mobile" \
	"$ROOT/Documents/Reports/Quarterly" \
	"$ROOT/Documents/Exports" \
	"$ROOT/Store Assets/Screenshots" \
	"$ROOT/Store Assets/Copy" \
	"$ROOT/Deep Archive"

i=1
while [ "$i" -le "$IMAGE_COUNT" ]; do
	case $((i % 4)) in
		0) dir="$ROOT/Photos/Large Gallery"; width=1600; height=1000 ;;
		1) dir="$ROOT/Photos/Portraits"; width=900; height=1400 ;;
		2) dir="$ROOT/Photos/Landscape"; width=1800; height=1100 ;;
		*) dir="$ROOT/Photos/Square"; width=1200; height=1200 ;;
	esac
	write_svg_image "$dir/demo-image-$(printf '%04d' "$i").svg" "$i" "$width" "$height"
	i=$((i + 1))
done

i=1
remote_failures=0
while [ "$i" -le "$REMOTE_IMAGE_COUNT" ]; do
	case $((i % 5)) in
		0) width=2048; height=1365 ;;
		1) width=1280; height=1920 ;;
		2) width=1920; height=1080 ;;
		3) width=1440; height=1440 ;;
		*) width=2560; height=1440 ;;
	esac
	file="$ROOT/Photos/Picsum/picsum-$(printf '%04d' "$i")-${width}x${height}.jpg"
	if ! download_remote_image "$file" "godrive-$i" "$width" "$height"; then
		rm -f "$file"
		echo "picsum-$(printf '%04d' "$i") ${width}x${height}" >> "$ROOT/Photos/Picsum/download-failures.txt"
		remote_failures=$((remote_failures + 1))
		if [ "$remote_failures" -ge 3 ]; then
			echo "Skipping remaining remote demo images after $remote_failures download failures." >> "$ROOT/Photos/Picsum/download-failures.txt"
			break
		fi
	else
		remote_failures=0
	fi
	i=$((i + 1))
done

i=1
deep="$ROOT/Deep Archive"
while [ "$i" -le "$NESTED_DEPTH" ]; do
	deep="$deep/level-$(printf '%02d' "$i")"
	mkdir -p "$deep"
	cat > "$deep/readme-level-$(printf '%02d' "$i").md" <<EOF
# Deep archive level $i

This folder exists to exercise breadcrumb navigation, tree expansion, mobile drawer navigation, and indexed search across deeply nested paths.

- Depth: $i
- Demo account: read-only
- Reset model: container restart
EOF
	i=$((i + 1))
done

i=1
while [ "$i" -le "$MODEL_COUNT" ]; do
	write_obj_model "$ROOT/Models/OBJ/demo-prism-$(printf '%02d' "$i").obj" "$i"
	i=$((i + 1))
done

cat > "$ROOT/Code/Web/app.ts" <<'EOF'
export type DemoFile = {
  path: string;
  name: string;
  previewKind?: "image" | "markdown" | "text" | "3d";
};

export function isPreviewable(file: DemoFile): boolean {
  return Boolean(file.previewKind);
}
EOF

cat > "$ROOT/Code/Backend/reindex.go" <<'EOF'
package demo

func ShouldReindex(seedChanged bool, indexEmpty bool) bool {
	return seedChanged || indexEmpty
}
EOF

cat > "$ROOT/Code/Mobile/share_upload.dart" <<'EOF'
class ShareUpload {
  const ShareUpload({required this.name, required this.bytes});

  final String name;
  final int bytes;
}
EOF

cat > "$ROOT/Documents/Reports/Quarterly/q2-readiness.md" <<'EOF'
# Q2 Release Readiness

The demo dataset includes generated images, nested folders, text previews, structured exports, code samples, and simple 3D models.

## Manual checks

- Search for `readiness`, `prism`, and `deep archive`.
- Open Markdown and CSV files.
- Switch grid, masonry, and list views.
- Open the admin panel in read-only demo mode.
EOF

cat > "$ROOT/Documents/Exports/storage-ledger.csv" <<'EOF'
date,category,files,bytes,status
2026-05-01,photos,420,19341200,indexed
2026-05-01,documents,38,884120,indexed
2026-05-01,models,18,15420,indexed
2026-05-01,code,12,42110,indexed
EOF

cat > "$ROOT/Documents/Exports/api-response.json" <<'EOF'
{
  "demo": true,
  "mode": "read-only",
  "features": ["browse", "search", "preview", "download", "admin-preview"],
  "reset": "container restart"
}
EOF

cat > "$ROOT/Store Assets/Copy/play-store-short-description.txt" <<'EOF'
Self-hosted file manager with previews, search, WebDAV, and mobile apps.
EOF

cat > "$ROOT/Store Assets/Copy/app-store-promotional-text.txt" <<'EOF'
Browse, preview, search, and manage your self-hosted files from web and mobile.
EOF

i=1
while [ "$i" -le 12 ]; do
	write_svg_image "$ROOT/Store Assets/Screenshots/mobile-screen-$(printf '%02d' "$i").svg" "$((900 + i))" 1290 2796
	i=$((i + 1))
done
