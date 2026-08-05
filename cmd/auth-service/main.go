// Package main implements the Auth microservice for the ticketing system.
// Handles login, register, email verification, password reset, and logout.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ticketing/config"
	"ticketing/controllers"
	"ticketing/middleware"
	"ticketing/models"
	"ticketing/services"
	"ticketing/utils"
)

func main() {
	port := os.Getenv("AUTH_SERVICE_PORT")
	if port == "" {
		port = "8081"
	}

	// Initialize dependencies
	cfg := config.LoadConfig()
	if err := config.InitDatabase(cfg); err != nil {
		log.Fatal(err)
	}
	config.InitSession(cfg.SessionSecret, cfg.SessionSecure)
	utils.InitTemplates()

	// Auto-migrate auth-related models with retry
	var err error
	for i := 0; i < 5; i++ {
		err = config.AutoMigrate(&models.User{}, &models.Group{})
		if err == nil {
			break
		}
		log.Printf("Migration failed (attempt %d/5): %v. Retrying in 2 seconds...", i+1, err)
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		log.Fatalf("Migration failed after 5 attempts: %v", err)
	}

	// Seed default admin data
	seedAdmin()

	// Initialize services
	jwtService := utils.NewJWTService(cfg)
	emailService := utils.NewEmailService(cfg)
	authService := services.NewAuthService(cfg, emailService, jwtService)
	authController := controllers.NewAuthController(authService)

	// Rate limiters
	loginLimiter := middleware.NewRateLimiter(5, 1*time.Minute)
	registerLimiter := middleware.NewRateLimiter(3, 1*time.Minute)
	forgotLimiter := middleware.NewRateLimiter(3, 1*time.Minute)

	mux := http.NewServeMux()

	// Routes
	mux.HandleFunc("/login", loginLimiter.Limit(middleware.GuestOnly(authController.Login)))
	mux.HandleFunc("/register", registerLimiter.Limit(middleware.GuestOnly(authController.Register)))
	mux.HandleFunc("/verify-email", authController.VerifyEmail)
	mux.HandleFunc("/logout", authController.Logout)
	mux.HandleFunc("/forgot-password", forgotLimiter.Limit(middleware.GuestOnly(authController.ForgotPassword)))
	mux.HandleFunc("/reset-password", middleware.GuestOnly(authController.ResetPassword))

	// Health check
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"service":"auth-service","status":"ok"}`))
	})

	// Apply middleware
	handler := middleware.SecurityHeaders(mux, cfg.Debug)
	handler = middleware.LoggingMiddleware(handler)

	server := &http.Server{
		Addr:    ":" + port,
		Handler: handler,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("[Auth Service] Starting on port %s", port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[Auth Service] Listen error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("[Auth Service] Shutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("[Auth Service] Shutdown error: %v", err)
	}
	log.Println("[Auth Service] Gracefully stopped")
}

// seedAdmin creates a default admin user with a random password if one doesn't exist.
func seedAdmin() {
	const adminEmail = "admin@local.test"
	const adminUsername = "admin"

	var existing models.User
	if config.DB.Where("email = ?", adminEmail).First(&existing).Error == nil {
		return // Admin already exists
	}

	// Generate random password
	passBytes := make([]byte, 16)
	rand.Read(passBytes)
	password := hex.EncodeToString(passBytes)[:16]

	hashed, err := utils.HashPassword(password)
	if err != nil {
		log.Printf("[Auth Service] Failed to hash admin password: %v", err)
		return
	}

	admin := models.User{
		Username:     adminUsername,
		Email:        adminEmail,
		Password:     hashed,
		IsActive:     true,
		IsVerified:   true,
		IsStaff:      true,
		IsSuperAdmin: true,
	}

	if err := config.DB.Create(&admin).Error; err != nil {
		log.Printf("[Auth Service] Failed to create admin: %v", err)
		return
	}
	log.Printf("[Auth Service][Security] Admin created: email=%s password=%s — CHANGE IMMEDIATELY!", adminEmail, password)
}
