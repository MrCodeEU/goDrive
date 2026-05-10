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
