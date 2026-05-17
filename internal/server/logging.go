package server

import (
	"bytes"
	"log/slog"
	"net/http"
	"time"
)

const slowRequestThreshold = 2 * time.Second

type loggingResponseWriter struct {
	http.ResponseWriter
	status  int
	bytes   int
	errBody bytes.Buffer
}

func (w *loggingResponseWriter) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *loggingResponseWriter) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	if w.status >= 500 && w.errBody.Len() < 512 {
		w.errBody.Write(data)
	}
	n, err := w.ResponseWriter.Write(data)
	w.bytes += n
	return n, err
}

func (w *loggingResponseWriter) Flush() {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &loggingResponseWriter{ResponseWriter: w}
		next.ServeHTTP(rec, r)
		status := rec.status
		if status == 0 {
			status = http.StatusOK
		}
		dur := time.Since(start)

		level := slog.LevelInfo
		if status >= 500 {
			level = slog.LevelError
		} else if status >= 400 && status != http.StatusNotFound {
			level = slog.LevelWarn
		} else if dur >= slowRequestThreshold {
			level = slog.LevelWarn
		}

		attrs := []slog.Attr{
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", status),
			slog.Int("bytes", rec.bytes),
			slog.Duration("duration", dur),
			slog.String("remote", r.RemoteAddr),
		}
		if status >= 500 && rec.errBody.Len() > 0 {
			attrs = append(attrs, slog.String("error", rec.errBody.String()))
		}
		if dur >= slowRequestThreshold {
			attrs = append(attrs, slog.Bool("slow", true))
		}

		s.log.LogAttrs(r.Context(), level, "http request", attrs...)
	})
}
