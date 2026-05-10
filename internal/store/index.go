package store

import (
	"context"
)

const upsertFileIndexSQL = `
	INSERT INTO file_index (
		user_id, path, name, type, size, modified_at, mime_type,
		preview_kind, last_seen_scan, updated_at
	)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(user_id, path) DO UPDATE SET
		name = excluded.name,
		type = excluded.type,
		size = excluded.size,
		modified_at = excluded.modified_at,
		mime_type = excluded.mime_type,
		preview_kind = excluded.preview_kind,
		last_seen_scan = excluded.last_seen_scan,
		updated_at = excluded.updated_at
`

func (s *Store) UpsertFileIndexEntry(ctx context.Context, entry FileIndexEntry) error {
	now := nowString()
	_, err := s.db.ExecContext(ctx, upsertFileIndexSQL, entry.UserID, entry.Path, entry.Name, entry.Type, entry.Size, timeString(entry.ModifiedAt), entry.MimeType, entry.PreviewKind, entry.LastSeenScan, now)
	return err
}

func (s *Store) UpsertFileIndexEntries(ctx context.Context, entries []FileIndexEntry) error {
	if len(entries) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, upsertFileIndexSQL)
	if err != nil {
		return err
	}
	defer stmt.Close()

	now := nowString()
	for _, entry := range entries {
		if _, err := stmt.ExecContext(ctx, entry.UserID, entry.Path, entry.Name, entry.Type, entry.Size, timeString(entry.ModifiedAt), entry.MimeType, entry.PreviewKind, entry.LastSeenScan, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) DeleteFileIndexEntriesNotSeen(ctx context.Context, userID int64, scanID string) (int64, error) {
	result, err := s.db.ExecContext(ctx, `
		DELETE FROM file_index
		WHERE user_id = ? AND last_seen_scan <> ?
	`, userID, scanID)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (s *Store) IndexStats(ctx context.Context) (IndexStats, error) {
	var stats IndexStats
	err := s.db.QueryRowContext(ctx, `
		SELECT
			COALESCE(SUM(CASE WHEN type = 'file' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN type = 'dir' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN type = 'file' THEN size ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN preview_kind IN ('image', 'video', 'pdf') THEN 1 ELSE 0 END), 0)
		FROM file_index
	`).Scan(&stats.IndexedFiles, &stats.IndexedDirectories, &stats.IndexedBytes, &stats.PreviewCandidates)
	return stats, err
}

func (s *Store) ListPreviewCandidates(ctx context.Context) ([]PreviewCandidate, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT fi.user_id, users.username, users.home_root, fi.path, fi.size, fi.modified_at, fi.preview_kind
		FROM file_index fi
		JOIN users ON users.id = fi.user_id
		WHERE users.disabled = 0
			AND fi.type = 'file'
			AND fi.preview_kind IN ('image', 'video', 'pdf')
		ORDER BY fi.user_id, fi.path
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var candidates []PreviewCandidate
	for rows.Next() {
		var candidate PreviewCandidate
		var modifiedAt string
		err := rows.Scan(
			&candidate.UserID,
			&candidate.Username,
			&candidate.HomeRoot,
			&candidate.Path,
			&candidate.Size,
			&modifiedAt,
			&candidate.PreviewKind,
		)
		if err != nil {
			return nil, err
		}
		candidate.ModifiedAt, err = scanTime(modifiedAt)
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, candidate)
	}
	return candidates, rows.Err()
}

func (s *Store) TrashStats(ctx context.Context) (count int64, bytes int64, err error) {
	err = s.db.QueryRowContext(ctx, `
		SELECT COUNT(*), COALESCE(SUM(size), 0)
		FROM trash_items
	`).Scan(&count, &bytes)
	return count, bytes, err
}

func (s *Store) UserStats(ctx context.Context) (total int64, disabled int64, err error) {
	err = s.db.QueryRowContext(ctx, `
		SELECT COUNT(*), COALESCE(SUM(CASE WHEN disabled = 1 THEN 1 ELSE 0 END), 0)
		FROM users
	`).Scan(&total, &disabled)
	return total, disabled, err
}
