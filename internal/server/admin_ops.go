package server

import (
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"godrive/internal/store"
)

type reindexRequest struct {
	Username string `json:"username"`
	Path     string `json:"path"`
}

func (s *Server) adminStats(w http.ResponseWriter, r *http.Request, admin store.User, session store.Session) {
	indexStats, err := s.store.IndexStats(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load index stats")
		return
	}
	totalUsers, disabledUsers, err := s.store.UserStats(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load user stats")
		return
	}
	trashCount, trashBytes, err := s.store.TrashStats(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load trash stats")
		return
	}
	cacheFiles, cacheBytes, err := directoryStats(s.cfg.PreviewDir)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load preview cache stats")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"users": map[string]any{
			"total":    totalUsers,
			"disabled": disabledUsers,
		},
		"index": indexStats,
		"trash": map[string]any{
			"items": trashCount,
			"bytes": trashBytes,
		},
		"preview_cache": map[string]any{
			"files": cacheFiles,
			"bytes": cacheBytes,
		},
		"preview": map[string]any{
			"workers": previewWarmupWorkerCount(s.cfg.PreviewWorkers),
			"sizes":   previewWarmupSizes,
			"tools":   PreviewToolStatuses(),
		},
		"watcher":        s.watcherStats(),
		"reconciliation": s.reconciliationStats(),
		"current_job":    s.jobs.Snapshot(),
	})
}

func (s *Server) watcherStats() map[string]any {
	if s.watcher == nil {
		return map[string]any{
			"enabled":       false,
			"roots":         0,
			"watched_paths": 0,
			"events":        0,
			"pending":       0,
			"errors":        0,
			"needs_rescan":  false,
		}
	}
	stats := s.watcher.Stats()
	return map[string]any{
		"enabled":       stats.Enabled,
		"roots":         stats.Roots,
		"watched_paths": stats.WatchedPaths,
		"events":        stats.Events,
		"pending":       stats.Pending,
		"errors":        stats.Errors,
		"last_error":    stats.LastError,
		"last_error_at": stats.LastErrorAt,
		"last_event_at": stats.LastEventAt,
		"last_index_at": stats.LastIndexAt,
		"needs_rescan":  stats.NeedsRescan,
	}
}

func (s *Server) reconciliationStats() map[string]any {
	interval := s.cfg.ReconcileInterval
	return map[string]any{
		"enabled":          interval > 0,
		"interval_seconds": int64(interval / time.Second),
		"interval":         interval.String(),
	}
}

func (s *Server) currentAdminJob(w http.ResponseWriter, r *http.Request, admin store.User, session store.Session) {
	writeJSON(w, http.StatusOK, map[string]any{"job": s.jobs.Snapshot()})
}

func (s *Server) cancelAdminJob(w http.ResponseWriter, r *http.Request, admin store.User, session store.Session) {
	job := s.jobs.CancelCurrent()
	if job == nil {
		writeError(w, http.StatusConflict, "no running admin job")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"job": job})
}

func (s *Server) startReindex(w http.ResponseWriter, r *http.Request, admin store.User, session store.Session) {
	var req reindexRequest
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			writeError(w, http.StatusBadRequest, "invalid reindex request")
			return
		}
	}
	var job *AdminJob
	var err error
	if req.Path != "" {
		if req.Username == "" {
			writeError(w, http.StatusBadRequest, "username is required for scoped reindex")
			return
		}
		job, err = s.startReindexPathJob(req.Username, req.Path)
	} else {
		job, err = s.startReindexJob()
	}
	if err != nil {
		if errors.Is(err, errJobRunning) {
			writeError(w, http.StatusConflict, "another admin job is already running")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to start reindex")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"job": job})
}

func (s *Server) startPreviewWarmup(w http.ResponseWriter, r *http.Request, admin store.User, session store.Session) {
	job, err := s.startPreviewWarmupJob()
	if err != nil {
		if errors.Is(err, errJobRunning) {
			writeError(w, http.StatusConflict, "another admin job is already running")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to start preview warmup")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"job": job})
}

func (s *Server) clearPreviewCache(w http.ResponseWriter, r *http.Request, admin store.User, session store.Session) {
	if err := s.cfg.ValidatePreviewCacheDir(); err != nil {
		writeError(w, http.StatusInternalServerError, "invalid preview cache directory")
		return
	}
	if err := os.RemoveAll(s.cfg.PreviewDir); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to clear preview cache")
		return
	}
	if err := os.MkdirAll(s.cfg.PreviewDir, 0o750); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to recreate preview cache")
		return
	}
	s.log.Info("preview cache cleared", "dir", s.cfg.PreviewDir)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func directoryStats(root string) (files int64, bytes int64, err error) {
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			if errors.Is(walkErr, fs.ErrNotExist) {
				return nil
			}
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		files++
		bytes += info.Size()
		return nil
	})
	if errors.Is(err, fs.ErrNotExist) {
		return 0, 0, nil
	}
	return files, bytes, err
}
