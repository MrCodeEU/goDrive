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
	PreviewWorkers         int
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
		TrashDir:               env("GODRIVE_TRASH_DIR", filepath.Join(appData, "trash")),
		BootstrapAdminUser:     env("GODRIVE_BOOTSTRAP_ADMIN_USER", "admin"),
		BootstrapAdminPassword: os.Getenv("GODRIVE_BOOTSTRAP_ADMIN_PASSWORD"),
		BootstrapAdminRoot:     env("GODRIVE_BOOTSTRAP_ADMIN_ROOT", filepath.Join(dataRoot, "admin")),
		SessionCookieName:      env("GODRIVE_SESSION_COOKIE", "godrive_session"),
		CSRFCookieName:         env("GODRIVE_CSRF_COOKIE", "godrive_csrf"),
		CookieSecure:           envBool("GODRIVE_COOKIE_SECURE", false),
		SessionTTL:             sessionTTL,
		EnableWatcher:          envBool("GODRIVE_ENABLE_WATCHER", true),
		PreviewWorkers:         envInt("GODRIVE_PREVIEW_WORKERS", 0),
		DevLatencyMin:          devLatencyMin,
		DevLatencyMax:          devLatencyMax,
	}

	if cfg.BootstrapAdminUser == "" {
		return Config{}, errors.New("GODRIVE_BOOTSTRAP_ADMIN_USER cannot be empty")
	}

	return cfg, nil
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
