package server

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sync"
	"time"

	"godrive/internal/auth"
	"godrive/internal/files"
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
		limitKey := authLimitKey(authScopePassword, clientIP(r))
		if !s.loginLimit.allow(limitKey) {
			w.Header().Set("WWW-Authenticate", `Basic realm="goDrive"`)
			writeError(w, http.StatusTooManyRequests, "too many failed authentication attempts, try again later")
			return
		}
		user, err := s.store.GetUserByUsername(r.Context(), username)
		if err != nil || user.Disabled {
			s.loginLimit.record(limitKey)
			w.Header().Set("WWW-Authenticate", `Basic realm="goDrive"`)
			writeError(w, http.StatusUnauthorized, "invalid credentials")
			return
		}
		if err := auth.VerifyPassword(password, user.PasswordHash); err != nil {
			s.loginLimit.record(limitKey)
			w.Header().Set("WWW-Authenticate", `Basic realm="goDrive"`)
			writeError(w, http.StatusUnauthorized, "invalid credentials")
			return
		}
		s.loginLimit.reset(limitKey)
		s.serveWebDAV(w, r, user, store.Session{})
		return
	}

	// No Basic Auth — use bearer/cookie auth. Challenge with Basic if rejected.
	user, session, viaCookie, ok := s.authenticate(w, r)
	if !ok {
		// authenticate already wrote the 401; also add WWW-Authenticate so
		// WebDAV clients know to prompt for credentials.
		w.Header().Set("WWW-Authenticate", `Basic realm="goDrive"`)
		return
	}
	if viaCookie && isStateChanging(r.Method) && !s.validCSRF(r, session) {
		writeError(w, http.StatusForbidden, "missing or invalid csrf token")
		return
	}
	s.serveWebDAV(w, r, user, session)
}

func (s *Server) serveWebDAV(w http.ResponseWriter, r *http.Request, user store.User, _ store.Session) {
	handler := &webdav.Handler{
		Prefix:     "/dav",
		FileSystem: confinedWebDAVFS{root: user.HomeRoot},
		LockSystem: davLocks.get(user.ID),
		Logger: func(r *http.Request, err error) {
			if err != nil {
				s.log.Warn("webdav", "method", r.Method, "path", r.URL.Path, "err", err)
			}
		},
	}
	handler.ServeHTTP(w, r)
}

type confinedWebDAVFS struct {
	root string
}

func (fsys confinedWebDAVFS) Mkdir(ctx context.Context, name string, perm os.FileMode) error {
	parent, base, err := files.ResolveParent(fsys.root, name)
	if err != nil {
		return webDAVPathError(err)
	}
	return os.Mkdir(filepath.Join(parent.Physical, base), perm)
}

func (fsys confinedWebDAVFS) OpenFile(ctx context.Context, name string, flag int, perm os.FileMode) (webdav.File, error) {
	physical, err := fsys.resolveOpenPath(name, flag)
	if err != nil {
		return nil, webDAVPathError(err)
	}
	file, err := os.OpenFile(physical, flag, perm)
	if err != nil {
		return nil, err
	}
	return file, nil
}

func (fsys confinedWebDAVFS) RemoveAll(ctx context.Context, name string) error {
	resolved, err := files.ResolveExisting(fsys.root, name)
	if err != nil {
		return webDAVPathError(err)
	}
	if resolved.Logical == "/" {
		return os.ErrInvalid
	}
	return os.RemoveAll(resolved.Physical)
}

func (fsys confinedWebDAVFS) Rename(ctx context.Context, oldName, newName string) error {
	oldResolved, err := files.ResolveExisting(fsys.root, oldName)
	if err != nil {
		return webDAVPathError(err)
	}
	if oldResolved.Logical == "/" {
		return os.ErrInvalid
	}

	newPhysical, newLogical, err := fsys.resolveRenameTarget(newName)
	if err != nil {
		return webDAVPathError(err)
	}
	if newLogical == "/" {
		return os.ErrInvalid
	}
	return os.Rename(oldResolved.Physical, newPhysical)
}

func (fsys confinedWebDAVFS) Stat(ctx context.Context, name string) (os.FileInfo, error) {
	resolved, err := files.ResolveExisting(fsys.root, name)
	if err != nil {
		return nil, webDAVPathError(err)
	}
	return os.Stat(resolved.Physical)
}

func (fsys confinedWebDAVFS) resolveOpenPath(name string, flag int) (string, error) {
	if flag&os.O_CREATE == 0 {
		resolved, err := files.ResolveExisting(fsys.root, name)
		if err != nil {
			return "", err
		}
		return resolved.Physical, nil
	}

	resolved, err := files.ResolveExisting(fsys.root, name)
	if err == nil {
		return resolved.Physical, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}

	parent, base, err := files.ResolveParent(fsys.root, name)
	if err != nil {
		return "", err
	}
	return filepath.Join(parent.Physical, base), nil
}

func (fsys confinedWebDAVFS) resolveRenameTarget(name string) (physical string, logical string, err error) {
	cleanLogical, err := files.CleanLogical(name)
	if err != nil {
		return "", "", err
	}
	if cleanLogical == "/" {
		return "", "", nil
	}
	if existing, err := files.ResolveExisting(fsys.root, cleanLogical); err == nil {
		return existing.Physical, existing.Logical, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", "", err
	}

	parentLogical := path.Dir(cleanLogical)
	parent, err := files.ResolveExisting(fsys.root, parentLogical)
	if err != nil {
		return "", "", err
	}
	if err := files.ValidateBaseName(path.Base(cleanLogical)); err != nil {
		return "", "", err
	}
	return filepath.Join(parent.Physical, path.Base(cleanLogical)), cleanLogical, nil
}

func webDAVPathError(err error) error {
	if errors.Is(err, files.ErrEscapesRoot) || errors.Is(err, files.ErrInvalidPath) {
		return os.ErrNotExist
	}
	return err
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
	if err := s.files.WriteContent(user, logical, r.Body, maxWrite); err != nil {
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
