package server

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"godrive/internal/config"
)

func TestRequestLogging(t *testing.T) {
	t.Parallel()

	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	srv := New(config.Config{}, nil, nil, logger)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.RemoteAddr = "192.0.2.1:12345"
	rec := httptest.NewRecorder()

	srv.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	output := logs.String()
	for _, want := range []string{`msg="http request"`, "method=GET", "path=/health", "status=200", "remote=192.0.2.1:12345"} {
		if !strings.Contains(output, want) {
			t.Fatalf("log output missing %q: %s", want, output)
		}
	}
}

func TestLoggingResponseWriterDefaultsStatus(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	lrw := &loggingResponseWriter{ResponseWriter: rec}

	if _, err := io.WriteString(lrw, "ok"); err != nil {
		t.Fatal(err)
	}
	if lrw.status != http.StatusOK {
		t.Fatalf("status = %d, want %d", lrw.status, http.StatusOK)
	}
	if lrw.bytes != 2 {
		t.Fatalf("bytes = %d, want 2", lrw.bytes)
	}
}
