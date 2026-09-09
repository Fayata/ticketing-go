package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"ticketing/config"
	"ticketing/handlers"
	"ticketing/internal/logging"
	"ticketing/middleware"
	"ticketing/models"
	"ticketing/services"
	"ticketing/utils"
)

// main is the entry point for the Admin and KB Service.
func main() {
	logging.Init("admin-service")

	// Initialize Config
	cfg := config.LoadConfig()
	port := os.Getenv("ADMIN_SERVICE_PORT")
	if port == "" {
		port = "8084"
	}

	// Initialize Database
	config.InitDatabase(cfg)
	db := config.DB

	// Auto-migrate ALL models with retry (to handle concurrent migration collisions)
	log.Println("Migrating database models...")
	var err error
	for i := 0; i < 5; i++ {
		err = db.AutoMigrate(
			&models.Company{},
			&models.User{},
			&models.Department{},
			&models.Group{},
			&models.SLAPolicy{},
			&models.Ticket{},
			&models.TicketReply{},
			&models.TicketAttachment{},
			&models.TicketAssignmentHistory{},
			&models.TicketRating{},
			&models.TicketPriorityHistory{},
			&models.Notification{},
			&models.KBCategory{},
			&models.KBArticle{},
		)
		if err == nil {
			break
		}
		log.Printf("Migration failed (attempt %d/5): %v. Retrying in 2 seconds...", i+1, err)
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		log.Fatalf("Failed to migrate database after 5 attempts: %v", err)
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
	adminMux.HandleFunc("/companies", adminHandler.ListCompanies)
	adminMux.HandleFunc("/companies/create", AuditLogWrapper("Create Company", adminHandler.CreateCompanyForm))
	adminMux.HandleFunc("/companies/edit/", AuditLogWrapper("Edit Company", adminHandler.EditCompanyForm))
	adminMux.HandleFunc("/companies/toggle/", AuditLogWrapper("Toggle Company Status", adminHandler.ToggleCompanyStatus))
	adminMux.HandleFunc("/sla-policies", AuditLogWrapper("List SLA Policies", adminHandler.ListSLAPolicies))
	adminMux.HandleFunc("/sla-policies/create", AuditLogWrapper("Create SLA Policy", adminHandler.CreateSLAPolicyForm))
	adminMux.HandleFunc("/sla-policies/edit/", AuditLogWrapper("Edit SLA Policy", adminHandler.EditSLAPolicyForm))
	adminMux.HandleFunc("/sla-policies/toggle/", AuditLogWrapper("Toggle SLA Policy Status", adminHandler.ToggleSLAPolicyStatus))
	
	adminHandlerFunc := middleware.AuthRequired(middleware.SuperAdminRequired(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/admin")
		if p == "" || p == "/" {
			p = "/dashboard"
		}
		r.URL.Path = p
		adminMux.ServeHTTP(w, r)
	})))
	mux.Handle("/admin/", adminHandlerFunc)
	mux.Handle("/admin", adminHandlerFunc)

	// Admin KB Routes (StaffOrSuperAdmin)
	kbAdminMux := http.NewServeMux()
	kbAdminMux.HandleFunc("/", adminHandler.ListKBAdmin)
	kbAdminMux.HandleFunc("/categories/create", adminHandler.CreateKBCategoryForm)
	kbAdminMux.HandleFunc("/categories/create/post", adminHandler.CreateKBCategoryPost)
	kbAdminMux.HandleFunc("/categories/edit/", adminHandler.EditKBCategory)
	kbAdminMux.HandleFunc("/categories/delete/", adminHandler.DeleteKBCategory)
	kbAdminMux.HandleFunc("/articles/create", adminHandler.CreateKBArticleForm)
	kbAdminMux.HandleFunc("/articles/create/post", adminHandler.CreateKBArticlePost)
	kbAdminMux.HandleFunc("/articles/edit/", adminHandler.EditKBArticle)
	kbAdminMux.HandleFunc("/articles/delete/", adminHandler.DeleteKBArticle)
	
	kbHandler := middleware.AuthRequired(middleware.StaffOrSuperAdminRequired(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/admin/knowledge-base")
		if p == "" || p == "/" {
			r.URL.Path = "/"
		} else {
			r.URL.Path = p
		}
		kbAdminMux.ServeHTTP(w, r)
	})))
	mux.Handle("/admin/knowledge-base/", kbHandler)
	mux.Handle("/admin/knowledge-base", kbHandler)

	// Department Routes
	deptMux := http.NewServeMux()
	deptMux.HandleFunc("/dashboard", departmentHandler.ShowDashboard)
	deptMux.HandleFunc("/all-tickets", departmentHandler.ShowAllTickets)
	deptMux.HandleFunc("/tiket/estimate/", departmentHandler.SetTicketEstimate)
	deptMux.HandleFunc("/tiket/priority/", departmentHandler.SetTicketPriority)
	deptMux.HandleFunc("/tiket/claim/", departmentHandler.ClaimTicket)
	deptMux.HandleFunc("/tiket/release/", departmentHandler.ReleaseTicket)
	deptMux.HandleFunc("/tiket/close/", departmentHandler.CloseTicket)
	deptMux.HandleFunc("/tiket/", departmentHandler.HandleTicketDetail)
	deptMux.HandleFunc("/logout-release", departmentHandler.LogoutAndRelease)

	deptHandler := middleware.AuthRequired(middleware.DepartmentRequired(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if strings.HasPrefix(p, "/departement") {
			p = strings.TrimPrefix(p, "/departement")
		} else if strings.HasPrefix(p, "/department") {
			p = strings.TrimPrefix(p, "/department")
		}
		if p == "" || p == "/" {
			p = "/dashboard"
		}
		r.URL.Path = p
		deptMux.ServeHTTP(w, r)
	})))
	mux.Handle("/departement/", deptHandler)
	mux.Handle("/departement", deptHandler)
	mux.Handle("/department/", deptHandler)
	mux.Handle("/department", deptHandler)

	// Wrapper endpoints (from handler.go)
	mux.HandleFunc("/health", HealthCheckHandler)

	// Apply global logging middleware
	handler := logging.PanicRecoveryMiddleware(middleware.LoggingMiddleware(mux))

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
	// Seed default company and backfill legacy departments & tickets
	defaultCompany, err := models.SeedDefaultCompanyAndMigrate(config.DB)
	if err != nil {
		log.Printf("[Migration] Warning: SeedDefaultCompanyAndMigrate encountered error: %v", err)
	}

	// Seed default SLA policy and backfill legacy tickets
	if err := models.SeedDefaultSLAPolicies(config.DB); err != nil {
		log.Printf("[Migration] Warning: SeedDefaultSLAPolicies encountered error: %v", err)
	}

	var portalGroup models.Group
	config.DB.FirstOrCreate(&portalGroup, models.Group{Name: "Portal Users"})
	departments := []string{"Technical Support", "Customer Service", "Billing", "General"}
	for _, deptName := range departments {
		var dept models.Department
		if err := config.DB.Where("name = ?", deptName).First(&dept).Error; err != nil {
			dept = models.Department{
				Name: deptName,
			}
			if defaultCompany != nil {
				dept.CompanyID = &defaultCompany.ID
			}
			config.DB.Create(&dept)
		} else if (dept.CompanyID == nil || *dept.CompanyID == 0) && defaultCompany != nil {
			config.DB.Model(&dept).Update("company_id", defaultCompany.ID)
		}
	}

	const defaultAdminUsername = "admin"
	const defaultAdminEmail = "admin@local.test"
	defaultAdminPassword := generateRandomPassword(16)

	var existing models.User
	err = config.DB.Where("email = ?", defaultAdminEmail).First(&existing).Error
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

	adminEmailCopy := defaultAdminEmail
	admin := models.User{
		Username:     defaultAdminUsername,
		Email:        &adminEmailCopy,
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
