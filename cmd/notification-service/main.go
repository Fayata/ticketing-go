package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ticketing/config"
	"ticketing/handlers"
	"ticketing/middleware"
	"ticketing/models"
	"ticketing/services"
)

func main() {
	port := os.Getenv("NOTIFICATION_SERVICE_PORT")
	if port == "" {
		port = "8083"
	}

	cfg := config.LoadConfig()
	config.InitDatabase(cfg)
	config.InitSession(cfg.SessionSecret, cfg.SessionSecure)

	// Initialize services
	notificationService := services.NewNotificationService()

	// Initialize handlers
	notificationHandler := handlers.NewNotificationHandler(cfg, notificationService)

	// Auto-migrate model
	err := config.DB.AutoMigrate(&models.Notification{})
	if err != nil {
		log.Fatalf("AutoMigrate failed: %v", err)
	}

	mux := http.NewServeMux()

	// Register routes
	mux.Handle("/api/notifications", RateLimiter(middleware.AuthRequired(notificationHandler.GetNotifications)))
	mux.Handle("/api/notifications/read", RateLimiter(middleware.AuthRequired(notificationHandler.MarkAsRead)))
	mux.Handle("/api/notifications/read-all", RateLimiter(middleware.AuthRequired(notificationHandler.MarkAllAsRead)))
	mux.Handle("/api/notifications/count", RateLimiter(middleware.AuthRequired(notificationHandler.GetUnreadCount)))
	
	// Health check
	mux.HandleFunc("/health", HealthCheckHandler)

	// Apply logging middleware
	loggedMux := middleware.LoggingMiddleware(mux)

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: loggedMux,
	}

	go func() {
		log.Printf("Notification Service listening on port %s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("ListenAndServe error: %v", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down Notification Service...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server Shutdown Failed: %v", err)
	}
	log.Println("Notification Service exited gracefully")
}
