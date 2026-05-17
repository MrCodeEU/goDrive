package watch

import (
	"context"
	"errors"
	"log/slog"
	"mime"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"godrive/internal/files"
	"godrive/internal/preview"
	"godrive/internal/store"
)

type root struct {
	user store.User
	path string
}

type Watcher struct {
	watcher  *fsnotify.Watcher
	store    *store.Store
	log      *slog.Logger
	mu       sync.Mutex
	seen     map[string]struct{}
	roots    []root
	health   Stats
	onChange func(ChangeEvent)
}

type ChangeEvent struct {
	User  store.User
	Event string
	Path  string
	Type  string
}

type Stats struct {
	Enabled      bool      `json:"enabled"`
	Roots        int       `json:"roots"`
	WatchedPaths int       `json:"watched_paths"`
	Events       int64     `json:"events"`
	Pending      int       `json:"pending"`
	Errors       int64     `json:"errors"`
	LastError    string    `json:"last_error,omitempty"`
	LastErrorAt  time.Time `json:"last_error_at,omitempty"`
	LastEventAt  time.Time `json:"last_event_at,omitempty"`
	LastIndexAt  time.Time `json:"last_index_at,omitempty"`
	NeedsRescan  bool      `json:"needs_rescan"`
}

const watcherDebounceDelay = 250 * time.Millisecond

func New(log *slog.Logger, st *store.Store) (*Watcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	return &Watcher{
		watcher: watcher,
		store:   st,
		log:     log,
		seen:    make(map[string]struct{}),
		health:  Stats{Enabled: true},
	}, nil
}

func (w *Watcher) Close() error {
	return w.watcher.Close()
}

func (w *Watcher) Stats() Stats {
	w.mu.Lock()
	defer w.mu.Unlock()
	return Stats{
		Enabled:      true,
		Roots:        len(w.roots),
		WatchedPaths: len(w.seen),
		Events:       w.health.Events,
		Pending:      w.health.Pending,
		Errors:       w.health.Errors,
		LastError:    w.health.LastError,
		LastErrorAt:  w.health.LastErrorAt,
		LastEventAt:  w.health.LastEventAt,
		LastIndexAt:  w.health.LastIndexAt,
		NeedsRescan:  w.health.NeedsRescan,
	}
}

func (w *Watcher) SetChangeHandler(handler func(ChangeEvent)) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.onChange = handler
}

func (w *Watcher) ClearNeedsRescan() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.health.NeedsRescan = false
}

func (w *Watcher) SetUserRoots(users []store.User) error {
	roots := make([]root, 0, len(users))
	for _, user := range users {
		if user.Disabled {
			continue
		}
		absolute, err := filepath.Abs(user.HomeRoot)
		if err != nil {
			return err
		}
		roots = append(roots, root{user: user, path: absolute})
	}

	w.mu.Lock()
	seen := make([]string, 0, len(w.seen))
	for path := range w.seen {
		seen = append(seen, path)
	}
	w.seen = make(map[string]struct{})
	w.roots = roots
	w.mu.Unlock()

	for _, path := range seen {
		if err := w.watcher.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			w.log.Debug("failed to remove old watcher path", "path", path, "err", err)
		}
	}

	for _, root := range roots {
		if err := w.AddRecursive(root.path); err != nil {
			return err
		}
	}
	w.log.Info("filesystem watcher roots reloaded", "roots", len(roots))
	return nil
}

func (w *Watcher) AddUserRoot(user store.User) error {
	absolute, err := filepath.Abs(user.HomeRoot)
	if err != nil {
		return err
	}
	w.mu.Lock()
	w.roots = append(w.roots, root{user: user, path: absolute})
	w.mu.Unlock()
	return w.AddRecursive(absolute)
}

func (w *Watcher) AddRecursive(rootPath string) error {
	return filepath.WalkDir(rootPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}
		if !d.IsDir() {
			return nil
		}
		return w.add(path)
	})
}

func (w *Watcher) Run(ctx context.Context) {
	pending := make(map[string]fsnotify.Event)
	var timer *time.Timer
	var timerC <-chan time.Time
	resetTimer := func() {
		if timer == nil {
			timer = time.NewTimer(watcherDebounceDelay)
			timerC = timer.C
			return
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(watcherDebounceDelay)
	}
	stopTimer := func() {
		if timer == nil {
			return
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timerC = nil
	}
	flush := func() {
		if len(pending) == 0 {
			stopTimer()
			w.setPending(0)
			return
		}
		events := coalescedEvents(pending)
		pending = make(map[string]fsnotify.Event)
		stopTimer()
		w.setPending(0)
		w.processEvents(ctx, events)
	}
	defer flush()

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			w.log.Debug("filesystem event", "path", event.Name, "op", event.Op.String())
			w.recordEvent()
			pending = enqueueEvent(pending, event)
			w.setPending(len(pending))
			resetTimer()
		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			w.recordError(err)
			w.log.Warn("filesystem watcher error", "err", err)
		case <-timerC:
			flush()
		}
	}
}

func (w *Watcher) recordEvent() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.health.Events++
	w.health.LastEventAt = time.Now().UTC()
}

func (w *Watcher) setPending(count int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.health.Pending = count
}

func (w *Watcher) recordError(err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.health.Errors++
	w.health.LastError = err.Error()
	w.health.LastErrorAt = time.Now().UTC()
	w.health.NeedsRescan = true
}

func (w *Watcher) recordIndexed() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.health.LastIndexAt = time.Now().UTC()
}

func enqueueEvent(pending map[string]fsnotify.Event, event fsnotify.Event) map[string]fsnotify.Event {
	if event.Name == "" {
		return pending
	}
	if previous, ok := pending[event.Name]; ok {
		previous.Op |= event.Op
		pending[event.Name] = previous
		return pending
	}
	pending[event.Name] = event
	return pending
}

func coalescedEvents(pending map[string]fsnotify.Event) []fsnotify.Event {
	events := make([]fsnotify.Event, 0, len(pending))
	for _, event := range pending {
		events = append(events, event)
	}
	return events
}

func (w *Watcher) processEvents(ctx context.Context, events []fsnotify.Event) {
	for _, event := range events {
		if err := w.handleEvent(ctx, event); err != nil {
			w.recordError(err)
			w.log.Warn("failed to index filesystem event", "path", event.Name, "op", event.Op.String(), "err", err)
		} else {
			w.recordIndexed()
		}
	}
}

func (w *Watcher) handleEvent(ctx context.Context, event fsnotify.Event) error {
	if event.Has(fsnotify.Create) || event.Has(fsnotify.Write) || event.Has(fsnotify.Chmod) {
		return w.upsertPath(ctx, event.Name)
	}
	if event.Has(fsnotify.Remove) || event.Has(fsnotify.Rename) {
		return w.deletePath(ctx, event.Name)
	}
	return nil
}

func (w *Watcher) upsertPath(ctx context.Context, physical string) error {
	user, logical, ok := w.resolve(physical)
	if !ok {
		return nil
	}
	info, err := os.Lstat(physical)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return w.deleteLogical(ctx, user, logical)
		}
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return w.deleteLogical(ctx, user, logical)
	}
	if info.IsDir() {
		if err := w.AddRecursive(physical); err != nil {
			return err
		}
		return w.indexRecursive(ctx, user, physical)
	}
	return w.upsertEntry(ctx, user, logical, physical, info, "watch")
}

func (w *Watcher) deletePath(ctx context.Context, physical string) error {
	user, logical, ok := w.resolve(physical)
	if !ok {
		return nil
	}
	return w.deleteLogical(ctx, user, logical)
}

func (w *Watcher) deleteLogical(ctx context.Context, user store.User, logical string) error {
	deleted, err := w.store.DeleteFileIndexPath(ctx, user.ID, logical)
	if err == nil && deleted > 0 {
		w.log.Info("file index removed external path", "path", logical, "entries", deleted)
		w.emitChange(ChangeEvent{
			User:  user,
			Event: "file.external_deleted",
			Path:  logical,
		})
	}
	return err
}

func (w *Watcher) indexRecursive(ctx context.Context, user store.User, physicalRoot string) error {
	scanID := "watch-" + time.Now().UTC().Format(time.RFC3339Nano)
	return filepath.WalkDir(physicalRoot, func(physical string, d os.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}
		info, err := d.Info()
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		_, logical, ok := w.resolve(physical)
		if !ok {
			return nil
		}
		return w.upsertEntry(ctx, user, logical, physical, info, scanID)
	})
}

func (w *Watcher) upsertEntry(ctx context.Context, user store.User, logical string, physical string, info os.FileInfo, scanID string) error {
	entryType := "file"
	if info.IsDir() {
		entryType = "dir"
	}
	entry := store.FileIndexEntry{
		UserID:       user.ID,
		Path:         logical,
		Name:         path.Base(logical),
		Type:         entryType,
		Size:         info.Size(),
		ModifiedAt:   info.ModTime().UTC(),
		MimeType:     mime.TypeByExtension(strings.ToLower(filepath.Ext(physical))),
		PreviewKind:  preview.KindForName(physical),
		LastSeenScan: scanID,
	}
	if err := w.store.UpsertFileIndexEntry(ctx, entry); err != nil {
		return err
	}
	if entry.Type == "file" && files.SupportsTextIndex(entry.PreviewKind) {
		content, err := files.ReadTextForIndex(physical)
		if err != nil {
			content = ""
		}
		if err := w.store.UpsertDocumentTextEntry(ctx, store.DocumentTextEntry{
			UserID:  user.ID,
			Path:    logical,
			Name:    entry.Name,
			Content: content,
		}); err != nil {
			return err
		}
	}
	w.log.Info("file index updated external path", "path", logical, "type", entryType)
	w.emitChange(ChangeEvent{
		User:  user,
		Event: "file.external_changed",
		Path:  logical,
		Type:  entryType,
	})
	return nil
}

func (w *Watcher) emitChange(event ChangeEvent) {
	w.mu.Lock()
	handler := w.onChange
	w.mu.Unlock()
	if handler != nil {
		handler(event)
	}
}

func (w *Watcher) resolve(physical string) (store.User, string, bool) {
	absolute, err := filepath.Abs(physical)
	if err != nil {
		return store.User{}, "", false
	}

	w.mu.Lock()
	roots := append([]root(nil), w.roots...)
	w.mu.Unlock()

	var match root
	for _, candidate := range roots {
		if !isWithin(candidate.path, absolute) {
			continue
		}
		if match.path == "" || len(candidate.path) > len(match.path) {
			match = candidate
		}
	}
	if match.path == "" {
		return store.User{}, "", false
	}
	rel, err := filepath.Rel(match.path, absolute)
	if err != nil || rel == "." {
		return store.User{}, "", false
	}
	logical, err := files.CleanLogical("/" + filepath.ToSlash(rel))
	if err != nil {
		return store.User{}, "", false
	}
	return match.user, logical, true
}

func isWithin(rootPath string, target string) bool {
	if target == rootPath {
		return true
	}
	rel, err := filepath.Rel(rootPath, target)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func (w *Watcher) add(path string) error {
	clean, err := filepath.Abs(path)
	if err != nil {
		return err
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	if _, ok := w.seen[clean]; ok {
		return nil
	}
	if err := w.watcher.Add(clean); err != nil {
		return err
	}
	w.seen[clean] = struct{}{}
	return nil
}
