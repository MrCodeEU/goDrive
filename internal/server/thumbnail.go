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
	"syscall"
	"time"

	"godrive/internal/files"
	"godrive/internal/preview"
	"godrive/internal/store"
)

const thumbnailCacheVersion = 3
const (
	defaultPreviewTimeout   = 45 * time.Second
	previewCommandOutputCap = 32 * 1024
	previewCommandCPUTime   = 120
	previewCommandMemory    = 4 * 1024 * 1024 * 1024
	previewCommandFileSize  = 512 * 1024 * 1024
	previewCommandOpenFiles = 256
)

var previewWarmupSizes = []int{240, 420, 1024, 2048}

type PreviewToolStatus struct {
	Name      string `json:"name"`
	Purpose   string `json:"purpose"`
	Available bool   `json:"available"`
	Path      string `json:"path,omitempty"`
	Error     string `json:"error,omitempty"`
}

var previewToolDefinitions = []PreviewToolStatus{
	{Name: "vipsthumbnail", Purpose: "image and RAW thumbnails"},
	{Name: "ffmpeg", Purpose: "image fallback, RAW fallback, and video poster frames"},
	{Name: "pdftoppm", Purpose: "PDF first-page thumbnails"},
	{Name: "libreoffice", Purpose: "Office document conversion"},
	{Name: "prlimit", Purpose: "optional preview command CPU/memory/file limits"},
}

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
	if !supportsThumbnail(kind) {
		writeError(w, http.StatusUnsupportedMediaType, "thumbnail is not supported for this file type")
		return
	}

	cachePath := thumbnailCachePathInode(s.cfg.PreviewDir, user.ID, resolved.Logical, info, size)
	if _, err := os.Stat(cachePath); errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(filepath.Dir(cachePath), 0o750); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to create thumbnail cache")
			return
		}
		if err := s.generateThumbnail(r.Context(), resolved.Physical, kind, size, cachePath); err != nil {
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

func (s *Server) trashThumbnail(w http.ResponseWriter, r *http.Request, user store.User, session store.Session) {
	size := parseThumbSize(r.URL.Query().Get("size"))
	item, err := s.store.GetTrashItem(r.Context(), user.ID, r.PathValue("id"))
	if err != nil {
		writeError(w, statusForError(err), err.Error())
		return
	}
	if item.IsDir {
		writeError(w, http.StatusUnsupportedMediaType, "thumbnail is not supported for folders")
		return
	}
	info, err := os.Stat(item.TrashPath)
	if err != nil {
		writeError(w, statusForError(err), err.Error())
		return
	}
	kind := preview.KindForName(item.OriginalName)
	if !supportsThumbnail(kind) {
		writeError(w, http.StatusUnsupportedMediaType, "thumbnail is not supported for this file type")
		return
	}

	cachePath := thumbnailCachePathInode(s.cfg.PreviewDir, user.ID, "/.trash/"+item.ID+"/"+item.OriginalName, info, size)
	if _, err := os.Stat(cachePath); errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(filepath.Dir(cachePath), 0o750); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to create thumbnail cache")
			return
		}
		if err := s.generateThumbnail(r.Context(), item.TrashPath, kind, size, cachePath); err != nil {
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

func (s *Server) generateThumbnailsAsync(user store.User, entry files.Entry) {
	ctx := context.Background()
	resolved, info, err := s.files.ResolveForRead(user, entry.Path)
	if err != nil {
		s.log.Debug("post-upload thumbnail: resolve failed", "path", entry.Path, "err", err)
		return
	}
	for _, size := range previewWarmupSizes {
		cachePath := thumbnailCachePathInode(s.cfg.PreviewDir, user.ID, resolved.Logical, info, size)
		if _, err := os.Stat(cachePath); err == nil {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(cachePath), 0o750); err != nil {
			s.log.Debug("post-upload thumbnail: mkdir failed", "err", err)
			continue
		}
		if err := s.generateThumbnail(ctx, resolved.Physical, entry.PreviewKind, size, cachePath); err != nil {
			s.log.Debug("post-upload thumbnail: generate failed", "path", entry.Path, "size", size, "err", err)
		}
	}
	s.log.Debug("post-upload thumbnails generated", "path", entry.Path)
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
		return 2048
	}
	return size
}

// inodeKey extracts (inode, device) from FileInfo on Linux.
// Returns (0, 0) on other platforms or if the cast fails — callers fall back to path-based key.
func inodeKey(info os.FileInfo) (inode, device uint64) {
	if sys, ok := info.Sys().(*syscall.Stat_t); ok {
		return sys.Ino, sys.Dev
	}
	return 0, 0
}

func thumbnailCachePath(cacheRoot string, userID int64, logical string, fileSize, modTime int64, thumbSize int) string {
	sum := sha256.Sum256(fmt.Appendf(nil, "%d\x00%d\x00%s\x00%d\x00%d\x00%d", thumbnailCacheVersion, userID, logical, fileSize, modTime, thumbSize))
	return filepath.Join(cacheRoot, "thumbs", hex.EncodeToString(sum[:2]), hex.EncodeToString(sum[:])+".jpg")
}

// thumbnailCachePathInode builds a cache path keyed by inode+device rather than logical path.
// Stable across renames/moves on the same filesystem.
// Falls back to path-based key when inode is unavailable (non-Linux, cross-FS).
func thumbnailCachePathInode(cacheRoot string, userID int64, logical string, info os.FileInfo, thumbSize int) string {
	inode, device := inodeKey(info)
	if inode == 0 {
		return thumbnailCachePath(cacheRoot, userID, logical, info.Size(), info.ModTime().UnixNano(), thumbSize)
	}
	sum := sha256.Sum256(fmt.Appendf(nil, "%d\x00%d\x00%d\x00%d\x00%d\x00%d\x00%d",
		thumbnailCacheVersion, userID, inode, device, info.Size(), info.ModTime().UnixNano(), thumbSize))
	return filepath.Join(cacheRoot, "thumbs", hex.EncodeToString(sum[:2]), hex.EncodeToString(sum[:])+".jpg")
}

func generateThumbnail(ctx context.Context, source, kind string, size int, target string) error {
	return generateThumbnailWithTimeout(ctx, defaultPreviewTimeout, source, kind, size, target)
}

func (s *Server) generateThumbnail(ctx context.Context, source, kind string, size int, target string) error {
	return generateThumbnailWithTimeout(ctx, s.previewTimeout(), source, kind, size, target)
}

func (s *Server) previewTimeout() time.Duration {
	if s.cfg.PreviewTimeout <= 0 {
		return defaultPreviewTimeout
	}
	return s.cfg.PreviewTimeout
}

func generateThumbnailWithTimeout(ctx context.Context, timeout time.Duration, source, kind string, size int, target string) error {
	var err error
	source, err = filepath.Abs(source)
	if err != nil {
		return err
	}
	target, err = filepath.Abs(target)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		return err
	}
	tmp := target + ".tmp.jpg"
	_ = os.Remove(tmp)

	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	switch kind {
	case "image":
		err = generateImageThumbnail(ctx, source, size, tmp)
	case "raw":
		err = generateRAWThumbnail(ctx, source, size, tmp)
	case "video":
		err = generateVideoThumbnail(ctx, source, size, tmp)
	case "pdf":
		err = generatePDFThumbnail(ctx, source, size, tmp)
	case "office":
		err = generateOfficeThumbnail(ctx, source, size, tmp)
	default:
		err = errors.New("thumbnail is not supported for this file type")
	}
	if err != nil {
		_ = os.Remove(tmp)
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("thumbnail generation timed out after %s", timeout)
		}
		return err
	}
	return os.Rename(tmp, target)
}

func supportsThumbnail(kind string) bool {
	switch kind {
	case "image", "raw", "video", "pdf", "office":
		return true
	default:
		return false
	}
}

func generateImageThumbnail(ctx context.Context, source string, size int, target string) error {
	// vipsthumbnail: best quality, auto-applies EXIF rotation.
	vipsErr := runCommand(ctx, "vipsthumbnail", source, "--size", fmt.Sprintf("%dx%d", size, size), "-o", target+"[Q=90]")
	if vipsErr == nil {
		return nil
	}
	// ffmpeg: auto-applies EXIF rotation (4.1+), covers all image formats vips handles.
	filter := fmt.Sprintf("scale='min(%d,iw)':'min(%d,ih)':force_original_aspect_ratio=decrease", size, size)
	ffmpegErr := runCommand(ctx, "ffmpeg", "-hide_banner", "-loglevel", "error", "-y", "-i", source, "-vf", filter, target)
	if ffmpegErr == nil {
		return nil
	}
	return fmt.Errorf("image thumbnail requires vipsthumbnail or ffmpeg: vipsthumbnail: %v; ffmpeg: %v", vipsErr, ffmpegErr)
}

func generateRAWThumbnail(ctx context.Context, source string, size int, target string) error {
	if err := generateImageThumbnail(ctx, source, size, target); err != nil {
		return fmt.Errorf("RAW thumbnail generation requires libvips or ffmpeg RAW support: %w", err)
	}
	return nil
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

func generateOfficeThumbnail(ctx context.Context, source string, size int, target string) error {
	workDir, err := os.MkdirTemp(filepath.Dir(target), "office-preview-")
	if err != nil {
		return err
	}
	defer func() {
		_ = os.RemoveAll(workDir)
	}()

	if err := runCommand(ctx,
		"libreoffice",
		"--headless",
		"--nologo",
		"--nofirststartwizard",
		"--nodefault",
		"--norestore",
		"--convert-to",
		"pdf",
		"--outdir",
		workDir,
		source,
	); err != nil {
		return err
	}

	pdfPath := filepath.Join(workDir, strings.TrimSuffix(filepath.Base(source), filepath.Ext(source))+".pdf")
	if _, err := os.Stat(pdfPath); err != nil {
		return fmt.Errorf("office conversion did not produce a PDF: %w", err)
	}
	return generatePDFThumbnail(ctx, pdfPath, size, target)
}

func generateImageThumbnailStdlib(source string, size int, target string) error {
	file, err := os.Open(source)
	if err != nil {
		return err
	}
	defer func() {
		_ = file.Close()
	}()

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

	if err := jpeg.Encode(out, thumb, &jpeg.Options{Quality: 90}); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
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
	path, commandArgs, err := previewCommandSpec(name, args, exec.LookPath)
	if err != nil {
		return err
	}
	workDir, err := os.MkdirTemp("", "godrive-preview-command-")
	if err != nil {
		return err
	}
	defer func() {
		_ = os.RemoveAll(workDir)
	}()

	cmd := exec.Command(path, commandArgs...)
	output := &limitedBuffer{limit: previewCommandOutputCap}
	cmd.Stdout = output
	cmd.Stderr = output
	cmd.Dir = workDir
	env, err := previewCommandEnv(os.Environ(), workDir)
	if err != nil {
		return err
	}
	cmd.Env = env
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGKILL}

	if err := cmd.Start(); err != nil {
		return err
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err = <-done:
	case <-ctx.Done():
		killProcessGroup(cmd.Process.Pid)
		<-done
		return ctx.Err()
	}
	if err != nil {
		text := strings.TrimSpace(output.String())
		if text != "" {
			return fmt.Errorf("%s failed: %s", name, text)
		}
		return err
	}
	return nil
}

func previewCommandSpec(name string, args []string, lookup func(string) (string, error)) (string, []string, error) {
	path, err := lookup(name)
	if err != nil {
		return "", nil, err
	}
	prlimit, err := lookup("prlimit")
	if err != nil {
		return path, args, nil
	}
	limitedArgs := []string{
		fmt.Sprintf("--cpu=%d", previewCommandCPUTime),
		fmt.Sprintf("--as=%d", previewCommandMemory),
		fmt.Sprintf("--fsize=%d", previewCommandFileSize),
		fmt.Sprintf("--nofile=%d", previewCommandOpenFiles),
		"--",
		path,
	}
	limitedArgs = append(limitedArgs, args...)
	return prlimit, limitedArgs, nil
}

func PreviewToolStatuses() []PreviewToolStatus {
	return previewToolStatuses(exec.LookPath)
}

func previewToolStatuses(lookup func(string) (string, error)) []PreviewToolStatus {
	statuses := make([]PreviewToolStatus, 0, len(previewToolDefinitions))
	for _, definition := range previewToolDefinitions {
		status := definition
		path, err := lookup(definition.Name)
		if err != nil {
			status.Error = err.Error()
		} else {
			status.Available = true
			status.Path = path
		}
		statuses = append(statuses, status)
	}
	return statuses
}

func previewCommandEnv(base []string, workDir string) ([]string, error) {
	home := filepath.Join(workDir, "home")
	configDir := filepath.Join(workDir, "config")
	cacheDir := filepath.Join(workDir, "cache")
	dataDir := filepath.Join(workDir, "data")
	tmpDir := filepath.Join(workDir, "tmp")
	for _, dir := range []string{home, configDir, cacheDir, dataDir, tmpDir} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return nil, err
		}
	}
	filtered := make([]string, 0, len(base)+5)
	for _, entry := range base {
		key, _, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		switch key {
		case "HOME", "XDG_CONFIG_HOME", "XDG_CACHE_HOME", "XDG_DATA_HOME", "TMPDIR":
			continue
		default:
			filtered = append(filtered, entry)
		}
	}
	return append(filtered,
		"HOME="+home,
		"XDG_CONFIG_HOME="+configDir,
		"XDG_CACHE_HOME="+cacheDir,
		"XDG_DATA_HOME="+dataDir,
		"TMPDIR="+tmpDir,
	), nil
}

func killProcessGroup(pid int) {
	if pid <= 0 {
		return
	}
	if err := syscall.Kill(-pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		_ = syscall.Kill(pid, syscall.SIGKILL)
	}
}

type limitedBuffer struct {
	limit     int
	truncated bool
	data      []byte
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.limit <= 0 {
		b.truncated = true
		return len(p), nil
	}
	remaining := b.limit - len(b.data)
	if remaining <= 0 {
		b.truncated = true
		return len(p), nil
	}
	if len(p) > remaining {
		b.data = append(b.data, p[:remaining]...)
		b.truncated = true
		return len(p), nil
	}
	b.data = append(b.data, p...)
	return len(p), nil
}

func (b *limitedBuffer) String() string {
	if !b.truncated {
		return string(b.data)
	}
	return string(b.data) + " ... output truncated"
}
