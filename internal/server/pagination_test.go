package server

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"godrive/internal/config"
	"godrive/internal/files"
	"godrive/internal/store"
)

func TestListFilesPagination(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	for i := range 25 {
		if err := os.WriteFile(filepath.Join(root, fmt.Sprintf("file%02d.txt", i)), []byte("x"), 0o640); err != nil {
			t.Fatal(err)
		}
	}

	st, err := store.Open(filepath.Join(t.TempDir(), "godrive.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := New(config.Config{}, st, files.NewService(t.TempDir(), st), log)
	user := store.User{HomeRoot: root}

	// First page.
	req := httptest.NewRequest(http.MethodGet, "/api/files/list?path=/&limit=10&offset=0", nil)
	rec := httptest.NewRecorder()
	srv.listFiles(rec, req, user, store.Session{})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body struct {
		Entries    []any  `json:"entries"`
		Total      int    `json:"total"`
		Offset     int    `json:"offset"`
		Limit      int    `json:"limit"`
		HasMore    bool   `json:"has_more"`
		NextCursor string `json:"next_cursor"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Entries) != 10 {
		t.Fatalf("entries = %d, want 10", len(body.Entries))
	}
	if body.Total != 25 {
		t.Fatalf("total = %d, want 25", body.Total)
	}
	if !body.HasMore {
		t.Fatal("has_more should be true for first page")
	}
	if body.NextCursor == "" {
		t.Fatal("next_cursor should be set for first page")
	}

	// Last page.
	req2 := httptest.NewRequest(http.MethodGet, "/api/files/list?path=/&limit=10&offset=20", nil)
	rec2 := httptest.NewRecorder()
	srv.listFiles(rec2, req2, user, store.Session{})

	var body2 struct {
		Entries []any `json:"entries"`
		HasMore bool  `json:"has_more"`
	}
	if err := json.NewDecoder(rec2.Body).Decode(&body2); err != nil {
		t.Fatal(err)
	}
	if len(body2.Entries) != 5 {
		t.Fatalf("last page entries = %d, want 5", len(body2.Entries))
	}
	if body2.HasMore {
		t.Fatal("has_more should be false for last page")
	}
}

func TestListFilesCursorPagination(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "docs"), 0o750); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"alpha.txt", "bravo.txt", "charlie.txt"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("x"), 0o640); err != nil {
			t.Fatal(err)
		}
	}

	st, err := store.Open(filepath.Join(t.TempDir(), "godrive.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}

	srv := New(config.Config{}, st, files.NewService(t.TempDir(), st), slog.New(slog.NewTextHandler(io.Discard, nil)))
	user := store.User{HomeRoot: root}

	req := httptest.NewRequest(http.MethodGet, "/api/files/list?path=/&limit=2", nil)
	rec := httptest.NewRecorder()
	srv.listFiles(rec, req, user, store.Session{})

	var first struct {
		Entries []struct {
			Name string `json:"name"`
			Type string `json:"type"`
		} `json:"entries"`
		HasMore    bool   `json:"has_more"`
		NextCursor string `json:"next_cursor"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&first); err != nil {
		t.Fatal(err)
	}
	if got := []string{first.Entries[0].Name, first.Entries[1].Name}; fmt.Sprint(got) != "[docs alpha.txt]" {
		t.Fatalf("first page = %v, want [docs alpha.txt]", got)
	}
	if !first.HasMore || first.NextCursor == "" {
		t.Fatalf("first cursor state = has_more %v cursor %q", first.HasMore, first.NextCursor)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/api/files/list?path=/&limit=2&cursor="+first.NextCursor, nil)
	rec2 := httptest.NewRecorder()
	srv.listFiles(rec2, req2, user, store.Session{})

	var second struct {
		Entries []struct {
			Name string `json:"name"`
		} `json:"entries"`
		HasMore    bool   `json:"has_more"`
		NextCursor string `json:"next_cursor"`
	}
	if err := json.NewDecoder(rec2.Body).Decode(&second); err != nil {
		t.Fatal(err)
	}
	if got := []string{second.Entries[0].Name, second.Entries[1].Name}; fmt.Sprint(got) != "[bravo.txt charlie.txt]" {
		t.Fatalf("second page = %v, want [bravo.txt charlie.txt]", got)
	}
	if second.HasMore || second.NextCursor != "" {
		t.Fatalf("second cursor state = has_more %v cursor %q", second.HasMore, second.NextCursor)
	}
}

func TestListFilesRejectsInvalidCursor(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	st, err := store.Open(filepath.Join(t.TempDir(), "godrive.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}

	srv := New(config.Config{}, st, files.NewService(t.TempDir(), st), slog.New(slog.NewTextHandler(io.Discard, nil)))
	req := httptest.NewRequest(http.MethodGet, "/api/files/list?path=/&cursor=not-valid", nil)
	rec := httptest.NewRecorder()
	srv.listFiles(rec, req, store.User{HomeRoot: root}, store.Session{})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestListFilesIndexedCursorPaginationRandomized(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
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
		HomeRoot:     root,
	})
	if err != nil {
		t.Fatal(err)
	}

	rng := rand.New(rand.NewSource(42))
	modifiedAt := time.Date(2026, 5, 12, 12, 0, 0, 0, time.UTC)
	var indexEntries []store.FileIndexEntry
	for i := range 180 {
		isDir := i%5 == 0
		name := fmt.Sprintf("%s_%06d_%03d", map[bool]string{true: "dir", false: "file"}[isDir], rng.Intn(1_000_000), i)
		entryType := "file"
		if isDir {
			entryType = "dir"
			if err := os.Mkdir(filepath.Join(root, name), 0o750); err != nil {
				t.Fatal(err)
			}
		} else if err := os.WriteFile(filepath.Join(root, name), []byte("x"), 0o640); err != nil {
			t.Fatal(err)
		}
		indexEntries = append(indexEntries, store.FileIndexEntry{
			UserID:       user.ID,
			Path:         "/" + name,
			Name:         name,
			Type:         entryType,
			Size:         1,
			ModifiedAt:   modifiedAt,
			MimeType:     "text/plain",
			PreviewKind:  "text",
			LastSeenScan: "random",
		})
	}
	if err := st.UpsertFileIndexEntries(t.Context(), indexEntries); err != nil {
		t.Fatal(err)
	}

	expectedEntries, err := files.NewService("", nil).List(t.Context(), user, "/")
	if err != nil {
		t.Fatal(err)
	}
	expected := make([]string, 0, len(expectedEntries))
	for _, entry := range expectedEntries {
		expected = append(expected, entry.Path)
	}

	srv := New(config.Config{}, st, files.NewService(t.TempDir(), st), slog.New(slog.NewTextHandler(io.Discard, nil)))
	var got []string
	cursor := ""
	for page := 0; ; page++ {
		limit := 1 + rng.Intn(17)
		target := fmt.Sprintf("/api/files/list?path=/&limit=%d", limit)
		if cursor != "" {
			target += "&cursor=" + url.QueryEscape(cursor)
		}
		req := httptest.NewRequest(http.MethodGet, target, nil)
		rec := httptest.NewRecorder()
		srv.listFiles(rec, req, user, store.Session{})
		if rec.Code != http.StatusOK {
			t.Fatalf("page %d status = %d body=%s", page, rec.Code, rec.Body.String())
		}
		var body struct {
			Entries []struct {
				Path string `json:"path"`
			} `json:"entries"`
			HasMore    bool   `json:"has_more"`
			NextCursor string `json:"next_cursor"`
			Source     string `json:"source"`
		}
		if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Source != "index" {
			t.Fatalf("page %d source = %q, want index", page, body.Source)
		}
		for _, entry := range body.Entries {
			got = append(got, entry.Path)
		}
		if !body.HasMore {
			if body.NextCursor != "" {
				t.Fatalf("last page next_cursor = %q, want empty", body.NextCursor)
			}
			break
		}
		if body.NextCursor == "" {
			t.Fatalf("page %d has_more without next_cursor", page)
		}
		cursor = body.NextCursor
		if page > 1000 {
			t.Fatal("cursor pagination did not terminate")
		}
	}

	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("cursor sequence mismatch\ngot  %v\nwant %v", got, expected)
	}
}
