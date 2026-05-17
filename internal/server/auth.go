package server

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"godrive/internal/auth"
	"godrive/internal/store"
)

type authedHandler func(http.ResponseWriter, *http.Request, store.User, store.Session)

func (s *Server) withUser(next authedHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, session, viaCookie, ok := s.authenticate(w, r)
		if !ok {
			return
		}
		if viaCookie && isStateChanging(r.Method) && !s.validCSRF(r, session) {
			writeError(w, http.StatusForbidden, "missing or invalid csrf token")
			return
		}
		next(w, r, user, session)
	}
}

func (s *Server) withAdmin(next authedHandler) http.HandlerFunc {
	return s.withUser(func(w http.ResponseWriter, r *http.Request, user store.User, session store.Session) {
		if !user.IsAdmin {
			writeError(w, http.StatusForbidden, "admin required")
			return
		}
		next(w, r, user, session)
	})
}

func (s *Server) authenticate(w http.ResponseWriter, r *http.Request) (store.User, store.Session, bool, bool) {
	token, viaCookie := bearerToken(r)
	if token == "" {
		cookie, err := r.Cookie(s.cfg.SessionCookieName)
		if err != nil || cookie.Value == "" {
			writeError(w, http.StatusUnauthorized, "authentication required")
			return store.User{}, store.Session{}, false, false
		}
		token = cookie.Value
		viaCookie = true
	}

	tokenHash := auth.HashToken(token)
	user, session, err := s.store.UserByValidSession(r.Context(), tokenHash, time.Now().UTC())
	if err == nil {
		return user, session, viaCookie, true
	}

	// Fall back to API key auth (only for non-cookie bearer tokens).
	if !viaCookie {
		apiUser, apiErr := s.store.UserByAPIKey(r.Context(), tokenHash)
		if apiErr == nil {
			return apiUser, store.Session{}, false, true
		}
	}

	writeError(w, http.StatusUnauthorized, "authentication required")
	return store.User{}, store.Session{}, false, false
}

func bearerToken(r *http.Request) (string, bool) {
	header := r.Header.Get("Authorization")
	if header == "" {
		return "", false
	}
	value, ok := strings.CutPrefix(header, "Bearer ")
	if !ok || value == "" {
		return "", false
	}
	return value, false
}

func (s *Server) validCSRF(r *http.Request, session store.Session) bool {
	token := r.Header.Get("X-CSRF-Token")
	if token == "" {
		return false
	}
	return auth.HashToken(token) == session.CSRFTokenHash
}

func isStateChanging(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	default:
		return true
	}
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	ip := clientIP(r)
	if !s.loginLimit.allow(ip) {
		writeError(w, http.StatusTooManyRequests, "too many failed login attempts, try again later")
		return
	}

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}

	user, err := s.store.GetUserByUsername(r.Context(), req.Username)
	if err != nil || user.Disabled {
		s.loginLimit.record(ip)
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if err := auth.VerifyPassword(req.Password, user.PasswordHash); err != nil {
		if errors.Is(err, auth.ErrPasswordMismatch) || errors.Is(err, auth.ErrInvalidHash) {
			s.loginLimit.record(ip)
			writeError(w, http.StatusUnauthorized, "invalid credentials")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to verify password")
		return
	}
	s.loginLimit.reset(ip)

	sessionToken, err := auth.RandomToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create session")
		return
	}
	csrfToken, err := auth.RandomToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create session")
		return
	}

	expiresAt := time.Now().UTC().Add(s.cfg.SessionTTL)
	session, err := s.store.CreateSession(
		r.Context(),
		user.ID,
		auth.HashToken(sessionToken),
		auth.HashToken(csrfToken),
		expiresAt,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create session")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     s.cfg.SessionCookieName,
		Value:    sessionToken,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: true,
		Secure:   s.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     s.cfg.CSRFCookieName,
		Value:    csrfToken,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: false,
		Secure:   s.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"user":       user,
		"session_id": session.ID,
		"csrf_token": csrfToken,
		"token":      sessionToken,
		"expires_at": expiresAt,
	})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request, user store.User, session store.Session) {
	_ = s.store.RevokeSession(r.Context(), session.TokenHash)
	clearCookie(w, s.cfg.SessionCookieName, s.cfg.CookieSecure)
	clearCookie(w, s.cfg.CSRFCookieName, s.cfg.CookieSecure)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) me(w http.ResponseWriter, r *http.Request, user store.User, session store.Session) {
	writeJSON(w, http.StatusOK, map[string]any{"user": user, "session_id": session.ID})
}

func clearCookie(w http.ResponseWriter, name string, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}
