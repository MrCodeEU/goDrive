# Previews & Thumbnails

goDrive generates thumbnails and cached previews for a wide range of file types.

## Supported Types

| Type | Thumbnails | Full Preview |
|---|---|---|
| JPEG, PNG, WebP, GIF | Yes | Yes |
| RAW photos (CR2, NEF, ARW, …) | Yes | Cached JPEG |
| Video (MP4, MKV, …) | Poster frame | In-browser player |
| PDF | Yes | Native browser viewer |
| Office (DOCX, XLSX, PPTX, …) | Yes | Cached image via LibreOffice |
| SVG | Yes | Inline |
| Text / Markdown | No | Syntax-highlighted |
| 3D (OBJ) | No | In-browser viewer |

## Inode-stable cache

Thumbnails are keyed by inode + modification time, not filename or path. Renaming or moving a file preserves its cached thumbnails — no regeneration needed.

## Warmup

After upload, all configured thumbnail sizes are generated asynchronously. The admin panel exposes a Preview Warmup job to pre-generate thumbnails for existing files.

## Configuration

```bash
GODRIVE_PREVIEW_WORKERS=0   # 0 = auto (CPU count)
GODRIVE_PREVIEW_TIMEOUT=45s # per-file timeout
```

## Toolchain

Preview generation uses:

- **libvips** — fast image resizing and conversion
- **ffmpeg** — video poster frames
- **poppler** (`pdftoppm`) — PDF thumbnails
- **LibreOffice** (headless) — Office document previews

All tools are included in the production Docker image.
