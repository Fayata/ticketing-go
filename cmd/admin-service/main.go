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
	"ticketing/handlers"
	"ticketing/middleware"
	"ticketing/models"
	"ticketing/services"
	"ticketing/utils"
)

// main is the entry point for the Admin and KB Service.
func main() {
	// Initialize Config
	cfg := config.LoadConfig()
	port := os.Getenv("ADMIN_SERVICE_PORT")
	if port == "" {
		port = "8084"
	}

	// Initialize Database
	config.InitDatabase(cfg)
	db := config.DB

	// Auto-migrate ALL models
	log.Println("Migrating database models...")
	err := db.AutoMigrate(
		&models.User{},
		&models.Department{},
		&models.Group{},
		&models.Ticket{},
		&models.KBCategory{},
		&models.KBArticle{},
	)
	if err != nil {
		log.Fatalf("Failed to migrate database: %v", err)
	}

	// Seed default data
	seedDefaultData()

	// Initialize session store
	config.InitSession(cfg.SessionSecret, cfg.SessionSecure)

	// Initialize templates
	utils.InitTemplates()

	// Initialize Services
	adminDashboardService := services.NewAdminDashboardService()
	aiService := services.NewAIService(cfg)
	adminSearchService := services.NewAdminSearchService()
	staffDashboardService := services.NewStaffDashboardService()
	emailService := utils.NewEmailService(cfg)
	// kbService := services.NewKBService(db) // Used later if handlers take this

	// Initialize Handlers
	adminHandler := handlers.NewAdminHandler(
		cfg,
		adminDashboardService,
		aiService,
		adminSearchService,
	)
	departmentHandler := handlers.NewDepartmentHandler(cfg, emailService, staffDashboardService)
	
	// Create multiplexer
	mux := http.NewServeMux()

	// Static files
	fs := http.FileServer(http.Dir("static"))
	mux.Handle("/static/", http.StripPrefix("/static/", fs))

	// Admin Routes (SuperAdmin)
	adminMux := http.NewServeMux()
	adminMux.HandleFunc("/dashboard", AuditLogWrapper("Show Admin Dashboard", adminHandler.ShowAdminDashboard))
	adminMux.HandleFunc("/search", adminHandler.SearchAdmin)
	adminMux.HandleFunc("/users", adminHandler.ListUsers)
	adminMux.HandleFunc("/users/create", ValidateUserCreation(adminHandler.CreateUserForm))
	adminMux.HandleFunc("/users/toggle/", AuditLogWrapper("Toggle User Status", adminHandler.ToggleUserStatus))
	adminMux.HandleFunc("/users/staff/", AuditLogWrapper("Toggle Staff Role", adminHandler.ToggleStaffRole))
	adminMux.HandleFunc("/departments", adminHandler.ListDepartments)
	adminMux.HandleFunc("/departments/create", AuditLogWrapper("Create Department", adminHandler.CreateDepartmentForm))
	
	mux.Handle("/admin/", http.StripPrefix("/admin", middleware.AuthRequired(middleware.SuperAdminRequired(adminMux.ServeHTTP))))

	// Admin KB Routes (StaffOrSuperAdmin)
	kbAdminMux := http.NewServeMux()
	kbAdminMux.HandleFunc("/knowledge-base", adminHandler.ListKBAdmin)
	// Additional KB routes can be added here
	mux.Handle("/admin/knowledge-base/", http.StripPrefix("/admin/knowledge-base", middleware.AuthRequired(middleware.StaffOrSuperAdminRequired(kbAdminMux.ServeHTTP))))

	// Department Routes
	deptMux := http.NewServeMux()
	deptMux.HandleFunc("/dashboard", departmentHandler.ShowDashboard)
	deptMux.HandleFunc("/tiket/claim", departmentHandler.ClaimTicket)
	deptMux.HandleFunc("/tiket/release", departmentHandler.ReleaseTicket)
	deptMux.HandleFunc("/tiket/close", departmentHandler.CloseTicket)

	mux.Handle("/departement/", http.StripPrefix("/departement", middleware.AuthRequired(middleware.DepartmentRequired(deptMux.ServeHTTP))))

	// Wrapper endpoints (from handler.go)
	mux.HandleFunc("/health", HealthCheckHandler)

	// Apply global logging middleware
	handler := middleware.LoggingMiddleware(mux)

	// Server setup
	srv := &http.Server{
		Addr:    ":" + port,
		Handler: handler,
	}

	// Graceful shutdown
	go func() {
		log.Printf("Admin Service is running on port %s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Listen: %s\n", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down Admin Service...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal("Server forced to shutdown:", err)
	}

	log.Println("Admin Service exiting")
}

func seedDefaultData() {
	var portalGroup models.Group
	config.DB.FirstOrCreate(&portalGroup, models.Group{Name: "Portal Users"})
	departments := []string{"Technical Support", "Customer Service", "Billing", "General"}
	for _, deptName := range departments {
		var dept models.Department
		config.DB.FirstOrCreate(&dept, models.Department{Name: deptName})
	}

	const defaultAdminUsername = "admin"
	const defaultAdminEmail = "admin@local.test"
	defaultAdminPassword := generateRandomPassword(16)

	var existing models.User
	err := config.DB.Where("email = ?", defaultAdminEmail).First(&existing).Error
	if err == nil {
		updates := map[string]interface{}{
			"is_active":      true,
			"is_verified":    true,
			"is_staff":       true,
			"is_super_admin": true,
			"department_id":  nil,
		}
		_ = config.DB.Model(&models.User{}).Where("id = ?", existing.ID).Updates(updates).Error
		return
	}

	hashed, herr := utils.HashPassword(defaultAdminPassword)
	if herr != nil {
		log.Printf("failed to hash default admin password: %v", herr)
		return
	}

	admin := models.User{
		Username:     defaultAdminUsername,
		Email:        defaultAdminEmail,
		Password:     hashed,
		IsActive:     true,
		IsVerified:   true,
		IsStaff:      true,
		IsSuperAdmin: true,
		DepartmentID: nil,
	}

	if cerr := config.DB.Create(&admin).Error; cerr != nil {
		log.Printf("failed to create default admin: %v", cerr)
		return
	}
	log.Printf("Default admin created: username=%s email=%s password=%s — CHANGE THIS IMMEDIATELY!", defaultAdminUsername, defaultAdminEmail, defaultAdminPassword)
}

func generateRandomPassword(length int) string {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "ChangeMe!2024SecureP@ss"
	}
	return hex.EncodeToString(bytes)[:length]
}
