package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
)

type requestIDKey struct{}

// RequestID adds a unique identifier to every request, makes it available via
// RequestIDOf(ctx), and echoes it in the X-Request-Id response header so a
// client can correlate a failure with server-side logs.
//
// An incoming X-Request-Id is honored only when it looks safe (alphanumeric,
// reasonable length); otherwise a fresh ID is generated. This avoids log
// injection from clients while still allowing a known reverse proxy to set
// the ID upstream.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := safeIncomingID(r.Header.Get("X-Request-Id"))
		if id == "" {
			id = newRequestID()
		}
		w.Header().Set("X-Request-Id", id)
		ctx := context.WithValue(r.Context(), requestIDKey{}, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequestIDOf returns the request ID stored on ctx, or "" if none.
func RequestIDOf(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

func newRequestID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func safeIncomingID(s string) string {
	if l := len(s); l == 0 || l > 128 {
		return ""
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= '0' && c <= '9':
		case c >= 'a' && c <= 'z':
		case c >= 'A' && c <= 'Z':
		case c == '-' || c == '_':
		default:
			return ""
		}
	}
	return s
}

// ContextLogHandler wraps a slog.Handler so that any request-scoped attrs
// (currently just the request ID) are added to every record automatically.
type ContextLogHandler struct {
	slog.Handler
}

func (h ContextLogHandler) Handle(ctx context.Context, r slog.Record) error {
	if id := RequestIDOf(ctx); id != "" {
		r.AddAttrs(slog.String("request_id", id))
	}
	return h.Handler.Handle(ctx, r)
}

func (h ContextLogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return ContextLogHandler{Handler: h.Handler.WithAttrs(attrs)}
}

func (h ContextLogHandler) WithGroup(name string) slog.Handler {
	return ContextLogHandler{Handler: h.Handler.WithGroup(name)}
}
