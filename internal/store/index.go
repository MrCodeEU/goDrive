package store

import (
	"context"
	"database/sql"
	"path"
	"sort"
	"strings"
	"unicode"
)

const upsertFileIndexSQL = `
	INSERT INTO file_index (
		user_id, path, parent_path, name, type, size, modified_at, mime_type,
		preview_kind, last_seen_scan, updated_at
	)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(user_id, path) DO UPDATE SET
		parent_path = excluded.parent_path,
		name = excluded.name,
		type = excluded.type,
		size = excluded.size,
		modified_at = excluded.modified_at,
		mime_type = excluded.mime_type,
		preview_kind = excluded.preview_kind,
		last_seen_scan = excluded.last_seen_scan,
		updated_at = excluded.updated_at
	RETURNING rowid
`

const insertFileIndexSearchSQL = `INSERT OR REPLACE INTO file_index_fts(rowid, user_id, path, name) VALUES (?, ?, ?, ?)`
const deleteDocumentTextSQL = `DELETE FROM document_fts WHERE user_id = ? AND path = ?`

type fileIndexExecutor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func (s *Store) UpsertFileIndexEntry(ctx context.Context, entry FileIndexEntry) error {
	now := nowString()
	entry.ParentPath = parentPathForIndex(entry.Path)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var rowID int64
	if err := tx.QueryRowContext(ctx, upsertFileIndexSQL, entry.UserID, entry.Path, entry.ParentPath, entry.Name, entry.Type, entry.Size, timeString(entry.ModifiedAt), entry.MimeType, entry.PreviewKind, entry.LastSeenScan, now).Scan(&rowID); err != nil {
		return err
	}
	if err := upsertFileIndexSearchEntry(ctx, tx, rowID, entry); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if s.engine != nil {
		_ = s.engine.IndexFile(ctx, entry.UserID, entry.Path, entry.Name, "")
	}
	return nil
}

func (s *Store) UpsertFileIndexEntries(ctx context.Context, entries []FileIndexEntry) error {
	if len(entries) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	stmt, err := tx.PrepareContext(ctx, upsertFileIndexSQL)
	if err != nil {
		return err
	}
	defer func() {
		_ = stmt.Close()
	}()

	now := nowString()
	for _, entry := range entries {
		entry.ParentPath = parentPathForIndex(entry.Path)
		var rowID int64
		if err := stmt.QueryRowContext(ctx, entry.UserID, entry.Path, entry.ParentPath, entry.Name, entry.Type, entry.Size, timeString(entry.ModifiedAt), entry.MimeType, entry.PreviewKind, entry.LastSeenScan, now).Scan(&rowID); err != nil {
			return err
		}
		if err := upsertFileIndexSearchEntry(ctx, tx, rowID, entry); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if s.engine != nil {
		for _, entry := range entries {
			_ = s.engine.IndexFile(ctx, entry.UserID, entry.Path, entry.Name, "")
		}
	}
	return nil
}

func upsertFileIndexSearchEntry(ctx context.Context, execer fileIndexExecutor, rowID int64, entry FileIndexEntry) error {
	if _, err := execer.ExecContext(ctx, insertFileIndexSearchSQL, rowID, entry.UserID, entry.Path, entry.Name); err != nil {
		return err
	}
	if entry.Type != "file" || (entry.PreviewKind != "text" && entry.PreviewKind != "markdown") {
		_, err := execer.ExecContext(ctx, deleteDocumentTextSQL, entry.UserID, entry.Path)
		return err
	}
	return nil
}

func (s *Store) UpsertDocumentTextEntry(ctx context.Context, entry DocumentTextEntry) error {
	return s.UpsertDocumentTextEntries(ctx, []DocumentTextEntry{entry})
}

func (s *Store) UpsertDocumentTextEntries(ctx context.Context, entries []DocumentTextEntry) error {
	if len(entries) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	deleteStmt, err := tx.PrepareContext(ctx, deleteDocumentTextSQL)
	if err != nil {
		return err
	}
	defer func() { _ = deleteStmt.Close() }()
	insertStmt, err := tx.PrepareContext(ctx, `INSERT INTO document_fts(user_id, path, content) VALUES (?, ?, ?)`)
	if err != nil {
		return err
	}
	defer func() { _ = insertStmt.Close() }()
	for _, entry := range entries {
		if _, err := deleteStmt.ExecContext(ctx, entry.UserID, entry.Path); err != nil {
			return err
		}
		if _, err := insertStmt.ExecContext(ctx, entry.UserID, entry.Path, entry.Content); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if s.engine != nil {
		for _, entry := range entries {
			_ = s.engine.IndexFile(ctx, entry.UserID, entry.Path, entry.Name, entry.Content)
		}
	}
	return nil
}

func (s *Store) DeleteDocumentText(ctx context.Context, userID int64, logical string) error {
	_, err := s.db.ExecContext(ctx, deleteDocumentTextSQL, userID, logical)
	return err
}

func (s *Store) BackfillFileIndexParentPaths(ctx context.Context) error {
	rows, err := s.db.QueryContext(ctx, `
		SELECT user_id, path
		FROM file_index
		WHERE parent_path = '/' AND substr(path, 2) LIKE '%/%'
	`)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()

	type row struct {
		userID int64
		path   string
	}
	var rowsToUpdate []row
	for rows.Next() {
		var item row
		if err := rows.Scan(&item.userID, &item.path); err != nil {
			return err
		}
		rowsToUpdate = append(rowsToUpdate, item)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(rowsToUpdate) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx, `UPDATE file_index SET parent_path = ? WHERE user_id = ? AND path = ?`)
	if err != nil {
		return err
	}
	defer func() { _ = stmt.Close() }()
	for _, item := range rowsToUpdate {
		if _, err := stmt.ExecContext(ctx, parentPathForIndex(item.path), item.userID, item.path); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func parentPathForIndex(logical string) string {
	if logical == "" || logical == "/" {
		return "/"
	}
	parent := path.Dir(logical)
	if parent == "." || parent == "" {
		return "/"
	}
	return parent
}

type ListFileIndexPage struct {
	Entries []FileIndexEntry
	Total   int
}

func (s *Store) ListFileIndexFolder(ctx context.Context, userID int64, parentPath string, afterType string, afterName string, afterPath string, offset int, limit int) (ListFileIndexPage, error) {
	if parentPath == "" {
		parentPath = "/"
	}
	if limit <= 0 {
		limit = 500
	}
	if limit > 2000 {
		limit = 2000
	}
	var total int
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM file_index
		WHERE user_id = ? AND parent_path = ?
	`, userID, parentPath).Scan(&total); err != nil {
		return ListFileIndexPage{}, err
	}
	if total == 0 {
		return ListFileIndexPage{Total: 0}, nil
	}

	afterName = strings.ToLower(afterName)
	args := []any{userID, parentPath}
	where := `fi.user_id = ? AND fi.parent_path = ?`
	if afterName != "" && afterPath != "" {
		where += ` AND (fi.type, fi.name COLLATE NOCASE, fi.path COLLATE NOCASE) > (?, ?, ?)`
		args = append(args, afterType, afterName, afterPath)
	}

	query := `
		SELECT fi.user_id, fi.path, fi.parent_path, fi.name, fi.type, fi.size, fi.modified_at, fi.mime_type, fi.preview_kind, fi.last_seen_scan, fi.updated_at,
		       CASE WHEN fi.preview_kind IN ('text', 'markdown') THEN COALESCE(SUBSTR(dt.content, 1, 300), '') ELSE '' END
		FROM file_index fi
		LEFT JOIN document_fts dt ON fi.user_id = dt.user_id AND fi.path = dt.path
		WHERE ` + where + `
		ORDER BY fi.type, fi.name COLLATE NOCASE, fi.path COLLATE NOCASE
	`
	if afterName == "" || afterPath == "" {
		query += ` LIMIT ?`
		args = append(args, limit)
		if offset > 0 {
			query += ` OFFSET ?`
			args = append(args, offset)
		}
	} else {
		query += ` LIMIT ?`
		args = append(args, limit)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return ListFileIndexPage{}, err
	}
	defer func() { _ = rows.Close() }()

	entries, err := scanFileIndexRowsWithTextSnippet(rows)
	if err != nil {
		return ListFileIndexPage{}, err
	}
	return ListFileIndexPage{Entries: entries, Total: total}, nil
}

func (s *Store) HasFileIndexDir(ctx context.Context, userID int64, logical string) (bool, error) {
	var exists int
	err := s.db.QueryRowContext(ctx, `
		SELECT 1
		FROM file_index
		WHERE user_id = ? AND path = ? AND type = 'dir'
		LIMIT 1
	`, userID, logical).Scan(&exists)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	return exists == 1, err
}

func (s *Store) ListFileIndexDirectories(ctx context.Context, userID int64) ([]FileIndexEntry, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT user_id, path, parent_path, name, type, size, modified_at, mime_type, preview_kind, last_seen_scan, updated_at
		FROM file_index
		WHERE user_id = ? AND type = 'dir'
		ORDER BY path COLLATE NOCASE
	`, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanFileIndexRows(rows)
}

func (s *Store) DeleteFileIndexEntriesNotSeen(ctx context.Context, userID int64, scanID string) (int64, error) {
	if s.engine != nil {
		rows, err := s.db.QueryContext(ctx,
			`SELECT path FROM file_index WHERE user_id = ? AND last_seen_scan <> ?`,
			userID, scanID)
		if err == nil {
			for rows.Next() {
				var p string
				if rows.Scan(&p) == nil {
					_ = s.engine.Delete(ctx, userID, p)
				}
			}
			_ = rows.Close()
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `
		DELETE FROM file_index_fts
		WHERE user_id = ?
			AND path IN (
				SELECT path
				FROM file_index
				WHERE user_id = ? AND last_seen_scan <> ?
			)
	`, userID, userID, scanID); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM document_fts
		WHERE user_id = ?
			AND path IN (
				SELECT path
				FROM file_index
				WHERE user_id = ? AND last_seen_scan <> ?
			)
	`, userID, userID, scanID); err != nil {
		return 0, err
	}

	result, err := tx.ExecContext(ctx, `
		DELETE FROM file_index
		WHERE user_id = ? AND last_seen_scan <> ?
	`, userID, scanID)
	if err != nil {
		return 0, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	return affected, tx.Commit()
}

func (s *Store) DeleteFileIndexEntriesNotSeenUnder(ctx context.Context, userID int64, scanID string, logical string) (int64, error) {
	if logical == "" || logical == "/" {
		return s.DeleteFileIndexEntriesNotSeen(ctx, userID, scanID)
	}
	if s.engine != nil {
		likePattern := escapeLikePattern(logical) + "/%"
		rows, err := s.db.QueryContext(ctx,
			`SELECT path FROM file_index WHERE user_id = ? AND (path = ? OR path LIKE ? ESCAPE '\') AND last_seen_scan <> ?`,
			userID, logical, likePattern, scanID)
		if err == nil {
			for rows.Next() {
				var p string
				if rows.Scan(&p) == nil {
					_ = s.engine.Delete(ctx, userID, p)
				}
			}
			_ = rows.Close()
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	likePattern := escapeLikePattern(logical) + "/%"
	where := `user_id = ? AND (path = ? OR path LIKE ? ESCAPE '\') AND last_seen_scan <> ?`
	args := []any{userID, logical, likePattern, scanID}

	if _, err := tx.ExecContext(ctx, `
		DELETE FROM file_index_fts
		WHERE user_id = ?
			AND path IN (
				SELECT path
				FROM file_index
				WHERE `+where+`
			)
	`, append([]any{userID}, args...)...); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM document_fts
		WHERE user_id = ?
			AND path IN (
				SELECT path
				FROM file_index
				WHERE `+where+`
			)
	`, append([]any{userID}, args...)...); err != nil {
		return 0, err
	}

	result, err := tx.ExecContext(ctx, `
		DELETE FROM file_index
		WHERE `+where, args...)
	if err != nil {
		return 0, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	return affected, tx.Commit()
}

func (s *Store) DeleteFileIndexPath(ctx context.Context, userID int64, logical string) (int64, error) {
	if s.engine != nil {
		_ = s.engine.DeletePrefix(ctx, userID, logical)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	likePattern := escapeLikePattern(logical) + "/%"
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM file_index_fts
		WHERE user_id = ? AND (path = ? OR path LIKE ? ESCAPE '\')
	`, userID, logical, likePattern); err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM document_fts
		WHERE user_id = ? AND (path = ? OR path LIKE ? ESCAPE '\')
	`, userID, logical, likePattern); err != nil {
		return 0, err
	}

	result, err := tx.ExecContext(ctx, `
		DELETE FROM file_index
		WHERE user_id = ? AND (path = ? OR path LIKE ? ESCAPE '\')
	`, userID, logical, likePattern)
	if err != nil {
		return 0, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	return affected, tx.Commit()
}

func (s *Store) searchViaEngine(ctx context.Context, userID int64, query string, limit int) ([]FileIndexEntry, error) {
	results, err := s.engine.Search(ctx, userID, query, limit)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, nil
	}

	paths := make([]string, len(results))
	snippetByPath := make(map[string]string, len(results))
	rankByPath := make(map[string]int, len(results))
	for i, r := range results {
		paths[i] = r.Path
		snippetByPath[r.Path] = r.Snippet
		rankByPath[r.Path] = i
	}

	placeholders := strings.Repeat("?,", len(paths))
	placeholders = placeholders[:len(placeholders)-1]

	args := make([]any, 0, 1+len(paths))
	args = append(args, userID)
	for _, p := range paths {
		args = append(args, p)
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT user_id, path, parent_path, name, type, size, modified_at, mime_type, preview_kind, last_seen_scan, updated_at
		 FROM file_index
		 WHERE user_id = ? AND path IN (`+placeholders+`)`,
		args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	entries, err := scanFileIndexRows(rows)
	if err != nil {
		return nil, err
	}

	for i := range entries {
		entries[i].Snippet = snippetByPath[entries[i].Path]
	}
	sort.Slice(entries, func(i, j int) bool {
		return rankByPath[entries[i].Path] < rankByPath[entries[j].Path]
	})
	return entries, nil
}

func (s *Store) SearchFileIndex(ctx context.Context, userID int64, query string, limit int) ([]FileIndexEntry, error) {
	if s.engine != nil {
		return s.searchViaEngine(ctx, userID, query, limit)
	}

	query = strings.TrimSpace(query)
	if query == "" {
		return []FileIndexEntry{}, nil
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	isFTS := isFileIndexFTSQuery(query)
	if isFTS {
		results, err := s.searchFileIndexFTS(ctx, userID, query, limit)
		if err == nil {
			return s.appendDocumentSearchResults(ctx, userID, query, limit, results)
		}
	}
	results, err := s.searchFileIndexLike(ctx, userID, query, limit)
	if err != nil {
		return nil, err
	}
	if isFTS {
		return s.appendDocumentSearchResults(ctx, userID, query, limit, results)
	}
	return results, nil
}

func (s *Store) searchFileIndexFTS(ctx context.Context, userID int64, query string, limit int) ([]FileIndexEntry, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT fi.user_id, fi.path, fi.parent_path, fi.name, fi.type, fi.size, fi.modified_at, fi.mime_type, fi.preview_kind, fi.last_seen_scan, fi.updated_at,
		       snippet(file_index_fts, 2, '<mark>', '</mark>', '…', 12)
		FROM file_index_fts
		JOIN file_index fi ON fi.user_id = file_index_fts.user_id AND fi.path = file_index_fts.path
		WHERE file_index_fts MATCH ?
			AND file_index_fts.user_id = ?
		LIMIT ?
	`, quoteFTS5Phrase(query), userID, limit)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	return scanFileIndexRowsWithSnippet(rows)
}

func (s *Store) searchFileIndexLike(ctx context.Context, userID int64, query string, limit int) ([]FileIndexEntry, error) {
	pattern := "%" + escapeLikePattern(query) + "%"
	rows, err := s.db.QueryContext(ctx, `
		SELECT user_id, path, parent_path, name, type, size, modified_at, mime_type, preview_kind, last_seen_scan, updated_at
		FROM file_index
		WHERE user_id = ?
			AND (name LIKE ? ESCAPE '\' OR path LIKE ? ESCAPE '\')
		ORDER BY type, name COLLATE NOCASE, path COLLATE NOCASE
		LIMIT ?
	`, userID, pattern, pattern, limit)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	return scanFileIndexRows(rows)
}

func (s *Store) appendDocumentSearchResults(ctx context.Context, userID int64, query string, limit int, existing []FileIndexEntry) ([]FileIndexEntry, error) {
	if len(existing) >= limit {
		return existing, nil
	}
	seen := make(map[string]struct{}, len(existing))
	for _, entry := range existing {
		seen[entry.Path] = struct{}{}
	}
	documentResults, err := s.searchDocumentTextFTS(ctx, userID, query, limit-len(existing))
	if err != nil {
		return existing, nil
	}
	for _, entry := range documentResults {
		if _, ok := seen[entry.Path]; ok {
			continue
		}
		existing = append(existing, entry)
		if len(existing) >= limit {
			break
		}
	}
	return existing, nil
}

func (s *Store) searchDocumentTextFTS(ctx context.Context, userID int64, query string, limit int) ([]FileIndexEntry, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT fi.user_id, fi.path, fi.parent_path, fi.name, fi.type, fi.size, fi.modified_at, fi.mime_type, fi.preview_kind, fi.last_seen_scan, fi.updated_at,
		       snippet(document_fts, 2, '<mark>', '</mark>', '…', 30)
		FROM document_fts
		JOIN file_index fi ON fi.user_id = document_fts.user_id AND fi.path = document_fts.path
		WHERE document_fts MATCH ?
			AND document_fts.user_id = ?
		ORDER BY bm25(document_fts)
		LIMIT ?
	`, quoteFTS5Phrase(query), userID, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanFileIndexRowsWithSnippet(rows)
}

func isFileIndexFTSQuery(query string) bool {
	if len([]rune(query)) < 3 {
		return false
	}
	for _, r := range query {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

func quoteFTS5Phrase(query string) string {
	return `"` + strings.ReplaceAll(query, `"`, `""`) + `"`
}

func scanFileIndexRows(rows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}) ([]FileIndexEntry, error) {
	var entries []FileIndexEntry
	for rows.Next() {
		var entry FileIndexEntry
		var modifiedAt string
		var updatedAt string
		if err := rows.Scan(
			&entry.UserID,
			&entry.Path,
			&entry.ParentPath,
			&entry.Name,
			&entry.Type,
			&entry.Size,
			&modifiedAt,
			&entry.MimeType,
			&entry.PreviewKind,
			&entry.LastSeenScan,
			&updatedAt,
		); err != nil {
			return nil, err
		}
		var err error
		entry.ModifiedAt, err = scanTime(modifiedAt)
		if err != nil {
			return nil, err
		}
		entry.UpdatedAt, err = scanTime(updatedAt)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

func scanFileIndexRowsWithTextSnippet(rows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}) ([]FileIndexEntry, error) {
	var entries []FileIndexEntry
	for rows.Next() {
		var entry FileIndexEntry
		var modifiedAt string
		var updatedAt string
		if err := rows.Scan(
			&entry.UserID,
			&entry.Path,
			&entry.ParentPath,
			&entry.Name,
			&entry.Type,
			&entry.Size,
			&modifiedAt,
			&entry.MimeType,
			&entry.PreviewKind,
			&entry.LastSeenScan,
			&updatedAt,
			&entry.Snippet,
		); err != nil {
			return nil, err
		}
		var err error
		entry.ModifiedAt, err = scanTime(modifiedAt)
		if err != nil {
			return nil, err
		}
		entry.UpdatedAt, err = scanTime(updatedAt)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

func scanFileIndexRowsWithSnippet(rows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}) ([]FileIndexEntry, error) {
	var entries []FileIndexEntry
	for rows.Next() {
		var entry FileIndexEntry
		var modifiedAt string
		var updatedAt string
		if err := rows.Scan(
			&entry.UserID,
			&entry.Path,
			&entry.ParentPath,
			&entry.Name,
			&entry.Type,
			&entry.Size,
			&modifiedAt,
			&entry.MimeType,
			&entry.PreviewKind,
			&entry.LastSeenScan,
			&updatedAt,
			&entry.Snippet,
		); err != nil {
			return nil, err
		}
		var err error
		entry.ModifiedAt, err = scanTime(modifiedAt)
		if err != nil {
			return nil, err
		}
		entry.UpdatedAt, err = scanTime(updatedAt)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

func (s *Store) IndexStats(ctx context.Context) (IndexStats, error) {
	var stats IndexStats
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*), COALESCE(SUM(size), 0)
		FROM file_index
		WHERE type = 'file'
	`).Scan(&stats.IndexedFiles, &stats.IndexedBytes); err != nil {
		return stats, err
	}
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM file_index
		WHERE type = 'dir'
	`).Scan(&stats.IndexedDirectories); err != nil {
		return stats, err
	}
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM file_index
		WHERE preview_kind IN ('image', 'raw', 'video', 'pdf', 'office')
	`).Scan(&stats.PreviewCandidates); err != nil {
		return stats, err
	}
	return stats, nil
}

func escapeLikePattern(value string) string {
	var builder strings.Builder
	builder.Grow(len(value))
	for _, r := range value {
		if r == '\\' || r == '%' || r == '_' {
			builder.WriteRune('\\')
		}
		builder.WriteRune(r)
	}
	return builder.String()
}

func (s *Store) ListPreviewCandidates(ctx context.Context) ([]PreviewCandidate, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT fi.user_id, users.username, users.home_root, fi.path, fi.size, fi.modified_at, fi.preview_kind
		FROM file_index fi
		JOIN users ON users.id = fi.user_id
		WHERE users.disabled = 0
			AND fi.type = 'file'
			AND fi.preview_kind IN ('image', 'raw', 'video', 'pdf', 'office')
		ORDER BY fi.user_id, fi.path
	`)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

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
