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
	config.InitSession(cfg.SessionSecret)
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

	mux := http.NewServeMux()
	// mux := http.NewServeMux()

	// Auto migrate models
	if err := config.AutoMigrate(
		&models.User{},
		&models.Group{},
		&models.Department{},
		&models.Ticket{},
		&models.TicketReply{},
	); err != nil {
		log.Fatal(err)
	}

	// Initialize session store
	config.InitSession(cfg.SessionSecret)

	// Load templates
	templates = loadTemplates()

	// Initialize handlers
	// emailService := utils.NewEmailService(cfg)
	// authHandler := handlers.NewAuthHandler(cfg)
	dashboardHandler := handlers.NewDashboardHandler(cfg)
	ticketHandler := handlers.NewTicketHandler(cfg, emailService)
	settingsHandler := handlers.NewSettingsHandler(cfg)

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
		http.Redirect(w, r, "/login", http.StatusSeeOther)
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

	// Protected routes
	mux.HandleFunc("/dashboard", middleware.AuthRequired(middleware.PortalUserRequired(dashboardHandler.ShowDashboard)))
	mux.HandleFunc("/tiket", middleware.AuthRequired(middleware.PortalUserRequired(ticketHandler.ShowMyTickets)))
	mux.HandleFunc("/tiket/", middleware.AuthRequired(middleware.PortalUserRequired(ticketHandler.HandleTicketDetail)))
	mux.HandleFunc("/kirim-tiket", middleware.AuthRequired(middleware.PortalUserRequired(ticketHandler.HandleCreateTicket)))
	mux.HandleFunc("/tiket/sukses/", middleware.AuthRequired(middleware.PortalUserRequired(ticketHandler.ShowTicketSuccess)))
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
	}

	tmpl := template.New("").Funcs(funcMap)
	tmpl = template.Must(tmpl.ParseGlob(filepath.Join("templates", "*.html")))
	tmpl = template.Must(tmpl.ParseGlob(filepath.Join("templates", "tickets", "*.html")))

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
}
