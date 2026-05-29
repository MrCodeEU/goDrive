package server

import (
	"context"
	"os"
	"time"
)

const demoCleanupInterval = 15 * time.Second

func (s *Server) StartDemoUploadCleanup(ctx context.Context) {
	if !s.cfg.DemoMode || s.cfg.DemoUploadTTL <= 0 {
		return
	}
	go func() {
		ticker := time.NewTicker(demoCleanupInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.cleanExpiredDemoUploads(ctx)
			}
		}
	}()
}

func (s *Server) cleanExpiredDemoUploads(ctx context.Context) {
	now := time.Now()
	s.demoUploadsMu.Lock()
	var expired []demoUploadMeta
	for _, meta := range s.demoUploads {
		if now.Sub(meta.uploadedAt) >= s.cfg.DemoUploadTTL {
			expired = append(expired, meta)
		}
	}
	s.demoUploadsMu.Unlock()

	for _, meta := range expired {
		if meta.physicalPath != "" {
			_ = os.Remove(meta.physicalPath)
		}
		s.deleteIndexPath(ctx, meta.user, meta.logicalPath)
		s.fireEvent(meta.user, "file.deleted", map[string]any{"path": meta.logicalPath})

		s.demoUploadsMu.Lock()
		delete(s.demoUploads, meta.logicalPath)
		s.demoUploadsMu.Unlock()
	}
}
