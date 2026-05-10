package server

import (
	"io"
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

	tests := []struct {
		name       string
		path       string
		statusCode int
		contains   string
	}{
		{name: "root", path: "/", statusCode: http.StatusOK, contains: "goDrive"},
		{name: "asset", path: "/assets/app.js", statusCode: http.StatusOK, contains: "use strict"},
		{name: "missing", path: "/missing", statusCode: http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != tt.statusCode {
				t.Fatalf("status = %d, want %d", rec.Code, tt.statusCode)
			}
			if tt.contains != "" && !strings.Contains(rec.Body.String(), tt.contains) {
				t.Fatalf("body does not contain %q", tt.contains)
			}
		})
	}
}
