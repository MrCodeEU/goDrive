package server

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"godrive/internal/store"
)

const maxTextPreviewBytes = 512 * 1024

func (s *Server) listFiles(w http.ResponseWriter, r *http.Request, user store.User, session store.Session) {
	logical := r.URL.Query().Get("path")
	entries, err := s.files.List(r.Context(), user, logical)
	if err != nil {
		writeError(w, statusForError(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"path": logicalPathOrRoot(logical), "entries": entries})
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
	defer file.Close()

	w.Header().Set("Content-Disposition", "attachment; filename="+quoteHeaderValue(filepath.Base(resolved.Physical)))
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
	defer file.Close()

	limit := int64(maxTextPreviewBytes)
	if info.Size() < limit {
		limit = info.Size()
	}
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
	writeJSON(w, http.StatusOK, map[string]any{"entry": entry})
}

func (s *Server) deleteFile(w http.ResponseWriter, r *http.Request, user store.User, session store.Session) {
	item, err := s.files.DeleteToTrash(r.Context(), user, r.URL.Query().Get("path"))
	if err != nil {
		writeError(w, statusForError(err), err.Error())
		return
	}
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
	writeJSON(w, http.StatusOK, map[string]any{"entry": entry})
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
	escaped := ""
	for _, r := range value {
		if r == '\\' || r == '"' {
			escaped += "\\"
		}
		escaped += string(r)
	}
	return `"` + escaped + `"`
}
