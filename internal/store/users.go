package store

import (
	"context"
	"database/sql"
)

func (s *Store) CountAdmins(ctx context.Context) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE is_admin = 1`).Scan(&count)
	return count, err
}

func (s *Store) CreateUser(ctx context.Context, user User) (User, error) {
	now := nowString()
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO users (username, password_hash, is_admin, disabled, home_root, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, user.Username, user.PasswordHash, boolInt(user.IsAdmin), boolInt(user.Disabled), user.HomeRoot, now, now)
	if err != nil {
		return User{}, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return User{}, err
	}
	return s.GetUserByID(ctx, id)
}

func (s *Store) GetUserByID(ctx context.Context, id int64) (User, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, username, password_hash, is_admin, disabled, home_root, created_at, updated_at
		FROM users
		WHERE id = ?
	`, id)
	return scanUser(row)
}

func (s *Store) GetUserByUsername(ctx context.Context, username string) (User, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, username, password_hash, is_admin, disabled, home_root, created_at, updated_at
		FROM users
		WHERE username = ?
	`, username)
	return scanUser(row)
}

func (s *Store) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, username, password_hash, is_admin, disabled, home_root, created_at, updated_at
		FROM users
		ORDER BY username
	`)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	var users []User
	for rows.Next() {
		user, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

func (s *Store) UpdateUser(ctx context.Context, user User) (User, error) {
	now := nowString()
	result, err := s.db.ExecContext(ctx, `
		UPDATE users
		SET username = ?, is_admin = ?, disabled = ?, home_root = ?, updated_at = ?
		WHERE id = ?
	`, user.Username, boolInt(user.IsAdmin), boolInt(user.Disabled), user.HomeRoot, now, user.ID)
	if err != nil {
		return User{}, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return User{}, err
	}
	if affected == 0 {
		return User{}, ErrNotFound
	}
	return s.GetUserByID(ctx, user.ID)
}

func (s *Store) SetPassword(ctx context.Context, id int64, passwordHash string) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE users
		SET password_hash = ?, updated_at = ?
		WHERE id = ?
	`, passwordHash, nowString(), id)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func scanUser(scanner interface {
	Scan(dest ...any) error
}) (User, error) {
	var user User
	var createdAt, updatedAt string
	var isAdmin, disabled int
	err := scanner.Scan(
		&user.ID,
		&user.Username,
		&user.PasswordHash,
		&isAdmin,
		&disabled,
		&user.HomeRoot,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return User{}, ErrNotFound
		}
		return User{}, err
	}

	user.IsAdmin = isAdmin == 1
	user.Disabled = disabled == 1
	var err1, err2 error
	user.CreatedAt, err1 = scanTime(createdAt)
	user.UpdatedAt, err2 = scanTime(updatedAt)
	if err1 != nil {
		return User{}, err1
	}
	if err2 != nil {
		return User{}, err2
	}
	return user, nil
}

// scanUserTimes parses the createdAt/updatedAt strings into u and returns the updated value.
// Used when user fields are scanned inline alongside other columns.
func scanUserTimes(u User, createdAt, updatedAt string) (User, error) {
	var err error
	if u.CreatedAt, err = scanTime(createdAt); err != nil {
		return User{}, err
	}
	if u.UpdatedAt, err = scanTime(updatedAt); err != nil {
		return User{}, err
	}
	return u, nil
}
