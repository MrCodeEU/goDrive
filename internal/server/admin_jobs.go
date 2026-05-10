package server

import (
	"context"
	"errors"
	"io/fs"
	"mime"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"godrive/internal/auth"
	"godrive/internal/files"
	"godrive/internal/preview"
	"godrive/internal/store"
)

type AdminJobs struct {
	mu      sync.Mutex
	current *AdminJob
}

type AdminJob struct {
	ID         string     `json:"id"`
	Type       string     `json:"type"`
	Status     string     `json:"status"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	Total      int64      `json:"total"`
	TotalKnown bool       `json:"total_known"`
	Done       int64      `json:"done"`
	Failed     int64      `json:"failed"`
	Message    string     `json:"message"`
}

var errJobRunning = errors.New("admin job already running")

const (
	reindexBatchSize       = 500
	previewWarmupMaxJobs   = 64
	previewWarmupMinJobs   = 2
	adminProgressBatchSize = 25
)

func NewAdminJobs() *AdminJobs {
	return &AdminJobs{}
}

func (jobs *AdminJobs) Snapshot() *AdminJob {
	jobs.mu.Lock()
	defer jobs.mu.Unlock()
	if jobs.current == nil {
		return nil
	}
	copy := *jobs.current
	return &copy
}

func (jobs *AdminJobs) start(kind string) (*AdminJob, error) {
	jobs.mu.Lock()
	defer jobs.mu.Unlock()
	if jobs.current != nil && jobs.current.Status == "running" {
		return nil, errJobRunning
	}
	id, err := auth.RandomID(8)
	if err != nil {
		return nil, err
	}
	job := &AdminJob{
		ID:        id,
		Type:      kind,
		Status:    "running",
		StartedAt: time.Now().UTC(),
		Message:   "starting",
	}
	jobs.current = job
	copy := *job
	return &copy, nil
}

func (jobs *AdminJobs) update(id string, fn func(*AdminJob)) {
	jobs.mu.Lock()
	defer jobs.mu.Unlock()
	if jobs.current == nil || jobs.current.ID != id {
		return
	}
	fn(jobs.current)
}

func (s *Server) startReindexJob() (*AdminJob, error) {
	job, err := s.jobs.start("reindex")
	if err != nil {
		return nil, err
	}
	s.log.Info("admin job started", "job_id", job.ID, "type", job.Type)
	go s.runReindexJob(context.Background(), job.ID)
	return job, nil
}

func (s *Server) startPreviewWarmupJob() (*AdminJob, error) {
	job, err := s.jobs.start("preview_warmup")
	if err != nil {
		return nil, err
	}
	s.log.Info("admin job started", "job_id", job.ID, "type", job.Type)
	go s.runPreviewWarmupJob(context.Background(), job.ID)
	return job, nil
}

func (s *Server) runReindexJob(ctx context.Context, jobID string) {
	scanID := jobID + "-" + time.Now().UTC().Format("20060102T150405Z")
	users, err := s.store.ListUsers(ctx)
	if err != nil {
		s.finishJob(jobID, "failed", err.Error())
		return
	}

	s.jobs.update(jobID, func(job *AdminJob) {
		job.TotalKnown = false
		job.Message = "scanning user roots"
	})

	for _, user := range users {
		if user.Disabled {
			continue
		}
		if err := s.scanUserRoot(ctx, user, scanID, jobID); err != nil {
			s.jobs.update(jobID, func(job *AdminJob) {
				job.Failed++
				job.Message = err.Error()
			})
			continue
		}
		if _, err := s.store.DeleteFileIndexEntriesNotSeen(ctx, user.ID, scanID); err != nil {
			s.jobs.update(jobID, func(job *AdminJob) {
				job.Failed++
				job.Message = err.Error()
			})
			continue
		}
		s.jobs.update(jobID, func(job *AdminJob) {
			job.Message = "scanning user roots"
		})
	}

	if snapshot := s.jobs.Snapshot(); snapshot != nil && snapshot.ID == jobID && snapshot.Failed > 0 {
		s.finishJob(jobID, "failed", "reindex completed with errors")
		return
	}
	s.finishJob(jobID, "completed", "reindex completed")
}

func (s *Server) scanUserRoot(ctx context.Context, user store.User, scanID string, jobID string) error {
	root, err := filepath.Abs(user.HomeRoot)
	if err != nil {
		return err
	}
	batch := make([]store.FileIndexEntry, 0, reindexBatchSize)
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		if err := s.store.UpsertFileIndexEntries(ctx, batch); err != nil {
			return err
		}
		count := len(batch)
		last := batch[count-1].Path
		batch = batch[:0]
		s.jobs.update(jobID, func(job *AdminJob) {
			job.Done += int64(count)
			job.Message = "indexed " + user.Username + ":" + last
		})
		return nil
	}

	if err := filepath.WalkDir(root, func(physical string, d os.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if physical == root {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
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

		rel, err := filepath.Rel(root, physical)
		if err != nil {
			return err
		}
		logical, err := files.CleanLogical("/" + filepath.ToSlash(rel))
		if err != nil {
			return err
		}

		entryType := "file"
		if info.IsDir() {
			entryType = "dir"
		}
		indexEntry := store.FileIndexEntry{
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
		batch = append(batch, indexEntry)
		if len(batch) >= reindexBatchSize {
			return flush()
		}
		return nil
	}); err != nil {
		return err
	}
	return flush()
}

func (s *Server) runPreviewWarmupJob(ctx context.Context, jobID string) {
	candidates, err := s.store.ListPreviewCandidates(ctx)
	if err != nil {
		s.finishJob(jobID, "failed", err.Error())
		return
	}

	sizes := []int{240, 420, 1024}
	s.jobs.update(jobID, func(job *AdminJob) {
		job.Total = int64(len(candidates) * len(sizes))
		job.TotalKnown = true
		job.Message = "warming thumbnails"
	})
	if len(candidates) == 0 {
		s.finishJob(jobID, "completed", "no indexed preview candidates; run full reindex first")
		return
	}

	s.runPreviewWarmupWorkers(ctx, jobID, candidates, sizes)

	if snapshot := s.jobs.Snapshot(); snapshot != nil && snapshot.ID == jobID && snapshot.Failed > 0 {
		s.finishJob(jobID, "failed", "preview warmup completed with errors")
		return
	}
	s.finishJob(jobID, "completed", "preview warmup completed")
}

type previewWarmupTask struct {
	candidate store.PreviewCandidate
	size      int
}

type previewWarmupResult struct {
	username string
	path     string
	err      error
}

func (s *Server) runPreviewWarmupWorkers(ctx context.Context, jobID string, candidates []store.PreviewCandidate, sizes []int) {
	workers := previewWarmupWorkerCount(s.cfg.PreviewWorkers)
	tasks := make(chan previewWarmupTask, workers*2)
	results := make(chan previewWarmupResult, workers*2)

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for task := range tasks {
				results <- s.warmPreview(ctx, task)
			}
		}()
	}

	go func() {
		for _, candidate := range candidates {
			for _, size := range sizes {
				tasks <- previewWarmupTask{candidate: candidate, size: size}
			}
		}
		close(tasks)
		wg.Wait()
		close(results)
	}()

	var done int64
	var failed int64
	var last previewWarmupResult
	for result := range results {
		done++
		last = result
		if result.err != nil {
			failed++
		}
		if done%adminProgressBatchSize == 0 || done == int64(len(candidates)*len(sizes)) {
			currentDone := done
			currentFailed := failed
			currentLast := last
			s.jobs.update(jobID, func(job *AdminJob) {
				job.Done = currentDone
				job.Failed = currentFailed
				if currentLast.err != nil {
					job.Message = currentLast.err.Error()
				} else {
					job.Message = "warming " + currentLast.username + ":" + currentLast.path
				}
			})
		}
	}
}

func (s *Server) warmPreview(ctx context.Context, task previewWarmupTask) previewWarmupResult {
	candidate := task.candidate
	result := previewWarmupResult{username: candidate.Username, path: candidate.Path}
	resolved, info, err := s.files.ResolveForRead(store.User{ID: candidate.UserID, HomeRoot: candidate.HomeRoot}, candidate.Path)
	if err != nil {
		result.err = err
		return result
	}
	cachePath := thumbnailCachePath(s.cfg.PreviewDir, candidate.UserID, resolved.Logical, info.Size(), info.ModTime().UnixNano(), task.size)
	if _, err := os.Stat(cachePath); errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(filepath.Dir(cachePath), 0o750); err != nil {
			result.err = err
		} else if err := generateThumbnail(ctx, resolved.Physical, candidate.PreviewKind, task.size, cachePath); err != nil {
			result.err = err
		}
	} else if err != nil {
		result.err = err
	}
	return result
}

func previewWarmupWorkerCount(configured int) int {
	if configured > 0 {
		if configured > previewWarmupMaxJobs {
			return previewWarmupMaxJobs
		}
		return configured
	}
	workers := runtime.NumCPU() / 2
	if workers < previewWarmupMinJobs {
		workers = previewWarmupMinJobs
	}
	if workers > previewWarmupMaxJobs {
		workers = previewWarmupMaxJobs
	}
	return workers
}

func (s *Server) finishJob(jobID, status, message string) {
	now := time.Now().UTC()
	s.jobs.update(jobID, func(job *AdminJob) {
		job.Status = status
		job.FinishedAt = &now
		job.Message = message
	})
	s.log.Info("admin job finished", "job_id", jobID, "status", status, "message", message)
}
