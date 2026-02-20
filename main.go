package main

import (
	"html/template"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"ticketing/config"
	"ticketing/controllers"
	"ticketing/handlers"
	"ticketing/middleware"
	"ticketing/models"
	"ticketing/services"
	"ticketing/utils"
)

var templates *template.Template

func main() {
	// Load configuration
	cfg := config.LoadConfig()

	// Initialize database
	if err := config.InitDatabase(cfg); err != nil {
		log.Fatal(err)
	}
	config.InitDatabase(cfg)
	config.InitSession(cfg.SessionSecret, cfg.SessionSecure)
	utils.InitTemplates()

	// Init Utils
	jwtService := utils.NewJWTService(cfg)
	emailService := utils.NewEmailService(cfg)

	// Init Services (Dependency Injection)
	authService := services.NewAuthService(cfg, emailService, jwtService)
	// ticketService := services.NewTicketService(...) // Lakukan hal sama utk tiket

	// Init Controllers
	authController := controllers.NewAuthController(authService)
	// ticketController := controllers.NewTicketController(...)
	adminHandler := handlers.NewAdminHandler(cfg)

	mux := http.NewServeMux()
	// mux := http.NewServeMux()

	// Auto migrate models
	if err := config.AutoMigrate(
		&models.User{},
		&models.Group{},
		&models.Department{},
		&models.Ticket{},
		&models.TicketReply{},
		&models.TicketAssignmentHistory{},
		&models.TicketRating{},
	); err != nil {
		log.Fatal(err)
	}

	// Initialize session store
	config.InitSession(cfg.SessionSecret, cfg.SessionSecure)

	// Load templates
	templates = loadTemplates()

	// Initialize handlers
	// emailService := utils.NewEmailService(cfg)
	// authHandler := handlers.NewAuthHandler(cfg)
	dashboardHandler := handlers.NewDashboardHandler(cfg)
	ticketHandler := handlers.NewTicketHandler(cfg, emailService)
	settingsHandler := handlers.NewSettingsHandler(cfg)
	departementHandler := handlers.NewDepartmentHandler(cfg, emailService)

	// Routes
	// mux := http.NewServeMux()

	// Static files
	fs := http.FileServer(http.Dir("./static"))
	mux.Handle("/static/", http.StripPrefix("/static/", fs))

	// Public routes
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		http.Redirect(w, r, config.Path("/login"), http.StatusSeeOther)
	})

	// mux.HandleFunc("/login", middleware.GuestOnly(authHandler.ShowLogin))
	// mux.HandleFunc("/login-post", authHandler.Login)
	// mux.HandleFunc("/register", middleware.GuestOnly(authHandler.ShowRegister))
	// mux.HandleFunc("/register-post", authHandler.Register)
	// mux.HandleFunc("/logout", authHandler.Logout)
	mux.HandleFunc("/login", middleware.GuestOnly(authController.Login))
	mux.HandleFunc("/register", middleware.GuestOnly(authController.Register))
	mux.HandleFunc("/verify-email", authController.VerifyEmail)
	mux.HandleFunc("/logout", authController.Logout)
	mux.HandleFunc("/forgot-password", middleware.GuestOnly(authController.ForgotPassword))
	mux.HandleFunc("/reset-password", middleware.GuestOnly(authController.ResetPassword))
	mux.HandleFunc("/departement/dashboard", middleware.AuthRequired(middleware.DepartmentRequired(departementHandler.ShowDashboard)))
	mux.HandleFunc("/admin/users", middleware.AuthRequired(middleware.SuperAdminRequired(adminHandler.ListUsers)))
	mux.HandleFunc("/admin/users/create", middleware.AuthRequired(middleware.SuperAdminRequired(adminHandler.CreateUserForm)))
	mux.HandleFunc("/admin/users/toggle/", middleware.AuthRequired(middleware.SuperAdminRequired(adminHandler.ToggleUserStatus)))
	mux.HandleFunc("/admin/users/staff/", middleware.AuthRequired(middleware.SuperAdminRequired(adminHandler.ToggleStaffRole)))
	mux.HandleFunc("/admin/departments", middleware.AuthRequired(middleware.SuperAdminRequired(adminHandler.ListDepartments)))
	mux.HandleFunc("/admin/departments/create", middleware.AuthRequired(middleware.SuperAdminRequired(adminHandler.CreateDepartmentForm)))
	mux.HandleFunc("/department/tiket/claim/", middleware.AuthRequired(middleware.DepartmentRequired(departementHandler.ClaimTicket)))
	mux.HandleFunc("/department/tiket/release/", middleware.AuthRequired(middleware.DepartmentRequired(departementHandler.ReleaseTicket)))
	mux.HandleFunc("/department/tiket/", middleware.AuthRequired(middleware.DepartmentRequired(departementHandler.HandleTicketDetail)))
	mux.HandleFunc("/department/tiket/close/", middleware.AuthRequired(middleware.DepartmentRequired(departementHandler.CloseTicket)))
	mux.HandleFunc("/department/logout-release", middleware.AuthRequired(middleware.DepartmentRequired(departementHandler.LogoutAndRelease)))
	mux.HandleFunc("/department/all-tickets", middleware.AuthRequired(middleware.DepartmentRequired(departementHandler.ShowAllTickets)))

	// Protected routes
	mux.HandleFunc("/dashboard", middleware.AuthRequired(middleware.PortalUserRequired(dashboardHandler.ShowDashboard)))
	mux.HandleFunc("/tiket", middleware.AuthRequired(middleware.PortalUserRequired(ticketHandler.ShowMyTickets)))
	mux.HandleFunc("/tiket/", middleware.AuthRequired(middleware.PortalUserRequired(ticketHandler.HandleTicketDetail)))
	mux.HandleFunc("/kirim-tiket", middleware.AuthRequired(middleware.PortalUserRequired(ticketHandler.HandleCreateTicket)))
	mux.HandleFunc("/tiket/sukses/", middleware.AuthRequired(middleware.PortalUserRequired(ticketHandler.ShowTicketSuccess)))
	mux.HandleFunc("/rating/", ticketHandler.HandleRating) // Public route (uses token auth)
	mux.HandleFunc("/settings", middleware.AuthRequired(middleware.PortalUserRequired(settingsHandler.HandleSettings)))

	// Seed Data
	seedDefaultData()

	// Start Server
	log.Printf("🚀 Server starting on port %s", cfg.Port)
	log.Printf("🌐 Visit: http://localhost:%s", cfg.Port)

	// Apply logging middleware
	loggedMux := middleware.LoggingMiddleware(mux)

	if err := http.ListenAndServe(":"+cfg.Port, loggedMux); err != nil {
		log.Fatal(err)
	}
}

func loadTemplates() *template.Template {
	funcMap := template.FuncMap{
		"slice": func(s string, start, end int) string {
			if start < 0 || end > len(s) || start > end {
				return s
			}
			return s[start:end]
		},
		"upper": strings.ToUpper,
		"date": func(t interface{}) string {
			if t == nil {
				return ""
			}
			switch v := t.(type) {
			case time.Time:
				return v.Format("02 Jan 2006, 15:04")
			case *time.Time:
				if v == nil {
					return ""
				}
				return v.Format("02 Jan 2006, 15:04")
			}
			return ""
		},
		"dateShort": func(t interface{}) string {
			if t == nil {
				return ""
			}
			switch v := t.(type) {
			case time.Time:
				return v.Format("02 Jan 2006")
			case *time.Time:
				if v == nil {
					return ""
				}
				return v.Format("02 Jan 2006")
			}
			return ""
		},
		"timeSince": func(t time.Time) string {
			now := time.Now()
			diff := now.Sub(t)
			days := int(diff.Hours() / 24)
			hours := int(diff.Hours())
			minutes := int(diff.Minutes())
			if days > 0 {
				return strings.Replace("{days} hari", "{days}", string(rune(days+'0')), 1)
			}
			if hours > 0 {
				return strings.Replace("{hours} jam", "{hours}", string(rune(hours+'0')), 1)
			}
			if minutes > 0 {
				return strings.Replace("{minutes} menit", "{minutes}", string(rune(minutes+'0')), 1)
			}
			return "Baru saja"
		},
		"getStatusClass": func(status interface{}) string {
			s := strings.ToUpper(status.(string))
			switch s {
			case "WAITING", "OPEN":
				return "open"
			case "IN_PROGRESS":
				return "in-progress"
			case "CLOSED", "RESOLVED":
				return "closed"
			default:
				return "closed"
			}
		},
		"getPriorityClass": func(priority interface{}) string {
			p := strings.ToUpper(priority.(string))
			switch p {
			case "HIGH":
				return "high"
			case "MEDIUM":
				return "medium"
			case "LOW":
				return "low"
			default:
				return "low"
			}
		},
		"eq":  func(a, b interface{}) bool { return a == b },
		"len": func(arr interface{}) int { return len(arr.([]interface{})) },
		"linebreaks": func(val interface{}) template.HTML {
			if val == nil {
				return ""
			}
			s := val.(string)
			s = strings.ReplaceAll(s, "\r\n", "<br>")
			s = strings.ReplaceAll(s, "\n", "<br>")
			return template.HTML(s)
		},
		"getFullName": func(user interface{}) string {
			if user == nil {
				return "User"
			}
			if u, ok := user.(*models.User); ok {
				if u.FirstName != "" || u.LastName != "" {
					return strings.TrimSpace(u.FirstName + " " + u.LastName)
				}
				return u.Username
			}
			if u, ok := user.(models.User); ok {
				if u.FirstName != "" || u.LastName != "" {
					return strings.TrimSpace(u.FirstName + " " + u.LastName)
				}
				return u.Username
			}
			return "User"
		},
		"seq": func(start, end int) []int {
			var result []int
			for i := start; i <= end; i++ {
				result = append(result, i)
			}
			return result
		},
	}

	tmpl := template.New("").Funcs(funcMap)
	tmpl = template.Must(tmpl.ParseGlob(filepath.Join("templates", "*.html")))
	tmpl = template.Must(tmpl.ParseGlob(filepath.Join("templates", "tickets", "*.html")))
	tmpl = template.Must(tmpl.ParseGlob(filepath.Join("templates", "admin", "*.html")))

	return tmpl
}
func seedDefaultData() {
	var portalGroup models.Group
	config.DB.FirstOrCreate(&portalGroup, models.Group{Name: "Portal Users"})
	departments := []string{"Technical Support", "Customer Service", "Billing", "General"}
	for _, deptName := range departments {
		var dept models.Department
		config.DB.FirstOrCreate(&dept, models.Department{Name: deptName})
	}

	// Seed default Super Admin (only if not exists)
	// NOTE: Change this password immediately after first login.
	const defaultAdminUsername = "admin"
	const defaultAdminEmail = "admin@local.test"
	const defaultAdminPassword = "admin12345"

	// Guarantee this default admin exists (without blocking on other existing super admins)
	var existing models.User
	err := config.DB.Where("email = ?", defaultAdminEmail).First(&existing).Error
	if err == nil {
		// Ensure flags are correct (idempotent)
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
