package server

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"godrive/internal/config"
	"godrive/internal/files"
	"godrive/internal/store"
)

func newWebhookServer(t *testing.T) (*Server, *store.Store) {
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
	srv := New(config.Config{}, st, files.NewService(t.TempDir(), st), log)
	return srv, st
}

func TestWebhookSignature(t *testing.T) {
	t.Parallel()

	secret := "test-secret"
	payload := []byte(`{"event":"upload.complete"}`)
	got := webhookSignature(secret, payload)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	want := hex.EncodeToString(mac.Sum(nil))

	if got != want {
		t.Fatalf("signature = %q, want %q", got, want)
	}
}

func TestCreateAndListWebhooks(t *testing.T) {
	t.Parallel()

	srv, st := newWebhookServer(t)
	admin := createTestUser(t, st, "admin", true)
	token, _ := createTestSession(t, st, admin.ID, time.Hour)

	body := `{"url":"https://example.com/hook","events":["upload.complete"],"description":"test"}`
	req := httptest.NewRequest(http.MethodPost, "/api/webhooks", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	srv.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201; body: %s", rec.Code, rec.Body.String())
	}
	var created struct {
		Webhook struct{ ID string `json:"id"` } `json:"webhook"`
		Secret  string                           `json:"secret"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.Secret == "" {
		t.Fatal("secret missing from creation response")
	}
	if created.Webhook.ID == "" {
		t.Fatal("webhook id missing")
	}

	// List should include the new webhook.
	listReq := httptest.NewRequest(http.MethodGet, "/api/webhooks", nil)
	listReq.Header.Set("Authorization", "Bearer "+token)
	listRec := httptest.NewRecorder()
	srv.routes().ServeHTTP(listRec, listReq)

	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d", listRec.Code)
	}
	var list struct {
		Webhooks []struct{ ID string `json:"id"` } `json:"webhooks"`
	}
	if err := json.NewDecoder(listRec.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	if len(list.Webhooks) != 1 || list.Webhooks[0].ID != created.Webhook.ID {
		t.Fatalf("list = %+v, want 1 webhook with id %s", list.Webhooks, created.Webhook.ID)
	}
}

func TestDeleteWebhook(t *testing.T) {
	t.Parallel()

	srv, st := newWebhookServer(t)
	admin := createTestUser(t, st, "admin2", true)
	token, _ := createTestSession(t, st, admin.ID, time.Hour)

	wh, err := st.CreateWebhook(t.Context(), store.Webhook{
		ID:     "test-wh",
		URL:    "https://example.com/hook",
		Secret: "s",
		Events: []string{"*"},
	})
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/webhooks/"+wh.ID, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	srv.routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("delete status = %d, want 200", rec.Code)
	}
	hooks, _ := st.ListWebhooks(t.Context())
	if len(hooks) != 0 {
		t.Fatal("webhook should be deleted")
	}
}

func TestFireEventDelivery(t *testing.T) {
	t.Parallel()

	type delivery struct {
		body []byte
		sig  string
	}
	delivered := make(chan delivery, 1)

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		sig := r.Header.Get("X-GoDrive-Signature")
		w.WriteHeader(http.StatusOK)
		delivered <- delivery{body: body, sig: sig}
	}))
	defer backend.Close()

	srv, st := newWebhookServer(t)
	wh, err := st.CreateWebhook(t.Context(), store.Webhook{
		ID:     "fire-test",
		URL:    backend.URL,
		Secret: "mysecret",
		Events: []string{"upload.complete"},
	})
	if err != nil {
		t.Fatal(err)
	}

	user := store.User{ID: 1, Username: "alice"}
	srv.fireEvent(user, "upload.complete", map[string]any{"path": "/photo.jpg"})

	select {
	case d := <-delivered:
		var evt WebhookEvent
		if err := json.Unmarshal(d.body, &evt); err != nil {
			t.Fatalf("invalid payload: %v", err)
		}
		if evt.Event != "upload.complete" {
			t.Fatalf("event = %q, want upload.complete", evt.Event)
		}
		wantSig := "sha256=" + webhookSignature(wh.Secret, d.body)
		if d.sig != wantSig {
			t.Fatalf("signature mismatch: got %q, want %q", d.sig, wantSig)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("webhook was not delivered within 5s")
	}
}
