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
	defer st.Close()

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
