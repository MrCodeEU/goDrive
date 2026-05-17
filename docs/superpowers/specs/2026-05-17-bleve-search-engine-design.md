# Search Engine Abstraction + Bleve Implementation

**Date:** 2026-05-17  
**Status:** Approved

## Problem

SQLite FTS5 phrase-wraps all queries (`"help plan"`), so multi-word searches never match across words ("help the planner"). No stemming, no fuzzy, no ranking. Moving to an abstracted search engine fixes this and allows future swap to Meilisearch or other backends.

## Goals

- Multi-token AND search: "help plan" matches "help the planner"
- Stemming: "planning" = "plan" = "planned"
- Highlighted snippets with `<mark>` tags (matches existing frontend)
- Clean `Engine` interface so Meilisearch is a future drop-in
- Graceful fallback to SQLite FTS if Bleve fails to open
- No new external processes — single binary

## Non-goals

- Typo/fuzzy tolerance (deferred; Meilisearch handles this better)
- Replacing SQLite for metadata storage
- Multi-language stemming (english only for now)

---

## Package Layout

```
internal/search/
├── engine.go    ← Engine interface + Result type
└── bleve.go     ← BleveEngine implementation
```

SQLite FTS code stays in `store/index.go` as fallback. Not extracted — too coupled to DB transactions.

---

## Interface (`engine.go`)

```go
package search

import "context"

type Result struct {
    UserID  int64
    Path    string
    Snippet string // <mark>…</mark> highlighted excerpt; "" if none
}

type Engine interface {
    // IndexFile indexes name + content for one file. content="" for dirs/binaries.
    // Calling with new content overwrites the previous doc for that path.
    IndexFile(ctx context.Context, userID int64, path, name, content string) error

    // Delete removes a single path from the index.
    Delete(ctx context.Context, userID int64, path string) error

    // DeletePrefix removes path and all paths under path/.
    DeletePrefix(ctx context.Context, userID int64, pathPrefix string) error

    // DeleteAllForUser removes every document belonging to userID.
    DeleteAllForUser(ctx context.Context, userID int64) error

    // Search returns ranked results. Snippet field populated when content was indexed.
    Search(ctx context.Context, userID int64, query string, limit int) ([]Result, error)

    Close() error
}
```

**Meilisearch path:** implement `Engine` in `internal/search/meili.go`, wire via config. No other changes needed.

---

## Bleve Document Schema (`bleve.go`)

```go
type bleveDoc struct {
    UserID  string // stored as string for TermQuery; not analyzed
    Path    string // stored; PrefixQuery on "path" field for DeletePrefix
    Name    string // analyzed: english stemmer + lowercase
    Content string // analyzed + stored (needed for snippet highlight)
}
```

**Document ID:** `fmt.Sprintf("%d\x00%s", userID, path)` — null-byte separator prevents path injection across users.

**Index mapping:**
- `user_id`: keyword field (no analysis)
- `path`: keyword field (no analysis, stored)
- `name`: text field, `en` analyzer (porter stemmer + lowercase)
- `content`: text field, `en` analyzer, stored=true (for highlight)

---

## Query Construction

Input `"help plan"` (2 tokens) produces:

```
user_id:42
AND (name:help OR content:help)     ← match "help", "helped", "helping"
AND (name:plan OR content:plan)     ← match "plan", "planner", "planning"
```

Name field gets boost=3.0 so filename matches rank above content matches.

Single-token queries below 3 chars skip FTS and use LIKE fallback (existing behavior preserved).

**Snippet:** `bleve/search/highlight` package with custom HTML formatter using `<mark>`/`</mark>` tags.

---

## Store Changes

### New field
```go
type Store struct {
    db     *sql.DB
    engine search.Engine // nil = use SQLite FTS fallback
}

func (s *Store) SetSearchEngine(e search.Engine) { s.engine = e }
```

### Mutation method changes

| Method | Additional engine call |
|---|---|
| `UpsertFileIndexEntry` | `engine.IndexFile(ctx, userID, path, name, "")` |
| `UpsertFileIndexEntries` | `engine.IndexFile(…)` for each entry |
| `UpsertDocumentTextEntries` | `engine.IndexFile(ctx, userID, path, name, content)` |
| `DeleteFileIndexPath` | `engine.DeletePrefix(ctx, userID, path)` |
| `DeleteFileIndexEntriesNotSeen` | query paths first → `engine.Delete` each |
| `DeleteFileIndexEntriesNotSeenUnder` | same pre-query pattern |
| *(no existing bulk-delete-user method)* | `engine.DeleteAllForUser` reserved for future user-deletion flow |

Engine calls are **best-effort**: log error, don't fail the SQL operation. SQLite remains authoritative for metadata; Bleve is rebuilable.

### `SearchFileIndex` routing
```go
if s.engine != nil {
    return s.searchViaEngine(ctx, userID, query, limit)
}
// existing SQLite FTS path unchanged
```

### `DocumentTextEntry` — add `Name` field
```go
type DocumentTextEntry struct {
    UserID  int64
    Path    string
    Name    string // NEW — required for engine.IndexFile
    Content string
}
```

Callers in `admin_jobs.go` already hold `FileIndexEntry` (with `Name`) when building the text entry — trivial to populate.

---

## `DeleteFileIndexEntriesNotSeen` — Path Pre-Query

Before the SQL DELETE, collect paths that will be removed:

```go
if s.engine != nil {
    rows, _ := s.db.QueryContext(ctx,
        `SELECT path FROM file_index WHERE user_id = ? AND last_seen_scan <> ?`,
        userID, scanID)
    // collect paths → engine.Delete each
}
// existing transaction DELETE unchanged
```

Same pattern for `DeleteFileIndexEntriesNotSeenUnder` (adds the path scope to the SELECT).

---

## Config + Wiring

### New config fields
```
GODRIVE_SEARCH_ENGINE   "bleve" | "sqlite"   default: "bleve"
GODRIVE_SEARCH_DIR      path                  default: AppDataDir/search
```

Add `SearchEngine string` and `SearchDir string` to `config.Config`.

### `main.go` startup sequence
```
store.Open
st.Migrate
if cfg.SearchEngine == "bleve":
    engine, err = search.OpenBleve(cfg.SearchDir)
    if err: log warning, continue (SQLite FTS fallback)
    else:
        st.SetSearchEngine(engine)
        defer engine.Close()
        if engine.IsEmpty(): trigger background reindex for all users
```

### Background reindex on empty index
`search.BleveEngine.IsEmpty() bool` — returns true if doc count == 0. If true at startup, kick off a goroutine calling the existing `reindexAllUsers` logic so existing data becomes searchable without manual admin action.

---

## Error Handling

- Bleve open failure → warn + fall back to SQLite FTS; no crash
- Individual `IndexFile`/`Delete` errors → log at warn level; don't propagate (SQLite op already committed)
- `Search` error → return error to HTTP handler (same as current SQLite FTS behavior)

---

## Testing

- Unit test `BleveEngine` in `internal/search/bleve_test.go`: index/delete/search roundtrip, multi-token AND, stemming, snippet presence, multi-user isolation
- Existing `store` tests continue to pass (engine=nil path unchanged)
- `server` integration tests unchanged (search endpoint contract unchanged)

---

## Migration / Rollback

- Set `GODRIVE_SEARCH_ENGINE=sqlite` to revert instantly
- Bleve index directory can be deleted and rebuilt via admin reindex job
- SQLite FTS tables retained — not dropped in this change

---

## Future: Meilisearch

Implement `internal/search/meili.go` satisfying `Engine`. Wire via `GODRIVE_SEARCH_ENGINE=meilisearch` + `GODRIVE_MEILI_URL` / `GODRIVE_MEILI_KEY`. No other code changes needed.
