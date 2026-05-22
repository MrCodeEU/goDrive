package server

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"godrive/internal/auth"
	"godrive/internal/config"
	"godrive/internal/store"
)

const testPassword = "test-password"

var testPasswordHash = sync.OnceValue(func() string {
	h, err := auth.HashPassword(testPassword)
	if err != nil {
		panic(err)
	}
	return h
})

func newTestServer(t *testing.T) (*Server, *store.Store) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "godrive.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Migrate(t.Context()); err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := New(config.Config{
		SessionCookieName: "godrive_session",
		CSRFCookieName:    "godrive_csrf",
		SessionTTL:        time.Hour,
	}, st, nil, log)
	return srv, st
}

func createTestUser(t *testing.T, st *store.Store, username string, isAdmin bool) store.User {
	t.Helper()
	user, err := st.CreateUser(t.Context(), store.User{
		Username:     username,
		PasswordHash: testPasswordHash(),
		HomeRoot:     t.TempDir(),
		IsAdmin:      isAdmin,
	})
	if err != nil {
		t.Fatal(err)
	}
	return user
}

func createTestSession(t *testing.T, st *store.Store, userID int64, ttl time.Duration) (token, csrfToken string) {
	t.Helper()
	var err error
	token, err = auth.RandomToken()
	if err != nil {
		t.Fatal(err)
	}
	csrfToken, err = auth.RandomToken()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateSession(t.Context(), userID, auth.HashToken(token), auth.HashToken(csrfToken), time.Now().Add(ttl)); err != nil {
		t.Fatal(err)
	}
	return token, csrfToken
}

func TestLoginReturnsTokenOnValidCredentials(t *testing.T) {
	t.Parallel()

	srv, st := newTestServer(t)
	createTestUser(t, st, "alice", false)

	body := `{"username":"alice","password":"` + testPassword + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.login(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Token string `json:"token"`
		User  struct {
			Username string `json:"username"`
		} `json:"user"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Token == "" {
		t.Fatal("expected non-empty token in response")
	}
	if resp.User.Username != "alice" {
		t.Fatalf("user.username = %q, want alice", resp.User.Username)
	}
}

func TestLoginSetsStrictSessionCookiesByDefault(t *testing.T) {
	t.Parallel()

	srv, st := newTestServer(t)
	createTestUser(t, st, "cookie-user", false)

	body := `{"username":"cookie-user","password":"` + testPassword + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.login(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	cookies := rec.Header().Values("Set-Cookie")
	if len(cookies) != 2 {
		t.Fatalf("Set-Cookie count = %d, want 2: %v", len(cookies), cookies)
	}
	joined := strings.Join(cookies, "\n")
	if strings.Count(joined, "SameSite=Strict") != 2 {
		t.Fatalf("Set-Cookie headers do not both use SameSite=Strict: %v", cookies)
	}
	if !strings.Contains(joined, "godrive_session=") || !strings.Contains(joined, "HttpOnly") {
		t.Fatalf("session cookie missing expected attributes: %v", cookies)
	}
	for _, cookie := range cookies {
		if strings.HasPrefix(cookie, "godrive_csrf=") && strings.Contains(cookie, "HttpOnly") {
			t.Fatalf("csrf cookie should remain readable by the browser client: %s", cookie)
		}
	}
}

func TestLoginRejectsWrongPassword(t *testing.T) {
	t.Parallel()

	srv, st := newTestServer(t)
	createTestUser(t, st, "bob", false)

	body := `{"username":"bob","password":"wrong-password"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.login(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestLoginRejectsUnknownUser(t *testing.T) {
	t.Parallel()

	srv, _ := newTestServer(t)

	body := `{"username":"nobody","password":"any"}`
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.login(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestWithUserRejectsUnauthenticated(t *testing.T) {
	t.Parallel()

	srv, _ := newTestServer(t)
	handler := srv.withUser(func(w http.ResponseWriter, r *http.Request, u store.User, s store.Session) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestWithUserAcceptsBearerToken(t *testing.T) {
	t.Parallel()

	srv, st := newTestServer(t)
	user := createTestUser(t, st, "carol", false)
	token, _ := createTestSession(t, st, user.ID, time.Hour)

	var gotUser store.User
	handler := srv.withUser(func(w http.ResponseWriter, r *http.Request, u store.User, s store.Session) {
		gotUser = u
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if gotUser.ID != user.ID {
		t.Fatalf("got user ID %d, want %d", gotUser.ID, user.ID)
	}
}

func TestWithUserRejectsExpiredSession(t *testing.T) {
	t.Parallel()

	srv, st := newTestServer(t)
	user := createTestUser(t, st, "dave", false)
	token, _ := createTestSession(t, st, user.ID, -time.Hour)

	handler := srv.withUser(func(w http.ResponseWriter, r *http.Request, u store.User, s store.Session) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestWithUserRequiresCSRFForCookieStateChanging(t *testing.T) {
	t.Parallel()

	srv, st := newTestServer(t)
	user := createTestUser(t, st, "eve", false)
	token, csrfToken := createTestSession(t, st, user.ID, time.Hour)

	handler := srv.withUser(func(w http.ResponseWriter, r *http.Request, u store.User, s store.Session) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/files/mkdir", strings.NewReader(`{"path":"/test"}`))
	req.AddCookie(&http.Cookie{Name: "godrive_session", Value: token})
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status without CSRF = %d, want 403", rec.Code)
	}

	req2 := httptest.NewRequest(http.MethodPost, "/api/files/mkdir", strings.NewReader(`{"path":"/test"}`))
	req2.AddCookie(&http.Cookie{Name: "godrive_session", Value: token})
	req2.Header.Set("X-CSRF-Token", csrfToken)
	rec2 := httptest.NewRecorder()
	handler(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Fatalf("status with CSRF = %d, want 200", rec2.Code)
	}
}

func TestWithAdminRejectsNonAdmin(t *testing.T) {
	t.Parallel()

	srv, st := newTestServer(t)
	user := createTestUser(t, st, "frank", false)
	token, _ := createTestSession(t, st, user.ID, time.Hour)

	handler := srv.withAdmin(func(w http.ResponseWriter, r *http.Request, u store.User, s store.Session) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/admin/stats", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}
