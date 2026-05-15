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

func newReindexServer(t *testing.T) (*Server, *store.Store, string) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "godrive.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := New(config.Config{PreviewDir: t.TempDir()}, st, files.NewService(t.TempDir(), st), log)
	return srv, st, root
}

func waitForJob(t *testing.T, srv *Server, jobID string) *AdminJob {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		snap := srv.jobs.Snapshot()
		if snap != nil && snap.ID == jobID && snap.Status != "running" {
			return snap
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("job %s did not complete within timeout", jobID)
	return nil
}

func TestReindexJobIndexesUserRoot(t *testing.T) {
	t.Parallel()

	srv, st, root := newReindexServer(t)

	user, err := st.CreateUser(t.Context(), store.User{
		Username:     "alice",
		PasswordHash: "hash",
		HomeRoot:     root,
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := os.MkdirAll(filepath.Join(root, "Photos"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "Photos", "a.jpg"), []byte("img"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "readme.txt"), []byte("this document mentions saffron"), 0o640); err != nil {
		t.Fatal(err)
	}

	job, err := srv.startReindexJob()
	if err != nil {
		t.Fatal(err)
	}
	if snap := waitForJob(t, srv, job.ID); snap.Status != "completed" {
		t.Fatalf("job status = %q, want completed", snap.Status)
	}

	results, err := st.SearchFileIndex(t.Context(), user.ID, "a.jpg", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Path != "/Photos/a.jpg" {
		t.Fatalf("search results = %+v, want /Photos/a.jpg", results)
	}
	results, err = st.SearchFileIndex(t.Context(), user.ID, "saffron", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Path != "/readme.txt" {
		t.Fatalf("document text search results = %+v, want /readme.txt", results)
	}

	stats, err := st.IndexStats(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if stats.IndexedFiles != 2 || stats.IndexedDirectories != 1 {
		t.Fatalf("index stats = %+v, want 2 files, 1 dir", stats)
	}
}

func TestReindexJobRemovesStaleEntries(t *testing.T) {
	t.Parallel()

	srv, st, root := newReindexServer(t)

	user, err := st.CreateUser(t.Context(), store.User{
		Username:     "bob",
		PasswordHash: "hash",
		HomeRoot:     root,
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := st.UpsertFileIndexEntry(t.Context(), store.FileIndexEntry{
		UserID:       user.ID,
		Path:         "/ghost.jpg",
		Name:         "ghost.jpg",
		Type:         "file",
		LastSeenScan: "old-scan",
	}); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(root, "real.txt"), []byte("data"), 0o640); err != nil {
		t.Fatal(err)
	}

	job, err := srv.startReindexJob()
	if err != nil {
		t.Fatal(err)
	}
	if snap := waitForJob(t, srv, job.ID); snap.Status != "completed" {
		t.Fatalf("job status = %q, want completed", snap.Status)
	}

	ghost, err := st.SearchFileIndex(t.Context(), user.ID, "ghost.jpg", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(ghost) != 0 {
		t.Fatalf("stale entry ghost.jpg still in index after reindex")
	}

	real, err := st.SearchFileIndex(t.Context(), user.ID, "real.txt", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(real) != 1 || real[0].Path != "/real.txt" {
		t.Fatalf("real.txt not indexed: %+v", real)
	}
}

func TestReindexJobSkipsDisabledUsers(t *testing.T) {
	t.Parallel()

	srv, st, root := newReindexServer(t)

	user, err := st.CreateUser(t.Context(), store.User{
		Username:     "carol",
		PasswordHash: "hash",
		HomeRoot:     root,
		Disabled:     true,
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(root, "file.txt"), []byte("data"), 0o640); err != nil {
		t.Fatal(err)
	}

	job, err := srv.startReindexJob()
	if err != nil {
		t.Fatal(err)
	}
	waitForJob(t, srv, job.ID)

	results, err := st.SearchFileIndex(t.Context(), user.ID, "file.txt", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("disabled user files should not be indexed, got %+v", results)
	}
}

func TestReindexPathRepairsSubfolderOnly(t *testing.T) {
	t.Parallel()

	srv, st, root := newReindexServer(t)

	user, err := st.CreateUser(t.Context(), store.User{
		Username:     "dana",
		PasswordHash: "hash",
		HomeRoot:     root,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "Projects"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "Projects", "notes.txt"), []byte("alpha scoped content"), 0o640); err != nil {
		t.Fatal(err)
	}
	for _, entry := range []store.FileIndexEntry{
		{UserID: user.ID, Path: "/Projects/old.txt", Name: "old.txt", Type: "file", LastSeenScan: "old-scan"},
		{UserID: user.ID, Path: "/outside-old.txt", Name: "outside-old.txt", Type: "file", LastSeenScan: "old-scan"},
	} {
		if err := st.UpsertFileIndexEntry(t.Context(), entry); err != nil {
			t.Fatal(err)
		}
	}

	job, err := srv.RunReindexPath(t.Context(), "dana", "/Projects")
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != "completed" {
		t.Fatalf("job status = %q, want completed: %+v", job.Status, job)
	}
	if job.Deleted != 1 || job.User != "dana" || job.Scope != "/Projects" {
		t.Fatalf("job diagnostics = %+v, want one scoped deletion for dana:/Projects", job)
	}

	projects, err := st.ListFileIndexFolder(t.Context(), user.ID, "/Projects", "", "", "", 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range projects.Entries {
		if entry.Path == "/Projects/old.txt" {
			t.Fatalf("scoped stale entry still indexed: %+v", projects.Entries)
		}
	}
	oldOutside, err := st.SearchFileIndex(t.Context(), user.ID, "outside-old.txt", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(oldOutside) != 1 || oldOutside[0].Path != "/outside-old.txt" {
		t.Fatalf("outside stale entry should be left alone, got %+v", oldOutside)
	}
	contentHit, err := st.SearchFileIndex(t.Context(), user.ID, "scoped", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(contentHit) != 1 || contentHit[0].Path != "/Projects/notes.txt" {
		t.Fatalf("document FTS not updated for scoped reindex: %+v", contentHit)
	}
}

func TestReindexPathRemovesMissingSubtree(t *testing.T) {
	t.Parallel()

	srv, st, root := newReindexServer(t)

	user, err := st.CreateUser(t.Context(), store.User{
		Username:     "erin",
		PasswordHash: "hash",
		HomeRoot:     root,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range []store.FileIndexEntry{
		{UserID: user.ID, Path: "/Missing", Name: "Missing", Type: "dir", LastSeenScan: "old-scan"},
		{UserID: user.ID, Path: "/Missing/ghost.txt", Name: "ghost.txt", Type: "file", LastSeenScan: "old-scan"},
	} {
		if err := st.UpsertFileIndexEntry(t.Context(), entry); err != nil {
			t.Fatal(err)
		}
	}

	job, err := srv.RunReindexPath(t.Context(), "erin", "/Missing")
	if err != nil {
		t.Fatal(err)
	}
	if job.Status != "completed" || job.Deleted != 2 {
		t.Fatalf("job = %+v, want completed with two deletions", job)
	}
	results, err := st.SearchFileIndex(t.Context(), user.ID, "ghost", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("missing subtree should be removed from index, got %+v", results)
	}
}
