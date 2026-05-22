package server

import (
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
