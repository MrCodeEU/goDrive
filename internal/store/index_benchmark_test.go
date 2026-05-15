package store

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

const benchmarkIndexRows = 400_000

func BenchmarkFileIndex400k(b *testing.B) {
	ctx := context.Background()
	st, userID := benchmarkStore(b, benchmarkIndexRows)

	b.Run("search_filename", func(b *testing.B) {
		for b.Loop() {
			results, err := st.SearchFileIndex(ctx, userID, "IMG_0399999", 20)
			if err != nil {
				b.Fatal(err)
			}
			if len(results) == 0 {
				b.Fatal("expected at least one result")
			}
		}
	})

	b.Run("search_path_prefix_text", func(b *testing.B) {
		for b.Loop() {
			results, err := st.SearchFileIndex(ctx, userID, "Inbox", 50)
			if err != nil {
				b.Fatal(err)
			}
			if len(results) == 0 {
				b.Fatal("expected path results")
			}
		}
	})

	b.Run("stats", func(b *testing.B) {
		for b.Loop() {
			if _, err := st.IndexStats(ctx); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("list_folder_first_page", func(b *testing.B) {
		for b.Loop() {
			page, err := st.ListFileIndexFolder(ctx, userID, "/Inbox", "", "", "", 0, 501)
			if err != nil {
				b.Fatal(err)
			}
			if len(page.Entries) != 501 {
				b.Fatalf("entries = %d, want 501", len(page.Entries))
			}
		}
	})

	b.Run("list_folder_after_cursor", func(b *testing.B) {
		for b.Loop() {
			page, err := st.ListFileIndexFolder(ctx, userID, "/Inbox", "file", "img_0199999.jpg", "/Inbox/IMG_0199999.jpg", 0, 501)
			if err != nil {
				b.Fatal(err)
			}
			if len(page.Entries) != 501 {
				b.Fatalf("entries = %d, want 501", len(page.Entries))
			}
		}
	})

	b.Run("batch_upsert_500", func(b *testing.B) {
		entries := benchmarkEntries(userID, benchmarkIndexRows, benchmarkIndexRows+500)
		for b.Loop() {
			if err := st.UpsertFileIndexEntries(ctx, entries); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func TestStoreOpenEnablesWAL(t *testing.T) {
	t.Parallel()

	st, err := Open(filepath.Join(t.TempDir(), "godrive.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	var mode string
	if err := st.db.QueryRow(`PRAGMA journal_mode`).Scan(&mode); err != nil {
		t.Fatal(err)
	}
	if mode != "wal" {
		t.Fatalf("journal_mode = %q, want wal", mode)
	}
}

func benchmarkStore(b *testing.B, rows int) (*Store, int64) {
	b.Helper()

	ctx := context.Background()
	st, err := Open(filepath.Join(b.TempDir(), "godrive.sqlite"))
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = st.Close() })
	if err := st.Migrate(ctx); err != nil {
		b.Fatal(err)
	}
	user, err := st.CreateUser(ctx, User{
		Username:     "bench",
		PasswordHash: "hash",
		HomeRoot:     b.TempDir(),
	})
	if err != nil {
		b.Fatal(err)
	}

	const batchSize = 1000
	for start := 0; start < rows; start += batchSize {
		end := start + batchSize
		if end > rows {
			end = rows
		}
		if err := st.UpsertFileIndexEntries(ctx, benchmarkEntries(user.ID, start, end)); err != nil {
			b.Fatal(err)
		}
	}
	return st, user.ID
}

func benchmarkEntries(userID int64, start, end int) []FileIndexEntry {
	modifiedAt := time.Date(2026, 5, 12, 12, 0, 0, 0, time.UTC)
	entries := make([]FileIndexEntry, 0, end-start)
	for i := start; i < end; i++ {
		entries = append(entries, FileIndexEntry{
			UserID:       userID,
			Path:         fmt.Sprintf("/Inbox/IMG_%07d.jpg", i),
			Name:         fmt.Sprintf("IMG_%07d.jpg", i),
			Type:         "file",
			Size:         int64(2_000_000 + i%1_000_000),
			ModifiedAt:   modifiedAt.Add(time.Duration(i) * time.Second),
			MimeType:     "image/jpeg",
			PreviewKind:  "image",
			LastSeenScan: "bench",
		})
	}
	return entries
}
