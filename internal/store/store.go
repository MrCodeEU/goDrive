package store

import (
	"database/sql"
	"errors"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

type User struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	IsAdmin      bool      `json:"is_admin"`
	Disabled     bool      `json:"disabled"`
	HomeRoot     string    `json:"home_root"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Session struct {
	ID            int64
	UserID        int64
	TokenHash     string
	CSRFTokenHash string
	CreatedAt     time.Time
	ExpiresAt     time.Time
	RevokedAt     sql.NullTime
}

type TrashItem struct {
	ID           string    `json:"id"`
	UserID       int64     `json:"user_id"`
	OriginalPath string    `json:"original_path"`
	OriginalName string    `json:"original_name"`
	TrashPath    string    `json:"-"`
	IsDir        bool      `json:"is_dir"`
	Size         int64     `json:"size"`
	DeletedAt    time.Time `json:"deleted_at"`
}

type Upload struct {
	ID           string
	UserID       int64
	UploadLength int64
	Offset       int64
	MetadataJSON string
	TargetDir    string
	Filename     string
	TempPath     string
	FinalPath    sql.NullString
	CreatedAt    time.Time
	UpdatedAt    time.Time
	CompletedAt  sql.NullTime
}

type FileIndexEntry struct {
	UserID       int64     `json:"user_id"`
	Path         string    `json:"path"`
	ParentPath   string    `json:"parent_path,omitempty"`
	Name         string    `json:"name"`
	Type         string    `json:"type"`
	Size         int64     `json:"size"`
	ModifiedAt   time.Time `json:"modified_at"`
	MimeType     string    `json:"mime_type,omitempty"`
	PreviewKind  string    `json:"preview_kind,omitempty"`
	LastSeenScan string    `json:"last_seen_scan"`
	UpdatedAt    time.Time `json:"updated_at"`
	Snippet      string    `json:"snippet,omitempty"`
}

type DocumentTextEntry struct {
	UserID  int64
	Path    string
	Content string
}

type PreviewCandidate struct {
	UserID      int64     `json:"user_id"`
	Username    string    `json:"username"`
	HomeRoot    string    `json:"home_root"`
	Path        string    `json:"path"`
	Size        int64     `json:"size"`
	ModifiedAt  time.Time `json:"modified_at"`
	PreviewKind string    `json:"preview_kind"`
}

type IndexStats struct {
	IndexedFiles       int64 `json:"indexed_files"`
	IndexedDirectories int64 `json:"indexed_directories"`
	IndexedBytes       int64 `json:"indexed_bytes"`
	PreviewCandidates  int64 `json:"preview_candidates"`
}

var ErrNotFound = errors.New("not found")

func Open(path string) (*Store, error) {
	dsn := "file:" + path + "?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(8)

	store := &Store{db: db}
	return store, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) DB() *sql.DB {
	return s.db
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func scanTime(value string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, value)
}

func nowString() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func timeString(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}
