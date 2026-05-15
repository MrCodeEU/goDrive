package server

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"godrive/internal/config"
	"godrive/internal/store"
	"godrive/internal/watch"
)

func TestAdminStatsIncludesRuntimeSettings(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	st, err := store.Open(filepath.Join(t.TempDir(), "godrive.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = st.Close()
	}()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	homeRoot := t.TempDir()
	user, err := st.CreateUser(ctx, store.User{
		Username:     "alice",
		PasswordHash: "hash",
		HomeRoot:     homeRoot,
	})
	if err != nil {
		t.Fatal(err)
	}

	previewDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(previewDir, "thumb.jpg"), []byte("cache"), 0o640); err != nil {
		t.Fatal(err)
	}

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	watcher, err := watch.New(log, st)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = watcher.Close()
	}()
	if err := watcher.SetUserRoots([]store.User{user}); err != nil {
		t.Fatal(err)
	}

	srv := New(config.Config{
		PreviewDir:        previewDir,
		PreviewWorkers:    3,
		ReconcileInterval: 6 * time.Hour,
	}, st, nil, log)
	srv.SetWatcher(watcher)

	req := httptest.NewRequest(http.MethodGet, "/api/admin/stats", nil)
	rec := httptest.NewRecorder()
	srv.adminStats(rec, req, user, store.Session{})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var body struct {
		PreviewCache struct {
			Files int64 `json:"files"`
			Bytes int64 `json:"bytes"`
		} `json:"preview_cache"`
		Preview struct {
			Workers int                 `json:"workers"`
			Sizes   []int               `json:"sizes"`
			Tools   []PreviewToolStatus `json:"tools"`
		} `json:"preview"`
		Watcher struct {
			Enabled      bool `json:"enabled"`
			Roots        int  `json:"roots"`
			WatchedPaths int  `json:"watched_paths"`
		} `json:"watcher"`
		Reconciliation struct {
			Enabled         bool   `json:"enabled"`
			IntervalSeconds int64  `json:"interval_seconds"`
			Interval        string `json:"interval"`
		} `json:"reconciliation"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}

	if body.PreviewCache.Files != 1 || body.PreviewCache.Bytes != 5 {
		t.Fatalf("preview cache stats = %+v, want 1 file/5 bytes", body.PreviewCache)
	}
	if body.Preview.Workers != 3 {
		t.Fatalf("preview workers = %d, want 3", body.Preview.Workers)
	}
	if len(body.Preview.Sizes) != len(previewWarmupSizes) {
		t.Fatalf("preview sizes = %+v, want %+v", body.Preview.Sizes, previewWarmupSizes)
	}
	for i := range previewWarmupSizes {
		if body.Preview.Sizes[i] != previewWarmupSizes[i] {
			t.Fatalf("preview sizes = %+v, want %+v", body.Preview.Sizes, previewWarmupSizes)
		}
	}
	if len(body.Preview.Tools) != len(previewToolDefinitions) {
		t.Fatalf("preview tools = %+v, want %d tools", body.Preview.Tools, len(previewToolDefinitions))
	}
	if body.Preview.Tools[0].Name != "vipsthumbnail" {
		t.Fatalf("first preview tool = %q, want vipsthumbnail", body.Preview.Tools[0].Name)
	}
	if !body.Watcher.Enabled || body.Watcher.Roots != 1 || body.Watcher.WatchedPaths == 0 {
		t.Fatalf("watcher stats = %+v, want enabled root with watched paths", body.Watcher)
	}
	if !body.Reconciliation.Enabled || body.Reconciliation.IntervalSeconds != int64((6*time.Hour)/time.Second) || body.Reconciliation.Interval != "6h0m0s" {
		t.Fatalf("reconciliation stats = %+v", body.Reconciliation)
	}
}

func TestClearPreviewCacheRejectsUnsafeDirectory(t *testing.T) {
	t.Parallel()

	srv := New(config.Config{PreviewDir: "/"}, nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	req := httptest.NewRequest(http.MethodDelete, "/api/admin/preview-cache", nil)
	rec := httptest.NewRecorder()

	srv.clearPreviewCache(rec, req, store.User{}, store.Session{})

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}

func TestClearPreviewCacheRejectsDataRootOverlap(t *testing.T) {
	t.Parallel()

	dataRoot := t.TempDir()
	previewDir := filepath.Join(dataRoot, "previews")
	if err := os.MkdirAll(previewDir, 0o750); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(previewDir, "keep.txt")
	if err := os.WriteFile(keep, []byte("cache"), 0o640); err != nil {
		t.Fatal(err)
	}

	srv := New(config.Config{DataRoot: dataRoot, PreviewDir: previewDir}, nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	req := httptest.NewRequest(http.MethodDelete, "/api/admin/preview-cache", nil)
	rec := httptest.NewRecorder()

	srv.clearPreviewCache(rec, req, store.User{}, store.Session{})

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatalf("overlapping cache file should not be removed: %v", err)
	}
}

func TestCreateUserCreatesHomeDirectory(t *testing.T) {
	t.Parallel()

	srv, st := newTestServer(t)
	admin := createTestUser(t, st, "admin", true)
	token, _ := createTestSession(t, st, admin.ID, time.Hour)

	homeRoot := filepath.Join(t.TempDir(), "users", "bob")
	// HomeRoot should NOT exist yet.
	if _, err := os.Stat(homeRoot); err == nil {
		t.Fatal("home root should not exist before user creation")
	}

	body := `{"username":"bob","password":"pass","home_root":"` + homeRoot + `","is_admin":false,"disabled":false}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/users", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body: %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(homeRoot); err != nil {
		t.Fatalf("home root should be created: %v", err)
	}
}

func TestCreateUserRejectsUncreatableHomeRoot(t *testing.T) {
	t.Parallel()

	srv, st := newTestServer(t)
	admin := createTestUser(t, st, "admin3", true)
	token, _ := createTestSession(t, st, admin.ID, time.Hour)

	// Path inside a file (not a directory) → MkdirAll fails.
	base := filepath.Join(t.TempDir(), "notadir")
	if err := os.WriteFile(base, []byte("x"), 0o640); err != nil {
		t.Fatal(err)
	}
	badRoot := base + "/subdir"

	body := `{"username":"carol","password":"pass","home_root":"` + badRoot + `","is_admin":false,"disabled":false}`
	req := httptest.NewRequest(http.MethodPost, "/api/admin/users", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}
