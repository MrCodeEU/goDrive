package store

import "context"

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
		`CREATE INDEX IF NOT EXISTS idx_file_index_preview_kind ON file_index(preview_kind)`,
	}

	for _, statement := range statements {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}
