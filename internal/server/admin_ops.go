package server

import (
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"

	"godrive/internal/store"
)

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
		"current_job": s.jobs.Snapshot(),
	})
}

func (s *Server) currentAdminJob(w http.ResponseWriter, r *http.Request, admin store.User, session store.Session) {
	writeJSON(w, http.StatusOK, map[string]any{"job": s.jobs.Snapshot()})
}

func (s *Server) startReindex(w http.ResponseWriter, r *http.Request, admin store.User, session store.Session) {
	job, err := s.startReindexJob()
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
