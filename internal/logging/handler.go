// Package logging provides a compact, colorized slog.Handler for terminal output.
// Falls back to slog.NewTextHandler when stdout is not a TTY.
package logging

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"
)

const (
	reset     = "\033[0m"
	dim       = "\033[2m"
	bold      = "\033[1m"
	red       = "\033[31m"
	redBold   = "\033[31;1m"
	green     = "\033[32m"
	yellow    = "\033[33m"
	blue      = "\033[34m"
	magenta   = "\033[35m"
	cyan      = "\033[36m"
	white     = "\033[97m"
	dimGreen  = "\033[2;32m"
	dimYellow = "\033[2;33m"
)

// New returns a colored handler when w is a terminal, otherwise a plain TextHandler.
func New(w io.Writer, level slog.Level) slog.Handler {
	if isTTY(w) {
		return &colorHandler{w: w, level: level}
	}
	return slog.NewTextHandler(w, &slog.HandlerOptions{Level: level})
}

func isTTY(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	st, err := f.Stat()
	if err != nil {
		return false
	}
	return st.Mode()&os.ModeCharDevice != 0
}

type colorHandler struct {
	w     io.Writer
	level slog.Level
	mu    sync.Mutex
	attrs []slog.Attr
	group string
}

func (h *colorHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *colorHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &colorHandler{w: h.w, level: h.level, attrs: append(h.attrs, attrs...), group: h.group}
}

func (h *colorHandler) WithGroup(name string) slog.Handler {
	return &colorHandler{w: h.w, level: h.level, attrs: h.attrs, group: name}
}

func (h *colorHandler) Handle(_ context.Context, r slog.Record) error {
	var buf bytes.Buffer

	// Timestamp
	buf.WriteString(dim)
	buf.WriteString(r.Time.Format("15:04:05.000"))
	buf.WriteString(reset)
	buf.WriteByte(' ')

	// Level
	buf.WriteString(levelColor(r.Level))
	buf.WriteString(levelLabel(r.Level))
	buf.WriteString(reset)
	buf.WriteByte(' ')

	// Collect attrs for special HTTP formatting
	attrs := make([]slog.Attr, 0, len(h.attrs)+r.NumAttrs())
	attrs = append(attrs, h.attrs...)
	r.Attrs(func(a slog.Attr) bool {
		attrs = append(attrs, a)
		return true
	})

	// Special compact format for HTTP request logs
	if r.Message == "http request" {
		writeHTTPLine(&buf, attrs)
	} else {
		// Message
		buf.WriteString(white)
		buf.WriteString(r.Message)
		buf.WriteString(reset)
		writeAttrs(&buf, attrs)
	}

	buf.WriteByte('\n')

	h.mu.Lock()
	_, err := h.w.Write(buf.Bytes())
	h.mu.Unlock()
	return err
}

func writeHTTPLine(buf *bytes.Buffer, attrs []slog.Attr) {
	var method, path, remote, errMsg string
	var statusVal int
	var dur time.Duration
	var byteCount int
	var slow bool

	for _, a := range attrs {
		switch a.Key {
		case "method":
			method = a.Value.String()
		case "path":
			path = a.Value.String()
		case "status":
			statusVal = int(a.Value.Int64())
		case "duration":
			dur = a.Value.Duration()
		case "bytes":
			byteCount = int(a.Value.Int64())
		case "remote":
			remote = a.Value.String()
		case "error":
			errMsg = a.Value.String()
		case "slow":
			slow = a.Value.Bool()
		}
	}

	// Method
	buf.WriteString(methodColor(method))
	fmt.Fprintf(buf, "%-7s", method)
	buf.WriteString(reset)

	// Path
	buf.WriteString(white)
	buf.WriteString(path)
	buf.WriteString(reset)

	// Status
	buf.WriteByte(' ')
	buf.WriteString(statusColor(statusVal))
	fmt.Fprintf(buf, "%d", statusVal)
	buf.WriteString(reset)

	// Duration (highlight slow requests)
	buf.WriteString("  ")
	if slow {
		buf.WriteString(yellow)
	} else {
		buf.WriteString(dim)
	}
	buf.WriteString(formatDuration(dur))
	buf.WriteString(reset)

	// Bytes
	if byteCount > 0 {
		buf.WriteString(dim)
		fmt.Fprintf(buf, "  %s", formatBytes(byteCount))
		buf.WriteString(reset)
	}

	// Remote
	if remote != "" {
		buf.WriteString(dim)
		fmt.Fprintf(buf, "  %s", remote)
		buf.WriteString(reset)
	}

	// Error body (5xx)
	if errMsg != "" {
		buf.WriteString("  ")
		buf.WriteString(red)
		buf.WriteString(errMsg)
		buf.WriteString(reset)
	}
}

func writeAttrs(buf *bytes.Buffer, attrs []slog.Attr) {
	for _, a := range attrs {
		buf.WriteString("  ")
		buf.WriteString(dimGreen)
		buf.WriteString(a.Key)
		buf.WriteString(reset)
		buf.WriteByte('=')
		buf.WriteString(formatAttrValue(a))
	}
}

func formatAttrValue(a slog.Attr) string {
	v := a.Value.Resolve()
	switch v.Kind() {
	case slog.KindDuration:
		return formatDuration(v.Duration())
	case slog.KindString:
		s := v.String()
		if needsQuote(s) {
			return fmt.Sprintf("%q", s)
		}
		return s
	default:
		return v.String()
	}
}

func needsQuote(s string) bool {
	for _, c := range s {
		if c == ' ' || c == '\t' || c == '"' || c == '=' {
			return true
		}
	}
	return false
}

func formatDuration(d time.Duration) string {
	switch {
	case d < time.Microsecond:
		return fmt.Sprintf("%.0fns", float64(d.Nanoseconds()))
	case d < time.Millisecond:
		return fmt.Sprintf("%.1fµs", float64(d.Nanoseconds())/1000)
	case d < time.Second:
		return fmt.Sprintf("%.1fms", float64(d.Microseconds())/1000)
	default:
		return fmt.Sprintf("%.2fs", d.Seconds())
	}
}

func formatBytes(n int) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%dB", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1fKB", float64(n)/1024)
	default:
		return fmt.Sprintf("%.1fMB", float64(n)/1024/1024)
	}
}

func levelColor(l slog.Level) string {
	switch {
	case l >= slog.LevelError:
		return redBold
	case l >= slog.LevelWarn:
		return yellow
	case l >= slog.LevelInfo:
		return green
	default:
		return cyan
	}
}

func levelLabel(l slog.Level) string {
	switch {
	case l >= slog.LevelError:
		return "ERROR"
	case l >= slog.LevelWarn:
		return "WARN "
	case l >= slog.LevelInfo:
		return "INFO "
	default:
		return "DEBUG"
	}
}

func methodColor(m string) string {
	switch m {
	case http.MethodGet:
		return blue
	case http.MethodPost:
		return green
	case http.MethodPut, http.MethodPatch:
		return cyan
	case http.MethodDelete:
		return red
	default:
		return magenta
	}
}

func statusColor(code int) string {
	switch {
	case code >= 500:
		return redBold
	case code >= 400:
		return yellow
	case code >= 300:
		return cyan
	default:
		return green
	}
}
