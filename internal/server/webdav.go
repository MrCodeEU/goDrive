package server

import (
	"net/http"
	"sync"

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

func (s *Server) serveWebDAV(w http.ResponseWriter, r *http.Request, user store.User, session store.Session) {
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
