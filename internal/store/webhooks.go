package store

import (
	"context"
	"encoding/json"
	"time"
)

type Webhook struct {
	ID          string    `json:"id"`
	URL         string    `json:"url"`
	Secret      string    `json:"-"`
	Events      []string  `json:"events"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (s *Store) CreateWebhook(ctx context.Context, wh Webhook) (Webhook, error) {
	eventsJSON, err := json.Marshal(wh.Events)
	if err != nil {
		return Webhook{}, err
	}
	now := nowString()
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO webhooks (id, url, secret, events, description, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, wh.ID, wh.URL, wh.Secret, string(eventsJSON), wh.Description, now, now)
	if err != nil {
		return Webhook{}, err
	}
	return s.GetWebhook(ctx, wh.ID)
}

func (s *Store) GetWebhook(ctx context.Context, id string) (Webhook, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, url, secret, events, description, created_at, updated_at FROM webhooks WHERE id = ?
	`, id)
	return scanWebhook(row)
}

func (s *Store) ListWebhooks(ctx context.Context) ([]Webhook, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, url, secret, events, description, created_at, updated_at FROM webhooks ORDER BY created_at
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []Webhook
	for rows.Next() {
		wh, err := scanWebhook(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, wh)
	}
	return out, rows.Err()
}

func (s *Store) DeleteWebhook(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM webhooks WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ListWebhooksForEvent returns all webhooks subscribed to the given event type.
// A webhook with an empty events list receives all events.
func (s *Store) ListWebhooksForEvent(ctx context.Context, event string) ([]Webhook, error) {
	all, err := s.ListWebhooks(ctx)
	if err != nil {
		return nil, err
	}
	var matched []Webhook
	for _, wh := range all {
		if len(wh.Events) == 0 {
			matched = append(matched, wh)
			continue
		}
		for _, e := range wh.Events {
			if e == event || e == "*" {
				matched = append(matched, wh)
				break
			}
		}
	}
	return matched, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanWebhook(s scanner) (Webhook, error) {
	var wh Webhook
	var eventsJSON, createdAt, updatedAt string
	if err := s.Scan(&wh.ID, &wh.URL, &wh.Secret, &eventsJSON, &wh.Description, &createdAt, &updatedAt); err != nil {
		return Webhook{}, err
	}
	if err := json.Unmarshal([]byte(eventsJSON), &wh.Events); err != nil {
		wh.Events = nil
	}
	var err1, err2 error
	wh.CreatedAt, err1 = scanTime(createdAt)
	wh.UpdatedAt, err2 = scanTime(updatedAt)
	if err1 != nil {
		return Webhook{}, err1
	}
	if err2 != nil {
		return Webhook{}, err2
	}
	return wh, nil
}
