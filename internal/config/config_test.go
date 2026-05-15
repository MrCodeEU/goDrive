package config

import (
	"path/filepath"
	"testing"
	"time"
)

func TestParseLatencyRange(t *testing.T) {
	t.Parallel()

	tests := []struct {
		raw     string
		wantMin time.Duration
		wantMax time.Duration
		wantErr bool
	}{
		{raw: "", wantMin: 0, wantMax: 0},
		{raw: "0", wantMin: 0, wantMax: 0},
		{raw: "15ms", wantMin: 15 * time.Millisecond, wantMax: 15 * time.Millisecond},
		{raw: "10ms-25ms", wantMin: 10 * time.Millisecond, wantMax: 25 * time.Millisecond},
		{raw: "25ms-10ms", wantErr: true},
		{raw: "-1ms", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			t.Parallel()

			gotMin, gotMax, err := parseLatencyRange(tt.raw)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if gotMin != tt.wantMin || gotMax != tt.wantMax {
				t.Fatalf("got %s-%s, want %s-%s", gotMin, gotMax, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestEnvInt(t *testing.T) {
	t.Setenv("GODRIVE_TEST_INT", "12")
	if got := envInt("GODRIVE_TEST_INT", 3); got != 12 {
		t.Fatalf("got %d, want 12", got)
	}

	t.Setenv("GODRIVE_TEST_INT", "invalid")
	if got := envInt("GODRIVE_TEST_INT", 3); got != 3 {
		t.Fatalf("got %d, want fallback", got)
	}

	t.Setenv("GODRIVE_TEST_INT", "")
	if got := envInt("GODRIVE_TEST_INT", 3); got != 3 {
		t.Fatalf("got %d, want fallback", got)
	}
}

func TestParseOptionalDuration(t *testing.T) {
	t.Setenv("GODRIVE_TEST_DURATION", "")
	got, err := parseOptionalDuration("GODRIVE_TEST_DURATION", "24h")
	if err != nil {
		t.Fatal(err)
	}
	if got != 24*time.Hour {
		t.Fatalf("got %s, want 24h", got)
	}

	t.Setenv("GODRIVE_TEST_DURATION", "0")
	got, err = parseOptionalDuration("GODRIVE_TEST_DURATION", "24h")
	if err != nil {
		t.Fatal(err)
	}
	if got != 0 {
		t.Fatalf("got %s, want disabled", got)
	}

	t.Setenv("GODRIVE_TEST_DURATION", "-1s")
	if _, err = parseOptionalDuration("GODRIVE_TEST_DURATION", "24h"); err == nil {
		t.Fatal("expected negative duration error")
	}
}

func TestValidateStorageLayoutRejectsOverlappingAppDataAndDataRoot(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	cfg := validStorageConfig(root)
	cfg.AppDataDir = filepath.Join(cfg.DataRoot, ".godrive")
	cfg.UploadDir = filepath.Join(cfg.AppDataDir, "uploads")
	cfg.PreviewDir = filepath.Join(cfg.AppDataDir, "previews")
	cfg.TrashDir = filepath.Join(cfg.AppDataDir, "trash")
	cfg.DatabasePath = filepath.Join(cfg.AppDataDir, "godrive.sqlite")

	if err := cfg.ValidateStorageLayout(); err == nil {
		t.Fatal("expected overlap error")
	}
}

func TestValidatePreviewCacheDirRejectsProtectedOverlap(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	cfg := validStorageConfig(root)
	cfg.PreviewDir = filepath.Join(cfg.DataRoot, "previews")

	if err := cfg.ValidatePreviewCacheDir(); err == nil {
		t.Fatal("expected data root overlap error")
	}
}

func TestValidatePreviewCacheDirRejectsDatabaseInsideCache(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	cfg := validStorageConfig(root)
	cfg.DatabasePath = filepath.Join(cfg.PreviewDir, "godrive.sqlite")

	if err := cfg.ValidatePreviewCacheDir(); err == nil {
		t.Fatal("expected database path error")
	}
}

func validStorageConfig(root string) Config {
	dataRoot := filepath.Join(root, "data")
	appData := filepath.Join(root, "appdata")
	return Config{
		DataRoot:           dataRoot,
		AppDataDir:         appData,
		UploadDir:          filepath.Join(appData, "uploads"),
		PreviewDir:         filepath.Join(appData, "previews"),
		TrashDir:           filepath.Join(appData, "trash"),
		DatabasePath:       filepath.Join(appData, "godrive.sqlite"),
		BootstrapAdminRoot: filepath.Join(dataRoot, "admin"),
	}
}
