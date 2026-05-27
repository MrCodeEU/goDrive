package server

import (
	"context"
	"errors"
	"fmt"
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
	Deleted    int64      `json:"deleted"`
	User       string     `json:"user,omitempty"`
	Scope      string     `json:"scope,omitempty"`
	Cancelable bool       `json:"cancelable"`
	Message    string     `json:"message"`
	context    context.Context
	cancel     context.CancelFunc
}

var errJobRunning = errors.New("admin job already running")

const (
	reindexBatchSize                 = 2000
	previewWarmupMaxJobs             = 64
	previewWarmupMinJobs             = 2
	adminProgressBatchSize           = 25
	watcherReconciliationCheckPeriod = time.Minute
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
	return jobs.startFrom(context.Background(), kind)
}

func (jobs *AdminJobs) startFrom(parent context.Context, kind string) (*AdminJob, error) {
	jobs.mu.Lock()
	defer jobs.mu.Unlock()
	if jobs.current != nil && jobs.current.Status == "running" {
		return nil, errJobRunning
	}
	id, err := auth.RandomID(8)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(parent)
	job := &AdminJob{
		ID:         id,
		Type:       kind,
		Status:     "running",
		StartedAt:  time.Now().UTC(),
		Cancelable: true,
		Message:    "starting",
		context:    ctx,
		cancel:     cancel,
	}
	jobs.current = job
	copy := *job
	return &copy, nil
}

func (jobs *AdminJobs) CancelCurrent() *AdminJob {
	jobs.mu.Lock()
	defer jobs.mu.Unlock()
	if jobs.current == nil || jobs.current.Status != "running" {
		return nil
	}
	jobs.current.Message = "cancel requested"
	jobs.current.cancel()
	copy := *jobs.current
	return &copy
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
	go s.runReindexJob(job.context, job.ID)
	return job, nil
}

func (s *Server) startReindexPathJob(username string, logical string) (*AdminJob, error) {
	user, err := s.store.GetUserByUsername(context.Background(), username)
	if err != nil {
		return nil, err
	}
	job, err := s.jobs.start("reindex")
	if err != nil {
		return nil, err
	}
	job.User = user.Username
	job.Scope = logical
	s.jobs.update(job.ID, func(current *AdminJob) {
		current.User = user.Username
		current.Scope = logical
	})
	s.log.Info("admin job started", "job_id", job.ID, "type", job.Type, "username", username, "scope", logical)
	go s.runReindexPath(job.context, job.ID, user, logical)
	return job, nil
}

func (s *Server) RunReindex(ctx context.Context) (*AdminJob, error) {
	job, err := s.jobs.startFrom(ctx, "reindex")
	if err != nil {
		return nil, err
	}
	s.log.Info("maintenance job started", "job_id", job.ID, "type", job.Type)
	s.runReindexJob(ctx, job.ID)
	return s.jobs.Snapshot(), nil
}

func (s *Server) RunReindexUser(ctx context.Context, username string) (*AdminJob, error) {
	user, err := s.store.GetUserByUsername(ctx, username)
	if err != nil {
		return nil, err
	}
	job, err := s.jobs.startFrom(ctx, "reindex")
	if err != nil {
		return nil, err
	}
	s.log.Info("maintenance job started", "job_id", job.ID, "type", job.Type, "username", username)
	s.runReindexUsers(ctx, job.ID, []store.User{user})
	return s.jobs.Snapshot(), nil
}

func (s *Server) RunReindexPath(ctx context.Context, username string, logical string) (*AdminJob, error) {
	user, err := s.store.GetUserByUsername(ctx, username)
	if err != nil {
		return nil, err
	}
	job, err := s.jobs.startFrom(ctx, "reindex")
	if err != nil {
		return nil, err
	}
	s.jobs.update(job.ID, func(current *AdminJob) {
		current.User = user.Username
		current.Scope = logical
	})
	s.log.Info("maintenance job started", "job_id", job.ID, "type", job.Type, "username", username, "scope", logical)
	s.runReindexPath(ctx, job.ID, user, logical)
	return s.jobs.Snapshot(), nil
}

func (s *Server) StartReconciliation(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		return
	}
	s.log.Info("reconciliation scanner enabled", "interval", interval.String())
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.startReconciliationJob(ctx, "scheduled", nil)
			}
		}
	}()
}

func (s *Server) StartWatcherReconciliation(ctx context.Context) {
	s.StartWatcherReconciliationCheck(ctx, watcherReconciliationCheckPeriod)
}

func (s *Server) StartWatcherReconciliationCheck(ctx context.Context, interval time.Duration) {
	if interval <= 0 || s.watcher == nil {
		return
	}
	s.log.Info("watcher reconciliation monitor enabled", "interval", interval.String())
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				stats := s.watcher.Stats()
				if !stats.NeedsRescan {
					continue
				}
				s.startReconciliationJob(ctx, "watcher_rescan", func(job *AdminJob) {
					if job != nil && job.Status == "completed" {
						s.watcher.ClearNeedsRescan()
					}
				})
			}
		}
	}()
}

func (s *Server) startReconciliationJob(ctx context.Context, reason string, after func(*AdminJob)) {
	job, err := s.jobs.startFrom(ctx, "reconciliation")
	if err != nil {
		if errors.Is(err, errJobRunning) {
			s.log.Info("reconciliation skipped because another admin job is running")
			return
		}
		s.log.Warn("failed to start reconciliation", "err", err)
		return
	}
	s.log.Info("admin job started", "job_id", job.ID, "type", job.Type, "reason", reason)
	go func() {
		s.runReindexJob(ctx, job.ID)
		if after != nil {
			after(s.jobs.Snapshot())
		}
	}()
}

func (s *Server) startPreviewWarmupJob() (*AdminJob, error) {
	job, err := s.jobs.start("preview_warmup")
	if err != nil {
		return nil, err
	}
	s.log.Info("admin job started", "job_id", job.ID, "type", job.Type)
	go s.runPreviewWarmupJob(job.context, job.ID)
	return job, nil
}

func (s *Server) RunPreviewWarmup(ctx context.Context) (*AdminJob, error) {
	job, err := s.jobs.startFrom(ctx, "preview_warmup")
	if err != nil {
		return nil, err
	}
	s.log.Info("maintenance job started", "job_id", job.ID, "type", job.Type)
	s.runPreviewWarmupJob(ctx, job.ID)
	return s.jobs.Snapshot(), nil
}

func (s *Server) runReindexJob(ctx context.Context, jobID string) {
	users, err := s.store.ListUsers(ctx)
	if err != nil {
		s.finishJob(jobID, "failed", err.Error())
		return
	}
	s.runReindexUsers(ctx, jobID, users)
}

func (s *Server) runReindexUsers(ctx context.Context, jobID string, users []store.User) {
	scanID := jobID + "-" + time.Now().UTC().Format("20060102T150405Z")
	s.jobs.update(jobID, func(job *AdminJob) {
		job.TotalKnown = false
		job.Message = "scanning user roots"
	})

	for _, user := range users {
		if err := ctx.Err(); err != nil {
			s.finishJob(jobID, "canceled", "reindex canceled")
			return
		}
		if user.Disabled {
			continue
		}
		if err := s.scanUserRoot(ctx, user, scanID, jobID); err != nil {
			if errors.Is(err, context.Canceled) {
				s.finishJob(jobID, "canceled", "reindex canceled")
				return
			}
			s.jobs.update(jobID, func(job *AdminJob) {
				job.Failed++
				job.Message = err.Error()
			})
			continue
		}
		deleted, err := s.store.DeleteFileIndexEntriesNotSeen(ctx, user.ID, scanID)
		if err != nil {
			s.jobs.update(jobID, func(job *AdminJob) {
				job.Failed++
				job.Message = err.Error()
			})
			continue
		}
		s.jobs.update(jobID, func(job *AdminJob) {
			job.Deleted += deleted
			job.Message = "scanning user roots"
		})
	}

	if snapshot := s.jobs.Snapshot(); snapshot != nil && snapshot.ID == jobID && snapshot.Failed > 0 {
		s.finishJob(jobID, "failed", "reindex completed with errors")
		return
	}
	if err := ctx.Err(); err != nil {
		s.finishJob(jobID, "canceled", "reindex canceled")
		return
	}
	s.jobs.update(jobID, func(job *AdminJob) {
		job.Message = "rebuilding search index"
	})
	if err := s.store.RebuildFileIndexFTS(ctx); err != nil {
		if errors.Is(err, context.Canceled) {
			s.finishJob(jobID, "canceled", "reindex canceled")
			return
		}
		s.finishJob(jobID, "failed", err.Error())
		return
	}
	s.finishJob(jobID, "completed", "reindex completed")
}

func (s *Server) runReindexPath(ctx context.Context, jobID string, user store.User, logical string) {
	if err := ctx.Err(); err != nil {
		s.finishJob(jobID, "canceled", "subpath reindex canceled")
		return
	}
	scanID := jobID + "-" + time.Now().UTC().Format("20060102T150405Z")
	cleanLogical, err := files.CleanLogical(logical)
	if err != nil {
		s.finishJob(jobID, "failed", err.Error())
		return
	}
	s.jobs.update(jobID, func(job *AdminJob) {
		job.TotalKnown = false
		job.User = user.Username
		job.Scope = cleanLogical
		job.Message = "scanning " + user.Username + ":" + cleanLogical
	})
	if user.Disabled {
		s.finishJob(jobID, "failed", "user is disabled")
		return
	}
	if err := s.scanUserPath(ctx, user, cleanLogical, scanID, jobID); err != nil {
		if errors.Is(err, context.Canceled) {
			s.finishJob(jobID, "canceled", "subpath reindex canceled")
			return
		}
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, os.ErrNotExist) {
			deleted, deleteErr := s.store.DeleteFileIndexPath(ctx, user.ID, cleanLogical)
			if deleteErr != nil {
				s.jobs.update(jobID, func(job *AdminJob) {
					job.Failed++
					job.Message = deleteErr.Error()
				})
				s.finishJob(jobID, "failed", "subpath repair completed with errors")
				return
			}
			s.jobs.update(jobID, func(job *AdminJob) {
				job.Deleted += deleted
			})
			s.finishJob(jobID, "completed", "subpath missing; stale index entries removed")
			return
		}
		s.jobs.update(jobID, func(job *AdminJob) {
			job.Failed++
			job.Message = err.Error()
		})
		s.finishJob(jobID, "failed", "subpath reindex completed with errors")
		return
	}
	deleted, err := s.store.DeleteFileIndexEntriesNotSeenUnder(ctx, user.ID, scanID, cleanLogical)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			s.finishJob(jobID, "canceled", "subpath reindex canceled")
			return
		}
		s.jobs.update(jobID, func(job *AdminJob) {
			job.Failed++
			job.Message = err.Error()
		})
		s.finishJob(jobID, "failed", "subpath reindex completed with errors")
		return
	}
	s.jobs.update(jobID, func(job *AdminJob) {
		job.Deleted += deleted
	})
	s.finishJob(jobID, "completed", "subpath reindex completed")
}

func (s *Server) scanUserRoot(ctx context.Context, user store.User, scanID string, jobID string) error {
	root, err := filepath.Abs(user.HomeRoot)
	if err != nil {
		return err
	}
	return s.scanUserTree(ctx, user, root, root, "", scanID, jobID)
}

func (s *Server) scanUserPath(ctx context.Context, user store.User, logical string, scanID string, jobID string) error {
	resolved, err := files.ResolveExisting(user.HomeRoot, logical)
	if err != nil {
		return err
	}
	root, err := filepath.Abs(user.HomeRoot)
	if err != nil {
		return err
	}
	info, err := os.Lstat(resolved.Physical)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		_, err := s.store.DeleteFileIndexPath(ctx, user.ID, logical)
		return err
	}
	if !info.IsDir() {
		return s.scanSinglePath(ctx, user, resolved.Physical, logical, info, scanID, jobID)
	}
	return s.scanUserTree(ctx, user, root, resolved.Physical, logical, scanID, jobID)
}

func (s *Server) scanUserTree(ctx context.Context, user store.User, root string, start string, startLogical string, scanID string, jobID string) error {
	type walkItem struct {
		physical string
		logical  string
		info     os.FileInfo
	}
	type indexedItem struct {
		entry     store.FileIndexEntry
		textEntry *store.DocumentTextEntry
	}

	numWorkers := runtime.NumCPU()
	if numWorkers > 8 {
		numWorkers = 8
	}
	if numWorkers < 2 {
		numWorkers = 2
	}

	walkCh := make(chan walkItem, numWorkers*8)
	resultCh := make(chan indexedItem, numWorkers*8)

	// Producer: walk the directory tree and emit items into walkCh.
	walkErrCh := make(chan error, 1)
	go func() {
		defer close(walkCh)
		var walkErr error

		sendItem := func(physical, logical string, info os.FileInfo) bool {
			select {
			case walkCh <- walkItem{physical, logical, info}:
				return true
			case <-ctx.Done():
				return false
			}
		}

		if startLogical != "" && startLogical != "/" {
			info, err := os.Lstat(start)
			if err != nil {
				walkErrCh <- err
				return
			}
			if !sendItem(start, startLogical, info) {
				return
			}
		}

		walkErr = filepath.WalkDir(start, func(physical string, d os.DirEntry, err error) error {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					return nil
				}
				return err
			}
			if physical == root || (startLogical != "" && physical == start) {
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
			if !sendItem(physical, logical, info) {
				return ctx.Err()
			}
			return nil
		})
		if walkErr != nil && !errors.Is(walkErr, context.Canceled) {
			walkErrCh <- walkErr
		}
	}()

	// Workers: call indexEntryForPath (may read file content) in parallel.
	var workerWg sync.WaitGroup
	for range numWorkers {
		workerWg.Add(1)
		go func() {
			defer workerWg.Done()
			for item := range walkCh {
				entry, textEntry := s.indexEntryForPath(user, item.physical, item.logical, item.info, scanID)
				select {
				case resultCh <- indexedItem{entry, textEntry}:
				case <-ctx.Done():
					return
				}
			}
		}()
	}
	go func() {
		workerWg.Wait()
		close(resultCh)
	}()

	// Batch accumulator: collect results and flush to DB.
	batch := make([]store.FileIndexEntry, 0, reindexBatchSize)
	textBatch := make([]store.DocumentTextEntry, 0, reindexBatchSize)
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		if err := s.store.UpsertIndexBatch(ctx, batch, textBatch); err != nil {
			return err
		}
		count := len(batch)
		last := batch[count-1].Path
		batch = batch[:0]
		textBatch = textBatch[:0]
		s.jobs.update(jobID, func(job *AdminJob) {
			job.Done += int64(count)
			job.Message = "indexed " + user.Username + ":" + last
		})
		return nil
	}

	var dbErr error
	for item := range resultCh {
		if ctx.Err() != nil {
			break
		}
		batch = append(batch, item.entry)
		if item.textEntry != nil {
			textBatch = append(textBatch, *item.textEntry)
		}
		if len(batch) >= reindexBatchSize {
			if err := flush(); err != nil {
				dbErr = err
				for range resultCh { //nolint:revive // drain so workers can exit
				}
				break
			}
		}
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}
	if dbErr != nil {
		return dbErr
	}
	select {
	case err := <-walkErrCh:
		return err
	default:
	}
	return flush()
}

func (s *Server) scanSinglePath(ctx context.Context, user store.User, physical string, logical string, info os.FileInfo, scanID string, jobID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	indexEntry, textEntry := s.indexEntryForPath(user, physical, logical, info, scanID)
	if err := s.store.UpsertFileIndexEntry(ctx, indexEntry); err != nil {
		return err
	}
	if textEntry != nil {
		if err := s.store.UpsertDocumentTextEntry(ctx, *textEntry); err != nil {
			return err
		}
	}
	s.jobs.update(jobID, func(job *AdminJob) {
		job.Done++
		job.Message = "indexed " + user.Username + ":" + logical
	})
	return nil
}

func (s *Server) indexEntryForPath(user store.User, physical string, logical string, info os.FileInfo, scanID string) (store.FileIndexEntry, *store.DocumentTextEntry) {
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
	if indexEntry.Type != "file" || !files.SupportsTextIndex(indexEntry.PreviewKind) {
		return indexEntry, nil
	}
	content, err := files.ReadTextForIndex(physical)
	if err != nil {
		content = ""
	}
	return indexEntry, &store.DocumentTextEntry{
		UserID:  user.ID,
		Path:    logical,
		Name:    indexEntry.Name,
		Content: content,
	}
}

func (s *Server) runPreviewWarmupJob(ctx context.Context, jobID string) {
	candidates, err := s.store.ListPreviewCandidates(ctx)
	if err != nil {
		s.finishJob(jobID, "failed", err.Error())
		return
	}

	sizes := previewWarmupSizes
	s.jobs.update(jobID, func(job *AdminJob) {
		job.Total = int64(len(candidates) * len(sizes))
		job.TotalKnown = true
		job.Message = "warming thumbnails"
	})
	if len(candidates) == 0 {
		s.finishJob(jobID, "completed", "no indexed preview candidates; run full reindex first")
		return
	}

	generated, failed := s.runPreviewWarmupWorkers(ctx, jobID, candidates, sizes)

	if err := ctx.Err(); err != nil {
		s.finishJob(jobID, "canceled", "preview warmup canceled")
		return
	}
	total := int64(len(candidates) * len(sizes))
	cached := total - generated - failed
	summary := fmt.Sprintf("generated %d new, %d already cached, %d failed", generated, cached, failed)
	s.log.Info("preview warmup finished", "generated", generated, "cached", cached, "failed", failed)
	if failed > 0 {
		s.finishJob(jobID, "failed", summary)
		return
	}
	s.finishJob(jobID, "completed", summary)
}

type previewWarmupTask struct {
	candidate store.PreviewCandidate
	size      int
}

type previewWarmupResult struct {
	username  string
	path      string
	size      int
	err       error
	generated bool
}

func (s *Server) runPreviewWarmupWorkers(ctx context.Context, jobID string, candidates []store.PreviewCandidate, sizes []int) (generated, failed int64) {
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
				select {
				case <-ctx.Done():
					close(tasks)
					wg.Wait()
					close(results)
					return
				case tasks <- previewWarmupTask{candidate: candidate, size: size}:
				}
			}
		}
		close(tasks)
		wg.Wait()
		close(results)
	}()

	var done int64
	var last previewWarmupResult
	total := int64(len(candidates) * len(sizes))
	for result := range results {
		done++
		last = result
		if result.err != nil {
			failed++
			s.log.Warn("preview warmup failed",
				"user", result.username,
				"path", result.path,
				"size", result.size,
				"err", result.err,
			)
		} else if result.generated {
			generated++
			s.log.Debug("preview warmup generated", "user", result.username, "path", result.path, "size", result.size)
		}
		if done%adminProgressBatchSize == 0 || done == total {
			currentDone := done
			currentFailed := failed
			currentGenerated := generated
			currentLast := last
			s.jobs.update(jobID, func(job *AdminJob) {
				job.Done = currentDone
				job.Failed = currentFailed
				if currentLast.err != nil {
					job.Message = fmt.Sprintf("error on %s:%s — %v", currentLast.username, currentLast.path, currentLast.err)
				} else {
					job.Message = fmt.Sprintf("generated %d new, %d already cached, %d failed", currentGenerated, currentDone-currentFailed-currentGenerated, currentFailed)
				}
			})
		}
	}
	return
}

func (s *Server) warmPreview(ctx context.Context, task previewWarmupTask) previewWarmupResult {
	candidate := task.candidate
	result := previewWarmupResult{username: candidate.Username, path: candidate.Path, size: task.size}
	resolved, info, err := s.files.ResolveForRead(store.User{ID: candidate.UserID, HomeRoot: candidate.HomeRoot}, candidate.Path)
	if err != nil {
		result.err = err
		return result
	}
	cachePath := thumbnailCachePathInode(s.cfg.PreviewDir, candidate.UserID, resolved.Logical, info, task.size)
	if _, err := os.Stat(cachePath); errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(filepath.Dir(cachePath), 0o750); err != nil {
			result.err = err
		} else if err := s.generateThumbnail(ctx, resolved.Physical, candidate.PreviewKind, task.size, cachePath); err != nil {
			result.err = fmt.Errorf("kind=%s: %w", candidate.PreviewKind, err)
		} else {
			result.generated = true
		}
	} else if err != nil {
		result.err = err
	}
	return result
}

func previewWarmupWorkerCount(configured int) int {
	return previewWorkerLimit(configured)
}

func previewWorkerLimit(configured int) int {
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
		job.Cancelable = false
		job.Message = message
	})
	s.log.Info("admin job finished", "job_id", jobID, "status", status, "message", message)
}
