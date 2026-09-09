package logging

import (
	"bufio"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"runtime/debug"
	"strings"
	"time"
)

// GenerateCorrelationID generates a unique correlation ID for tracing.
func GenerateCorrelationID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return fmt.Sprintf("req-%x", b)
}

type statusResponseWriter struct {
	http.ResponseWriter
	status      int
	bytesWritten int64
}

func (w *statusResponseWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusResponseWriter) Write(b []byte) (int, error) {
	n, err := w.ResponseWriter.Write(b)
	w.bytesWritten += int64(n)
	return n, err
}

func (w *statusResponseWriter) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (w *statusResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hijacker, ok := w.ResponseWriter.(http.Hijacker); ok {
		return hijacker.Hijack()
	}
	return nil, nil, errors.New("http.Hijacker not implemented by underlying response writer")
}

// GetClientIP returns the real client IP, respecting X-Forwarded-For if behind a reverse proxy.
func GetClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

// HTTPMiddleware logs every HTTP access into logs/http_ui/access.json.log.
func HTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		corrID := r.Header.Get("X-Correlation-ID")
		if corrID == "" {
			corrID = GenerateCorrelationID()
		}
		w.Header().Set("X-Correlation-ID", corrID)

		ctx := WithCorrelationID(r.Context(), corrID)
		r = r.WithContext(ctx)

		sw := &statusResponseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)

		elapsed := time.Since(start)
		elapsedMs := float64(elapsed.Microseconds()) / 1000.0

		// Don't flood logs with polling messages if desired, but for full transparency log requests
		HTTPAccess.Info("HTTP Request",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", sw.status),
			slog.Float64("duration_ms", elapsedMs),
			slog.Int64("bytes", sw.bytesWritten),
			slog.String("client_ip", GetClientIP(r)),
			slog.String("user_agent", r.UserAgent()),
			slog.String("correlation_id", corrID),
		)
	})
}

// PanicRecoveryMiddleware recovers from panics, logs full stack trace to logs/system/panic.json.log,
// and safely renders a 500 error response.
func PanicRecoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				stack := debug.Stack()
				corrID := GetCorrelationID(r.Context())

				SystemPanic.Error("CRITICAL: Panic recovered in HTTP handler",
					slog.Any("panic_error", rec),
					slog.String("stack_trace", string(stack)),
					slog.String("path", r.URL.Path),
					slog.String("method", r.Method),
					slog.String("correlation_id", corrID),
				)

				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
