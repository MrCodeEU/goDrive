package store

import (
	"context"
	"database/sql"
)

func (s *Store) CreateTrashItem(ctx context.Context, item TrashItem) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO trash_items (id, user_id, original_path, original_name, trash_path, is_dir, size, deleted_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, item.ID, item.UserID, item.OriginalPath, item.OriginalName, item.TrashPath, boolInt(item.IsDir), item.Size, timeString(item.DeletedAt))
	return err
}

func (s *Store) GetTrashItem(ctx context.Context, userID int64, id string) (TrashItem, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, user_id, original_path, original_name, trash_path, is_dir, size, deleted_at
		FROM trash_items
		WHERE user_id = ? AND id = ?
	`, userID, id)
	return scanTrashItem(row)
}

func (s *Store) ListTrashItems(ctx context.Context, userID int64) ([]TrashItem, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, user_id, original_path, original_name, trash_path, is_dir, size, deleted_at
		FROM trash_items
		WHERE user_id = ?
		ORDER BY deleted_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []TrashItem
	for rows.Next() {
		item, err := scanTrashItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) DeleteTrashItem(ctx context.Context, userID int64, id string) error {
	result, err := s.db.ExecContext(ctx, `
		DELETE FROM trash_items
		WHERE user_id = ? AND id = ?
	`, userID, id)
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

func scanTrashItem(scanner interface {
	Scan(dest ...any) error
}) (TrashItem, error) {
	var item TrashItem
	var deletedAt string
	var isDir int
	err := scanner.Scan(
		&item.ID,
		&item.UserID,
		&item.OriginalPath,
		&item.OriginalName,
		&item.TrashPath,
		&isDir,
		&item.Size,
		&deletedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return TrashItem{}, ErrNotFound
		}
		return TrashItem{}, err
	}
	item.IsDir = isDir == 1
	var parseErr error
	item.DeletedAt, parseErr = scanTime(deletedAt)
	if parseErr != nil {
		return TrashItem{}, parseErr
	}
	return item, nil
}
