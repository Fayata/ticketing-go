package main

import (
	"log"
	"net/http"
	"strings"

	"ticketing/config"
	"ticketing/controllers"
	"ticketing/handlers"
	"ticketing/middleware"
	"ticketing/models"
	"ticketing/services"
	"ticketing/utils"
)

func main() {
	cfg := config.LoadConfig()
	if err := config.InitDatabase(cfg); err != nil {
		log.Fatal(err)
	}
	config.InitSession(cfg.SessionSecret, cfg.SessionSecure)
	utils.InitTemplates()

	jwtService := utils.NewJWTService(cfg)
	emailService := utils.NewEmailService(cfg)
	authService := services.NewAuthService(cfg, emailService, jwtService)
	authController := controllers.NewAuthController(authService)
	adminHandler := handlers.NewAdminHandler(cfg)

	dashboardService := services.NewDashboardService()
	kbService := services.NewKBService()
	notificationService := services.NewNotificationService()
	ticketService := services.NewTicketService(jwtService)
	settingsService := services.NewSettingsService()

	mux := http.NewServeMux()

	// migrate tabel kalo belum ada
	if err := config.AutoMigrate(
		&models.User{},
		&models.Group{},
		&models.Department{},
		&models.Ticket{},
		&models.TicketReply{},
		&models.TicketAssignmentHistory{},
		&models.TicketRating{},
		&models.Notification{},
		&models.KBCategory{},
		&models.KBArticle{},
	); err != nil {
		log.Fatal(err)
	}

	dashboardHandler := handlers.NewDashboardHandler(cfg, dashboardService, kbService)
	ticketHandler := handlers.NewTicketHandler(cfg, emailService, ticketService)
	settingsHandler := handlers.NewSettingsHandler(cfg, settingsService)
	departementHandler := handlers.NewDepartmentHandler(cfg, emailService)
	notificationHandler := handlers.NewNotificationHandler(cfg, notificationService)

	// file static (css, js, gambar)
	fs := http.FileServer(http.Dir("./static"))
	mux.Handle("/static/", http.StripPrefix("/static/", fs))

	// rute yang ga perlu login
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		http.Redirect(w, r, config.Path("/login"), http.StatusSeeOther)
	})

	mux.HandleFunc("/login", middleware.GuestOnly(authController.Login))
	mux.HandleFunc("/register", middleware.GuestOnly(authController.Register))
	mux.HandleFunc("/verify-email", authController.VerifyEmail)
	mux.HandleFunc("/logout", authController.Logout)
	mux.HandleFunc("/forgot-password", middleware.GuestOnly(authController.ForgotPassword))
	mux.HandleFunc("/reset-password", middleware.GuestOnly(authController.ResetPassword))

	// admin & staff
	mux.HandleFunc("/departement/dashboard", middleware.AuthRequired(middleware.DepartmentRequired(departementHandler.ShowDashboard)))
	mux.HandleFunc("/admin/users", middleware.AuthRequired(middleware.SuperAdminRequired(adminHandler.ListUsers)))
	mux.HandleFunc("/admin/users/create", middleware.AuthRequired(middleware.SuperAdminRequired(adminHandler.CreateUserForm)))
	mux.HandleFunc("/admin/users/toggle/", middleware.AuthRequired(middleware.SuperAdminRequired(adminHandler.ToggleUserStatus)))
	mux.HandleFunc("/admin/users/staff/", middleware.AuthRequired(middleware.SuperAdminRequired(adminHandler.ToggleStaffRole)))
	mux.HandleFunc("/admin/departments", middleware.AuthRequired(middleware.SuperAdminRequired(adminHandler.ListDepartments)))
	mux.HandleFunc("/admin/departments/create", middleware.AuthRequired(middleware.SuperAdminRequired(adminHandler.CreateDepartmentForm)))
	mux.HandleFunc("/admin/knowledge-base", middleware.AuthRequired(middleware.StaffOrSuperAdminRequired(adminHandler.ListKBAdmin)))
	mux.HandleFunc("/admin/knowledge-base/categories/create", middleware.AuthRequired(middleware.StaffOrSuperAdminRequired(adminHandler.CreateKBCategoryForm)))
	mux.HandleFunc("/admin/knowledge-base/categories/create/post", middleware.AuthRequired(middleware.StaffOrSuperAdminRequired(adminHandler.CreateKBCategoryPost)))
	mux.HandleFunc("/admin/knowledge-base/articles/create", middleware.AuthRequired(middleware.StaffOrSuperAdminRequired(adminHandler.CreateKBArticleForm)))
	mux.HandleFunc("/admin/knowledge-base/articles/create/post", middleware.AuthRequired(middleware.StaffOrSuperAdminRequired(adminHandler.CreateKBArticlePost)))
	mux.HandleFunc("/department/tiket/claim/", middleware.AuthRequired(middleware.DepartmentRequired(departementHandler.ClaimTicket)))
	mux.HandleFunc("/department/tiket/release/", middleware.AuthRequired(middleware.DepartmentRequired(departementHandler.ReleaseTicket)))
	mux.HandleFunc("/department/tiket/", middleware.AuthRequired(middleware.DepartmentRequired(departementHandler.HandleTicketDetail)))
	mux.HandleFunc("/department/tiket/close/", middleware.AuthRequired(middleware.DepartmentRequired(departementHandler.CloseTicket)))
	mux.HandleFunc("/department/logout-release", middleware.AuthRequired(middleware.DepartmentRequired(departementHandler.LogoutAndRelease)))
	mux.HandleFunc("/department/all-tickets", middleware.AuthRequired(middleware.DepartmentRequired(departementHandler.ShowAllTickets)))

	// user portal (dashboard, tiket, settings, dll)
	mux.HandleFunc("/dashboard", middleware.AuthRequired(middleware.PortalUserRequired(dashboardHandler.ShowDashboard)))
	mux.HandleFunc("/tiket", middleware.AuthRequired(middleware.PortalUserRequired(ticketHandler.ShowMyTickets)))
	mux.HandleFunc("/tiket/", middleware.AuthRequired(middleware.PortalUserRequired(ticketHandler.HandleTicketDetail)))
	mux.HandleFunc("/kirim-tiket", middleware.AuthRequired(middleware.PortalUserRequired(ticketHandler.HandleCreateTicket)))
	mux.HandleFunc("/tiket/sukses/", middleware.AuthRequired(middleware.PortalUserRequired(ticketHandler.ShowTicketSuccess)))
	mux.HandleFunc("/rating/", ticketHandler.HandleRating) // akses pake token di URL
	mux.HandleFunc("/settings", middleware.AuthRequired(middleware.PortalUserRequired(settingsHandler.HandleSettings)))
	mux.HandleFunc("/knowledge-base", middleware.AuthRequired(middleware.PortalUserRequired(dashboardHandler.ShowKnowledgeBase)))
	mux.HandleFunc("/knowledge-base/article/", middleware.AuthRequired(middleware.PortalUserRequired(dashboardHandler.ShowKBArticle)))
	
	// API buat panel notifikasi
	mux.HandleFunc("/api/notifications", middleware.AuthRequired(notificationHandler.GetNotifications))
	mux.HandleFunc("/api/notifications/read", middleware.AuthRequired(notificationHandler.MarkAsRead))
	mux.HandleFunc("/api/notifications/read-all", middleware.AuthRequired(notificationHandler.MarkAllAsRead))
	mux.HandleFunc("/api/notifications/count", middleware.AuthRequired(notificationHandler.GetUnreadCount))

	seedDefaultData()

	log.Printf("🚀 Server starting on port %s", cfg.Port)
	log.Printf("🌐 Visit: http://localhost:%s", cfg.Port)

	loggedMux := middleware.LoggingMiddleware(mux)
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
	if err := http.ListenAndServe(":"+cfg.Port, handler); err != nil {
		log.Fatal(err)
	}
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
	const defaultAdminPassword = "admin12345"

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
}
