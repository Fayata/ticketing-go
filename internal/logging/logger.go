package logging

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"
)

type correlationIDKey struct{}

// InitLogger initializes a slog.Logger with JSON handler writing to both stdout and a file
func InitLogger(logFile string) *slog.Logger {
	var writer io.Writer = os.Stdout

	file, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err == nil {
		writer = io.MultiWriter(os.Stdout, file)
	} else {
		fmt.Printf("Failed to open log file: %v\n", err)
	}

	handler := slog.NewJSONHandler(writer, nil)
	logger := slog.New(handler)
	slog.SetDefault(logger)
	return logger
}

type responseWriterWrapper struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriterWrapper) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

// RequestLogger middleware
func RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Generate Correlation ID
		b := make([]byte, 16)
		_, _ = rand.Read(b)
		corrID := fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])

		ctx := context.WithValue(r.Context(), correlationIDKey{}, corrID)
		r = r.WithContext(ctx)

		w.Header().Set("X-Correlation-ID", corrID)

		rw := &responseWriterWrapper{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(rw, r)

		duration := time.Since(start)

		if r.Method != http.MethodGet || rw.status >= 400 {
			slog.Info("HTTP Request",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", rw.status),
				slog.Duration("duration", duration),
				slog.String("ip", r.RemoteAddr),
				slog.String("user_agent", r.UserAgent()),
				slog.String("correlation_id", corrID),
			)
		}
	})
}
