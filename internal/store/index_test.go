package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestFileIndexStatsCandidatesAndCleanup(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	st, err := Open(filepath.Join(t.TempDir(), "godrive.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = st.Close()
	}()

	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	user, err := st.CreateUser(ctx, User{
		Username:     "alice",
		PasswordHash: "hash",
		HomeRoot:     t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}

	modifiedAt := time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC)
	entries := []FileIndexEntry{
		{
			UserID:       user.ID,
			Path:         "/Photos",
			Name:         "Photos",
			Type:         "dir",
			ModifiedAt:   modifiedAt,
			LastSeenScan: "scan-1",
		},
		{
			UserID:       user.ID,
			Path:         "/Photos/a.jpg",
			Name:         "a.jpg",
			Type:         "file",
			Size:         100,
			ModifiedAt:   modifiedAt,
			MimeType:     "image/jpeg",
			PreviewKind:  "image",
			LastSeenScan: "scan-1",
		},
		{
			UserID:       user.ID,
			Path:         "/notes.txt",
			Name:         "notes.txt",
			Type:         "file",
			Size:         25,
			ModifiedAt:   modifiedAt,
			MimeType:     "text/plain",
			PreviewKind:  "text",
			LastSeenScan: "old-scan",
		},
	}

	for _, entry := range entries {
		if err := st.UpsertFileIndexEntry(ctx, entry); err != nil {
			t.Fatal(err)
		}
	}

	stats, err := st.IndexStats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.IndexedFiles != 2 || stats.IndexedDirectories != 1 || stats.IndexedBytes != 125 || stats.PreviewCandidates != 1 {
		t.Fatalf("unexpected stats: %+v", stats)
	}

	candidates, err := st.ListPreviewCandidates(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || candidates[0].Path != "/Photos/a.jpg" {
		t.Fatalf("unexpected candidates: %+v", candidates)
	}

	deleted, err := st.DeleteFileIndexEntriesNotSeen(ctx, user.ID, "scan-1")
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 1 {
		t.Fatalf("deleted = %d, want 1", deleted)
	}

	stats, err = st.IndexStats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.IndexedFiles != 1 || stats.IndexedDirectories != 1 || stats.IndexedBytes != 100 {
		t.Fatalf("unexpected stats after cleanup: %+v", stats)
	}
}

func TestUpsertFileIndexEntriesBatchUpdatesExistingRows(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	st, err := Open(filepath.Join(t.TempDir(), "godrive.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = st.Close()
	}()

	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	user, err := st.CreateUser(ctx, User{
		Username:     "alice",
		PasswordHash: "hash",
		HomeRoot:     t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}

	modifiedAt := time.Date(2026, 5, 10, 10, 0, 0, 0, time.UTC)
	entries := []FileIndexEntry{
		{
			UserID:       user.ID,
			Path:         "/a.jpg",
			Name:         "a.jpg",
			Type:         "file",
			Size:         100,
			ModifiedAt:   modifiedAt,
			MimeType:     "image/jpeg",
			PreviewKind:  "image",
			LastSeenScan: "scan-1",
		},
		{
			UserID:       user.ID,
			Path:         "/b.jpg",
			Name:         "b.jpg",
			Type:         "file",
			Size:         200,
			ModifiedAt:   modifiedAt,
			MimeType:     "image/jpeg",
			PreviewKind:  "image",
			LastSeenScan: "scan-1",
		},
	}
	if err := st.UpsertFileIndexEntries(ctx, entries); err != nil {
		t.Fatal(err)
	}

	entries[0].Size = 150
	entries[0].LastSeenScan = "scan-2"
	if err := st.UpsertFileIndexEntries(ctx, entries[:1]); err != nil {
		t.Fatal(err)
	}

	stats, err := st.IndexStats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.IndexedFiles != 2 || stats.IndexedBytes != 350 || stats.PreviewCandidates != 2 {
		t.Fatalf("unexpected stats: %+v", stats)
	}

	deleted, err := st.DeleteFileIndexEntriesNotSeen(ctx, user.ID, "scan-2")
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 1 {
		t.Fatalf("deleted = %d, want 1", deleted)
	}
}

func TestSearchFileIndexFindsNameAndPathWithEscaping(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	st, err := Open(filepath.Join(t.TempDir(), "godrive.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = st.Close()
	}()

	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	user, err := st.CreateUser(ctx, User{
		Username:     "alice",
		PasswordHash: "hash",
		HomeRoot:     t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}

	otherUser, err := st.CreateUser(ctx, User{
		Username:     "bob",
		PasswordHash: "hash",
		HomeRoot:     t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}

	modifiedAt := time.Date(2026, 5, 10, 11, 0, 0, 0, time.UTC)
	entries := []FileIndexEntry{
		{
			UserID:       user.ID,
			Path:         "/Photos",
			Name:         "Photos",
			Type:         "dir",
			ModifiedAt:   modifiedAt,
			LastSeenScan: "scan-1",
		},
		{
			UserID:       user.ID,
			Path:         "/Photos/Vienna Trip.jpg",
			Name:         "Vienna Trip.jpg",
			Type:         "file",
			Size:         100,
			ModifiedAt:   modifiedAt,
			MimeType:     "image/jpeg",
			PreviewKind:  "image",
			LastSeenScan: "scan-1",
		},
		{
			UserID:       user.ID,
			Path:         "/Documents/100% literal.md",
			Name:         "100% literal.md",
			Type:         "file",
			Size:         50,
			ModifiedAt:   modifiedAt,
			MimeType:     "text/markdown",
			PreviewKind:  "text",
			LastSeenScan: "scan-1",
		},
		{
			UserID:       otherUser.ID,
			Path:         "/Photos/Vienna Secret.jpg",
			Name:         "Vienna Secret.jpg",
			Type:         "file",
			Size:         100,
			ModifiedAt:   modifiedAt,
			MimeType:     "image/jpeg",
			PreviewKind:  "image",
			LastSeenScan: "scan-1",
		},
	}
	if err := st.UpsertFileIndexEntries(ctx, entries); err != nil {
		t.Fatal(err)
	}

	results, err := st.SearchFileIndex(ctx, user.ID, "vienna", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Path != "/Photos/Vienna Trip.jpg" {
		t.Fatalf("unexpected name results: %+v", results)
	}

	results, err = st.SearchFileIndex(ctx, user.ID, "documents", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Path != "/Documents/100% literal.md" {
		t.Fatalf("unexpected path results: %+v", results)
	}

	results, err = st.SearchFileIndex(ctx, user.ID, "%", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Path != "/Documents/100% literal.md" {
		t.Fatalf("unexpected escaped results: %+v", results)
	}
}

func TestSearchFileIndexFindsDocumentText(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	st, err := Open(filepath.Join(t.TempDir(), "godrive.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = st.Close()
	}()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	user, err := st.CreateUser(ctx, User{
		Username:     "alice",
		PasswordHash: "hash",
		HomeRoot:     t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	otherUser, err := st.CreateUser(ctx, User{
		Username:     "bob",
		PasswordHash: "hash",
		HomeRoot:     t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}

	modifiedAt := time.Date(2026, 5, 10, 11, 0, 0, 0, time.UTC)
	entries := []FileIndexEntry{
		{UserID: user.ID, Path: "/notes.txt", Name: "notes.txt", Type: "file", Size: 10, ModifiedAt: modifiedAt, MimeType: "text/plain", PreviewKind: "text", LastSeenScan: "scan"},
		{UserID: otherUser.ID, Path: "/secret.txt", Name: "secret.txt", Type: "file", Size: 10, ModifiedAt: modifiedAt, MimeType: "text/plain", PreviewKind: "text", LastSeenScan: "scan"},
	}
	if err := st.UpsertFileIndexEntries(ctx, entries); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertDocumentTextEntries(ctx, []DocumentTextEntry{
		{UserID: user.ID, Path: "/notes.txt", Content: "family recipe uses saffron"},
		{UserID: otherUser.ID, Path: "/secret.txt", Content: "family recipe uses saffron"},
	}); err != nil {
		t.Fatal(err)
	}

	results, err := st.SearchFileIndex(ctx, user.ID, "saffron", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Path != "/notes.txt" {
		t.Fatalf("unexpected document results: %+v", results)
	}
}

func TestDeleteFileIndexPathDeletesSubtreeOnly(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	st, err := Open(filepath.Join(t.TempDir(), "godrive.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = st.Close()
	}()

	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	user, err := st.CreateUser(ctx, User{
		Username:     "alice",
		PasswordHash: "hash",
		HomeRoot:     t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}

	modifiedAt := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	entries := []FileIndexEntry{
		{UserID: user.ID, Path: "/Photos", Name: "Photos", Type: "dir", ModifiedAt: modifiedAt, LastSeenScan: "scan"},
		{UserID: user.ID, Path: "/Photos/a.jpg", Name: "a.jpg", Type: "file", ModifiedAt: modifiedAt, LastSeenScan: "scan"},
		{UserID: user.ID, Path: "/Photoshop/readme.txt", Name: "readme.txt", Type: "file", ModifiedAt: modifiedAt, LastSeenScan: "scan"},
	}
	if err := st.UpsertFileIndexEntries(ctx, entries); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertDocumentTextEntry(ctx, DocumentTextEntry{UserID: user.ID, Path: "/Photos/a.jpg", Content: "remove me"}); err != nil {
		t.Fatal(err)
	}

	deleted, err := st.DeleteFileIndexPath(ctx, user.ID, "/Photos")
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 2 {
		t.Fatalf("deleted = %d, want 2", deleted)
	}

	results, err := st.SearchFileIndex(ctx, user.ID, "photo", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Path != "/Photoshop/readme.txt" {
		t.Fatalf("unexpected remaining results: %+v", results)
	}
	results, err = st.SearchFileIndex(ctx, user.ID, "remove", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("deleted document text should not remain searchable: %+v", results)
	}
}

func TestBackfillFileIndexParentPathsUpdatesLegacyNestedRows(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	st, err := Open(filepath.Join(t.TempDir(), "godrive.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	user, err := st.CreateUser(ctx, User{
		Username:     "alice",
		PasswordHash: "hash",
		HomeRoot:     t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.ExecContext(ctx, `
		INSERT INTO file_index (
			user_id, path, parent_path, name, type, size, modified_at,
			mime_type, preview_kind, last_seen_scan, updated_at
		)
		VALUES (?, '/Photos/2026/a.jpg', '/', 'a.jpg', 'file', 1, ?, 'image/jpeg', 'image', 'scan', ?)
	`, user.ID, timeString(time.Now().UTC()), timeString(time.Now().UTC())); err != nil {
		t.Fatal(err)
	}
	if err := st.BackfillFileIndexParentPaths(ctx); err != nil {
		t.Fatal(err)
	}
	var parent string
	if err := st.db.QueryRowContext(ctx, `SELECT parent_path FROM file_index WHERE path = '/Photos/2026/a.jpg'`).Scan(&parent); err != nil {
		t.Fatal(err)
	}
	if parent != "/Photos/2026" {
		t.Fatalf("parent_path = %q, want /Photos/2026", parent)
	}
}

func TestMigrateAddsParentPathToExistingFileIndex(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	st, err := Open(filepath.Join(t.TempDir(), "godrive.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()

	if _, err := st.db.ExecContext(ctx, `
		CREATE TABLE file_index (
			user_id INTEGER NOT NULL,
			path TEXT NOT NULL,
			name TEXT NOT NULL,
			type TEXT NOT NULL,
			size INTEGER NOT NULL,
			modified_at TEXT NOT NULL,
			mime_type TEXT NOT NULL,
			preview_kind TEXT NOT NULL,
			last_seen_scan TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			PRIMARY KEY (user_id, path)
		)
	`); err != nil {
		t.Fatal(err)
	}
	now := timeString(time.Now().UTC())
	if _, err := st.db.ExecContext(ctx, `
		INSERT INTO file_index (
			user_id, path, name, type, size, modified_at,
			mime_type, preview_kind, last_seen_scan, updated_at
		)
		VALUES (1, '/Photos/2026/a.jpg', 'a.jpg', 'file', 1, ?, 'image/jpeg', 'image', 'scan', ?)
	`, now, now); err != nil {
		t.Fatal(err)
	}

	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	var parent string
	if err := st.db.QueryRowContext(ctx, `SELECT parent_path FROM file_index WHERE path = '/Photos/2026/a.jpg'`).Scan(&parent); err != nil {
		t.Fatal(err)
	}
	if parent != "/Photos/2026" {
		t.Fatalf("parent_path = %q, want /Photos/2026", parent)
	}
}
