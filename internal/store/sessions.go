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
	row := s.db.QueryRowContext(ctx, `
		SELECT
			s.id, s.user_id, s.token_hash, s.csrf_token_hash, s.created_at, s.expires_at, s.revoked_at,
			u.id, u.username, u.password_hash, u.is_admin, u.disabled, u.home_root, u.created_at, u.updated_at
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = ?
			AND s.revoked_at IS NULL
			AND s.expires_at > ?
			AND u.disabled = 0
	`, tokenHash, timeString(now))

	var session Session
	var user User
	var sCreatedAt, sExpiresAt string
	var sRevokedAt sql.NullString
	var uCreatedAt, uUpdatedAt string
	var isAdmin, disabled int

	err := row.Scan(
		&session.ID, &session.UserID, &session.TokenHash, &session.CSRFTokenHash,
		&sCreatedAt, &sExpiresAt, &sRevokedAt,
		&user.ID, &user.Username, &user.PasswordHash, &isAdmin, &disabled,
		&user.HomeRoot, &uCreatedAt, &uUpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return User{}, Session{}, ErrNotFound
		}
		return User{}, Session{}, err
	}

	var err1, err2 error
	session.CreatedAt, err1 = scanTime(sCreatedAt)
	session.ExpiresAt, err2 = scanTime(sExpiresAt)
	if err1 != nil {
		return User{}, Session{}, err1
	}
	if err2 != nil {
		return User{}, Session{}, err2
	}
	if sRevokedAt.Valid {
		parsed, err := scanTime(sRevokedAt.String)
		if err != nil {
			return User{}, Session{}, err
		}
		session.RevokedAt = sql.NullTime{Time: parsed, Valid: true}
	}

	user.IsAdmin = isAdmin == 1
	user.Disabled = disabled == 1
	user.CreatedAt, err1 = scanTime(uCreatedAt)
	user.UpdatedAt, err2 = scanTime(uUpdatedAt)
	if err1 != nil {
		return User{}, Session{}, err1
	}
	if err2 != nil {
		return User{}, Session{}, err2
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
