package server

import (
	"math/rand"
	"net/http"
	"time"
)

func (s *Server) devLatency(next http.Handler) http.Handler {
	if s.cfg.DevLatencyMax <= 0 {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		delay := s.cfg.DevLatencyMin
		if s.cfg.DevLatencyMax > s.cfg.DevLatencyMin {
			jitter := s.cfg.DevLatencyMax - s.cfg.DevLatencyMin
			delay += time.Duration(rand.Int63n(int64(jitter) + 1))
		}
		if delay > 0 {
			time.Sleep(delay)
		}
		next.ServeHTTP(w, r)
	})
}
