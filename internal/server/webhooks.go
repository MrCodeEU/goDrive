package server

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"godrive/internal/auth"
	"godrive/internal/store"
)

const (
	webhookDeliveryTimeout = 10 * time.Second
	webhookRetryDelays     = 3
)

var webhookRetryBackoff = []time.Duration{0, 5 * time.Second, 30 * time.Second}

// WebhookEvent is the JSON payload sent to subscribers.
type WebhookEvent struct {
	ID        string         `json:"id"`
	Event     string         `json:"event"`
	Timestamp time.Time      `json:"timestamp"`
	UserID    int64          `json:"user_id"`
	Username  string         `json:"username"`
	Data      map[string]any `json:"data"`
}

// fireEvent dispatches an event to all matching webhook subscribers asynchronously.
// Returns immediately; delivery happens in background goroutines.
func (s *Server) fireEvent(user store.User, event string, data map[string]any) {
	evtID, _ := auth.RandomID(12)
	evt := WebhookEvent{
		ID:        "evt_" + evtID,
		Event:     event,
		Timestamp: time.Now().UTC(),
		UserID:    user.ID,
		Username:  user.Username,
		Data:      data,
	}
	s.publishEvent(evt)
	if s.store == nil {
		return
	}
	go func() {
		// Short-lived context just for the DB lookup.
		queryCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		hooks, err := s.store.ListWebhooksForEvent(queryCtx, event)
		cancel()
		if err != nil || len(hooks) == 0 {
			return
		}

		payload, err := json.Marshal(evt)
		if err != nil {
			return
		}

		for _, hook := range hooks {
			// Each delivery gets its own independent context so the parent cancel
			// does not abort in-flight HTTP requests when the outer goroutine exits.
			deliveryCtx, deliveryCancel := context.WithTimeout(context.Background(), 2*time.Minute)
			go func(h store.Webhook, dc context.CancelFunc) {
				defer dc()
				s.deliverWebhook(deliveryCtx, h, evt.ID, event, payload)
			}(hook, deliveryCancel)
		}
	}()
}

func (s *Server) deliverWebhook(ctx context.Context, hook store.Webhook, deliveryID, event string, payload []byte) {
	sig := webhookSignature(hook.Secret, payload)

	for attempt, delay := range webhookRetryBackoff {
		if delay > 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(delay):
			}
		}

		err := s.postWebhook(ctx, hook.URL, deliveryID, event, sig, payload)
		if err == nil {
			s.log.Debug("webhook delivered", "url", hook.URL, "event", event, "attempt", attempt+1)
			return
		}
		s.log.Warn("webhook delivery failed", "url", hook.URL, "event", event, "attempt", attempt+1, "err", err)
	}
}

func (s *Server) postWebhook(ctx context.Context, url, deliveryID, event, sig string, payload []byte) error {
	reqCtx, cancel := context.WithTimeout(ctx, webhookDeliveryTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GoDrive-Event", event)
	req.Header.Set("X-GoDrive-Delivery", deliveryID)
	req.Header.Set("X-GoDrive-Signature", "sha256="+sig)
	req.Header.Set("User-Agent", "goDrive-Webhook/1.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil
}

func webhookSignature(secret string, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}
