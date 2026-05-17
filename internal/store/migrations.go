package store

import (
	"context"
	"fmt"
)

func (s *Store) Migrate(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			is_admin INTEGER NOT NULL DEFAULT 0,
			disabled INTEGER NOT NULL DEFAULT 0,
			home_root TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS sessions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			token_hash TEXT NOT NULL UNIQUE,
			csrf_token_hash TEXT NOT NULL,
			created_at TEXT NOT NULL,
			expires_at TEXT NOT NULL,
			revoked_at TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_token_hash ON sessions(token_hash)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id)`,
		`CREATE TABLE IF NOT EXISTS trash_items (
			id TEXT PRIMARY KEY,
			user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			original_path TEXT NOT NULL,
			original_name TEXT NOT NULL,
			trash_path TEXT NOT NULL,
			is_dir INTEGER NOT NULL,
			size INTEGER NOT NULL,
			deleted_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_trash_user_id ON trash_items(user_id)`,
		`CREATE TABLE IF NOT EXISTS uploads (
			id TEXT PRIMARY KEY,
			user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			upload_length INTEGER NOT NULL,
			offset INTEGER NOT NULL,
			metadata_json TEXT NOT NULL,
			target_dir TEXT NOT NULL,
			filename TEXT NOT NULL,
			temp_path TEXT NOT NULL,
			final_path TEXT,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			completed_at TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_uploads_user_id ON uploads(user_id)`,
		`CREATE TABLE IF NOT EXISTS file_index (
			user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			path TEXT NOT NULL,
			parent_path TEXT NOT NULL DEFAULT '/',
			name TEXT NOT NULL,
			type TEXT NOT NULL,
			size INTEGER NOT NULL,
			modified_at TEXT NOT NULL,
			mime_type TEXT NOT NULL,
			preview_kind TEXT NOT NULL,
			last_seen_scan TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			PRIMARY KEY (user_id, path)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_file_index_user_type ON file_index(user_id, type)`,
		`CREATE INDEX IF NOT EXISTS idx_file_index_user_name ON file_index(user_id, name)`,
		`CREATE INDEX IF NOT EXISTS idx_file_index_user_path ON file_index(user_id, path)`,
		`CREATE INDEX IF NOT EXISTS idx_file_index_preview_kind ON file_index(preview_kind)`,
		`CREATE INDEX IF NOT EXISTS idx_file_index_type_size ON file_index(type, size)`,
		`CREATE INDEX IF NOT EXISTS idx_file_index_preview_candidate ON file_index(preview_kind, type)`,
		`CREATE TABLE IF NOT EXISTS webhooks (
			id          TEXT PRIMARY KEY,
			url         TEXT NOT NULL,
			secret      TEXT NOT NULL,
			events      TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			created_at  TEXT NOT NULL,
			updated_at  TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_webhooks_id ON webhooks(id)`,
		`CREATE TABLE IF NOT EXISTS api_keys (
			id          TEXT PRIMARY KEY,
			user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			name        TEXT NOT NULL,
			token_hash  TEXT NOT NULL UNIQUE,
			created_at  TEXT NOT NULL,
			last_used_at TEXT,
			revoked_at  TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_api_keys_token_hash ON api_keys(token_hash)`,
		`CREATE INDEX IF NOT EXISTS idx_api_keys_user_id ON api_keys(user_id)`,
	}

	for _, statement := range statements {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	if err := s.ensureColumn(ctx, "file_index", "parent_path", "TEXT NOT NULL DEFAULT '/'"); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_file_index_user_parent_order ON file_index(user_id, parent_path, type, name COLLATE NOCASE, path COLLATE NOCASE)`); err != nil {
		return err
	}
	if err := s.BackfillFileIndexParentPaths(ctx); err != nil {
		return err
	}
	if err := s.ensureFileIndexSearch(ctx); err != nil {
		return err
	}
	return s.ensureDocumentSearch(ctx)
}

func (s *Store) ensureColumn(ctx context.Context, table, column, definition string) error {
	rows, err := s.db.QueryContext(ctx, "PRAGMA table_info("+table+")")
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var cid int
		var name string
		var typ string
		var notNull int
		var defaultValue any
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			return err
		}
		if name == column {
			return rows.Err()
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, definition))
	return err
}

func (s *Store) ensureFileIndexSearch(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `
		CREATE VIRTUAL TABLE IF NOT EXISTS file_index_fts
		USING fts5(user_id UNINDEXED, path, name, tokenize = 'trigram')
	`); err != nil {
		return err
	}

	var indexedRows int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM file_index_fts`).Scan(&indexedRows); err != nil {
		return err
	}
	var sourceRows int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM file_index`).Scan(&sourceRows); err != nil {
		return err
	}
	if indexedRows == sourceRows {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM file_index_fts`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO file_index_fts(rowid, user_id, path, name)
		SELECT rowid, user_id, path, name
		FROM file_index
	`); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ensureDocumentSearch(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
		CREATE VIRTUAL TABLE IF NOT EXISTS document_fts
		USING fts5(user_id UNINDEXED, path UNINDEXED, content)
	`)
	return err
}
