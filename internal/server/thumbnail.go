package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"godrive/internal/preview"
	"godrive/internal/store"
)

const thumbnailCacheVersion = 2

func init() {
	image.RegisterFormat("jpeg", "\xff\xd8", jpeg.Decode, jpeg.DecodeConfig)
	image.RegisterFormat("png", "\x89PNG\r\n\x1a\n", png.Decode, png.DecodeConfig)
	image.RegisterFormat("gif", "GIF8?a", gif.Decode, gif.DecodeConfig)
}

func (s *Server) thumbnail(w http.ResponseWriter, r *http.Request, user store.User, session store.Session) {
	size := parseThumbSize(r.URL.Query().Get("size"))
	resolved, info, err := s.files.ResolveForRead(user, r.URL.Query().Get("path"))
	if err != nil {
		writeError(w, statusForError(err), err.Error())
		return
	}

	kind := preview.KindForName(resolved.Physical)
	if kind != "image" && kind != "video" && kind != "pdf" {
		writeError(w, http.StatusUnsupportedMediaType, "thumbnail is not supported for this file type")
		return
	}

	cachePath := thumbnailCachePath(s.cfg.PreviewDir, user.ID, resolved.Logical, info.Size(), info.ModTime().UnixNano(), size)
	if _, err := os.Stat(cachePath); errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(filepath.Dir(cachePath), 0o750); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to create thumbnail cache")
			return
		}
		if err := generateThumbnail(r.Context(), resolved.Physical, kind, size, cachePath); err != nil {
			writeError(w, http.StatusUnsupportedMediaType, err.Error())
			return
		}
	} else if err != nil {
		writeError(w, statusForError(err), err.Error())
		return
	}

	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "private, max-age=86400")
	http.ServeFile(w, r, cachePath)
}

func parseThumbSize(raw string) int {
	size, err := strconv.Atoi(raw)
	if err != nil {
		return 256
	}
	if size < 96 {
		return 96
	}
	if size > 1024 {
		return 1024
	}
	return size
}

func thumbnailCachePath(cacheRoot string, userID int64, logical string, fileSize, modTime int64, thumbSize int) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d\x00%d\x00%s\x00%d\x00%d\x00%d", thumbnailCacheVersion, userID, logical, fileSize, modTime, thumbSize)))
	return filepath.Join(cacheRoot, "thumbs", hex.EncodeToString(sum[:2]), hex.EncodeToString(sum[:])+".jpg")
}

func generateThumbnail(ctx context.Context, source, kind string, size int, target string) error {
	tmp := target + ".tmp.jpg"
	_ = os.Remove(tmp)

	var err error
	switch kind {
	case "image":
		err = generateImageThumbnail(ctx, source, size, tmp)
	case "video":
		err = generateVideoThumbnail(ctx, source, size, tmp)
	case "pdf":
		err = generatePDFThumbnail(ctx, source, size, tmp)
	default:
		err = errors.New("thumbnail is not supported for this file type")
	}
	if err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, target)
}

func generateImageThumbnail(ctx context.Context, source string, size int, target string) error {
	if err := runCommand(ctx, "vipsthumbnail", source, "--size", fmt.Sprintf("%dx%d", size, size), "-o", target+"[Q=88]"); err == nil {
		return nil
	}
	return generateImageThumbnailStdlib(source, size, target)
}

func generateVideoThumbnail(ctx context.Context, source string, size int, target string) error {
	filter := fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease", size, size)
	return runCommand(ctx, "ffmpeg", "-hide_banner", "-loglevel", "error", "-y", "-ss", "00:00:01", "-i", source, "-frames:v", "1", "-vf", filter, target)
}

func generatePDFThumbnail(ctx context.Context, source string, size int, target string) error {
	tmpBase := strings.TrimSuffix(target, filepath.Ext(target))
	if err := runCommand(ctx, "pdftoppm", "-jpeg", "-singlefile", "-f", "1", "-scale-to", strconv.Itoa(size), source, tmpBase); err != nil {
		return err
	}
	if _, err := os.Stat(target); err == nil {
		return nil
	}
	generated := tmpBase + ".jpg"
	if generated != target {
		return os.Rename(generated, target)
	}
	return nil
}

func generateImageThumbnailStdlib(source string, size int, target string) error {
	file, err := os.Open(source)
	if err != nil {
		return err
	}
	defer file.Close()

	img, _, err := image.Decode(file)
	if err != nil {
		return err
	}

	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	if width <= 0 || height <= 0 {
		return errors.New("invalid image dimensions")
	}

	targetWidth, targetHeight := scaledDimensions(width, height, size)
	thumb := image.NewRGBA(image.Rect(0, 0, targetWidth, targetHeight))
	for y := 0; y < targetHeight; y++ {
		sourceY := bounds.Min.Y + y*height/targetHeight
		for x := 0; x < targetWidth; x++ {
			sourceX := bounds.Min.X + x*width/targetWidth
			thumb.Set(x, y, img.At(sourceX, sourceY))
		}
	}

	out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o640)
	if err != nil {
		return err
	}
	defer out.Close()

	return jpeg.Encode(out, thumb, &jpeg.Options{Quality: 88})
}

func scaledDimensions(width, height, maxSize int) (int, int) {
	if width >= height {
		scaledHeight := height * maxSize / width
		if scaledHeight < 1 {
			scaledHeight = 1
		}
		return maxSize, scaledHeight
	}
	scaledWidth := width * maxSize / height
	if scaledWidth < 1 {
		scaledWidth = 1
	}
	return scaledWidth, maxSize
}

func runCommand(ctx context.Context, name string, args ...string) error {
	if _, err := exec.LookPath(name); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, name, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if len(output) > 0 {
			return fmt.Errorf("%s failed: %s", name, strings.TrimSpace(string(output)))
		}
		return err
	}
	return nil
}
