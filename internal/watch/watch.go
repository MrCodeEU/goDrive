package watch

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/fsnotify/fsnotify"
)

type Watcher struct {
	watcher *fsnotify.Watcher
	log     *slog.Logger
	mu      sync.Mutex
	seen    map[string]struct{}
}

func New(log *slog.Logger) (*Watcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	return &Watcher{
		watcher: watcher,
		log:     log,
		seen:    make(map[string]struct{}),
	}, nil
}

func (w *Watcher) Close() error {
	return w.watcher.Close()
}

func (w *Watcher) AddRecursive(root string) error {
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
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
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			w.log.Debug("filesystem event", "path", event.Name, "op", event.Op.String())
			if event.Has(fsnotify.Create) {
				info, err := os.Stat(event.Name)
				if err == nil && info.IsDir() {
					if err := w.AddRecursive(event.Name); err != nil {
						w.log.Warn("failed to add created directory watcher", "path", event.Name, "err", err)
					}
				}
			}
		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			w.log.Warn("filesystem watcher error", "err", err)
		}
	}
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
