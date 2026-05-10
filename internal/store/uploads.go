package store

import (
	"context"
	"database/sql"
)

func (s *Store) CreateUpload(ctx context.Context, upload Upload) error {
	now := nowString()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO uploads (
			id, user_id, upload_length, offset, metadata_json, target_dir, filename,
			temp_path, created_at, updated_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, upload.ID, upload.UserID, upload.UploadLength, upload.Offset, upload.MetadataJSON, upload.TargetDir, upload.Filename, upload.TempPath, now, now)
	return err
}

func (s *Store) GetUpload(ctx context.Context, id string) (Upload, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, user_id, upload_length, offset, metadata_json, target_dir, filename,
			temp_path, final_path, created_at, updated_at, completed_at
		FROM uploads
		WHERE id = ?
	`, id)
	return scanUpload(row)
}

func (s *Store) UpdateUploadOffset(ctx context.Context, id string, offset int64) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE uploads
		SET offset = ?, updated_at = ?
		WHERE id = ? AND completed_at IS NULL
	`, offset, nowString(), id)
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

func (s *Store) CompleteUpload(ctx context.Context, id string, finalPath string) error {
	result, err := s.db.ExecContext(ctx, `
		UPDATE uploads
		SET offset = upload_length, final_path = ?, updated_at = ?, completed_at = ?
		WHERE id = ? AND completed_at IS NULL
	`, finalPath, nowString(), nowString(), id)
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

func (s *Store) DeleteUpload(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM uploads WHERE id = ?`, id)
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

func scanUpload(scanner interface {
	Scan(dest ...any) error
}) (Upload, error) {
	var upload Upload
	var createdAt, updatedAt string
	var completedAt sql.NullString
	err := scanner.Scan(
		&upload.ID,
		&upload.UserID,
		&upload.UploadLength,
		&upload.Offset,
		&upload.MetadataJSON,
		&upload.TargetDir,
		&upload.Filename,
		&upload.TempPath,
		&upload.FinalPath,
		&createdAt,
		&updatedAt,
		&completedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return Upload{}, ErrNotFound
		}
		return Upload{}, err
	}

	var err1, err2 error
	upload.CreatedAt, err1 = scanTime(createdAt)
	upload.UpdatedAt, err2 = scanTime(updatedAt)
	if err1 != nil {
		return Upload{}, err1
	}
	if err2 != nil {
		return Upload{}, err2
	}
	if completedAt.Valid {
		parsed, err := scanTime(completedAt.String)
		if err != nil {
			return Upload{}, err
		}
		upload.CompletedAt = sql.NullTime{Time: parsed, Valid: true}
	}
	return upload, nil
}
