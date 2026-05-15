package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"godrive/internal/store"
)

func (s *Server) subscribeEvents(userID int64) chan WebhookEvent {
	ch := make(chan WebhookEvent, 32)
	s.eventsMu.Lock()
	defer s.eventsMu.Unlock()
	if s.eventsSubs[userID] == nil {
		s.eventsSubs[userID] = make(map[chan WebhookEvent]struct{})
	}
	s.eventsSubs[userID][ch] = struct{}{}
	return ch
}

func (s *Server) unsubscribeEvents(userID int64, ch chan WebhookEvent) {
	s.eventsMu.Lock()
	defer s.eventsMu.Unlock()
	if subscribers := s.eventsSubs[userID]; subscribers != nil {
		delete(subscribers, ch)
		if len(subscribers) == 0 {
			delete(s.eventsSubs, userID)
		}
	}
	close(ch)
}

func (s *Server) publishEvent(event WebhookEvent) {
	s.eventsMu.Lock()
	defer s.eventsMu.Unlock()
	for ch := range s.eventsSubs[event.UserID] {
		select {
		case ch <- event:
		default:
		}
	}
}

func (s *Server) events(w http.ResponseWriter, r *http.Request, user store.User, session store.Session) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming is not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	events := s.subscribeEvents(user.ID)
	defer s.unsubscribeEvents(user.ID, events)

	_, _ = fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()

	heartbeat := time.NewTicker(25 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-heartbeat.C:
			_, _ = fmt.Fprint(w, ": heartbeat\n\n")
			flusher.Flush()
		case event := <-events:
			payload, err := json.Marshal(event)
			if err != nil {
				continue
			}
			_, _ = fmt.Fprintf(w, "event: %s\n", event.Event)
			_, _ = fmt.Fprintf(w, "data: %s\n\n", payload)
			flusher.Flush()
		}
	}
}
