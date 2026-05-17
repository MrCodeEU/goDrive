# Bleve Search Engine Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace SQLite FTS5 phrase search with a Bleve-backed search engine behind a clean `Engine` interface, enabling multi-token AND search, stemming, and highlighted snippets — while keeping SQLite FTS as a fallback.

**Architecture:** A new `internal/search` package defines an `Engine` interface and a `BleveEngine` implementation. `Store` gains an optional `engine` field; all index-mutation methods call the engine after committing to SQLite. `SearchFileIndex` routes through the engine when set. Config controls which backend is active.

**Tech Stack:** Go 1.26, `github.com/blevesearch/bleve/v2`, `github.com/blevesearch/bleve/v2/analysis/lang/en`, existing `modernc.org/sqlite`

---

## File Map

| File | Action | Responsibility |
|---|---|---|
| `internal/search/engine.go` | Create | Engine interface + Result type |
| `internal/search/bleve.go` | Create | BleveEngine implementation |
| `internal/search/bleve_test.go` | Create | Unit tests for BleveEngine |
| `internal/store/store.go` | Modify | Add `engine` field + `SetSearchEngine`, add `Name` to `DocumentTextEntry` |
| `internal/store/index.go` | Modify | Wire engine calls into upsert/delete/search methods |
| `internal/server/admin_jobs.go` | Modify | Populate `Name` in `DocumentTextEntry` construction |
| `internal/config/config.go` | Modify | Add `SearchEngine`, `SearchDir` fields |
| `cmd/godrive/main.go` | Modify | Open BleveEngine, wire to store |

---

## Task 1: Add Bleve dependency

**Files:**
- Modify: `go.mod`, `go.sum`

- [ ] **Step 1: Add the dependency**

```bash
go get github.com/blevesearch/bleve/v2@latest
go get github.com/blevesearch/bleve/v2/analysis/lang/en
```

- [ ] **Step 2: Verify it resolves**

```bash
go mod tidy && go build ./...
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "chore: add bleve/v2 dependency"
```

---

## Task 2: Engine interface

**Files:**
- Create: `internal/search/engine.go`

- [ ] **Step 1: Create the file**

```go
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
```

- [ ] **Step 2: Verify it compiles**

```bash
go build ./internal/search/...
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/search/engine.go
git commit -m "feat(search): add Engine interface"
```

---

## Task 3: Write BleveEngine tests (TDD — write first, fail first)

**Files:**
- Create: `internal/search/bleve_test.go`

- [ ] **Step 1: Create the test file**

```go
package search_test

import (
	"context"
	"strings"
	"testing"

	"godrive/internal/search"
)

func newTestEngine(t *testing.T) *search.BleveEngine {
	t.Helper()
	e, err := search.OpenBleve(t.TempDir() + "/idx")
	if err != nil {
		t.Fatalf("OpenBleve: %v", err)
	}
	t.Cleanup(func() { _ = e.Close() })
	return e
}

func TestBleveEngine_IndexAndSearchByName(t *testing.T) {
	e := newTestEngine(t)
	ctx := context.Background()

	if err := e.IndexFile(ctx, 1, "/docs/report.txt", "report.txt", ""); err != nil {
		t.Fatal(err)
	}
	results, err := e.Search(ctx, 1, "report", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Path != "/docs/report.txt" {
		t.Fatalf("expected 1 result /docs/report.txt, got %v", results)
	}
}

func TestBleveEngine_MultiTokenAND(t *testing.T) {
	e := newTestEngine(t)
	ctx := context.Background()

	// "help plan" must match a file whose content contains "help the planner"
	if err := e.IndexFile(ctx, 1, "/notes/tasks.txt", "tasks.txt", "help the planner with tasks"); err != nil {
		t.Fatal(err)
	}
	// Add a decoy that matches only one token
	if err := e.IndexFile(ctx, 1, "/notes/help.txt", "help.txt", "some help documentation"); err != nil {
		t.Fatal(err)
	}

	results, err := e.Search(ctx, 1, "help plan", 10)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, r := range results {
		if r.Path == "/notes/tasks.txt" {
			found = true
		}
		if r.Path == "/notes/help.txt" {
			t.Errorf("/notes/help.txt should not match 'help plan' (only has 'help')")
		}
	}
	if !found {
		t.Errorf("expected /notes/tasks.txt in results for 'help plan', got %v", results)
	}
}

func TestBleveEngine_Stemming(t *testing.T) {
	e := newTestEngine(t)
	ctx := context.Background()

	if err := e.IndexFile(ctx, 1, "/q/planning.txt", "planning.txt", "quarterly planning document"); err != nil {
		t.Fatal(err)
	}
	results, err := e.Search(ctx, 1, "plan", 10)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, r := range results {
		if r.Path == "/q/planning.txt" {
			found = true
		}
	}
	if !found {
		t.Errorf("stemming: 'plan' should match content 'planning', got %v", results)
	}
}

func TestBleveEngine_UserIsolation(t *testing.T) {
	e := newTestEngine(t)
	ctx := context.Background()

	if err := e.IndexFile(ctx, 1, "/secret.txt", "secret.txt", "classified"); err != nil {
		t.Fatal(err)
	}
	results, err := e.Search(ctx, 2, "classified", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Errorf("user 2 must not see user 1 results, got %v", results)
	}
}

func TestBleveEngine_Delete(t *testing.T) {
	e := newTestEngine(t)
	ctx := context.Background()

	if err := e.IndexFile(ctx, 1, "/docs/old.txt", "old.txt", ""); err != nil {
		t.Fatal(err)
	}
	if err := e.Delete(ctx, 1, "/docs/old.txt"); err != nil {
		t.Fatal(err)
	}
	results, err := e.Search(ctx, 1, "old", 10)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range results {
		if r.Path == "/docs/old.txt" {
			t.Error("deleted path must not appear in search results")
		}
	}
}

func TestBleveEngine_DeletePrefix(t *testing.T) {
	e := newTestEngine(t)
	ctx := context.Background()

	for _, p := range []struct{ path, name, content string }{
		{"/tree/a.txt", "a.txt", "alpha content"},
		{"/tree/sub/b.txt", "b.txt", "beta content"},
		{"/other/c.txt", "c.txt", "alpha content"},
	} {
		if err := e.IndexFile(ctx, 1, p.path, p.name, p.content); err != nil {
			t.Fatal(err)
		}
	}

	if err := e.DeletePrefix(ctx, 1, "/tree"); err != nil {
		t.Fatal(err)
	}

	results, err := e.Search(ctx, 1, "alpha", 10)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range results {
		if r.Path == "/tree/a.txt" || r.Path == "/tree/sub/b.txt" {
			t.Errorf("path under deleted prefix must not appear: %v", r.Path)
		}
	}
	// /other/c.txt must still be findable
	var found bool
	for _, r := range results {
		if r.Path == "/other/c.txt" {
			found = true
		}
	}
	if !found {
		t.Error("/other/c.txt must still appear after DeletePrefix(/tree)")
	}
}

func TestBleveEngine_SnippetHasMarkTags(t *testing.T) {
	e := newTestEngine(t)
	ctx := context.Background()

	if err := e.IndexFile(ctx, 1, "/guide.txt", "guide.txt", "this document explains the planning process in detail"); err != nil {
		t.Fatal(err)
	}
	results, err := e.Search(ctx, 1, "planning", 10)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, r := range results {
		if r.Path == "/guide.txt" && strings.Contains(r.Snippet, "<mark>") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected snippet with <mark> tags for /guide.txt; results: %v", results)
	}
}

func TestBleveEngine_IsEmpty(t *testing.T) {
	e := newTestEngine(t)
	ctx := context.Background()

	empty, err := e.IsEmpty()
	if err != nil || !empty {
		t.Fatalf("new engine should be empty, got empty=%v err=%v", empty, err)
	}
	if err := e.IndexFile(ctx, 1, "/x.txt", "x.txt", ""); err != nil {
		t.Fatal(err)
	}
	empty, err = e.IsEmpty()
	if err != nil || empty {
		t.Fatalf("after indexing should not be empty, got empty=%v err=%v", empty, err)
	}
}
```

- [ ] **Step 2: Run tests — expect compile failure (BleveEngine not defined)**

```bash
go test ./internal/search/... 2>&1 | head -20
```

Expected: `undefined: search.BleveEngine` or `undefined: search.OpenBleve`.

---

## Task 4: Implement BleveEngine

**Files:**
- Create: `internal/search/bleve.go`

- [ ] **Step 1: Create the implementation**

```go
package search

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/analysis/lang/en"
	"github.com/blevesearch/bleve/v2/mapping"
	bsearch "github.com/blevesearch/bleve/v2/search"
	"github.com/blevesearch/bleve/v2/search/query"
)

// BleveEngine implements Engine using an on-disk Bleve index.
type BleveEngine struct {
	index bleve.Index
}

// bleveDoc is the struct Bleve indexes. Field names must match the mapping.
type bleveDoc struct {
	UserID  string
	Path    string
	Name    string
	Content string
}

func docID(userID int64, path string) string {
	return fmt.Sprintf("%d\x00%s", userID, path)
}

func buildMapping() *mapping.IndexMappingImpl {
	im := bleve.NewIndexMapping()
	dm := bleve.NewDocumentMapping()

	keyword := bleve.NewTextFieldMapping()
	keyword.Analyzer = "keyword"

	keywordStored := bleve.NewTextFieldMapping()
	keywordStored.Analyzer = "keyword"
	keywordStored.Store = true

	textEN := bleve.NewTextFieldMapping()
	textEN.Analyzer = en.AnalyzerName

	textENStored := bleve.NewTextFieldMapping()
	textENStored.Analyzer = en.AnalyzerName
	textENStored.Store = true

	dm.AddFieldMappingsAt("UserID", keyword)
	dm.AddFieldMappingsAt("Path", keywordStored)
	dm.AddFieldMappingsAt("Name", textEN)
	dm.AddFieldMappingsAt("Content", textENStored)

	im.DefaultMapping = dm
	return im
}

// OpenBleve opens an existing Bleve index at dir, or creates one if absent.
func OpenBleve(dir string) (*BleveEngine, error) {
	index, err := bleve.Open(dir)
	if err == bleve.ErrorIndexPathDoesNotExist {
		index, err = bleve.New(dir, buildMapping())
	}
	if err != nil {
		return nil, err
	}
	return &BleveEngine{index: index}, nil
}

func (e *BleveEngine) IsEmpty() (bool, error) {
	n, err := e.index.DocCount()
	return n == 0, err
}

func (e *BleveEngine) IndexFile(ctx context.Context, userID int64, path, name, content string) error {
	return e.index.Index(docID(userID, path), bleveDoc{
		UserID:  strconv.FormatInt(userID, 10),
		Path:    path,
		Name:    name,
		Content: content,
	})
}

func (e *BleveEngine) Delete(_ context.Context, userID int64, path string) error {
	return e.index.Delete(docID(userID, path))
}

func (e *BleveEngine) DeletePrefix(ctx context.Context, userID int64, pathPrefix string) error {
	userQ := bleve.NewTermQuery(strconv.FormatInt(userID, 10))
	userQ.SetField("UserID")

	exactQ := bleve.NewTermQuery(pathPrefix)
	exactQ.SetField("Path")
	childQ := bleve.NewPrefixQuery(pathPrefix + "/")
	childQ.SetField("Path")
	pathQ := bleve.NewDisjunctionQuery(exactQ, childQ)

	req := bleve.NewSearchRequest(bleve.NewConjunctionQuery(userQ, pathQ))
	req.Size = 10000
	req.Fields = []string{}

	result, err := e.index.SearchInContext(ctx, req)
	if err != nil {
		return err
	}
	if len(result.Hits) == 0 {
		return nil
	}
	batch := e.index.NewBatch()
	for _, hit := range result.Hits {
		batch.Delete(hit.ID)
	}
	return e.index.Batch(batch)
}

func (e *BleveEngine) DeleteAllForUser(ctx context.Context, userID int64) error {
	userQ := bleve.NewTermQuery(strconv.FormatInt(userID, 10))
	userQ.SetField("UserID")

	req := bleve.NewSearchRequest(userQ)
	req.Size = 10000
	req.Fields = []string{}

	result, err := e.index.SearchInContext(ctx, req)
	if err != nil {
		return err
	}
	if len(result.Hits) == 0 {
		return nil
	}
	batch := e.index.NewBatch()
	for _, hit := range result.Hits {
		batch.Delete(hit.ID)
	}
	return e.index.Batch(batch)
}

func (e *BleveEngine) Search(ctx context.Context, userID int64, query_ string, limit int) ([]Result, error) {
	tokens := strings.Fields(query_)
	if len(tokens) == 0 {
		return nil, nil
	}

	userQ := bleve.NewTermQuery(strconv.FormatInt(userID, 10))
	userQ.SetField("UserID")

	// Each token must match in name OR content (AND across tokens).
	tokenQueries := make([]query.Query, len(tokens))
	for i, tok := range tokens {
		nameQ := bleve.NewMatchQuery(tok)
		nameQ.SetField("Name")
		nameQ.SetBoost(3.0)
		contentQ := bleve.NewMatchQuery(tok)
		contentQ.SetField("Content")
		tokenQueries[i] = bleve.NewDisjunctionQuery(nameQ, contentQ)
	}

	combined := bleve.NewConjunctionQuery(append([]query.Query{userQ}, tokenQueries...)...)

	req := bleve.NewSearchRequest(combined)
	req.Size = limit
	req.Fields = []string{"Path"}
	style := "html"
	req.Highlight = &bsearch.HighlightRequest{
		Style:  &style,
		Fields: []string{"Content", "Name"},
	}

	result, err := e.index.SearchInContext(ctx, req)
	if err != nil {
		return nil, err
	}

	out := make([]Result, 0, len(result.Hits))
	for _, hit := range result.Hits {
		path, _ := hit.Fields["Path"].(string)
		snippet := extractSnippet(hit.Fragments)
		out = append(out, Result{
			UserID:  userID,
			Path:    path,
			Snippet: snippet,
		})
	}
	return out, nil
}

func extractSnippet(fragments map[string][]string) string {
	for _, field := range []string{"Content", "Name"} {
		if frags, ok := fragments[field]; ok && len(frags) > 0 {
			s := frags[0]
			// Bleve HTML style uses <em>; frontend expects <mark>.
			s = strings.ReplaceAll(s, "<em>", "<mark>")
			s = strings.ReplaceAll(s, "</em>", "</mark>")
			return s
		}
	}
	return ""
}

func (e *BleveEngine) Close() error {
	return e.index.Close()
}
```

- [ ] **Step 2: Run tests**

```bash
go test ./internal/search/... -v 2>&1
```

Expected: all tests pass. If `bleve.ErrorIndexPathDoesNotExist` is not the right sentinel, check the bleve v2 docs:
```bash
grep -r "ErrorIndex" $(go env GOPATH)/pkg/mod/github.com/blevesearch/bleve/v2*/
```

- [ ] **Step 3: Commit**

```bash
git add internal/search/
git commit -m "feat(search): add BleveEngine implementation with tests"
```

---

## Task 5: Add `Name` to `DocumentTextEntry` and fix caller

**Files:**
- Modify: `internal/store/store.go` lines around `DocumentTextEntry`
- Modify: `internal/server/admin_jobs.go:576-580`

- [ ] **Step 1: Add `Name` field to `DocumentTextEntry` in `store.go`**

Find the `DocumentTextEntry` struct (currently lines ~73-77 in `internal/store/store.go`):

```go
// BEFORE:
type DocumentTextEntry struct {
	UserID  int64
	Path    string
	Content string
}

// AFTER:
type DocumentTextEntry struct {
	UserID  int64
	Path    string
	Name    string
	Content string
}
```

- [ ] **Step 2: Populate `Name` in `admin_jobs.go`**

Find the `DocumentTextEntry` literal at line ~576 in `internal/server/admin_jobs.go`. It currently reads:

```go
return indexEntry, &store.DocumentTextEntry{
    UserID:  user.ID,
    Path:    logical,
    Content: content,
}
```

Change to:

```go
return indexEntry, &store.DocumentTextEntry{
    UserID:  user.ID,
    Path:    logical,
    Name:    indexEntry.Name,
    Content: content,
}
```

- [ ] **Step 3: Build to catch any other callers**

```bash
go build ./... 2>&1
```

Expected: no errors. If other `DocumentTextEntry{}` literals appear, add `Name: ""` to them (the field is optional for the SQLite FTS path which ignores it).

- [ ] **Step 4: Run tests**

```bash
go test ./... 2>&1 | tail -20
```

Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add internal/store/store.go internal/server/admin_jobs.go
git commit -m "feat(store): add Name field to DocumentTextEntry"
```

---

## Task 6: Add `engine` field to `Store` + wire into upsert methods

**Files:**
- Modify: `internal/store/store.go`
- Modify: `internal/store/index.go`

- [ ] **Step 1: Add `engine` field and `SetSearchEngine` to `store.go`**

Add the import and field. Find the `Store` struct and `Open` func:

```go
// Add import at top of store.go:
import (
    "database/sql"
    "errors"
    "time"

    "godrive/internal/search"
    _ "modernc.org/sqlite"
)

// Update Store struct:
type Store struct {
    db     *sql.DB
    engine search.Engine
}

// Add method after Close():
func (s *Store) SetSearchEngine(e search.Engine) {
    s.engine = e
}
```

- [ ] **Step 2: Wire engine into `UpsertFileIndexEntry` in `index.go`**

`UpsertFileIndexEntry` currently has its own TX. Add the engine call after `tx.Commit()`:

```go
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
```

- [ ] **Step 3: Wire engine into `UpsertFileIndexEntries` in `index.go`**

Add engine calls after `tx.Commit()`:

```go
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
```

- [ ] **Step 4: Wire engine into `UpsertDocumentTextEntries` in `index.go`**

Add engine calls after `tx.Commit()`:

```go
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
```

- [ ] **Step 5: Build and test**

```bash
go build ./... && go test ./internal/store/... -v 2>&1 | tail -30
```

Expected: all pass.

- [ ] **Step 6: Commit**

```bash
git add internal/store/store.go internal/store/index.go
git commit -m "feat(store): wire Engine into upsert methods"
```

---

## Task 7: Wire engine into delete methods

**Files:**
- Modify: `internal/store/index.go`

- [ ] **Step 1: Wire engine into `DeleteFileIndexPath`**

Add engine call before the SQL transaction (best-effort, order doesn't affect correctness since stale Bleve entries are filtered at search time via SQL JOIN):

```go
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
```

- [ ] **Step 2: Wire engine into `DeleteFileIndexEntriesNotSeen`**

Before the SQL TX, query the paths about to be deleted, then remove from engine:

```go
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
```

- [ ] **Step 3: Wire engine into `DeleteFileIndexEntriesNotSeenUnder`**

Same pre-query pattern, scoped to `logical` path:

```go
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
```

- [ ] **Step 4: Build and test**

```bash
go build ./... && go test ./internal/store/... -v 2>&1 | tail -20
```

Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add internal/store/index.go
git commit -m "feat(store): wire Engine into delete methods"
```

---

## Task 8: Route `SearchFileIndex` through engine

**Files:**
- Modify: `internal/store/index.go`

- [ ] **Step 1: Add `sort` import to `index.go`**

The current imports in `index.go`:
```go
import (
    "context"
    "database/sql"
    "path"
    "strings"
    "unicode"
)
```

Add `"sort"`:
```go
import (
    "context"
    "database/sql"
    "path"
    "sort"
    "strings"
    "unicode"
)
```

- [ ] **Step 2: Add `searchViaEngine` method**

Add this new method to `index.go` (place it before `searchFileIndexFTS`):

```go
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
```

- [ ] **Step 3: Route `SearchFileIndex` through engine when set**

Find `SearchFileIndex` in `index.go` (around line 435). Add the engine branch at the top:

```go
func (s *Store) SearchFileIndex(ctx context.Context, userID int64, query string, limit int) ([]FileIndexEntry, error) {
    if s.engine != nil {
        return s.searchViaEngine(ctx, userID, query, limit)
    }

    // existing SQLite FTS path unchanged below this line
    isFTS := isFileIndexFTSQuery(query)
    if isFTS {
        // ... rest of existing code
```

- [ ] **Step 4: Build and test**

```bash
go build ./... && go test ./... 2>&1 | tail -30
```

Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add internal/store/index.go
git commit -m "feat(store): route SearchFileIndex through Engine when set"
```

---

## Task 9: Config fields + main.go wiring

**Files:**
- Modify: `internal/config/config.go`
- Modify: `cmd/godrive/main.go`

- [ ] **Step 1: Add `SearchEngine` and `SearchDir` to `config.go`**

In the `Config` struct (around line 17, after `AppDataDir`):
```go
AppDataDir             string
SearchEngine           string // "bleve" or "sqlite"
SearchDir              string // path to bleve index files
```

In `Load()` (after the `PreviewDir` line, around line 82):
```go
PreviewDir:             env("GODRIVE_PREVIEW_DIR", filepath.Join(appData, "previews")),
SearchEngine:           env("GODRIVE_SEARCH_ENGINE", "bleve"),
SearchDir:              env("GODRIVE_SEARCH_DIR", filepath.Join(appData, "search")),
```

- [ ] **Step 2: Wire Bleve into `main.go`**

In `cmd/godrive/main.go`, add the import for `godrive/internal/search` and insert the engine setup block after `st.Migrate(ctx)` and before `bootstrapAdmin`:

```go
// Add to imports:
"godrive/internal/search"

// Insert after st.Migrate(ctx):
if cfg.SearchEngine == "bleve" {
    bleveEngine, bleveErr := search.OpenBleve(cfg.SearchDir)
    if bleveErr != nil {
        log.Warn("bleve search engine failed to open, falling back to SQLite FTS", "err", bleveErr)
    } else {
        defer func() {
            if err := bleveEngine.Close(); err != nil {
                log.Warn("failed to close search engine", "err", err)
            }
        }()
        st.SetSearchEngine(bleveEngine)
        if empty, err := bleveEngine.IsEmpty(); err == nil && empty {
            log.Info("search index is empty — run a full reindex via admin UI to populate Bleve")
        }
    }
}
```

This block must appear **after** `st.Migrate(ctx)` and **before** `fileService := files.NewService(...)`.

- [ ] **Step 3: Build**

```bash
go build ./...
```

Expected: no errors.

- [ ] **Step 4: Run full test suite**

```bash
go test ./... 2>&1
```

Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go cmd/godrive/main.go
git commit -m "feat: wire Bleve search engine via config (GODRIVE_SEARCH_ENGINE)"
```

---

## Task 10: Smoke test end-to-end

- [ ] **Step 1: Start the server**

```bash
go run ./cmd/godrive
```

Expected: log line `"search index is empty — run a full reindex via admin UI to populate Bleve"`.

- [ ] **Step 2: Log in and trigger a full reindex**

Open the admin UI → Jobs → run "Full reindex". Watch server logs for indexing activity.

- [ ] **Step 3: Search**

Search for a multi-word query like `"help plan"` or a filename stem. Verify results appear with highlighted snippets.

- [ ] **Step 4: Verify fallback**

Set `GODRIVE_SEARCH_ENGINE=sqlite` and restart. Verify search still works (SQLite FTS path).

- [ ] **Step 5: Final commit**

```bash
git add -A
git commit -m "feat: Bleve search engine with Engine abstraction layer

Multi-token AND search, english stemming, highlighted snippets.
SQLite FTS remains as fallback via GODRIVE_SEARCH_ENGINE=sqlite.
Meilisearch can be added by implementing internal/search.Engine."
```
