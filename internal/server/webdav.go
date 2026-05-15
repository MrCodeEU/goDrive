package server

import (
	"net/http"
	"sync"
	"time"

	"godrive/internal/auth"
	"godrive/internal/store"
	"golang.org/x/net/webdav"
)

type webdavLocks struct {
	mu    sync.Mutex
	locks map[int64]webdav.LockSystem
}

func (wl *webdavLocks) get(userID int64) webdav.LockSystem {
	wl.mu.Lock()
	defer wl.mu.Unlock()
	if ls, ok := wl.locks[userID]; ok {
		return ls
	}
	ls := webdav.NewMemLS()
	wl.locks[userID] = ls
	return ls
}

var davLocks = &webdavLocks{locks: make(map[int64]webdav.LockSystem)}

// serveWebDAVHTTP handles all /dav/ requests.
// Tries HTTP Basic Auth first (Finder, iOS Files, rclone, etc.),
// then falls back to bearer/cookie auth via the standard middleware.
func (s *Server) serveWebDAVHTTP(w http.ResponseWriter, r *http.Request) {
	if username, password, ok := r.BasicAuth(); ok {
		user, err := s.store.GetUserByUsername(r.Context(), username)
		if err != nil || user.Disabled {
			w.Header().Set("WWW-Authenticate", `Basic realm="goDrive"`)
			writeError(w, http.StatusUnauthorized, "invalid credentials")
			return
		}
		if err := auth.VerifyPassword(password, user.PasswordHash); err != nil {
			w.Header().Set("WWW-Authenticate", `Basic realm="goDrive"`)
			writeError(w, http.StatusUnauthorized, "invalid credentials")
			return
		}
		s.serveWebDAV(w, r, user, store.Session{})
		return
	}

	// No Basic Auth — use bearer/cookie auth. Challenge with Basic if rejected.
	user, session, _, ok := s.authenticate(w, r)
	if !ok {
		// authenticate already wrote the 401; also add WWW-Authenticate so
		// WebDAV clients know to prompt for credentials.
		w.Header().Set("WWW-Authenticate", `Basic realm="goDrive"`)
		return
	}
	s.serveWebDAV(w, r, user, session)
}

func (s *Server) serveWebDAV(w http.ResponseWriter, r *http.Request, user store.User, _ store.Session) {
	handler := &webdav.Handler{
		Prefix:     "/dav",
		FileSystem: webdav.Dir(user.HomeRoot),
		LockSystem: davLocks.get(user.ID),
		Logger: func(r *http.Request, err error) {
			if err != nil {
				s.log.Warn("webdav", "method", r.Method, "path", r.URL.Path, "err", err)
			}
		},
	}
	handler.ServeHTTP(w, r)
}

// writeFileContent handles PATCH /api/files/content — saves text content to an existing file.
// Used by the web code editor. Capped at 10 MB to prevent accidental overwrites of huge files.
func (s *Server) writeFileContent(w http.ResponseWriter, r *http.Request, user store.User, session store.Session) {
	const maxWrite = 10 * 1024 * 1024
	logical := r.URL.Query().Get("path")
	if logical == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	resolved, info, err := s.files.ResolveForRead(user, logical)
	if err != nil {
		writeError(w, statusForError(err), err.Error())
		return
	}
	if info.Size() > maxWrite {
		writeError(w, http.StatusRequestEntityTooLarge, "file too large to edit via API (max 10 MB)")
		return
	}
	if err := s.files.WriteContent(user, resolved.Physical, r.Body, maxWrite); err != nil {
		writeError(w, statusForError(err), err.Error())
		return
	}
	s.refreshIndexPath(r.Context(), user, logical)
	s.fireEvent(user, "file.modified", map[string]any{"path": logical})
	writeJSON(w, http.StatusOK, map[string]any{
		"path":        logical,
		"modified_at": time.Now().UTC(),
	})
}
