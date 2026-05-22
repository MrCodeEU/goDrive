package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"godrive/internal/files"
)

func TestWebDAVRejectsSymlinkEscape(t *testing.T) {
	t.Parallel()

	srv, st := newTestServer(t)
	user := createTestUser(t, st, "dav-user", false)

	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("outside-secret"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(user.HomeRoot, "outside")); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/dav/outside/secret.txt", nil)
	req.SetBasicAuth(user.Username, testPassword)
	rec := httptest.NewRecorder()

	srv.routes().ServeHTTP(rec, req)

	if rec.Code == http.StatusOK {
		t.Fatalf("status = 200, body = %q; want symlink escape rejected", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "outside-secret") {
		t.Fatalf("response leaked outside file content: %q", rec.Body.String())
	}
}

func TestWebDAVAllowsRegularFileRead(t *testing.T) {
	t.Parallel()

	srv, st := newTestServer(t)
	user := createTestUser(t, st, "dav-reader", false)
	if err := os.WriteFile(filepath.Join(user.HomeRoot, "note.txt"), []byte("inside"), 0o640); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/dav/note.txt", nil)
	req.SetBasicAuth(user.Username, testPassword)
	rec := httptest.NewRecorder()

	srv.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	body, err := io.ReadAll(rec.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "inside" {
		t.Fatalf("body = %q, want inside", string(body))
	}
}

func TestWebDAVRejectsSymlinkEscapeWrite(t *testing.T) {
	t.Parallel()

	srv, st := newTestServer(t)
	user := createTestUser(t, st, "dav-writer", false)

	outside := t.TempDir()
	secretPath := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(secretPath, []byte("outside-secret"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(user.HomeRoot, "outside")); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPut, "/dav/outside/secret.txt", strings.NewReader("overwritten"))
	req.SetBasicAuth(user.Username, testPassword)
	rec := httptest.NewRecorder()

	srv.routes().ServeHTTP(rec, req)

	if rec.Code >= 200 && rec.Code < 300 {
		t.Fatalf("status = %d, want symlink escape write rejected", rec.Code)
	}
	content, err := os.ReadFile(secretPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "outside-secret" {
		t.Fatalf("outside content = %q, want unchanged", string(content))
	}
}

func TestWebDAVRejectsSymlinkEscapeList(t *testing.T) {
	t.Parallel()

	srv, st := newTestServer(t)
	user := createTestUser(t, st, "dav-list", false)

	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("outside-secret"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(user.HomeRoot, "outside")); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("PROPFIND", "/dav/outside", nil)
	req.Header.Set("Depth", "1")
	req.SetBasicAuth(user.Username, testPassword)
	rec := httptest.NewRecorder()

	srv.routes().ServeHTTP(rec, req)

	if rec.Code >= 200 && rec.Code < 300 {
		t.Fatalf("status = %d, want symlink escape list rejected; body: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "secret.txt") {
		t.Fatalf("response leaked outside listing: %q", rec.Body.String())
	}
}

func TestWebDAVRejectsSymlinkEscapeDelete(t *testing.T) {
	t.Parallel()

	srv, st := newTestServer(t)
	user := createTestUser(t, st, "dav-delete", false)

	outside := t.TempDir()
	secretPath := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(secretPath, []byte("outside-secret"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(user.HomeRoot, "outside")); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/dav/outside/secret.txt", nil)
	req.SetBasicAuth(user.Username, testPassword)
	rec := httptest.NewRecorder()

	srv.routes().ServeHTTP(rec, req)

	if rec.Code >= 200 && rec.Code < 300 {
		t.Fatalf("status = %d, want symlink escape delete rejected", rec.Code)
	}
	if _, err := os.Stat(secretPath); err != nil {
		t.Fatalf("outside file was removed or became inaccessible: %v", err)
	}
}

func TestWebDAVBasicAuthRateLimitBlocksAfterMaxAttempts(t *testing.T) {
	t.Parallel()

	srv, st := newTestServer(t)
	user := createTestUser(t, st, "dav-brute", false)

	for i := range loginMaxAttempts {
		req := httptest.NewRequest(http.MethodGet, "/dav/", nil)
		req.RemoteAddr = "192.0.2.1:1234"
		req.SetBasicAuth(user.Username, "wrong")
		rec := httptest.NewRecorder()
		srv.routes().ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: status = %d, want 401", i+1, rec.Code)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/dav/", nil)
	req.RemoteAddr = "192.0.2.1:1234"
	req.SetBasicAuth(user.Username, testPassword)
	rec := httptest.NewRecorder()
	srv.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("after block: status = %d, want 429", rec.Code)
	}
}

func TestWebDAVCookieMutationRequiresCSRF(t *testing.T) {
	t.Parallel()

	srv, st := newTestServer(t)
	user := createTestUser(t, st, "dav-cookie", false)
	token, csrfToken := createTestSession(t, st, user.ID, time.Hour)

	req := httptest.NewRequest(http.MethodPut, "/dav/note.txt", strings.NewReader("body"))
	req.AddCookie(&http.Cookie{Name: "godrive_session", Value: token})
	rec := httptest.NewRecorder()
	srv.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("without csrf: status = %d, want 403", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPut, "/dav/note.txt", strings.NewReader("body"))
	req.AddCookie(&http.Cookie{Name: "godrive_session", Value: token})
	req.Header.Set("X-CSRF-Token", csrfToken)
	rec = httptest.NewRecorder()
	srv.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated && rec.Code != http.StatusNoContent {
		t.Fatalf("with csrf: status = %d, want 201 or 204; body: %s", rec.Code, rec.Body.String())
	}
}

func TestWriteFileContentRejectsSymlinkEscape(t *testing.T) {
	t.Parallel()

	srv, st := newTestServer(t)
	srv.files = files.NewService(t.TempDir(), st)
	user := createTestUser(t, st, "text-edit-escape", false)
	token, _ := createTestSession(t, st, user.ID, time.Hour)

	outside := t.TempDir()
	secretPath := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(secretPath, []byte("outside-secret"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(user.HomeRoot, "outside")); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPatch, "/api/files/content?path=/outside/secret.txt", strings.NewReader("overwritten"))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()

	srv.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body.String())
	}
	content, err := os.ReadFile(secretPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "outside-secret" {
		t.Fatalf("outside content = %q, want unchanged", string(content))
	}
}
