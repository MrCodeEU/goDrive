package server

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"godrive/internal/config"
	"godrive/internal/files"
	"godrive/internal/store"
)

func TestCompleteUploadRejectsUnexpectedTempPath(t *testing.T) {
	t.Parallel()

	srv, st, user, _ := newUploadTestServer(t)
	outside := filepath.Join(t.TempDir(), "outside.part")
	if err := os.WriteFile(outside, []byte("payload"), 0o640); err != nil {
		t.Fatal(err)
	}

	if err := st.CreateUpload(t.Context(), store.Upload{
		ID:           "badfinalize",
		UserID:       user.ID,
		UploadLength: 7,
		Offset:       7,
		MetadataJSON: "{}",
		TargetDir:    "/",
		Filename:     "photo.jpg",
		TempPath:     outside,
	}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPatch, "/api/tus/badfinalize", nil)
	if _, err := srv.completeUpload(req, user, "badfinalize"); err == nil {
		t.Fatal("expected invalid temp path error")
	}

	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("unexpected temp path should not be moved: %v", err)
	}
	if _, err := os.Stat(filepath.Join(user.HomeRoot, "photo.jpg")); !os.IsNotExist(err) {
		t.Fatalf("final file should not exist, stat = %v", err)
	}
}

func TestTusDeleteRejectsUnexpectedTempPath(t *testing.T) {
	t.Parallel()

	srv, st, user, _ := newUploadTestServer(t)
	outside := filepath.Join(t.TempDir(), "outside.part")
	if err := os.WriteFile(outside, []byte("payload"), 0o640); err != nil {
		t.Fatal(err)
	}

	if err := st.CreateUpload(t.Context(), store.Upload{
		ID:           "baddelete",
		UserID:       user.ID,
		UploadLength: 7,
		Offset:       3,
		MetadataJSON: "{}",
		TargetDir:    "/",
		Filename:     "photo.jpg",
		TempPath:     outside,
	}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/tus/baddelete", nil)
	req.Header.Set("Tus-Resumable", tusVersion)
	req.SetPathValue("id", "baddelete")
	rec := httptest.NewRecorder()

	srv.tusDelete(rec, req, user, store.Session{})

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("unexpected temp path should not be removed: %v", err)
	}
	if _, err := st.GetUpload(t.Context(), "baddelete"); err != nil {
		t.Fatalf("upload record should remain after rejected delete: %v", err)
	}
}

func TestTusPatchRejectsSymlinkTempPath(t *testing.T) {
	t.Parallel()

	srv, st, user, uploadDir := newUploadTestServer(t)
	id := "badsymlink"
	userUploadDir := uploadUserDir(uploadDir, user.ID)
	if err := os.MkdirAll(userUploadDir, 0o750); err != nil {
		t.Fatal(err)
	}
	tempPath := expectedUploadTempPath(uploadDir, user.ID, id)
	outside := filepath.Join(t.TempDir(), "outside.part")
	if err := os.WriteFile(outside, []byte("safe"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, tempPath); err != nil {
		t.Fatal(err)
	}

	if err := st.CreateUpload(t.Context(), store.Upload{
		ID:           id,
		UserID:       user.ID,
		UploadLength: 5,
		Offset:       0,
		MetadataJSON: "{}",
		TargetDir:    "/",
		Filename:     "photo.jpg",
		TempPath:     tempPath,
	}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPatch, "/api/tus/"+id, strings.NewReader("x"))
	req.Header.Set("Tus-Resumable", tusVersion)
	req.Header.Set("Content-Type", "application/offset+octet-stream")
	req.Header.Set("Upload-Offset", "0")
	req.SetPathValue("id", id)
	rec := httptest.NewRecorder()

	srv.tusPatch(rec, req, user, store.Session{})

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	data, err := os.ReadFile(outside)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "safe" {
		t.Fatalf("symlink target was modified: %q", string(data))
	}
}

func newUploadTestServer(t *testing.T) (*Server, *store.Store, store.User, string) {
	t.Helper()

	st, err := store.Open(filepath.Join(t.TempDir(), "godrive.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}

	homeRoot := t.TempDir()
	user, err := st.CreateUser(t.Context(), store.User{
		Username:     "alice",
		PasswordHash: "hash",
		HomeRoot:     homeRoot,
	})
	if err != nil {
		t.Fatal(err)
	}

	uploadDir := t.TempDir()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := New(config.Config{UploadDir: uploadDir}, st, files.NewService(t.TempDir(), st), log)
	return srv, st, user, uploadDir
}
