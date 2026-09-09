package main

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/http"
	"strings"
	"time"

	"ticketing/config"
	"ticketing/controllers"
	"ticketing/handlers"
	"ticketing/internal/logging"
	"ticketing/middleware"
	"ticketing/models"
	"ticketing/services"
	"ticketing/utils"
)

func main() {
	logging.Init("ticketing-app")

	cfg := config.LoadConfig()
	if err := config.InitDatabase(cfg); err != nil {
		log.Fatal(err)
	}
	config.InitSession(cfg.SessionSecret, cfg.SessionSecure)
	utils.InitTemplates()

	// [Security] Initialize rate limiters per endpoint
	loginLimiter := middleware.NewRateLimiter(5, 1*time.Minute)
	registerLimiter := middleware.NewRateLimiter(3, 1*time.Minute)
	forgotPwLimiter := middleware.NewRateLimiter(3, 1*time.Minute)
	globalLimiter := middleware.NewRateLimiter(60, 1*time.Minute)
	log.Println("[Security] Rate limiters initialized: login=5/min, register=3/min, forgot=3/min, global=60/min")

	jwtService := utils.NewJWTService(cfg)
	emailService := utils.NewEmailService(cfg)
	authService := services.NewAuthService(cfg, emailService, jwtService)
	authController := controllers.NewAuthController(authService)
	adminDashboardService := services.NewAdminDashboardService()
	aiService := services.NewAIService(cfg)
	adminSearchService := services.NewAdminSearchService()
	adminHandler := handlers.NewAdminHandler(cfg, adminDashboardService, aiService, adminSearchService)

	dashboardService := services.NewDashboardService()
	kbService := services.NewKBService()
	notificationService := services.NewNotificationService()
	ticketService := services.NewTicketService(jwtService)
	settingsService := services.NewSettingsService()

	mux := http.NewServeMux()

	if err := config.AutoMigrate(
		&models.Company{},
		&models.User{},
		&models.Group{},
		&models.Department{},
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
	); err != nil {
		log.Fatal(err)
	}

	// Pastikan kolom email bisa NULL (untuk akun yang dibuat admin tanpa email)
	config.DB.Exec("ALTER TABLE users ALTER COLUMN email DROP NOT NULL")

	// Real-time WebSocket Hub
	wsHub := services.NewWSHub()
	go wsHub.Run()
	wsHandler := handlers.NewWSHandler(wsHub)

	dashboardHandler := handlers.NewDashboardHandler(cfg, dashboardService, kbService)
	ticketHandler := handlers.NewTicketHandler(cfg, emailService, ticketService, wsHub)
	settingsHandler := handlers.NewSettingsHandler(cfg, settingsService)
	staffDashboardService := services.NewStaffDashboardService()
	departementHandler := handlers.NewDepartmentHandler(cfg, emailService, staffDashboardService, wsHub)
	notificationHandler := handlers.NewNotificationHandler(cfg, notificationService)
	setEmailHandler := handlers.NewSetEmailHandler(cfg, emailService, jwtService)

	fs := http.FileServer(http.Dir("./static"))
	mux.Handle("/static/", http.StripPrefix("/static/", fs))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		http.Redirect(w, r, config.Path("/login"), http.StatusSeeOther)
	})

	mux.HandleFunc("/login", loginLimiter.Limit(middleware.GuestOnly(authController.Login)))
	mux.HandleFunc("/register", registerLimiter.Limit(middleware.GuestOnly(authController.Register)))
	mux.HandleFunc("/verify-email", authController.VerifyEmail)
	mux.HandleFunc("/logout", authController.Logout)
	mux.HandleFunc("/forgot-password", forgotPwLimiter.Limit(middleware.GuestOnly(authController.ForgotPassword)))
	mux.HandleFunc("/reset-password", middleware.GuestOnly(authController.ResetPassword))

	// Halaman set email — wajib setelah login jika akun belum punya email
	mux.HandleFunc("/set-email", middleware.AuthRequired(setEmailHandler.HandleSetEmail))

	// Real-time WebSocket endpoint for ticket chat (authenticated users)
	mux.HandleFunc("/ws/ticket/", middleware.AuthRequired(wsHandler.HandleTicketWS))

	mux.HandleFunc("/departement/dashboard", middleware.AuthRequired(middleware.EmailRequired(middleware.DepartmentRequired(departementHandler.ShowDashboard))))
	mux.HandleFunc("/admin/dashboard", middleware.AuthRequired(middleware.EmailRequired(middleware.SuperAdminRequired(adminHandler.ShowAdminDashboard))))
	mux.HandleFunc("/admin/search", middleware.AuthRequired(middleware.EmailRequired(middleware.SuperAdminRequired(adminHandler.SearchAdmin))))
	mux.HandleFunc("/admin/users", middleware.AuthRequired(middleware.EmailRequired(middleware.SuperAdminRequired(adminHandler.ListUsers))))
	mux.HandleFunc("/admin/users/create", middleware.AuthRequired(middleware.EmailRequired(middleware.SuperAdminRequired(adminHandler.CreateUserForm))))
	mux.HandleFunc("/admin/users/toggle/", middleware.AuthRequired(middleware.EmailRequired(middleware.SuperAdminRequired(adminHandler.ToggleUserStatus))))
	mux.HandleFunc("/admin/users/staff/", middleware.AuthRequired(middleware.EmailRequired(middleware.SuperAdminRequired(adminHandler.ToggleStaffRole))))
	mux.HandleFunc("/admin/departments", middleware.AuthRequired(middleware.EmailRequired(middleware.SuperAdminRequired(adminHandler.ListDepartments))))
	mux.HandleFunc("/admin/departments/create", middleware.AuthRequired(middleware.EmailRequired(middleware.SuperAdminRequired(adminHandler.CreateDepartmentForm))))
	mux.HandleFunc("/admin/companies", middleware.AuthRequired(middleware.EmailRequired(middleware.SuperAdminRequired(adminHandler.ListCompanies))))
	mux.HandleFunc("/admin/companies/create", middleware.AuthRequired(middleware.EmailRequired(middleware.SuperAdminRequired(adminHandler.CreateCompanyForm))))
	mux.HandleFunc("/admin/companies/edit/", middleware.AuthRequired(middleware.EmailRequired(middleware.SuperAdminRequired(adminHandler.EditCompanyForm))))
	mux.HandleFunc("/admin/companies/toggle/", middleware.AuthRequired(middleware.EmailRequired(middleware.SuperAdminRequired(adminHandler.ToggleCompanyStatus))))
	mux.HandleFunc("/admin/sla-policies", middleware.AuthRequired(middleware.EmailRequired(middleware.SuperAdminRequired(adminHandler.ListSLAPolicies))))
	mux.HandleFunc("/admin/sla-policies/create", middleware.AuthRequired(middleware.EmailRequired(middleware.SuperAdminRequired(adminHandler.CreateSLAPolicyForm))))
	mux.HandleFunc("/admin/sla-policies/edit/", middleware.AuthRequired(middleware.EmailRequired(middleware.SuperAdminRequired(adminHandler.EditSLAPolicyForm))))
	mux.HandleFunc("/admin/sla-policies/toggle/", middleware.AuthRequired(middleware.EmailRequired(middleware.SuperAdminRequired(adminHandler.ToggleSLAPolicyStatus))))
	mux.HandleFunc("/admin/knowledge-base", middleware.AuthRequired(middleware.EmailRequired(middleware.StaffOrSuperAdminRequired(adminHandler.ListKBAdmin))))
	mux.HandleFunc("/admin/knowledge-base/categories/create", middleware.AuthRequired(middleware.EmailRequired(middleware.StaffOrSuperAdminRequired(adminHandler.CreateKBCategoryForm))))
	mux.HandleFunc("/admin/knowledge-base/categories/create/post", middleware.AuthRequired(middleware.EmailRequired(middleware.StaffOrSuperAdminRequired(adminHandler.CreateKBCategoryPost))))
	mux.HandleFunc("/admin/knowledge-base/categories/edit/", middleware.AuthRequired(middleware.EmailRequired(middleware.StaffOrSuperAdminRequired(adminHandler.EditKBCategory))))
	mux.HandleFunc("/admin/knowledge-base/categories/delete/", middleware.AuthRequired(middleware.EmailRequired(middleware.StaffOrSuperAdminRequired(adminHandler.DeleteKBCategory))))
	mux.HandleFunc("/admin/knowledge-base/articles/create", middleware.AuthRequired(middleware.EmailRequired(middleware.StaffOrSuperAdminRequired(adminHandler.CreateKBArticleForm))))
	mux.HandleFunc("/admin/knowledge-base/articles/create/post", middleware.AuthRequired(middleware.EmailRequired(middleware.StaffOrSuperAdminRequired(adminHandler.CreateKBArticlePost))))
	mux.HandleFunc("/admin/knowledge-base/articles/edit/", middleware.AuthRequired(middleware.EmailRequired(middleware.StaffOrSuperAdminRequired(adminHandler.EditKBArticle))))
	mux.HandleFunc("/admin/knowledge-base/articles/delete/", middleware.AuthRequired(middleware.EmailRequired(middleware.StaffOrSuperAdminRequired(adminHandler.DeleteKBArticle))))
	mux.HandleFunc("/department/tiket/claim/", middleware.AuthRequired(middleware.EmailRequired(middleware.DepartmentRequired(departementHandler.ClaimTicket))))
	mux.HandleFunc("/department/tiket/release/", middleware.AuthRequired(middleware.EmailRequired(middleware.DepartmentRequired(departementHandler.ReleaseTicket))))
	mux.HandleFunc("/department/tiket/estimate/", middleware.AuthRequired(middleware.EmailRequired(middleware.DepartmentRequired(departementHandler.SetTicketEstimate))))
	mux.HandleFunc("/department/tiket/priority/", middleware.AuthRequired(middleware.EmailRequired(middleware.DepartmentRequired(departementHandler.SetTicketPriority))))
	mux.HandleFunc("/department/tiket/", middleware.AuthRequired(middleware.EmailRequired(middleware.DepartmentRequired(departementHandler.HandleTicketDetail))))
	mux.HandleFunc("/department/tiket/close/", middleware.AuthRequired(middleware.EmailRequired(middleware.DepartmentRequired(departementHandler.CloseTicket))))
	mux.HandleFunc("/department/logout-release", middleware.AuthRequired(middleware.EmailRequired(middleware.DepartmentRequired(departementHandler.LogoutAndRelease))))
	mux.HandleFunc("/department/all-tickets", middleware.AuthRequired(middleware.EmailRequired(middleware.DepartmentRequired(departementHandler.ShowAllTickets))))
	mux.HandleFunc("/departement/tiket/claim/", middleware.AuthRequired(middleware.EmailRequired(middleware.DepartmentRequired(departementHandler.ClaimTicket))))
	mux.HandleFunc("/departement/tiket/release/", middleware.AuthRequired(middleware.EmailRequired(middleware.DepartmentRequired(departementHandler.ReleaseTicket))))
	mux.HandleFunc("/departement/tiket/estimate/", middleware.AuthRequired(middleware.EmailRequired(middleware.DepartmentRequired(departementHandler.SetTicketEstimate))))
	mux.HandleFunc("/departement/tiket/priority/", middleware.AuthRequired(middleware.EmailRequired(middleware.DepartmentRequired(departementHandler.SetTicketPriority))))
	mux.HandleFunc("/departement/tiket/", middleware.AuthRequired(middleware.EmailRequired(middleware.DepartmentRequired(departementHandler.HandleTicketDetail))))
	mux.HandleFunc("/departement/tiket/close/", middleware.AuthRequired(middleware.EmailRequired(middleware.DepartmentRequired(departementHandler.CloseTicket))))
	mux.HandleFunc("/departement/logout-release", middleware.AuthRequired(middleware.EmailRequired(middleware.DepartmentRequired(departementHandler.LogoutAndRelease))))
	mux.HandleFunc("/departement/all-tickets", middleware.AuthRequired(middleware.EmailRequired(middleware.DepartmentRequired(departementHandler.ShowAllTickets))))

	mux.HandleFunc("/dashboard", middleware.AuthRequired(middleware.EmailRequired(middleware.PortalUserRequired(dashboardHandler.ShowDashboard))))
	mux.HandleFunc("/tiket", middleware.AuthRequired(middleware.EmailRequired(middleware.PortalUserRequired(ticketHandler.ShowMyTickets))))
	mux.HandleFunc("/tiket/", middleware.AuthRequired(middleware.EmailRequired(middleware.PortalUserRequired(ticketHandler.HandleTicketDetail))))
	mux.HandleFunc("/kirim-tiket", middleware.AuthRequired(middleware.EmailRequired(middleware.PortalUserRequired(ticketHandler.HandleCreateTicket))))
	mux.HandleFunc("/tiket/sukses/", middleware.AuthRequired(middleware.EmailRequired(middleware.PortalUserRequired(ticketHandler.ShowTicketSuccess))))
		mux.HandleFunc("/rating/", middleware.AuthRequired(middleware.EmailRequired(ticketHandler.HandleRating)))
	mux.HandleFunc("/settings", middleware.AuthRequired(middleware.EmailRequired(middleware.PortalUserRequired(settingsHandler.HandleSettings))))
	mux.HandleFunc("/knowledge-base", middleware.AuthRequired(middleware.EmailRequired(middleware.PortalUserRequired(dashboardHandler.ShowKnowledgeBase))))
	mux.HandleFunc("/knowledge-base/article/", middleware.AuthRequired(middleware.EmailRequired(middleware.PortalUserRequired(dashboardHandler.ShowKBArticle))))

	mux.HandleFunc("/api/notifications", middleware.AuthRequired(notificationHandler.GetNotifications))
	mux.HandleFunc("/api/notifications/read", middleware.AuthRequired(notificationHandler.MarkAsRead))
	mux.HandleFunc("/api/notifications/read-all", middleware.AuthRequired(notificationHandler.MarkAllAsRead))
	mux.HandleFunc("/api/notifications/count", middleware.AuthRequired(notificationHandler.GetUnreadCount))
	mux.HandleFunc("/api/kb/article/view", middleware.AuthRequired(middleware.EmailRequired(middleware.PortalUserRequired(dashboardHandler.RecordKBArticleView))))
	mux.HandleFunc("/api/ticket/", middleware.AuthRequired(ticketHandler.GetTicketMessagesAPI))

	seedDefaultData()

	// Start background SLA notification worker
	go services.StartSLANotificationWorker(config.DB)

	log.Println("[Security] OWASP mitigations active: SecurityHeaders, RateLimiter, CSRF, InputValidation")
	log.Printf("[Security] Debug mode: %v | Session secure: %v", cfg.Debug, cfg.SessionSecure)

	log.Printf("Server starting on port %s", cfg.Port)
	log.Printf("Visit: http://localhost:%s", cfg.Port)

	// [Security] Apply security middleware stack (order matters: outermost runs first)
	csrfProtected := middleware.CSRFMiddleware(mux)
	securedMux := middleware.SecurityHeaders(csrfProtected, cfg.Debug)
	rateLimited := globalLimiter.LimitHandler(securedMux)
	loggedMux := logging.PanicRecoveryMiddleware(middleware.LoggingMiddleware(rateLimited))
	log.Println("[Security] Full middleware stack applied: PanicRecovery → Logging → RateLimit → SecurityHeaders → CSRF")
	var handler http.Handler = loggedMux
	if config.AppBasePath != "" && config.AppBasePath != "/" {
		prefix := strings.TrimRight(config.AppBasePath, "/")
		handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, prefix) {
				oldPath := r.URL.Path
				r.URL.Path = strings.TrimPrefix(r.URL.Path, prefix)
				if r.URL.Path == "" {
					r.URL.Path = "/"
				}
				loggedMux.ServeHTTP(w, r)
				r.URL.Path = oldPath
			} else {
				loggedMux.ServeHTTP(w, r)
			}
		})
	}

	// [Security] Request body size limiter (32MB for up to 5x 5MB attachments + form data)
	handler = http.MaxBytesHandler(handler, 32<<20)
	if err := http.ListenAndServe(":"+cfg.Port, handler); err != nil {
		log.Fatal(err)
	}
}

// seedDefaultData membuat group Portal Users, departemen default, dan user admin jika belum ada.
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
	// [Security] Generate random admin password instead of hardcoded
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
	log.Printf("[Security] Default admin created: username=%s email=%s password=%s — CHANGE THIS IMMEDIATELY!", defaultAdminUsername, defaultAdminEmail, defaultAdminPassword)
}

// generateRandomPassword creates a cryptographically random password of the specified length.
func generateRandomPassword(length int) string {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "ChangeMe!2024SecureP@ss" // fallback — still better than hardcoded simple password
	}
	return hex.EncodeToString(bytes)[:length]
}
