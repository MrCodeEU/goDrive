package server

import (
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"godrive/internal/config"
	"godrive/internal/files"
	"godrive/internal/store"
)

func TestGenerateImageThumbnailStdlib(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	source := filepath.Join(dir, "source.jpg")
	target := filepath.Join(dir, "thumb.jpg")

	img := image.NewRGBA(image.Rect(0, 0, 400, 200))
	for y := range 200 {
		for x := range 400 {
			img.Set(x, y, color.RGBA{R: 20, G: 120, B: 90, A: 255})
		}
	}

	file, err := os.Create(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := jpeg.Encode(file, img, &jpeg.Options{Quality: 90}); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	if err := generateImageThumbnailStdlib(source, 128, target); err != nil {
		t.Fatal(err)
	}

	generated, err := os.Open(target)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = generated.Close()
	}()

	cfg, _, err := image.DecodeConfig(generated)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Width != 128 || cfg.Height != 64 {
		t.Fatalf("thumbnail size = %dx%d, want 128x64", cfg.Width, cfg.Height)
	}
}

func TestRawFileServesInlineContent(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	source := filepath.Join(dir, "photo.jpg")
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))

	file, err := os.Create(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := jpeg.Encode(file, img, &jpeg.Options{Quality: 90}); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	srv := New(config.Config{}, nil, files.NewService("", nil), slog.New(slog.NewTextHandler(io.Discard, nil)))
	req := httptest.NewRequest(http.MethodGet, "/api/files/raw?path=/photo.jpg", nil)
	rec := httptest.NewRecorder()

	srv.rawFile(rec, req, store.User{HomeRoot: dir}, store.Session{})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Disposition"); got != `inline; filename="photo.jpg"` {
		t.Fatalf("content disposition = %q", got)
	}
	if got := rec.Header().Get("Content-Type"); got != "image/jpeg" {
		t.Fatalf("content type = %q, want image/jpeg", got)
	}
	if got := rec.Header().Get("Content-Security-Policy"); got != "sandbox" {
		t.Fatalf("content security policy = %q, want sandbox", got)
	}
}

func TestRawFileServesSVGInlineWithSandbox(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	source := filepath.Join(dir, "diagram.svg")
	if err := os.WriteFile(source, []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script><rect width="10" height="10"/></svg>`), 0o640); err != nil {
		t.Fatal(err)
	}

	srv := New(config.Config{}, nil, files.NewService("", nil), slog.New(slog.NewTextHandler(io.Discard, nil)))
	req := httptest.NewRequest(http.MethodGet, "/api/files/raw?path=/diagram.svg", nil)
	rec := httptest.NewRecorder()

	srv.rawFile(rec, req, store.User{HomeRoot: dir}, store.Session{})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Disposition"); got != `inline; filename="diagram.svg"` {
		t.Fatalf("content disposition = %q", got)
	}
	if got := rec.Header().Get("Content-Type"); got != "image/svg+xml" {
		t.Fatalf("content type = %q, want image/svg+xml", got)
	}
	if got := rec.Header().Get("Content-Security-Policy"); got != "sandbox" {
		t.Fatalf("content security policy = %q, want sandbox", got)
	}
}

func TestRawFileServesActiveContentAsAttachment(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	source := filepath.Join(dir, "page.html")
	if err := os.WriteFile(source, []byte(`<!doctype html><script>fetch("/api/me")</script>`), 0o640); err != nil {
		t.Fatal(err)
	}

	srv := New(config.Config{}, nil, files.NewService("", nil), slog.New(slog.NewTextHandler(io.Discard, nil)))
	req := httptest.NewRequest(http.MethodGet, "/api/files/raw?path=/page.html", nil)
	rec := httptest.NewRecorder()

	srv.rawFile(rec, req, store.User{HomeRoot: dir}, store.Session{})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Disposition"); got != `attachment; filename="page.html"` {
		t.Fatalf("content disposition = %q", got)
	}
	if got := rec.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Fatalf("content type = %q, want text/html; charset=utf-8", got)
	}
	if got := rec.Header().Get("Content-Security-Policy"); got != "" {
		t.Fatalf("content security policy = %q, want empty for attachment", got)
	}
}

func TestThumbnailCachePathChangesOnFileModification(t *testing.T) {
	t.Parallel()

	// Cache path encodes size and mtime — a file update produces a different key,
	// preventing stale thumbnails from being served after content changes.
	path1 := thumbnailCachePath("/cache", 1, "/photo.jpg", 1000, 1_000_000, 240)
	path2 := thumbnailCachePath("/cache", 1, "/photo.jpg", 1001, 1_000_000, 240) // different size
	path3 := thumbnailCachePath("/cache", 1, "/photo.jpg", 1000, 1_000_001, 240) // different mtime
	path4 := thumbnailCachePath("/cache", 1, "/photo.jpg", 1000, 1_000_000, 420) // different thumb size

	if path1 == path2 {
		t.Error("size change should produce different cache path")
	}
	if path1 == path3 {
		t.Error("mtime change should produce different cache path")
	}
	if path1 == path4 {
		t.Error("thumb size change should produce different cache path")
	}

	// Same inputs always produce the same path (deterministic).
	if thumbnailCachePath("/cache", 1, "/photo.jpg", 1000, 1_000_000, 240) != path1 {
		t.Error("cache path is not deterministic")
	}
}

func TestThumbnailCachePathIsolatedByUser(t *testing.T) {
	t.Parallel()

	path1 := thumbnailCachePath("/cache", 1, "/photo.jpg", 1000, 1_000_000, 240)
	path2 := thumbnailCachePath("/cache", 2, "/photo.jpg", 1000, 1_000_000, 240)
	if path1 == path2 {
		t.Error("different users should have different cache paths for same logical path")
	}
}

func TestInodeCachePathStableAcrossRename(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	src := filepath.Join(dir, "photo.jpg")
	dst := filepath.Join(dir, "renamed.jpg")
	if err := os.WriteFile(src, []byte("data"), 0o640); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(src)
	if err != nil {
		t.Fatal(err)
	}
	pathBefore := thumbnailCachePathInode("/cache", 1, "/photo.jpg", info, 240)

	if err := os.Rename(src, dst); err != nil {
		t.Fatal(err)
	}
	infoAfter, err := os.Stat(dst)
	if err != nil {
		t.Fatal(err)
	}
	pathAfter := thumbnailCachePathInode("/cache", 1, "/renamed.jpg", infoAfter, 240)

	// Inode unchanged after rename → same cache path even though logical path differs.
	inode, _ := inodeKey(info)
	if inode == 0 {
		t.Skip("inode not available on this platform")
	}
	if pathBefore != pathAfter {
		t.Errorf("cache path changed after rename: %q → %q", pathBefore, pathAfter)
	}
}

func TestInodeCachePathDifferentAfterCopy(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	a := filepath.Join(dir, "a.jpg")
	b := filepath.Join(dir, "b.jpg")
	if err := os.WriteFile(a, []byte("data"), 0o640); err != nil {
		t.Fatal(err)
	}
	// Copy: new file = new inode.
	data, _ := os.ReadFile(a)
	if err := os.WriteFile(b, data, 0o640); err != nil {
		t.Fatal(err)
	}

	infoA, _ := os.Stat(a)
	infoB, _ := os.Stat(b)
	inodeA, _ := inodeKey(infoA)
	inodeB, _ := inodeKey(infoB)

	if inodeA == 0 {
		t.Skip("inode not available on this platform")
	}
	if inodeA == inodeB {
		t.Skip("copy produced same inode (unexpected on this filesystem)")
	}

	pathA := thumbnailCachePathInode("/cache", 1, "/a.jpg", infoA, 240)
	pathB := thumbnailCachePathInode("/cache", 1, "/b.jpg", infoB, 240)
	if pathA == pathB {
		t.Error("copy should produce different cache path (different inode)")
	}
}

func TestGenerateThumbnailsAsyncCreatesAllSizes(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	previewDir := t.TempDir()

	// Create a small JPEG.
	source := filepath.Join(dir, "photo.jpg")
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	f, err := os.Create(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := jpeg.Encode(f, img, &jpeg.Options{Quality: 90}); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	_ = f.Close()

	st, err := store.Open(filepath.Join(t.TempDir(), "godrive.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := New(config.Config{PreviewDir: previewDir}, st, files.NewService(t.TempDir(), st), log)

	entry := files.Entry{
		Name:        "photo.jpg",
		Path:        "/photo.jpg",
		Type:        "file",
		PreviewKind: "image",
	}
	user := store.User{HomeRoot: dir}

	srv.generateThumbnailsAsync(user, entry)

	// Wait for async generation — should complete quickly for a tiny JPEG.
	deadline := time.Now().Add(10 * time.Second)
	var cached int
	for time.Now().Before(deadline) {
		cached = 0
		info, statErr := os.Stat(source)
		if statErr != nil {
			t.Fatal(statErr)
		}
		for _, size := range previewWarmupSizes {
			cachePath := thumbnailCachePathInode(previewDir, user.ID, entry.Path, info, size)
			if _, err := os.Stat(cachePath); err == nil {
				cached++
			}
		}
		if cached == len(previewWarmupSizes) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if cached == 0 {
		t.Fatal("generateThumbnailsAsync produced no thumbnails within timeout")
	}
	// Log which sizes were produced (some may fail if ffmpeg/vips absent in CI).
	t.Logf("generated %d/%d thumbnail sizes", cached, len(previewWarmupSizes))
}

func TestClearPreviewCacheRemovesFilesAndKeepsDirectory(t *testing.T) {
	t.Parallel()

	previewDir := filepath.Join(t.TempDir(), "previews")
	nested := filepath.Join(previewDir, "thumbs", "aa")
	if err := os.MkdirAll(nested, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "thumb.jpg"), []byte("cache"), 0o640); err != nil {
		t.Fatal(err)
	}

	srv := New(config.Config{PreviewDir: previewDir}, nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	req := httptest.NewRequest(http.MethodDelete, "/api/admin/preview-cache", nil)
	rec := httptest.NewRecorder()

	srv.clearPreviewCache(rec, req, store.User{}, store.Session{})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if _, err := os.Stat(previewDir); err != nil {
		t.Fatalf("preview dir was not recreated: %v", err)
	}
	if _, err := os.Stat(filepath.Join(nested, "thumb.jpg")); !os.IsNotExist(err) {
		t.Fatalf("thumbnail still exists or stat failed unexpectedly: %v", err)
	}
}

func TestLimitedBufferCapsOutput(t *testing.T) {
	t.Parallel()

	buf := &limitedBuffer{limit: 5}
	if n, err := buf.Write([]byte("hello world")); err != nil || n != len("hello world") {
		t.Fatalf("write = %d, %v", n, err)
	}
	if got := buf.String(); got != "hello ... output truncated" {
		t.Fatalf("buffer = %q", got)
	}
}

func TestGenerateThumbnailWithTimeout(t *testing.T) {
	t.Parallel()

	target := filepath.Join(t.TempDir(), "thumb.jpg")
	err := generateThumbnailWithTimeout(context.Background(), time.Nanosecond, "missing.jpg", "image", 128, target)
	if err == nil {
		t.Fatal("expected timeout or renderer error")
	}
	if _, statErr := os.Stat(target + ".tmp.jpg"); !os.IsNotExist(statErr) {
		t.Fatalf("temporary thumbnail was not removed: %v", statErr)
	}
}

func TestGenerateThumbnailWithRelativeTargetUsesProcessCwd(t *testing.T) {
	dir := t.TempDir()
	binDir := filepath.Join(dir, "bin")
	if err := os.MkdirAll(binDir, 0o750); err != nil {
		t.Fatal(err)
	}
	commandPath := filepath.Join(binDir, "vipsthumbnail")
	script := `#!/bin/sh
out=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "-o" ]; then
    shift
    out="${1%%[*}"
  fi
  shift
done
mkdir -p "$(dirname "$out")"
printf thumb > "$out"
`
	if err := os.WriteFile(commandPath, []byte(script), 0o750); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	source := filepath.Join(dir, "source.jpg")
	if err := os.WriteFile(source, []byte("fake"), 0o640); err != nil {
		t.Fatal(err)
	}
	relativeTarget := filepath.Join("var", "appdata", "previews", "thumb.jpg")
	if err := generateThumbnailWithTimeout(context.Background(), time.Second, source, "image", 128, relativeTarget); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, relativeTarget)); err != nil {
		t.Fatalf("relative target was not written under process cwd: %v", err)
	}
}

func TestRunCommandKillsProcessGroupOnTimeout(t *testing.T) {
	dir := t.TempDir()
	binDir := filepath.Join(dir, "bin")
	if err := os.MkdirAll(binDir, 0o750); err != nil {
		t.Fatal(err)
	}
	commandPath := filepath.Join(binDir, "spawn-child")
	script := "#!/bin/sh\n(sleep 1; echo alive > \"$1\") &\nsleep 10\n"
	if err := os.WriteFile(commandPath, []byte(script), 0o750); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	marker := filepath.Join(dir, "child-alive")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := runCommand(ctx, "spawn-child", marker)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("runCommand error = %v, want deadline exceeded", err)
	}
	time.Sleep(1500 * time.Millisecond)
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("child process survived timeout; marker stat = %v", err)
	}
}

func TestRunCommandUsesIsolatedHome(t *testing.T) {
	dir := t.TempDir()
	binDir := filepath.Join(dir, "bin")
	if err := os.MkdirAll(binDir, 0o750); err != nil {
		t.Fatal(err)
	}
	commandPath := filepath.Join(binDir, "print-home")
	script := "#!/bin/sh\nprintf '%s\\n' \"$HOME\" > \"$1\"\n"
	if err := os.WriteFile(commandPath, []byte(script), 0o750); err != nil {
		t.Fatal(err)
	}

	realHome := filepath.Join(dir, "real-home")
	if err := os.MkdirAll(realHome, 0o750); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", realHome)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	out := filepath.Join(dir, "home.txt")
	if err := runCommand(context.Background(), "print-home", out); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data[:len(data)-1])
	if got == realHome {
		t.Fatal("preview command inherited real HOME")
	}
	if _, err := os.Stat(got); !os.IsNotExist(err) {
		t.Fatalf("isolated HOME should be cleaned up after command, stat = %v", err)
	}
}

func TestPreviewCommandSpecUsesPrlimitWhenAvailable(t *testing.T) {
	t.Parallel()

	lookup := func(name string) (string, error) {
		switch name {
		case "ffmpeg":
			return "/usr/bin/ffmpeg", nil
		case "prlimit":
			return "/usr/bin/prlimit", nil
		default:
			return "", os.ErrNotExist
		}
	}

	path, args, err := previewCommandSpec("ffmpeg", []string{"-version"}, lookup)
	if err != nil {
		t.Fatal(err)
	}
	if path != "/usr/bin/prlimit" {
		t.Fatalf("path = %q, want prlimit", path)
	}
	want := []string{
		"--cpu=120",
		"--as=536870912",
		"--fsize=536870912",
		"--nofile=256",
		"--",
		"/usr/bin/ffmpeg",
		"-version",
	}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("args = %#v, want %#v", args, want)
	}
}

func TestGenerateThumbnailHonorsPreviewSemaphore(t *testing.T) {
	t.Parallel()

	srv := New(config.Config{PreviewDir: t.TempDir(), PreviewWorkers: 1}, nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	srv.previewSem <- struct{}{}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	err := srv.generateThumbnail(ctx, filepath.Join(t.TempDir(), "missing.jpg"), "image", 96, filepath.Join(t.TempDir(), "thumb.jpg"))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("generateThumbnail err = %v, want deadline exceeded while waiting for semaphore", err)
	}

	<-srv.previewSem
}

func TestPreviewCommandSpecFallsBackWithoutPrlimit(t *testing.T) {
	t.Parallel()

	lookup := func(name string) (string, error) {
		switch name {
		case "ffmpeg":
			return "/usr/bin/ffmpeg", nil
		default:
			return "", os.ErrNotExist
		}
	}

	path, args, err := previewCommandSpec("ffmpeg", []string{"-version"}, lookup)
	if err != nil {
		t.Fatal(err)
	}
	if path != "/usr/bin/ffmpeg" {
		t.Fatalf("path = %q, want ffmpeg", path)
	}
	want := []string{"-version"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("args = %#v, want %#v", args, want)
	}
}

func TestPreviewToolStatusesReportsAvailability(t *testing.T) {
	t.Parallel()

	lookup := func(name string) (string, error) {
		if name == "ffmpeg" {
			return "/bin/ffmpeg", nil
		}
		return "", os.ErrNotExist
	}

	statuses := previewToolStatuses(lookup)
	if len(statuses) != len(previewToolDefinitions) {
		t.Fatalf("statuses = %d, want %d", len(statuses), len(previewToolDefinitions))
	}

	var ffmpeg, prlimit PreviewToolStatus
	for _, status := range statuses {
		switch status.Name {
		case "ffmpeg":
			ffmpeg = status
		case "prlimit":
			prlimit = status
		}
	}
	if !ffmpeg.Available || ffmpeg.Path != "/bin/ffmpeg" || ffmpeg.Error != "" {
		t.Fatalf("ffmpeg status = %+v", ffmpeg)
	}
	if prlimit.Available || prlimit.Error == "" {
		t.Fatalf("prlimit status = %+v, want unavailable with error", prlimit)
	}
}

func TestGenerateImageThumbnailReportsFallbackErrors(t *testing.T) {
	binDir := t.TempDir()
	t.Setenv("PATH", binDir)

	err := generateImageThumbnail(context.Background(), "missing.jpg", 128, filepath.Join(t.TempDir(), "thumb.jpg"))
	if err == nil {
		t.Fatal("expected missing tool error")
	}
	text := err.Error()
	if !strings.Contains(text, "vipsthumbnail") || !strings.Contains(text, "ffmpeg") {
		t.Fatalf("error = %q, want both renderer names", text)
	}
}

func TestSupportsThumbnail(t *testing.T) {
	t.Parallel()

	tests := map[string]bool{
		"image":    true,
		"raw":      true,
		"video":    true,
		"pdf":      true,
		"office":   true,
		"text":     false,
		"markdown": false,
		"3d":       false,
		"":         false,
	}

	for kind, want := range tests {
		kind, want := kind, want
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			if got := supportsThumbnail(kind); got != want {
				t.Fatalf("supportsThumbnail(%q) = %v, want %v", kind, got, want)
			}
		})
	}
}
