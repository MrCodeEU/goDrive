package server

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"godrive/internal/store"
)

const uploadCleanupInterval = 6 * time.Hour

type UploadCleanupResult struct {
	ExpiredRecords int
	ExpiredFiles   int
	OrphanedFiles  int
}

func (s *Server) StartUploadCleanup(ctx context.Context, uploadDir string, ttl time.Duration) {
	if ttl <= 0 {
		return
	}
	s.cleanExpiredUploads(ctx, uploadDir, ttl)
	ticker := time.NewTicker(uploadCleanupInterval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.cleanExpiredUploads(ctx, uploadDir, ttl)
			}
		}
	}()
}

func (s *Server) cleanExpiredUploads(ctx context.Context, uploadDir string, ttl time.Duration) {
	_, _ = s.RunUploadCleanup(ctx, uploadDir, ttl)
}

func (s *Server) RunUploadCleanup(ctx context.Context, uploadDir string, ttl time.Duration) (UploadCleanupResult, error) {
	var result UploadCleanupResult
	if ttl <= 0 {
		return result, nil
	}
	cutoff := time.Now().UTC().Add(-ttl)
	expired, err := s.store.ListExpiredUploads(ctx, cutoff)
	if err != nil {
		return result, err
	}

	for i := range expired {
		tempPath, err := validateUploadTempPath(uploadDir, expired[i])
		if err != nil {
			s.log.Warn("upload cleanup: skipped unsafe temp path", "id", expired[i].ID, "err", err)
		} else if err := os.Remove(tempPath); err == nil {
			result.ExpiredFiles++
		}
		if err := s.store.DeleteUpload(ctx, expired[i].ID); err == nil {
			result.ExpiredRecords++
		}
	}
	if result.ExpiredRecords > 0 {
		s.log.Info("upload cleanup: removed expired uploads", "count", result.ExpiredRecords)
	}

	// Remove orphaned .part files — temp files with no DB record.
	orphans := s.scanOrphanedParts(ctx, uploadDir, expired)
	for _, path := range orphans {
		if err := os.Remove(path); err == nil {
			result.OrphanedFiles++
			s.log.Info("upload cleanup: removed orphaned temp file", "path", filepath.Base(path))
		}
	}
	return result, nil
}

func (s *Server) scanOrphanedParts(ctx context.Context, uploadDir string, justDeleted []store.Upload) []string {
	known := make(map[string]struct{}, len(justDeleted))
	for _, u := range justDeleted {
		known[filepath.Clean(u.TempPath)] = struct{}{}
	}

	var orphans []string
	_ = filepath.WalkDir(uploadDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".part") {
			return err
		}
		if _, ok := known[filepath.Clean(path)]; ok {
			return nil
		}
		// Check if this file has a DB record.
		id := strings.TrimSuffix(filepath.Base(path), ".part")
		if _, err := s.store.GetUpload(ctx, id); err != nil {
			// No record — orphan.
			orphans = append(orphans, path)
		}
		return nil
	})
	return orphans
}
