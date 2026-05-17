package search

import "context"

// Result is one hit returned by Engine.Search.
type Result struct {
	UserID  int64
	Path    string
	Snippet string // <mark>…</mark> highlighted excerpt; "" when none
}

// Engine is the search backend interface. BleveEngine implements it.
// A Meilisearch implementation would satisfy this same interface.
type Engine interface {
	// IndexFile indexes or replaces the document for path.
	// content="" is valid for dirs and binary files (name-only indexing).
	IndexFile(ctx context.Context, userID int64, path, name, content string) error

	// Delete removes a single path from the index.
	Delete(ctx context.Context, userID int64, path string) error

	// DeletePrefix removes path and all children (path + "/*").
	DeletePrefix(ctx context.Context, userID int64, pathPrefix string) error

	// DeleteAllForUser removes every document belonging to userID.
	DeleteAllForUser(ctx context.Context, userID int64) error

	// Search returns ranked results for query. Snippet populated when content was indexed.
	Search(ctx context.Context, userID int64, query string, limit int) ([]Result, error)

	// IsEmpty returns true when the index contains no documents.
	IsEmpty() (bool, error)

	Close() error
}
