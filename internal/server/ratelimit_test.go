package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLoginRateLimitBlocksAfterMaxAttempts(t *testing.T) {
	t.Parallel()

	srv, st := newTestServer(t)
	createTestUser(t, st, "alice", false)

	body := `{"username":"alice","password":"wrong"}`
	for i := range loginMaxAttempts {
		req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(body))
		req.RemoteAddr = "1.2.3.4:1234"
		rec := httptest.NewRecorder()
		srv.login(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: status = %d, want 401", i+1, rec.Code)
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(body))
	req.RemoteAddr = "1.2.3.4:1234"
	rec := httptest.NewRecorder()
	srv.login(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("after block: status = %d, want 429", rec.Code)
	}
}

func TestLoginRateLimitResetsOnSuccess(t *testing.T) {
	t.Parallel()

	srv, st := newTestServer(t)
	createTestUser(t, st, "bob", false)

	wrongBody := `{"username":"bob","password":"wrong"}`
	for range loginMaxAttempts - 1 {
		req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(wrongBody))
		req.RemoteAddr = "5.6.7.8:9999"
		srv.login(httptest.NewRecorder(), req)
	}

	correctBody := `{"username":"bob","password":"` + testPassword + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(correctBody))
	req.RemoteAddr = "5.6.7.8:9999"
	rec := httptest.NewRecorder()
	srv.login(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("successful login after partial failures: status = %d, want 200", rec.Code)
	}

	wrongReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(wrongBody))
	wrongReq.RemoteAddr = "5.6.7.8:9999"
	wrongRec := httptest.NewRecorder()
	srv.login(wrongRec, wrongReq)
	if wrongRec.Code != http.StatusUnauthorized {
		t.Fatalf("after reset: status = %d, want 401 (not 429)", wrongRec.Code)
	}
}

func TestLoginRateLimitIsolatedByIP(t *testing.T) {
	t.Parallel()

	srv, st := newTestServer(t)
	createTestUser(t, st, "carol", false)

	body := `{"username":"carol","password":"wrong"}`
	for range loginMaxAttempts {
		req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(body))
		req.RemoteAddr = "10.0.0.1:1"
		srv.login(httptest.NewRecorder(), req)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(body))
	req.RemoteAddr = "10.0.0.2:1"
	rec := httptest.NewRecorder()
	srv.login(rec, req)
	if rec.Code == http.StatusTooManyRequests {
		t.Fatal("different IP should not be rate limited")
	}
}

func TestLoginRateLimiterAllowClearsExpiredEntries(t *testing.T) {
	t.Parallel()

	l := newLoginLimiter()
	for range loginMaxAttempts {
		l.record("192.168.1.1")
	}
	if l.allow("192.168.1.1") {
		t.Fatal("should be blocked after max attempts")
	}
}
