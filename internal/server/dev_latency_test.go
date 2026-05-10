package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"godrive/internal/config"
)

func TestDevLatencyMiddleware(t *testing.T) {
	t.Parallel()

	srv := &Server{cfg: config.Config{
		DevLatencyMin: 5 * time.Millisecond,
		DevLatencyMax: 5 * time.Millisecond,
	}}
	handler := srv.devLatency(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	start := time.Now()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if elapsed := time.Since(start); elapsed < 5*time.Millisecond {
		t.Fatalf("elapsed = %s, want at least 5ms", elapsed)
	}
}
