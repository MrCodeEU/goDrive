package store

import (
	"context"
	"database/sql"
	"time"
)

func (s *Store) CreateSession(ctx context.Context, userID int64, tokenHash, csrfTokenHash string, expiresAt time.Time) (Session, error) {
	now := nowString()
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO sessions (user_id, token_hash, csrf_token_hash, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?)
	`, userID, tokenHash, csrfTokenHash, now, timeString(expiresAt))
	if err != nil {
		return Session{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return Session{}, err
	}
	return s.GetSessionByID(ctx, id)
}

func (s *Store) GetSessionByID(ctx context.Context, id int64) (Session, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, user_id, token_hash, csrf_token_hash, created_at, expires_at, revoked_at
		FROM sessions
		WHERE id = ?
	`, id)
	return scanSession(row)
}

func (s *Store) GetSessionByTokenHash(ctx context.Context, tokenHash string) (Session, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, user_id, token_hash, csrf_token_hash, created_at, expires_at, revoked_at
		FROM sessions
		WHERE token_hash = ?
	`, tokenHash)
	return scanSession(row)
}

func (s *Store) UserByValidSession(ctx context.Context, tokenHash string, now time.Time) (User, Session, error) {
	session, err := s.GetSessionByTokenHash(ctx, tokenHash)
	if err != nil {
		return User{}, Session{}, err
	}
	if session.RevokedAt.Valid || !session.ExpiresAt.After(now) {
		return User{}, Session{}, ErrNotFound
	}

	user, err := s.GetUserByID(ctx, session.UserID)
	if err != nil {
		return User{}, Session{}, err
	}
	if user.Disabled {
		return User{}, Session{}, ErrNotFound
	}

	return user, session, nil
}

func (s *Store) RevokeSession(ctx context.Context, tokenHash string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE sessions
		SET revoked_at = ?
		WHERE token_hash = ? AND revoked_at IS NULL
	`, nowString(), tokenHash)
	return err
}

func (s *Store) RevokeUserSessions(ctx context.Context, userID int64) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE sessions
		SET revoked_at = ?
		WHERE user_id = ? AND revoked_at IS NULL
	`, nowString(), userID)
	return err
}

func (s *Store) DeleteExpiredSessions(ctx context.Context, now time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM sessions
		WHERE expires_at <= ? OR revoked_at IS NOT NULL
	`, timeString(now))
	return err
}

func scanSession(scanner interface {
	Scan(dest ...any) error
}) (Session, error) {
	var session Session
	var createdAt, expiresAt string
	var revokedAt sql.NullString
	err := scanner.Scan(
		&session.ID,
		&session.UserID,
		&session.TokenHash,
		&session.CSRFTokenHash,
		&createdAt,
		&expiresAt,
		&revokedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return Session{}, ErrNotFound
		}
		return Session{}, err
	}

	parsedCreatedAt, err := scanTime(createdAt)
	if err != nil {
		return Session{}, err
	}
	parsedExpiresAt, err := scanTime(expiresAt)
	if err != nil {
		return Session{}, err
	}
	session.CreatedAt = parsedCreatedAt
	session.ExpiresAt = parsedExpiresAt

	if revokedAt.Valid {
		parsed, err := scanTime(revokedAt.String)
		if err != nil {
			return Session{}, err
		}
		session.RevokedAt = sql.NullTime{Time: parsed, Valid: true}
	}

	return session, nil
}
