package server

import (
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"godrive/internal/config"
)

func TestWebRootAndAssets(t *testing.T) {
	t.Parallel()

	srv := New(config.Config{}, nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	handler := srv.routes()

	t.Run("root", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "goDrive") {
			t.Fatal("index.html does not contain 'goDrive'")
		}
	})

	t.Run("asset", func(t *testing.T) {
		t.Parallel()
		// Find actual asset filename (vite uses content hashes).
		assetPath := ""
		_ = fs.WalkDir(webAssets, "static/assets", func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			if strings.HasSuffix(path, ".js") && assetPath == "" {
				assetPath = "/assets/" + strings.TrimPrefix(path, "static/assets/")
			}
			return nil
		})
		if assetPath == "" {
			t.Skip("no bundled JS asset found")
		}
		req := httptest.NewRequest(http.MethodGet, assetPath, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("asset %s status = %d, want 200", assetPath, rec.Code)
		}
	})

	t.Run("missing", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/missing", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", rec.Code)
		}
	})
}

func TestSecurityHeadersIncludeHSTSWhenConfigured(t *testing.T) {
	t.Parallel()

	srv := New(config.Config{HSTS: true}, nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	srv.routes().ServeHTTP(rec, req)

	if got := rec.Header().Get("Strict-Transport-Security"); got != "max-age=31536000" {
		t.Fatalf("Strict-Transport-Security = %q, want max-age=31536000", got)
	}
}

func TestSecurityHeadersOmitHSTSByDefault(t *testing.T) {
	t.Parallel()

	srv := New(config.Config{}, nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	srv.routes().ServeHTTP(rec, req)

	if got := rec.Header().Get("Strict-Transport-Security"); got != "" {
		t.Fatalf("Strict-Transport-Security = %q, want empty", got)
	}
}
