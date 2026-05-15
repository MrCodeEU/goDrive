package watch

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/fsnotify/fsnotify"

	"godrive/internal/store"
)

func TestWatcherUpsertAndDeletePathUpdatesIndex(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "godrive.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = db.Close()
	}()
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	user, err := db.CreateUser(ctx, store.User{
		Username:     "alice",
		PasswordHash: "hash",
		HomeRoot:     root,
	})
	if err != nil {
		t.Fatal(err)
	}

	watcher, err := New(slog.Default(), db)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = watcher.Close()
	}()
	if err := watcher.AddUserRoot(user); err != nil {
		t.Fatal(err)
	}

	photos := filepath.Join(root, "Photos")
	if err := os.Mkdir(photos, 0o750); err != nil {
		t.Fatal(err)
	}
	photo := filepath.Join(photos, "a.jpg")
	if err := os.WriteFile(photo, []byte("fake"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := watcher.upsertPath(ctx, photos); err != nil {
		t.Fatal(err)
	}

	results, err := db.SearchFileIndex(ctx, user.ID, "a.jpg", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Path != "/Photos/a.jpg" {
		t.Fatalf("unexpected search results: %+v", results)
	}

	if err := os.Remove(photo); err != nil {
		t.Fatal(err)
	}
	if err := watcher.deletePath(ctx, photo); err != nil {
		t.Fatal(err)
	}
	results, err = db.SearchFileIndex(ctx, user.ID, "a.jpg", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("expected deleted file to be removed from index: %+v", results)
	}
}

func TestWatcherStatsTrackErrorsAndEvents(t *testing.T) {
	t.Parallel()

	watcher, err := New(slog.Default(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = watcher.Close()
	}()

	watcher.recordEvent()
	watcher.recordError(errors.New("overflow"))

	stats := watcher.Stats()
	if stats.Events != 1 {
		t.Fatalf("events = %d, want 1", stats.Events)
	}
	if stats.Errors != 1 || stats.LastError != "overflow" || stats.LastErrorAt.IsZero() {
		t.Fatalf("unexpected error stats: %+v", stats)
	}
	if !stats.NeedsRescan {
		t.Fatal("needs_rescan should be true after watcher error")
	}
	if stats.LastEventAt.IsZero() {
		t.Fatal("last_event_at should be set after watcher event")
	}

	watcher.ClearNeedsRescan()
	stats = watcher.Stats()
	if stats.NeedsRescan {
		t.Fatal("needs_rescan should clear after successful reconciliation")
	}
}

func TestWatcherCoalescesEventsByPath(t *testing.T) {
	t.Parallel()

	pending := make(map[string]fsnotify.Event)
	pending = enqueueEvent(pending, fsnotify.Event{Name: "/tmp/a.txt", Op: fsnotify.Write})
	pending = enqueueEvent(pending, fsnotify.Event{Name: "/tmp/a.txt", Op: fsnotify.Chmod})
	pending = enqueueEvent(pending, fsnotify.Event{Name: "/tmp/b.txt", Op: fsnotify.Create})

	events := coalescedEvents(pending)
	if len(events) != 2 {
		t.Fatalf("events = %d, want 2", len(events))
	}

	var merged fsnotify.Event
	for _, event := range events {
		if event.Name == "/tmp/a.txt" {
			merged = event
		}
	}
	if !merged.Has(fsnotify.Write) || !merged.Has(fsnotify.Chmod) {
		t.Fatalf("merged event missing ops: %s", merged.Op)
	}
}

func TestWatcherMixedCreateRemoveUsesFinalFilesystemState(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "godrive.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = db.Close()
	}()
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	user, err := db.CreateUser(ctx, store.User{
		Username:     "alice",
		PasswordHash: "hash",
		HomeRoot:     root,
	})
	if err != nil {
		t.Fatal(err)
	}

	watcher, err := New(slog.Default(), db)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = watcher.Close()
	}()
	if err := watcher.AddUserRoot(user); err != nil {
		t.Fatal(err)
	}

	physical := filepath.Join(root, "atomic.txt")
	if err := os.WriteFile(physical, []byte("content"), 0o640); err != nil {
		t.Fatal(err)
	}
	event := fsnotify.Event{Name: physical, Op: fsnotify.Remove | fsnotify.Create}
	if err := watcher.handleEvent(ctx, event); err != nil {
		t.Fatal(err)
	}

	results, err := db.SearchFileIndex(ctx, user.ID, "atomic.txt", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Path != "/atomic.txt" {
		t.Fatalf("unexpected results: %+v", results)
	}
}

func TestWatcherIndexesTextContent(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "godrive.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = db.Close()
	}()
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	user, err := db.CreateUser(ctx, store.User{
		Username:     "alice",
		PasswordHash: "hash",
		HomeRoot:     root,
	})
	if err != nil {
		t.Fatal(err)
	}

	watcher, err := New(slog.Default(), db)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = watcher.Close()
	}()
	if err := watcher.AddUserRoot(user); err != nil {
		t.Fatal(err)
	}

	note := filepath.Join(root, "note.txt")
	if err := os.WriteFile(note, []byte("the workshop checklist mentions turmeric"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := watcher.upsertPath(ctx, note); err != nil {
		t.Fatal(err)
	}
	results, err := db.SearchFileIndex(ctx, user.ID, "turmeric", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Path != "/note.txt" {
		t.Fatalf("unexpected text search results: %+v", results)
	}
}

func TestWatcherDoesNotIndexSymlinkTarget(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "godrive.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = db.Close()
	}()
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	user, err := db.CreateUser(ctx, store.User{
		Username:     "alice",
		PasswordHash: "hash",
		HomeRoot:     root,
	})
	if err != nil {
		t.Fatal(err)
	}

	watcher, err := New(slog.Default(), db)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = watcher.Close()
	}()
	if err := watcher.AddUserRoot(user); err != nil {
		t.Fatal(err)
	}

	outside := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outside, []byte("outside"), 0o640); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "linked-secret.txt")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}

	if err := watcher.upsertPath(ctx, link); err != nil {
		t.Fatal(err)
	}
	results, err := db.SearchFileIndex(ctx, user.ID, "linked-secret", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("symlink should not be indexed: %+v", results)
	}
}

func TestResolveUsesLongestMatchingRoot(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	parent := filepath.Join(base, "parent")
	child := filepath.Join(parent, "child")
	if err := os.MkdirAll(child, 0o750); err != nil {
		t.Fatal(err)
	}

	watcher, err := New(slog.Default(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = watcher.Close()
	}()
	watcher.roots = []root{
		{user: store.User{ID: 1, HomeRoot: parent}, path: parent},
		{user: store.User{ID: 2, HomeRoot: child}, path: child},
	}

	user, logical, ok := watcher.resolve(filepath.Join(child, "nested", "file.txt"))
	if !ok {
		t.Fatal("resolve failed")
	}
	if user.ID != 2 || logical != "/nested/file.txt" {
		t.Fatalf("resolved user=%d logical=%q", user.ID, logical)
	}
}

func TestSetUserRootsReplacesRootSet(t *testing.T) {
	t.Parallel()

	first := t.TempDir()
	second := t.TempDir()
	watcher, err := New(slog.Default(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = watcher.Close()
	}()

	if err := watcher.SetUserRoots([]store.User{{ID: 1, HomeRoot: first}}); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := watcher.resolve(filepath.Join(first, "file.txt")); !ok {
		t.Fatal("first root did not resolve")
	}

	if err := watcher.SetUserRoots([]store.User{{ID: 2, HomeRoot: second}}); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := watcher.resolve(filepath.Join(first, "file.txt")); ok {
		t.Fatal("old root still resolved")
	}
	user, logical, ok := watcher.resolve(filepath.Join(second, "file.txt"))
	if !ok {
		t.Fatal("second root did not resolve")
	}
	if user.ID != 2 || logical != "/file.txt" {
		t.Fatalf("resolved user=%d logical=%q", user.ID, logical)
	}
}
