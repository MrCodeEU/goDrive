#!/usr/bin/env python3
"""
Generate ~400k files for goDrive performance testing.

Layout:
  <data_dir>/
    images/         ~100k small JPEG files (~2KB each, ~200MB)
    documents/      ~150k small text files (~200B each, ~30MB)
    mixed/          ~150k mixed files (text + copies of base images)

Total target: <300MB, <400k files.

Usage:
  python3 scripts/perf-seed.py [data_dir] [options]

Environment overrides:
  PERF_IMAGE_COUNT   (default 100000)
  PERF_TEXT_COUNT    (default 300000)
  PERF_NEST_DEPTH    (default 3)       directory nesting depth
"""

import argparse
import math
import os
import random
import shutil
import struct
import subprocess
import sys
import zlib
from pathlib import Path

IMAGE_COUNT = int(os.environ.get("PERF_IMAGE_COUNT", "100000"))
TEXT_COUNT  = int(os.environ.get("PERF_TEXT_COUNT",  "300000"))
NEST_DEPTH  = int(os.environ.get("PERF_NEST_DEPTH",  "3"))

WORDS = (
    "alpha beta gamma delta epsilon zeta eta theta iota kappa lambda mu nu xi "
    "omicron pi rho sigma tau upsilon phi chi psi omega red green blue yellow "
    "photo video document report notes readme backup archive project family "
    "vacation birthday holiday travel work meeting schedule plan draft final "
    "review edit copy original source export import data log config settings"
).split()


# ---------------------------------------------------------------------------
# Minimal solid-color JPEG generator (no external deps)
# ---------------------------------------------------------------------------

def _make_jpeg(r: int, g: int, b: int) -> bytes:
    """Return bytes of a minimal 8×8 solid-color JPEG."""
    # Use ffmpeg if available — produces a more realistic JPEG for preview pipeline.
    try:
        color = f"0x{r:02x}{g:02x}{b:02x}"
        result = subprocess.run(
            [
                "ffmpeg", "-y",
                "-f", "lavfi",
                "-i", f"color=c={color}:size=64x64:rate=1",
                "-frames:v", "1",
                "-q:v", "8",
                "-f", "mjpeg",
                "pipe:1",
            ],
            capture_output=True,
            timeout=5,
        )
        if result.returncode == 0 and result.stdout:
            return result.stdout
    except (FileNotFoundError, subprocess.TimeoutExpired):
        pass

    # Fallback: minimal valid 1×1 PNG using stdlib only.
    return _make_png_1x1(r, g, b)


def _make_png_1x1(r: int, g: int, b: int) -> bytes:
    """Return a minimal valid 1×1 RGB PNG."""
    def chunk(name: bytes, data: bytes) -> bytes:
        c = struct.pack(">I", len(data)) + name + data
        return c + struct.pack(">I", zlib.crc32(name + data) & 0xFFFFFFFF)

    sig  = b"\x89PNG\r\n\x1a\n"
    ihdr = chunk(b"IHDR", struct.pack(">IIBBBBB", 1, 1, 8, 2, 0, 0, 0))
    # Filter byte 0x00 + RGB pixel
    raw  = b"\x00" + bytes([r, g, b])
    idat = chunk(b"IDAT", zlib.compress(raw, 9))
    iend = chunk(b"IEND", b"")
    return sig + ihdr + idat + iend


# ---------------------------------------------------------------------------
# Text content generator
# ---------------------------------------------------------------------------

def _random_text(min_words: int = 20, max_words: int = 80) -> str:
    n = random.randint(min_words, max_words)
    lines = []
    while n > 0:
        line_len = random.randint(1, min(n, 15))
        lines.append(" ".join(random.choices(WORDS, k=line_len)))
        n -= line_len
    return "\n".join(lines) + "\n"


# ---------------------------------------------------------------------------
# Directory helper
# ---------------------------------------------------------------------------

def _dir_for_index(base: Path, idx: int, depth: int, buckets: int = 100) -> Path:
    """Map a flat index to a nested directory path."""
    path = base
    remaining = idx
    for _ in range(depth):
        bucket = remaining % buckets
        path = path / f"{bucket:02d}"
        remaining //= buckets
    return path


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

def main() -> None:
    parser = argparse.ArgumentParser(description="Seed goDrive perf test data")
    parser.add_argument("data_dir", nargs="?", default="./var/perf-data",
                        help="Root directory for test files (default: ./var/perf-data)")
    parser.add_argument("--images", type=int, default=IMAGE_COUNT)
    parser.add_argument("--text",   type=int, default=TEXT_COUNT)
    parser.add_argument("--depth",  type=int, default=NEST_DEPTH)
    parser.add_argument("--force",  action="store_true",
                        help="Re-seed even if data_dir already has files")
    args = parser.parse_args()

    data = Path(args.data_dir).resolve()
    images_dir = data / "images"
    docs_dir   = data / "documents"
    mixed_dir  = data / "mixed"

    if data.exists() and any(data.iterdir()) and not args.force:
        print(f"[perf-seed] {data} already exists. Use --force to re-seed.", file=sys.stderr)
        sys.exit(0)

    print(f"[perf-seed] Seeding {args.images} images + {args.text} text files → {data}")

    # --- Generate base images (one per color, used as copy source) ----------
    print("[perf-seed] Generating base images...")
    tmp_dir = data / ".base"
    tmp_dir.mkdir(parents=True, exist_ok=True)

    n_bases = 20
    base_images: list[Path] = []
    for i in range(n_bases):
        r = (i * 37 + 80) % 256
        g = (i * 71 + 130) % 256
        b = (i * 113 + 200) % 256
        p = tmp_dir / f"base_{i:02d}.jpg"
        p.write_bytes(_make_jpeg(r, g, b))
        base_images.append(p)
        print(f"  base image {i+1}/{n_bases}", end="\r", flush=True)
    print()

    # --- Seed images ---------------------------------------------------------
    print(f"[perf-seed] Writing {args.images} image files...")
    images_dir.mkdir(parents=True, exist_ok=True)
    extensions = [".jpg", ".jpeg", ".png", ".webp"]
    for i in range(args.images):
        d = _dir_for_index(images_dir, i, args.depth)
        d.mkdir(parents=True, exist_ok=True)
        ext = extensions[i % len(extensions)]
        name = f"img_{i:07d}{ext}"
        shutil.copy2(base_images[i % n_bases], d / name)
        if i % 10000 == 0:
            print(f"  {i}/{args.images}", end="\r", flush=True)
    print(f"  {args.images}/{args.images}")

    # --- Seed text files -----------------------------------------------------
    print(f"[perf-seed] Writing {args.text} text files...")
    docs_dir.mkdir(parents=True, exist_ok=True)
    mixed_dir.mkdir(parents=True, exist_ok=True)
    text_exts  = [".txt", ".md", ".log", ".csv", ".json"]
    topic_words = WORDS[:30]

    for i in range(args.text):
        # Split evenly between documents/ and mixed/
        base = docs_dir if i % 2 == 0 else mixed_dir
        d = _dir_for_index(base, i, args.depth)
        d.mkdir(parents=True, exist_ok=True)
        ext = text_exts[i % len(text_exts)]
        topic = random.choice(topic_words)
        name = f"{topic}_{i:07d}{ext}"
        (d / name).write_text(_random_text())
        if i % 20000 == 0:
            print(f"  {i}/{args.text}", end="\r", flush=True)
    print(f"  {args.text}/{args.text}")

    # --- Cleanup base images -------------------------------------------------
    shutil.rmtree(tmp_dir)

    # --- Summary -------------------------------------------------------------
    total = sum(1 for _ in data.rglob("*") if _.is_file())
    size_bytes = sum(f.stat().st_size for f in data.rglob("*") if f.is_file())
    print(f"[perf-seed] Done. {total} files, {size_bytes / 1_048_576:.1f} MB")


if __name__ == "__main__":
    main()
