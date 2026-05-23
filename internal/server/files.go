package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"godrive/internal/files"
	"godrive/internal/store"
)

const (
	maxTextPreviewBytes = files.MaxIndexedTextBytes
	defaultListLimit    = 500
	maxListLimit        = 2000
)

type fileEntryResponse struct {
	Name        string    `json:"name"`
	Path        string    `json:"path"`
	Type        string    `json:"type"`
	Size        int64     `json:"size"`
	ModifiedAt  time.Time `json:"modified_at"`
	MimeType    string    `json:"mime_type,omitempty"`
	PreviewKind string    `json:"preview_kind,omitempty"`
	Snippet     string    `json:"snippet,omitempty"`
}

type listCursor struct {
	Type string `json:"type"`
	Name string `json:"name"`
	Path string `json:"path"`
}

func (s *Server) listFiles(w http.ResponseWriter, r *http.Request, user store.User, session store.Session) {
	logical := r.URL.Query().Get("path")

	limit := defaultListLimit
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = min(parsed, maxListLimit)
		}
	}
	offset := 0
	cursor := ""
	if raw := r.URL.Query().Get("offset"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed >= 0 {
			offset = parsed
		}
	}
	if raw := r.URL.Query().Get("cursor"); raw != "" {
		cursor = raw
		offset = 0
	}

	cleanLogical, err := files.CleanLogical(logicalPathOrRoot(logical))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if s.store != nil {
		served, err := s.listFilesFromIndex(w, r, user, cleanLogical, cursor, offset, limit)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to list indexed folder")
			return
		}
		if served {
			return
		}
	}

	all, err := s.files.List(r.Context(), user, cleanLogical)
	if err != nil {
		writeError(w, statusForError(err), err.Error())
		return
	}

	total := len(all)
	start := offset
	if cursor != "" {
		decoded, err := decodeListCursor(cursor)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid cursor")
			return
		}
		start = cursorStart(all, decoded)
	}
	start = min(start, total)
	end := min(start+limit, total)
	page := all[start:end]
	nextCursor := ""
	if end < total && len(page) > 0 {
		nextCursor = encodeListCursor(page[len(page)-1].Type, page[len(page)-1].Name, page[len(page)-1].Path)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"path":        cleanLogical,
		"entries":     page,
		"total":       total,
		"offset":      start,
		"limit":       limit,
		"has_more":    end < total,
		"next_cursor": nextCursor,
	})
}

func (s *Server) fileTree(w http.ResponseWriter, r *http.Request, user store.User, session store.Session) {
	seen := make(map[string]fileEntryResponse)
	if s.store != nil {
		entries, err := s.store.ListFileIndexDirectories(r.Context(), user.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to list indexed folders")
			return
		}
		for _, entry := range fileIndexEntriesToResponse(entries) {
			seen[entry.Path] = entry
		}
	}

	err := filepath.WalkDir(user.HomeRoot, func(physical string, d os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if physical == user.HomeRoot {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(user.HomeRoot, physical)
		if err != nil {
			return err
		}
		logical, err := files.CleanLogical("/" + filepath.ToSlash(rel))
		if err != nil {
			return err
		}
		seen[logical] = fileEntryResponse{
			Name:       path.Base(logical),
			Path:       logical,
			Type:       "dir",
			Size:       info.Size(),
			ModifiedAt: info.ModTime().UTC(),
		}
		return nil
	})
	if err != nil {
		writeError(w, statusForError(err), err.Error())
		return
	}
	response := make([]fileEntryResponse, 0, len(seen))
	for _, entry := range seen {
		response = append(response, entry)
	}
	sort.Slice(response, func(i, j int) bool {
		return strings.ToLower(response[i].Path) < strings.ToLower(response[j].Path)
	})
	writeJSON(w, http.StatusOK, map[string]any{"entries": response})
}

func (s *Server) listFilesFromIndex(w http.ResponseWriter, r *http.Request, user store.User, logical string, rawCursor string, offset int, limit int) (bool, error) {
	resolved, err := files.ResolveExisting(user.HomeRoot, logical)
	if err != nil {
		return false, err
	}
	info, err := os.Stat(resolved.Physical)
	if err != nil {
		return false, err
	}
	if !info.IsDir() {
		return false, files.ErrInvalidPath
	}

	var cursor listCursor
	if rawCursor != "" {
		cursor, err = decodeListCursor(rawCursor)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid cursor")
			return true, nil
		}
	}

	page, err := s.store.ListFileIndexFolder(r.Context(), user.ID, logical, cursor.Type, cursor.Name, cursor.Path, offset, limit+1)
	if err != nil {
		return false, err
	}
	if page.Total == 0 {
		if logical == "/" {
			return false, nil
		}
		indexed, err := s.store.HasFileIndexDir(r.Context(), user.ID, logical)
		if err != nil {
			return false, err
		}
		if !indexed {
			return false, nil
		}
	}

	entries := page.Entries
	hasMore := len(entries) > limit
	if hasMore {
		entries = entries[:limit]
	}
	nextCursor := ""
	if hasMore && len(entries) > 0 {
		last := entries[len(entries)-1]
		nextCursor = encodeListCursor(last.Type, last.Name, last.Path)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"path":        logical,
		"entries":     fileIndexEntriesToResponse(entries),
		"total":       page.Total,
		"offset":      offset,
		"limit":       limit,
		"has_more":    hasMore,
		"next_cursor": nextCursor,
		"source":      "index",
	})
	return true, nil
}

func fileIndexEntriesToResponse(entries []store.FileIndexEntry) []fileEntryResponse {
	response := make([]fileEntryResponse, 0, len(entries))
	for _, entry := range entries {
		response = append(response, fileEntryResponse{
			Name:        entry.Name,
			Path:        entry.Path,
			Type:        entry.Type,
			Size:        entry.Size,
			ModifiedAt:  entry.ModifiedAt,
			MimeType:    entry.MimeType,
			PreviewKind: entry.PreviewKind,
			Snippet:     entry.Snippet,
		})
	}
	return response
}

func encodeListCursor(entryType, name, logicalPath string) string {
	cursor := listCursor{Type: entryType, Name: strings.ToLower(name), Path: logicalPath}
	data, _ := json.Marshal(cursor)
	return base64.RawURLEncoding.EncodeToString(data)
}

func decodeListCursor(raw string) (listCursor, error) {
	data, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return listCursor{}, err
	}
	var cursor listCursor
	if err := json.Unmarshal(data, &cursor); err != nil {
		return listCursor{}, err
	}
	cursor.Name = strings.ToLower(cursor.Name)
	if cursor.Name == "" || cursor.Path == "" || (cursor.Type != "dir" && cursor.Type != "file") {
		return listCursor{}, errInvalidCursor
	}
	return cursor, nil
}

var errInvalidCursor = &cursorError{}

type cursorError struct{}

func (*cursorError) Error() string { return "invalid cursor" }

func cursorStart(entries []files.Entry, cursor listCursor) int {
	for i, entry := range entries {
		if compareEntryCursor(entry.Type, entry.Name, entry.Path, cursor) > 0 {
			return i
		}
	}
	return len(entries)
}

func compareEntryCursor(entryType, name, logicalPath string, cursor listCursor) int {
	entryRank := entryTypeRank(entryType)
	cursorRank := entryTypeRank(cursor.Type)
	if entryRank != cursorRank {
		if entryRank < cursorRank {
			return -1
		}
		return 1
	}
	entryName := strings.ToLower(name)
	if entryName < cursor.Name {
		return -1
	}
	if entryName > cursor.Name {
		return 1
	}
	if strings.ToLower(logicalPath) <= strings.ToLower(cursor.Path) {
		return -1
	}
	return 1
}

func entryTypeRank(entryType string) int {
	if entryType == "dir" {
		return 0
	}
	return 1
}

func (s *Server) searchFiles(w http.ResponseWriter, r *http.Request, user store.User, session store.Session) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	limit := 50
	if rawLimit := r.URL.Query().Get("limit"); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid limit")
			return
		}
		limit = parsed
	}
	entries, err := s.store.SearchFileIndex(r.Context(), user.ID, query, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to search files")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"query": query, "entries": fileIndexEntriesToResponse(entries)})
}

func (s *Server) mkdir(w http.ResponseWriter, r *http.Request, user store.User, session store.Session) {
	var req struct {
		Path string `json:"path"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	entry, err := s.files.Mkdir(r.Context(), user, req.Path)
	if err != nil {
		writeError(w, statusForError(err), err.Error())
		return
	}
	s.refreshIndexPath(r.Context(), user, entry.Path)
	s.fireEvent(user, "file.created", map[string]any{"path": entry.Path, "type": "dir"})
	writeJSON(w, http.StatusCreated, map[string]any{"entry": entry})
}

func (s *Server) download(w http.ResponseWriter, r *http.Request, user store.User, session store.Session) {
	resolved, info, err := s.files.ResolveForRead(user, r.URL.Query().Get("path"))
	if err != nil {
		writeError(w, statusForError(err), err.Error())
		return
	}
	file, err := os.Open(resolved.Physical)
	if err != nil {
		writeError(w, statusForError(err), err.Error())
		return
	}
	defer func() {
		_ = file.Close()
	}()

	w.Header().Set("Content-Disposition", "attachment; filename="+quoteHeaderValue(filepath.Base(resolved.Physical)))
	http.ServeContent(w, r, filepath.Base(resolved.Physical), info.ModTime(), file)
}

func (s *Server) rawFile(w http.ResponseWriter, r *http.Request, user store.User, session store.Session) {
	resolved, info, err := s.files.ResolveForRead(user, r.URL.Query().Get("path"))
	if err != nil {
		writeError(w, statusForError(err), err.Error())
		return
	}
	file, err := os.Open(resolved.Physical)
	if err != nil {
		writeError(w, statusForError(err), err.Error())
		return
	}
	defer func() {
		_ = file.Close()
	}()

	contentType := mime.TypeByExtension(strings.ToLower(filepath.Ext(resolved.Physical)))
	if contentType == "" {
		buffer := make([]byte, 512)
		n, readErr := file.Read(buffer)
		if readErr != nil && readErr != io.EOF {
			writeError(w, http.StatusInternalServerError, "failed to read file")
			return
		}
		contentType = http.DetectContentType(buffer[:n])
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to seek file")
			return
		}
	}

	w.Header().Set("Content-Type", contentType)
	disposition := "attachment"
	if safeInlineContentType(contentType) {
		disposition = "inline"
		w.Header().Set("Content-Security-Policy", "sandbox")
	}
	w.Header().Set("Content-Disposition", disposition+"; filename="+quoteHeaderValue(filepath.Base(resolved.Physical)))
	w.Header().Set("Cache-Control", "private, max-age=86400")
	http.ServeContent(w, r, filepath.Base(resolved.Physical), info.ModTime(), file)
}

func (s *Server) textPreview(w http.ResponseWriter, r *http.Request, user store.User, session store.Session) {
	resolved, info, err := s.files.ResolveForRead(user, r.URL.Query().Get("path"))
	if err != nil {
		writeError(w, statusForError(err), err.Error())
		return
	}
	file, err := os.Open(resolved.Physical)
	if err != nil {
		writeError(w, statusForError(err), err.Error())
		return
	}
	defer func() {
		_ = file.Close()
	}()

	limit := min(int64(maxTextPreviewBytes), info.Size())
	content, err := io.ReadAll(io.LimitReader(file, limit))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read preview")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"path":        resolved.Logical,
		"name":        filepath.Base(resolved.Physical),
		"size":        info.Size(),
		"truncated":   info.Size() > maxTextPreviewBytes,
		"max_bytes":   maxTextPreviewBytes,
		"content":     strings.ToValidUTF8(string(content), "\uFFFD"),
		"mime_type":   http.DetectContentType(content),
		"modified_at": info.ModTime().UTC(),
	})
}

func (s *Server) move(w http.ResponseWriter, r *http.Request, user store.User, session store.Session) {
	var req struct {
		From string `json:"from"`
		To   string `json:"to"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	entry, err := s.files.Move(r.Context(), user, req.From, req.To)
	if err != nil {
		writeError(w, statusForError(err), err.Error())
		return
	}
	s.deleteIndexPath(r.Context(), user, req.From)
	s.refreshIndexPath(r.Context(), user, entry.Path)
	s.fireEvent(user, "file.moved", map[string]any{"old_path": req.From, "path": entry.Path})
	writeJSON(w, http.StatusOK, map[string]any{"entry": entry})
}

func (s *Server) deleteFile(w http.ResponseWriter, r *http.Request, user store.User, session store.Session) {
	path := r.URL.Query().Get("path")
	item, err := s.files.DeleteToTrash(r.Context(), user, path)
	if err != nil {
		writeError(w, statusForError(err), err.Error())
		return
	}
	s.deleteIndexPath(r.Context(), user, path)
	s.fireEvent(user, "file.deleted", map[string]any{"path": path, "trash_id": item.ID})
	writeJSON(w, http.StatusOK, map[string]any{"trash_item": item})
}

func (s *Server) listTrash(w http.ResponseWriter, r *http.Request, user store.User, session store.Session) {
	items, err := s.store.ListTrashItems(r.Context(), user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list trash")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) restoreTrash(w http.ResponseWriter, r *http.Request, user store.User, session store.Session) {
	entry, err := s.files.RestoreTrash(r.Context(), user, r.PathValue("id"))
	if err != nil {
		writeError(w, statusForError(err), err.Error())
		return
	}
	s.refreshIndexPath(r.Context(), user, entry.Path)
	s.fireEvent(user, "file.restored", map[string]any{"path": entry.Path})
	writeJSON(w, http.StatusOK, map[string]any{"entry": entry})
}

func (s *Server) refreshIndexPath(ctx context.Context, user store.User, logical string) {
	if s.store == nil {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := s.scanUserPath(ctx, user, logical, "api-"+time.Now().UTC().Format(time.RFC3339Nano), ""); err != nil {
		s.log.Warn("failed to refresh file index path", "path", logical, "err", err)
	}
}

func (s *Server) deleteIndexPath(ctx context.Context, user store.User, logical string) {
	if s.store == nil {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if _, err := s.store.DeleteFileIndexPath(ctx, user.ID, logical); err != nil {
		s.log.Warn("failed to delete file index path", "path", logical, "err", err)
	}
}

func (s *Server) permanentlyDeleteTrash(w http.ResponseWriter, r *http.Request, user store.User, session store.Session) {
	if err := s.files.PermanentlyDeleteTrash(r.Context(), user, r.PathValue("id")); err != nil {
		writeError(w, statusForError(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func logicalPathOrRoot(logical string) string {
	if logical == "" {
		return "/"
	}
	return logical
}

func quoteHeaderValue(value string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range value {
		if r == '\\' || r == '"' {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	b.WriteByte('"')
	return b.String()
}

func safeInlineContentType(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		mediaType = contentType
	}
	mediaType = strings.ToLower(mediaType)
	if mediaType == "application/pdf" || mediaType == "text/plain" {
		return true
	}
	if strings.HasPrefix(mediaType, "image/") {
		return true
	}
	return strings.HasPrefix(mediaType, "video/") || strings.HasPrefix(mediaType, "audio/")
}
