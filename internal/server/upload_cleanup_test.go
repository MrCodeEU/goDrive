package server

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"godrive/internal/config"
	"godrive/internal/files"
	"godrive/internal/store"
)

func TestCleanExpiredUploadsRemovesStaleRecordsAndFiles(t *testing.T) {
	t.Parallel()

	st, err := store.Open(filepath.Join(t.TempDir(), "godrive.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}

	user, err := st.CreateUser(t.Context(), store.User{
		Username:     "alice",
		PasswordHash: "hash",
		HomeRoot:     t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}

	uploadDir := t.TempDir()
	partFile := filepath.Join(uploadUserDir(uploadDir, user.ID), "abc123.part")
	if err := os.MkdirAll(filepath.Dir(partFile), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(partFile, []byte("partial"), 0o640); err != nil {
		t.Fatal(err)
	}

	if err := st.CreateUpload(t.Context(), store.Upload{
		ID:           "abc123",
		UserID:       user.ID,
		UploadLength: 100,
		Offset:       10,
		MetadataJSON: "{}",
		TargetDir:    "/",
		Filename:     "test.jpg",
		TempPath:     partFile,
	}); err != nil {
		t.Fatal(err)
	}
	// Backdate updated_at so it appears 2 hours old.
	oldTime := time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339Nano)
	if _, err := st.DB().ExecContext(t.Context(), `UPDATE uploads SET updated_at = ? WHERE id = 'abc123'`, oldTime); err != nil {
		t.Fatal(err)
	}

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := New(config.Config{}, st, files.NewService(t.TempDir(), st), log)

	// TTL = 1h, upload is 2h old → should be cleaned.
	srv.cleanExpiredUploads(t.Context(), uploadDir, time.Hour)

	if _, err := os.Stat(partFile); !os.IsNotExist(err) {
		t.Fatal("expired temp file should be removed")
	}
	if _, err := st.GetUpload(t.Context(), "abc123"); err == nil {
		t.Fatal("expired upload DB record should be removed")
	}
}

func TestCleanExpiredUploadsIgnoresRecentUploads(t *testing.T) {
	t.Parallel()

	st, err := store.Open(filepath.Join(t.TempDir(), "godrive.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}

	user, err := st.CreateUser(t.Context(), store.User{
		Username:     "bob",
		PasswordHash: "hash",
		HomeRoot:     t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}

	uploadDir := t.TempDir()
	partFile := filepath.Join(uploadUserDir(uploadDir, user.ID), "recent.part")
	if err := os.MkdirAll(filepath.Dir(partFile), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(partFile, []byte("partial"), 0o640); err != nil {
		t.Fatal(err)
	}

	if err := st.CreateUpload(t.Context(), store.Upload{
		ID:           "recent",
		UserID:       user.ID,
		UploadLength: 100,
		Offset:       50,
		MetadataJSON: "{}",
		TargetDir:    "/",
		Filename:     "recent.jpg",
		TempPath:     partFile,
	}); err != nil {
		t.Fatal(err)
	}

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := New(config.Config{}, st, files.NewService(t.TempDir(), st), log)

	// TTL = 48h, upload is brand new → should not be cleaned.
	srv.cleanExpiredUploads(t.Context(), uploadDir, 48*time.Hour)

	if _, err := os.Stat(partFile); err != nil {
		t.Fatal("recent temp file should not be removed")
	}
	if _, err := st.GetUpload(t.Context(), "recent"); err != nil {
		t.Fatal("recent upload DB record should not be removed")
	}
}

func TestCleanExpiredUploadsRemovesOrphanedPartFiles(t *testing.T) {
	t.Parallel()

	st, err := store.Open(filepath.Join(t.TempDir(), "godrive.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}

	uploadDir := t.TempDir()
	orphan := filepath.Join(uploadDir, "orphanid.part")
	if err := os.WriteFile(orphan, []byte("orphan"), 0o640); err != nil {
		t.Fatal(err)
	}

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := New(config.Config{}, st, files.NewService(t.TempDir(), st), log)

	srv.cleanExpiredUploads(t.Context(), uploadDir, time.Hour)

	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		t.Fatal("orphaned .part file should be removed")
	}
}

func TestCleanExpiredUploadsDoesNotRemoveUnexpectedTempPath(t *testing.T) {
	t.Parallel()

	st, err := store.Open(filepath.Join(t.TempDir(), "godrive.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}

	user, err := st.CreateUser(t.Context(), store.User{
		Username:     "mallory",
		PasswordHash: "hash",
		HomeRoot:     t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}

	uploadDir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.part")
	if err := os.WriteFile(outside, []byte("do not delete"), 0o640); err != nil {
		t.Fatal(err)
	}

	if err := st.CreateUpload(t.Context(), store.Upload{
		ID:           "tampered",
		UserID:       user.ID,
		UploadLength: 100,
		Offset:       10,
		MetadataJSON: "{}",
		TargetDir:    "/",
		Filename:     "test.jpg",
		TempPath:     outside,
	}); err != nil {
		t.Fatal(err)
	}
	oldTime := time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339Nano)
	if _, err := st.DB().ExecContext(t.Context(), `UPDATE uploads SET updated_at = ? WHERE id = 'tampered'`, oldTime); err != nil {
		t.Fatal(err)
	}

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := New(config.Config{}, st, files.NewService(t.TempDir(), st), log)

	result, err := srv.RunUploadCleanup(t.Context(), uploadDir, time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	if result.ExpiredFiles != 0 {
		t.Fatalf("expired files = %d, want 0", result.ExpiredFiles)
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("unexpected temp path should not be removed: %v", err)
	}
	if _, err := st.GetUpload(t.Context(), "tampered"); err == nil {
		t.Fatal("expired upload DB record should still be removed")
	}
}
