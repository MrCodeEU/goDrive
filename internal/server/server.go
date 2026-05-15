package server

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"godrive/internal/config"
	"godrive/internal/files"
	"godrive/internal/store"
	"godrive/internal/watch"
)

type Server struct {
	cfg        config.Config
	store      *store.Store
	files      *files.Service
	log        *slog.Logger
	jobs       *AdminJobs
	watcher    *watch.Watcher
	loginLimit *loginLimiter
	httpServer *http.Server
	eventsMu   sync.Mutex
	eventsSubs map[int64]map[chan WebhookEvent]struct{}
}

func New(cfg config.Config, st *store.Store, fileService *files.Service, log *slog.Logger) *Server {
	server := &Server{
		cfg:        cfg,
		store:      st,
		files:      fileService,
		log:        log,
		jobs:       NewAdminJobs(),
		loginLimit: newLoginLimiter(),
		eventsSubs: make(map[int64]map[chan WebhookEvent]struct{}),
	}
	server.httpServer = &http.Server{
		Addr:    cfg.Addr,
		Handler: server.routes(),
	}
	return server
}

func (s *Server) ListenAndServe() error {
	return s.httpServer.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

func (s *Server) StartSessionCleanup(ctx context.Context) {
	ticker := time.NewTicker(time.Hour)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := s.store.DeleteExpiredSessions(ctx, time.Now().UTC()); err != nil {
					s.log.Warn("session cleanup failed", "err", err)
				}
			}
		}
	}()
}

func (s *Server) SetWatcher(watcher *watch.Watcher) {
	s.watcher = watcher
	watcher.SetChangeHandler(func(event watch.ChangeEvent) {
		s.fireEvent(event.User, event.Event, map[string]any{
			"path": event.Path,
			"type": event.Type,
		})
	})
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /", s.index)
	mux.Handle("GET /assets/", s.assets())
	mux.HandleFunc("GET /health", s.health)

	mux.HandleFunc("POST /api/auth/login", s.login)
	mux.HandleFunc("POST /api/auth/logout", s.withUser(s.logout))
	mux.HandleFunc("GET /api/me", s.withUser(s.me))
	mux.HandleFunc("GET /api/events", s.withUser(s.events))

	mux.HandleFunc("GET /api/admin/users", s.withAdmin(s.listUsers))
	mux.HandleFunc("POST /api/admin/users", s.withAdmin(s.createUser))
	mux.HandleFunc("PATCH /api/admin/users/{id}", s.withAdmin(s.updateUser))
	mux.HandleFunc("POST /api/admin/users/{id}/password", s.withAdmin(s.setPassword))
	mux.HandleFunc("GET /api/admin/stats", s.withAdmin(s.adminStats))
	mux.HandleFunc("GET /api/admin/jobs/current", s.withAdmin(s.currentAdminJob))
	mux.HandleFunc("POST /api/admin/jobs/cancel", s.withAdmin(s.cancelAdminJob))
	mux.HandleFunc("POST /api/admin/jobs/reindex", s.withAdmin(s.startReindex))
	mux.HandleFunc("POST /api/admin/jobs/preview-warmup", s.withAdmin(s.startPreviewWarmup))
	mux.HandleFunc("DELETE /api/admin/preview-cache", s.withAdmin(s.clearPreviewCache))

	mux.HandleFunc("GET /api/webhooks", s.withAdmin(s.listWebhooks))
	mux.HandleFunc("POST /api/webhooks", s.withAdmin(s.createWebhook))
	mux.HandleFunc("DELETE /api/webhooks/{id}", s.withAdmin(s.deleteWebhook))
	mux.HandleFunc("POST /api/webhooks/{id}/test", s.withAdmin(s.testWebhook))

	mux.HandleFunc("GET /api/files/list", s.withUser(s.listFiles))
	mux.HandleFunc("GET /api/files/tree", s.withUser(s.fileTree))
	mux.HandleFunc("GET /api/files/search", s.withUser(s.searchFiles))
	mux.HandleFunc("POST /api/files/mkdir", s.withUser(s.mkdir))
	mux.HandleFunc("GET /api/files/download", s.withUser(s.download))
	mux.HandleFunc("GET /api/files/raw", s.withUser(s.rawFile))
	mux.HandleFunc("GET /api/files/text", s.withUser(s.textPreview))
	mux.HandleFunc("GET /api/files/thumbnail", s.withUser(s.thumbnail))
	mux.HandleFunc("POST /api/files/move", s.withUser(s.move))
	mux.HandleFunc("DELETE /api/files", s.withUser(s.deleteFile))
	mux.HandleFunc("POST /api/files/bulk/delete", s.withUser(s.bulkDelete))
	mux.HandleFunc("POST /api/files/bulk/move", s.withUser(s.bulkMove))
	mux.HandleFunc("POST /api/files/bulk/download", s.withUser(s.bulkDownload))

	mux.HandleFunc("GET /api/trash", s.withUser(s.listTrash))
	mux.HandleFunc("GET /api/trash/{id}/thumbnail", s.withUser(s.trashThumbnail))
	mux.HandleFunc("POST /api/trash/{id}/restore", s.withUser(s.restoreTrash))
	mux.HandleFunc("DELETE /api/trash/{id}", s.withUser(s.permanentlyDeleteTrash))

	mux.HandleFunc("OPTIONS /api/tus", s.tusOptions)
	mux.HandleFunc("OPTIONS /api/tus/{id}", s.tusOptions)
	mux.HandleFunc("POST /api/tus", s.withUser(s.tusCreate))
	mux.HandleFunc("HEAD /api/tus/{id}", s.withUser(s.tusHead))
	mux.HandleFunc("PATCH /api/tus/{id}", s.withUser(s.tusPatch))
	mux.HandleFunc("DELETE /api/tus/{id}", s.withUser(s.tusDelete))

	mux.HandleFunc("PATCH /api/files/content", s.withUser(s.writeFileContent))

	// WebDAV must not be added to mux: "/dav/" (all-methods) conflicts with
	// "GET /" (catch-all) in Go 1.22+ mux. Intercept in the outer handler instead.
	// Supports both Basic Auth (for Finder/iOS Files) and bearer/cookie auth.
	return s.logRequests(s.devLatency(securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/dav/") || r.URL.Path == "/dav" {
			s.serveWebDAVHTTP(w, r)
			return
		}
		mux.ServeHTTP(w, r)
	}))))
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func decodeJSON(r *http.Request, target any) error {
	defer func() {
		_ = r.Body.Close()
	}()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func statusForError(err error) int {
	switch {
	case errors.Is(err, store.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, fs.ErrNotExist):
		return http.StatusNotFound
	case errors.Is(err, fs.ErrExist):
		return http.StatusConflict
	case errors.Is(err, files.ErrInvalidPath), errors.Is(err, files.ErrEscapesRoot):
		return http.StatusBadRequest
	case errors.Is(err, http.ErrMissingFile):
		return http.StatusNotFound
	default:
		return http.StatusInternalServerError
	}
}
