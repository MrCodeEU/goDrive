package server

import (
	"runtime"
	"testing"
)

func TestPreviewWarmupWorkerCount(t *testing.T) {
	t.Parallel()

	if got := previewWarmupWorkerCount(7); got != 7 {
		t.Fatalf("configured workers = %d, want 7", got)
	}
	if got := previewWarmupWorkerCount(previewWarmupMaxJobs + 10); got != previewWarmupMaxJobs {
		t.Fatalf("clamped workers = %d, want %d", got, previewWarmupMaxJobs)
	}
	got := previewWarmupWorkerCount(0)
	if got < previewWarmupMinJobs {
		t.Fatalf("auto workers = %d, want at least %d", got, previewWarmupMinJobs)
	}
	if got > previewWarmupMaxJobs {
		t.Fatalf("auto workers = %d, want at most %d", got, previewWarmupMaxJobs)
	}
	if runtime.NumCPU() >= previewWarmupMinJobs*2 && got != runtime.NumCPU()/2 {
		t.Fatalf("auto workers = %d, want %d", got, runtime.NumCPU()/2)
	}
}

func TestAdminJobsPreventsOverlappingJobs(t *testing.T) {
	t.Parallel()

	jobs := NewAdminJobs()
	if _, err := jobs.start("reindex"); err != nil {
		t.Fatal(err)
	}
	if _, err := jobs.start("reconciliation"); err != errJobRunning {
		t.Fatalf("second start err = %v, want errJobRunning", err)
	}
}

func TestAdminJobsCancelCurrent(t *testing.T) {
	t.Parallel()

	jobs := NewAdminJobs()
	job, err := jobs.start("preview_warmup")
	if err != nil {
		t.Fatal(err)
	}
	if !job.Cancelable {
		t.Fatal("new job should be cancelable")
	}
	canceled := jobs.CancelCurrent()
	if canceled == nil || canceled.Message != "cancel requested" {
		t.Fatalf("cancel result = %+v", canceled)
	}
	if err := job.context.Err(); err == nil {
		t.Fatal("job context was not canceled")
	}
}
