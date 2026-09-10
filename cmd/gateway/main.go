package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"ticketing/internal/logging"
)

func main() {
	logging.Init("gateway")

	port := os.Getenv("GATEWAY_PORT")
	if port == "" {
		port = "8080"
	}

	authURL := os.Getenv("AUTH_SERVICE_URL")
	if authURL == "" {
		authURL = "http://localhost:8081"
	}

	ticketURL := os.Getenv("TICKET_SERVICE_URL")
	if ticketURL == "" {
		ticketURL = "http://localhost:8082"
	}

	notificationURL := os.Getenv("NOTIFICATION_SERVICE_URL")
	if notificationURL == "" {
		notificationURL = "http://localhost:8083"
	}

	adminURL := os.Getenv("ADMIN_SERVICE_URL")
	if adminURL == "" {
		adminURL = "http://localhost:8084"
	}

	mux := http.NewServeMux()

	// Proxies
	authProxy := newProxy(authURL)
	ticketProxy := newProxy(ticketURL)
	notificationProxy := newProxy(notificationURL)
	adminProxy := newProxy(adminURL)

	// Auth routes
	mux.Handle("/login", authProxy)
	mux.Handle("/register", authProxy)
	mux.Handle("/logout", authProxy)
	mux.Handle("/verify-email", authProxy)
	mux.Handle("/set-email", authProxy)
	mux.Handle("/forgot-password", authProxy)
	mux.Handle("/reset-password", authProxy)

	// Ticket routes
	mux.Handle("/tiket/", ticketProxy)
	mux.Handle("/tiket", ticketProxy)
	mux.Handle("/kirim-tiket", ticketProxy)
	mux.Handle("/rating/", ticketProxy)
	mux.Handle("/dashboard", ticketProxy)
	mux.Handle("/settings", ticketProxy)
	mux.Handle("/knowledge-base/", ticketProxy)
	mux.Handle("/knowledge-base", ticketProxy)
	mux.Handle("/api/ticket/", ticketProxy)
	mux.Handle("/api/ticket", ticketProxy)
	mux.Handle("/api/dashboard/", ticketProxy)
	mux.Handle("/api/dashboard", ticketProxy)
	mux.Handle("/ws/ticket/", ticketProxy)
	mux.Handle("/ws/ticket", ticketProxy)

	// Notification routes
	mux.Handle("/api/notifications/", notificationProxy)
	mux.Handle("/api/notifications", notificationProxy)

	// Admin routes
	mux.Handle("/admin", adminProxy)
	mux.Handle("/admin/", adminProxy)
	mux.Handle("/departement", adminProxy)
	mux.Handle("/departement/", adminProxy)
	mux.Handle("/department", adminProxy)
	mux.Handle("/department/", adminProxy)

	// Static files
	fs := http.FileServer(http.Dir("./static"))
	mux.Handle("/static/", http.StripPrefix("/static/", fs))

	// Health check
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "ok",
			"services": []string{
				"auth", "ticket", "notification", "admin",
			},
		})
	})

	// Global Middleware
	handler := applyGlobalMiddleware(mux)

	server := &http.Server{
		Addr:    ":" + port,
		Handler: handler,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		logging.SystemLifecycle.Info("Gateway starting", "port", port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logging.SystemLifecycle.Error("Gateway listen error", "error", err.Error())
			log.Fatalf("Gateway listen error: %v", err)
		}
	}()

	<-ctx.Done()
	logging.SystemLifecycle.Info("Shutting down Gateway...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logging.SystemLifecycle.Error("Gateway shutdown error", "error", err.Error())
		log.Fatalf("Gateway shutdown error: %v", err)
	}
	logging.SystemLifecycle.Info("Gateway gracefully stopped")
}

func newProxy(target string) http.Handler {
	targetURL, err := url.Parse(target)
	if err != nil {
		logging.SystemGateway.Error("Invalid target URL", "target", target, "error", err.Error())
		log.Fatalf("Invalid target URL %s: %v", target, err)
	}
	proxy := httputil.NewSingleHostReverseProxy(targetURL)
	proxy.ModifyResponse = func(resp *http.Response) error {
		if loc := resp.Header.Get("Location"); loc != "" {
			if loc == "/" {
				resp.Header.Set("Location", "/Ticketing/")
			} else if strings.HasPrefix(loc, "/") && !strings.HasPrefix(loc, "/Ticketing") {
				resp.Header.Set("Location", "/Ticketing"+loc)
			}
		}
		return nil
	}
	return proxy
}

func applyGlobalMiddleware(next http.Handler) http.Handler {
	return logging.PanicRecoveryMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Correlation ID
		reqID := r.Header.Get("X-Correlation-ID")
		if reqID == "" {
			reqID = logging.GenerateCorrelationID()
			r.Header.Set("X-Correlation-ID", reqID)
		}
		w.Header().Set("X-Correlation-ID", reqID)

		ctx := logging.WithCorrelationID(r.Context(), reqID)
		r = r.WithContext(ctx)

		// Limit Body Size (32MB for up to 5x 5MB attachments + form fields)
		r.Body = http.MaxBytesReader(w, r.Body, 32*1024*1024)

		// Strip /Ticketing prefix if present (from base href / redirects)
		if strings.HasPrefix(r.URL.Path, "/Ticketing") {
			r.URL.Path = strings.TrimPrefix(r.URL.Path, "/Ticketing")
			if r.URL.Path == "" {
				r.URL.Path = "/"
			}
		}

		// Security Headers
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")

		// Logging
		start := time.Now()
		
		// Wrap ResponseWriter to capture status code
		ww := &responseWriterWrapper{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(ww, r)

		elapsed := time.Since(start)
		elapsedMs := float64(elapsed.Microseconds()) / 1000.0

		logging.HTTPAccess.Info("HTTP Request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", ww.status,
			"duration_ms", elapsedMs,
			"client_ip", logging.GetClientIP(r),
			"user_agent", r.UserAgent(),
			"correlation_id", reqID,
		)
	}))
}

type responseWriterWrapper struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriterWrapper) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

// Hijack enables WebSocket connection hijacking through the reverse proxy.
func (rw *responseWriterWrapper) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := rw.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, errors.New("http.Hijacker not implemented by underlying response writer")
}

// Flush enables streaming data to client.
func (rw *responseWriterWrapper) Flush() {
	if fl, ok := rw.ResponseWriter.(http.Flusher); ok {
		fl.Flush()
	}
}
