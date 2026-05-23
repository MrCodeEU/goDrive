package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDemoModeBlocksDangerousRoutes(t *testing.T) {
	t.Parallel()

	srv, _ := newTestServer(t)
	srv.cfg.DemoMode = true
	handler := srv.routes()

	cases := []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodPost, path: "/api/admin/api-keys", body: `{"name":"demo"}`},
		{method: http.MethodPost, path: "/api/webhooks", body: `{"url":"https://example.com/hook"}`},
		{method: http.MethodPost, path: "/api/tus"},
		{method: http.MethodPost, path: "/api/admin/jobs/reindex"},
		{method: http.MethodPatch, path: "/api/admin/users/1", body: `{"disabled":true}`},
		{method: http.MethodDelete, path: "/api/admin/api-keys/key_demo"},
		{method: http.MethodDelete, path: "/api/files?path=/note.txt"},
		{method: http.MethodPatch, path: "/api/files/content", body: `{"path":"/note.txt","content":"x"}`},
		{method: http.MethodPut, path: "/dav/note.txt", body: "x"},
	}

	for _, tc := range cases {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s %s status = %d, want 403; body: %s", tc.method, tc.path, rec.Code, rec.Body.String())
		}
	}
}

func TestPublicConfigOnlyExposesDemoCredentialsInDemoMode(t *testing.T) {
	t.Parallel()

	srv, _ := newTestServer(t)
	handler := srv.routes()

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/public/config", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	var normal map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&normal); err != nil {
		t.Fatal(err)
	}
	if normal["demo_mode"] != false {
		t.Fatalf("demo_mode = %v, want false", normal["demo_mode"])
	}
	if _, ok := normal["demo_password"]; ok {
		t.Fatal("normal config exposed demo_password")
	}

	srv.cfg.DemoMode = true
	srv.cfg.DemoUser = "demo"
	srv.cfg.DemoPassword = "demo"
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/public/config", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("demo status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	var demo map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&demo); err != nil {
		t.Fatal(err)
	}
	if demo["demo_user"] != "demo" || demo["demo_password"] != "demo" {
		t.Fatalf("demo credentials = %v/%v, want demo/demo", demo["demo_user"], demo["demo_password"])
	}
}

func TestDemoModeAllowsLoginAndReadRoutes(t *testing.T) {
	t.Parallel()

	srv, _ := newTestServer(t)
	srv.cfg.DemoMode = true
	handler := srv.routes()

	cases := []struct {
		method string
		path   string
	}{
		{method: http.MethodPost, path: "/api/auth/login"},
		{method: http.MethodGet, path: "/api/files/list"},
		{method: http.MethodGet, path: "/api/admin/stats"},
		{method: http.MethodGet, path: "/api/admin/users"},
		{method: http.MethodGet, path: "/api/admin/api-keys"},
		{method: http.MethodGet, path: "/api/admin/jobs/current"},
		{method: http.MethodPost, path: "/api/files/bulk/download"},
	}

	for _, tc := range cases {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{}`))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code == http.StatusForbidden {
			t.Fatalf("%s %s was blocked in demo mode; body: %s", tc.method, tc.path, rec.Body.String())
		}
	}
}
