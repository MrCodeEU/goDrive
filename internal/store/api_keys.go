package store

import (
	"context"
	"database/sql"
	"time"
)

type APIKey struct {
	ID         string     `json:"id"`
	UserID     int64      `json:"user_id"`
	Username   string     `json:"username"`
	Name       string     `json:"name"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
}

func (s *Store) CreateAPIKey(ctx context.Context, id string, userID int64, name, tokenHash string) (APIKey, error) {
	now := nowString()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO api_keys (id, user_id, name, token_hash, created_at)
		VALUES (?, ?, ?, ?, ?)
	`, id, userID, name, tokenHash, now)
	if err != nil {
		return APIKey{}, err
	}
	t, err := scanTime(now)
	if err != nil {
		return APIKey{}, err
	}
	return APIKey{ID: id, UserID: userID, Name: name, CreatedAt: t}, nil
}

func (s *Store) GetAPIKey(ctx context.Context, id string) (APIKey, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT ak.id, ak.user_id, u.username, ak.name, ak.created_at, ak.last_used_at, ak.revoked_at
		FROM api_keys ak
		JOIN users u ON u.id = ak.user_id
		WHERE ak.id = ?
	`, id)
	return scanAPIKey(row)
}

func (s *Store) ListAPIKeys(ctx context.Context) ([]APIKey, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT ak.id, ak.user_id, u.username, ak.name, ak.created_at, ak.last_used_at, ak.revoked_at
		FROM api_keys ak
		JOIN users u ON u.id = ak.user_id
		ORDER BY ak.created_at DESC
		LIMIT 500
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []APIKey
	for rows.Next() {
		key, err := scanAPIKey(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, key)
	}
	return out, rows.Err()
}

func (s *Store) RevokeAPIKey(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx,
		`UPDATE api_keys SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL`,
		nowString(), id)
	if err != nil {
		return err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// UserByAPIKey looks up the user for a valid (non-revoked) API key token hash.
// Updates last_used_at at most once per 5 minutes to reduce write pressure.
func (s *Store) UserByAPIKey(ctx context.Context, tokenHash string) (User, error) {
	var lastUsedAt sql.NullString
	row := s.db.QueryRowContext(ctx, `
		SELECT u.id, u.username, u.password_hash, u.is_admin, u.disabled, u.home_root, u.created_at, u.updated_at,
		       ak.last_used_at
		FROM api_keys ak
		JOIN users u ON u.id = ak.user_id
		WHERE ak.token_hash = ? AND ak.revoked_at IS NULL AND u.disabled = 0
	`, tokenHash)
	var u User
	var createdAt, updatedAt string
	if err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.IsAdmin, &u.Disabled, &u.HomeRoot, &createdAt, &updatedAt, &lastUsedAt); err != nil {
		if err == sql.ErrNoRows {
			return User{}, ErrNotFound
		}
		return User{}, err
	}
	u, err := scanUserTimes(u, createdAt, updatedAt)
	if err != nil {
		return User{}, err
	}

	// Only write last_used_at if it's stale by more than 5 minutes.
	stale := true
	if lastUsedAt.Valid {
		if t, err := scanTime(lastUsedAt.String); err == nil {
			stale = time.Since(t) > 5*time.Minute
		}
	}
	if stale {
		_, _ = s.db.ExecContext(ctx, `UPDATE api_keys SET last_used_at = ? WHERE token_hash = ?`, nowString(), tokenHash)
	}
	return u, nil
}

func scanAPIKey(s scanner) (APIKey, error) {
	var key APIKey
	var createdAt string
	var lastUsedAt, revokedAt sql.NullString
	if err := s.Scan(&key.ID, &key.UserID, &key.Username, &key.Name, &createdAt, &lastUsedAt, &revokedAt); err != nil {
		return APIKey{}, err
	}
	var err error
	if key.CreatedAt, err = scanTime(createdAt); err != nil {
		return APIKey{}, err
	}
	if lastUsedAt.Valid {
		t, err := scanTime(lastUsedAt.String)
		if err != nil {
			return APIKey{}, err
		}
		key.LastUsedAt = &t
	}
	if revokedAt.Valid {
		t, err := scanTime(revokedAt.String)
		if err != nil {
			return APIKey{}, err
		}
		key.RevokedAt = &t
	}
	return key, nil
}
