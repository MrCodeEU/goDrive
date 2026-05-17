package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Addr                   string
	DatabasePath           string
	DataRoot               string
	AppDataDir             string
	SearchEngine           string // "bleve" or "sqlite"
	SearchDir              string // path to bleve index files
	UploadDir              string
	PreviewDir             string
	TrashDir               string
	BootstrapAdminUser     string
	BootstrapAdminPassword string
	BootstrapAdminRoot     string
	SessionCookieName      string
	CSRFCookieName         string
	CookieSecure           bool
	SessionTTL             time.Duration
	EnableWatcher          bool
	ReconcileInterval      time.Duration
	UploadTTL              time.Duration
	PreviewWorkers         int
	PreviewTimeout         time.Duration
	MaxUploadBytes         int64
	DevLatencyMin          time.Duration
	DevLatencyMax          time.Duration
}

func Load() (Config, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return Config{}, err
	}

	dataRoot := env("GODRIVE_DATA_ROOT", filepath.Join(cwd, "var", "data"))
	appData := env("GODRIVE_APPDATA_DIR", filepath.Join(cwd, "var", "appdata"))

	sessionTTL, err := time.ParseDuration(env("GODRIVE_SESSION_TTL", "720h"))
	if err != nil {
		return Config{}, err
	}
	reconcileInterval, err := parseOptionalDuration("GODRIVE_RECONCILE_INTERVAL", "24h")
	if err != nil {
		return Config{}, err
	}
	uploadTTL, err := parseOptionalDuration("GODRIVE_UPLOAD_TTL", "48h")
	if err != nil {
		return Config{}, err
	}
	previewTimeout, err := parseOptionalDuration("GODRIVE_PREVIEW_TIMEOUT", "45s")
	if err != nil {
		return Config{}, err
	}
	maxUploadBytes := int64(0)
	if raw := os.Getenv("GODRIVE_MAX_UPLOAD_BYTES"); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed < 0 {
			return Config{}, fmt.Errorf("GODRIVE_MAX_UPLOAD_BYTES must be a non-negative integer")
		}
		maxUploadBytes = parsed
	}
	devLatencyMin, devLatencyMax, err := parseLatencyRange(os.Getenv("GODRIVE_DEV_LATENCY"))
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		Addr:                   env("GODRIVE_ADDR", "127.0.0.1:8080"),
		DatabasePath:           env("GODRIVE_DB_PATH", filepath.Join(appData, "godrive.sqlite")),
		DataRoot:               dataRoot,
		AppDataDir:             appData,
		UploadDir:              env("GODRIVE_UPLOAD_DIR", filepath.Join(appData, "uploads")),
		PreviewDir:             env("GODRIVE_PREVIEW_DIR", filepath.Join(appData, "previews")),
		SearchEngine:           env("GODRIVE_SEARCH_ENGINE", "bleve"),
		SearchDir:              env("GODRIVE_SEARCH_DIR", filepath.Join(appData, "search")),
		TrashDir:               env("GODRIVE_TRASH_DIR", filepath.Join(appData, "trash")),
		BootstrapAdminUser:     env("GODRIVE_BOOTSTRAP_ADMIN_USER", "admin"),
		BootstrapAdminPassword: os.Getenv("GODRIVE_BOOTSTRAP_ADMIN_PASSWORD"),
		BootstrapAdminRoot:     env("GODRIVE_BOOTSTRAP_ADMIN_ROOT", filepath.Join(dataRoot, "admin")),
		SessionCookieName:      env("GODRIVE_SESSION_COOKIE", "godrive_session"),
		CSRFCookieName:         env("GODRIVE_CSRF_COOKIE", "godrive_csrf"),
		CookieSecure:           envBool("GODRIVE_COOKIE_SECURE", false),
		SessionTTL:             sessionTTL,
		EnableWatcher:          envBool("GODRIVE_ENABLE_WATCHER", true),
		ReconcileInterval:      reconcileInterval,
		UploadTTL:              uploadTTL,
		PreviewWorkers:         envInt("GODRIVE_PREVIEW_WORKERS", 0),
		PreviewTimeout:         previewTimeout,
		MaxUploadBytes:         maxUploadBytes,
		DevLatencyMin:          devLatencyMin,
		DevLatencyMax:          devLatencyMax,
	}

	if cfg.BootstrapAdminUser == "" {
		return Config{}, errors.New("GODRIVE_BOOTSTRAP_ADMIN_USER cannot be empty")
	}
	if err := cfg.ValidateStorageLayout(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func parseOptionalDuration(key string, fallback string) (time.Duration, error) {
	raw := strings.TrimSpace(env(key, fallback))
	if raw == "" || raw == "0" {
		return 0, nil
	}
	value, err := time.ParseDuration(raw)
	if err != nil {
		return 0, err
	}
	if value < 0 {
		return 0, fmt.Errorf("%s cannot be negative", key)
	}
	return value, nil
}

func parseLatencyRange(raw string) (time.Duration, time.Duration, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "0" {
		return 0, 0, nil
	}

	if left, right, ok := strings.Cut(raw, "-"); ok {
		min, err := time.ParseDuration(strings.TrimSpace(left))
		if err != nil {
			return 0, 0, err
		}
		max, err := time.ParseDuration(strings.TrimSpace(right))
		if err != nil {
			return 0, 0, err
		}
		if min < 0 || max < 0 || max < min {
			return 0, 0, fmt.Errorf("invalid latency range %q", raw)
		}
		return min, max, nil
	}

	value, err := time.ParseDuration(raw)
	if err != nil {
		return 0, 0, err
	}
	if value < 0 {
		return 0, 0, fmt.Errorf("invalid latency %q", raw)
	}
	return value, value, nil
}

func (c Config) EnsureDirs() error {
	for _, dir := range []string{
		c.DataRoot,
		c.AppDataDir,
		c.UploadDir,
		c.PreviewDir,
		c.TrashDir,
		filepath.Dir(c.DatabasePath),
		c.BootstrapAdminRoot,
	} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return err
		}
	}
	return nil
}

func (c Config) ValidateStorageLayout() error {
	if err := validateNonRootDir("GODRIVE_DATA_ROOT", c.DataRoot); err != nil {
		return err
	}
	if err := validateNonRootDir("GODRIVE_APPDATA_DIR", c.AppDataDir); err != nil {
		return err
	}
	if err := validateNonRootDir("GODRIVE_UPLOAD_DIR", c.UploadDir); err != nil {
		return err
	}
	if err := validateNonRootDir("GODRIVE_TRASH_DIR", c.TrashDir); err != nil {
		return err
	}
	if err := c.ValidatePreviewCacheDir(); err != nil {
		return err
	}
	if err := validateNoOverlap("GODRIVE_DATA_ROOT", c.DataRoot, "GODRIVE_APPDATA_DIR", c.AppDataDir); err != nil {
		return err
	}
	for _, dir := range []struct {
		name string
		path string
	}{
		{name: "GODRIVE_UPLOAD_DIR", path: c.UploadDir},
		{name: "GODRIVE_TRASH_DIR", path: c.TrashDir},
	} {
		if err := validateNoOverlap(dir.name, dir.path, "GODRIVE_DATA_ROOT", c.DataRoot); err != nil {
			return err
		}
		if err := validateNotEqual(dir.name, dir.path, "GODRIVE_APPDATA_DIR", c.AppDataDir); err != nil {
			return err
		}
	}
	if err := validateNoOverlap("GODRIVE_UPLOAD_DIR", c.UploadDir, "GODRIVE_TRASH_DIR", c.TrashDir); err != nil {
		return err
	}
	if c.DatabasePath != "" {
		if err := validatePathNotInDir("GODRIVE_DB_PATH", c.DatabasePath, "GODRIVE_UPLOAD_DIR", c.UploadDir); err != nil {
			return err
		}
		if err := validatePathNotInDir("GODRIVE_DB_PATH", c.DatabasePath, "GODRIVE_TRASH_DIR", c.TrashDir); err != nil {
			return err
		}
	}
	return nil
}

func (c Config) ValidatePreviewCacheDir() error {
	if err := validateNonRootDir("GODRIVE_PREVIEW_DIR", c.PreviewDir); err != nil {
		return err
	}
	if err := validateNotEqual("GODRIVE_PREVIEW_DIR", c.PreviewDir, "GODRIVE_APPDATA_DIR", c.AppDataDir); err != nil {
		return err
	}
	for _, protected := range []struct {
		name string
		path string
	}{
		{name: "GODRIVE_DATA_ROOT", path: c.DataRoot},
		{name: "GODRIVE_UPLOAD_DIR", path: c.UploadDir},
		{name: "GODRIVE_TRASH_DIR", path: c.TrashDir},
	} {
		if err := validateNoOverlap("GODRIVE_PREVIEW_DIR", c.PreviewDir, protected.name, protected.path); err != nil {
			return err
		}
	}
	if c.DatabasePath != "" {
		if err := validatePathNotInDir("GODRIVE_DB_PATH", c.DatabasePath, "GODRIVE_PREVIEW_DIR", c.PreviewDir); err != nil {
			return err
		}
	}
	return nil
}

func validateNonRootDir(name, dir string) error {
	if strings.TrimSpace(dir) == "" {
		return fmt.Errorf("%s cannot be empty", name)
	}
	clean, err := cleanAbs(dir)
	if err != nil {
		return fmt.Errorf("%s is invalid: %w", name, err)
	}
	if clean == filepath.VolumeName(clean)+string(filepath.Separator) {
		return fmt.Errorf("%s cannot be the filesystem root", name)
	}
	return nil
}

func validateNoOverlap(leftName, left, rightName, right string) error {
	if strings.TrimSpace(left) == "" || strings.TrimSpace(right) == "" {
		return nil
	}
	overlap, err := pathsOverlap(left, right)
	if err != nil {
		return err
	}
	if overlap {
		return fmt.Errorf("%s must not overlap %s", leftName, rightName)
	}
	return nil
}

func validateNotEqual(leftName, left, rightName, right string) error {
	if strings.TrimSpace(left) == "" || strings.TrimSpace(right) == "" {
		return nil
	}
	leftAbs, err := cleanAbs(left)
	if err != nil {
		return fmt.Errorf("%s is invalid: %w", leftName, err)
	}
	rightAbs, err := cleanAbs(right)
	if err != nil {
		return fmt.Errorf("%s is invalid: %w", rightName, err)
	}
	if leftAbs == rightAbs {
		return fmt.Errorf("%s must not equal %s", leftName, rightName)
	}
	return nil
}

func validatePathNotInDir(pathName, path, dirName, dir string) error {
	if strings.TrimSpace(path) == "" || strings.TrimSpace(dir) == "" {
		return nil
	}
	contained, err := pathContains(dir, path)
	if err != nil {
		return err
	}
	if contained {
		return fmt.Errorf("%s must not be inside %s", pathName, dirName)
	}
	return nil
}

func pathsOverlap(left, right string) (bool, error) {
	leftContainsRight, err := pathContains(left, right)
	if err != nil {
		return false, err
	}
	rightContainsLeft, err := pathContains(right, left)
	if err != nil {
		return false, err
	}
	return leftContainsRight || rightContainsLeft, nil
}

func pathContains(parent, child string) (bool, error) {
	parentAbs, err := cleanAbs(parent)
	if err != nil {
		return false, err
	}
	childAbs, err := cleanAbs(child)
	if err != nil {
		return false, err
	}
	rel, err := filepath.Rel(parentAbs, childAbs)
	if err != nil {
		return false, err
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))), nil
}

func cleanAbs(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}
