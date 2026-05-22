package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"godrive/internal/config"
)

func TestValidateWebhookURLDefaults(t *testing.T) {
	t.Parallel()

	policy := webhookEgressPolicy{}
	tests := []struct {
		name    string
		raw     string
		wantErr bool
	}{
		{name: "https public", raw: "https://example.com/hook"},
		{name: "http blocked", raw: "http://example.com/hook", wantErr: true},
		{name: "loopback blocked", raw: "https://127.0.0.1/hook", wantErr: true},
		{name: "private blocked", raw: "https://192.168.1.10/hook", wantErr: true},
		{name: "link local blocked", raw: "https://169.254.169.254/latest", wantErr: true},
		{name: "userinfo blocked", raw: "https://user:pass@example.com/hook", wantErr: true},
		{name: "relative blocked", raw: "/hook", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := validateWebhookURL(tt.raw, policy)
			if tt.wantErr && err == nil {
				t.Fatal("expected error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidateWebhookURLAllowsExplicitHomeLabOptions(t *testing.T) {
	t.Parallel()

	policy := webhookEgressPolicy{allowHTTP: true, allowPrivate: true}
	if _, err := validateWebhookURL("http://192.168.1.10/hook", policy); err != nil {
		t.Fatalf("expected private http webhook to be allowed with explicit policy: %v", err)
	}
}

func TestCreateWebhookRejectsUnsafeURLByDefault(t *testing.T) {
	t.Parallel()

	srv, st := newTestServer(t)
	admin := createTestUser(t, st, "webhook-admin", true)
	token, _ := createTestSession(t, st, admin.ID, time.Hour)

	req := httptest.NewRequest(http.MethodPost, "/api/webhooks", strings.NewReader(`{"url":"http://127.0.0.1/hook"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	srv.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body: %s", rec.Code, rec.Body.String())
	}
}

func TestPostWebhookRejectsLoopbackByDefault(t *testing.T) {
	t.Parallel()

	receiverCalled := false
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receiverCalled = true
		w.WriteHeader(http.StatusOK)
	}))
	defer receiver.Close()

	srv, _ := newTestServer(t)
	err := srv.postWebhook(t.Context(), receiver.URL, "delivery", "ping", "sig", []byte(`{}`))
	if err == nil {
		t.Fatal("expected loopback webhook delivery to be rejected")
	}
	if receiverCalled {
		t.Fatal("receiver should not have been called")
	}
}

func TestPostWebhookAllowsLoopbackWithExplicitPolicy(t *testing.T) {
	t.Parallel()

	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer receiver.Close()

	srv, _ := newTestServer(t)
	srv.cfg = config.Config{WebhookAllowHTTP: true, WebhookAllowPrivate: true}

	if err := srv.postWebhook(t.Context(), receiver.URL, "delivery", "ping", "sig", []byte(`{}`)); err != nil {
		t.Fatalf("expected explicit policy to allow loopback delivery: %v", err)
	}
}
