package server

import (
	"context"
	"net/http"
	"os"
	"strconv"

	"godrive/internal/auth"
	"godrive/internal/store"
)

func (s *Server) listUsers(w http.ResponseWriter, r *http.Request, admin store.User, session store.Session) {
	users, err := s.store.ListUsers(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list users")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": users})
}

func (s *Server) createUser(w http.ResponseWriter, r *http.Request, admin store.User, session store.Session) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		HomeRoot string `json:"home_root"`
		IsAdmin  bool   `json:"is_admin"`
		Disabled bool   `json:"disabled"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.Username == "" || req.Password == "" || req.HomeRoot == "" {
		writeError(w, http.StatusBadRequest, "username, password, and home_root are required")
		return
	}
	if err := os.MkdirAll(req.HomeRoot, 0o750); err != nil {
		writeError(w, http.StatusBadRequest, "failed to create home root: "+err.Error())
		return
	}
	passwordHash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}
	user, err := s.store.CreateUser(r.Context(), store.User{
		Username:     req.Username,
		PasswordHash: passwordHash,
		IsAdmin:      req.IsAdmin,
		Disabled:     req.Disabled,
		HomeRoot:     req.HomeRoot,
	})
	if err != nil {
		writeError(w, statusForError(err), "failed to create user")
		return
	}
	if err := s.reloadWatcherRoots(r.Context()); err != nil {
		s.log.Warn("failed to reload watcher roots after user create", "err", err)
	}
	writeJSON(w, http.StatusCreated, map[string]any{"user": user})
}

func (s *Server) updateUser(w http.ResponseWriter, r *http.Request, admin store.User, session store.Session) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}

	existing, err := s.store.GetUserByID(r.Context(), id)
	if err != nil {
		writeError(w, statusForError(err), "user not found")
		return
	}

	var req struct {
		Username *string `json:"username"`
		HomeRoot *string `json:"home_root"`
		IsAdmin  *bool   `json:"is_admin"`
		Disabled *bool   `json:"disabled"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.Username != nil {
		if *req.Username == "" {
			writeError(w, http.StatusBadRequest, "username cannot be empty")
			return
		}
		existing.Username = *req.Username
	}
	if req.HomeRoot != nil {
		if err := os.MkdirAll(*req.HomeRoot, 0o750); err != nil {
			writeError(w, http.StatusBadRequest, "failed to create home root")
			return
		}
		existing.HomeRoot = *req.HomeRoot
	}
	if req.IsAdmin != nil {
		existing.IsAdmin = *req.IsAdmin
	}
	if req.Disabled != nil {
		existing.Disabled = *req.Disabled
	}

	updated, err := s.store.UpdateUser(r.Context(), existing)
	if err != nil {
		writeError(w, statusForError(err), "failed to update user")
		return
	}
	if updated.Disabled {
		_ = s.store.RevokeUserSessions(r.Context(), updated.ID)
	}
	if err := s.reloadWatcherRoots(r.Context()); err != nil {
		s.log.Warn("failed to reload watcher roots after user update", "err", err)
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": updated})
}

func (s *Server) setPassword(w http.ResponseWriter, r *http.Request, admin store.User, session store.Session) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var req struct {
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.Password == "" {
		writeError(w, http.StatusBadRequest, "password is required")
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}
	if err := s.store.SetPassword(r.Context(), id, hash); err != nil {
		writeError(w, statusForError(err), "failed to set password")
		return
	}
	_ = s.store.RevokeUserSessions(r.Context(), id)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid id")
		return 0, false
	}
	return id, true
}

func (s *Server) reloadWatcherRoots(ctx context.Context) error {
	if s.watcher == nil {
		return nil
	}
	users, err := s.store.ListUsers(ctx)
	if err != nil {
		return err
	}
	return s.watcher.SetUserRoots(users)
}
