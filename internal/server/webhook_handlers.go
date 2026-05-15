package server

import (
	"encoding/json"
	"net/http"
	"time"

	"godrive/internal/auth"
	"godrive/internal/store"
)

func (s *Server) listWebhooks(w http.ResponseWriter, r *http.Request, admin store.User, session store.Session) {
	hooks, err := s.store.ListWebhooks(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list webhooks")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"webhooks": hooks})
}

func (s *Server) createWebhook(w http.ResponseWriter, r *http.Request, admin store.User, session store.Session) {
	var req struct {
		URL         string   `json:"url"`
		Events      []string `json:"events"`
		Description string   `json:"description"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.URL == "" {
		writeError(w, http.StatusBadRequest, "url is required")
		return
	}

	id, err := auth.RandomID(12)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate id")
		return
	}
	secret, err := auth.RandomID(32)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate secret")
		return
	}
	if req.Events == nil {
		req.Events = []string{}
	}

	wh, err := s.store.CreateWebhook(r.Context(), store.Webhook{
		ID:          id,
		URL:         req.URL,
		Secret:      secret,
		Events:      req.Events,
		Description: req.Description,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create webhook")
		return
	}

	// Return the secret in the creation response only — it is not retrievable later.
	writeJSON(w, http.StatusCreated, map[string]any{
		"webhook": wh,
		"secret":  secret,
	})
}

func (s *Server) deleteWebhook(w http.ResponseWriter, r *http.Request, admin store.User, session store.Session) {
	id := r.PathValue("id")
	if err := s.store.DeleteWebhook(r.Context(), id); err != nil {
		writeError(w, statusForError(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) testWebhook(w http.ResponseWriter, r *http.Request, admin store.User, session store.Session) {
	id := r.PathValue("id")
	hook, err := s.store.GetWebhook(r.Context(), id)
	if err != nil {
		writeError(w, statusForError(err), err.Error())
		return
	}

	pingEvtID, _ := auth.RandomID(12)
	pingEvt := WebhookEvent{
		ID:        "evt_" + pingEvtID,
		Event:     "ping",
		Timestamp: time.Now().UTC(),
		UserID:    admin.ID,
		Username:  admin.Username,
		Data:      map[string]any{"message": "test delivery from goDrive"},
	}

	payload, _ := json.Marshal(pingEvt)
	sig := webhookSignature(hook.Secret, payload)
	err = s.postWebhook(r.Context(), hook.URL, pingEvt.ID, "ping", sig, payload)
	if err != nil {
		writeError(w, http.StatusBadGateway, "delivery failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
