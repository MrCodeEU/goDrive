package server

import (
	"net/http"
	"strings"

	"godrive/internal/auth"
	"godrive/internal/store"
)

func (s *Server) listAPIKeys(w http.ResponseWriter, r *http.Request, _ store.User, _ store.Session) {
	keys, err := s.store.ListAPIKeys(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list API keys")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"api_keys": keys})
}

func (s *Server) createAPIKey(w http.ResponseWriter, r *http.Request, _ store.User, _ store.Session) {
	var req struct {
		UserID int64  `json:"user_id"`
		Name   string `json:"name"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.UserID == 0 {
		writeError(w, http.StatusBadRequest, "user_id is required")
		return
	}

	token, err := auth.RandomToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}
	plaintext := "gdk_" + token

	id, err := auth.RandomID(12)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate key id")
		return
	}

	key, err := s.store.CreateAPIKey(r.Context(), "key_"+id, req.UserID, req.Name, auth.HashToken(plaintext))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create API key")
		return
	}

	// Token returned only here — never stored in plaintext.
	writeJSON(w, http.StatusCreated, map[string]any{
		"api_key": key,
		"token":   plaintext,
	})
}

func (s *Server) revokeAPIKey(w http.ResponseWriter, r *http.Request, _ store.User, _ store.Session) {
	id := r.PathValue("id")
	if err := s.store.RevokeAPIKey(r.Context(), id); err != nil {
		writeError(w, http.StatusNotFound, "api key not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "revoked"})
}
