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
	"ticketing/utils"
)

func main() {
	port := os.Getenv("TICKET_SERVICE_PORT")
	if port == "" {
		port = "8082"
	}

	cfg := config.LoadConfig()
	config.InitDatabase(cfg)
	config.InitSession(cfg.SessionSecret, cfg.SessionSecure)
	utils.InitTemplates()

	// Initialize services
	jwtService := utils.NewJWTService(cfg) // Ensure it's initialized if needed
	emailService := utils.NewEmailService(cfg)

	ticketService := services.NewTicketService(jwtService)
	// ticketService := NewEnhancedTicketService(baseTicketService, config.DB)

	dashboardService := services.NewDashboardService()
	kbService := services.NewKBService()
	settingsService := services.NewSettingsService()
	
	ticketHandler := handlers.NewTicketHandler(cfg, emailService, ticketService)
	dashboardHandler := handlers.NewDashboardHandler(cfg, dashboardService, kbService)
	settingsHandler := handlers.NewSettingsHandler(cfg, settingsService)

	// Auto-migrate models with retry
	log.Println("Migrating ticket models...")
	var err error
	for i := 0; i < 5; i++ {
		err = config.DB.AutoMigrate(
			&models.Company{},
			&models.Department{},
			&models.SLAPolicy{},
			&models.Ticket{},
			&models.TicketReply{},
			&models.TicketAttachment{},
			&models.TicketAssignmentHistory{},
			&models.TicketRating{},
			&models.TicketPriorityHistory{},
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
		log.Fatalf("AutoMigrate failed after 5 attempts: %v", err)
	}

	if err := models.SeedDefaultSLAPolicies(config.DB); err != nil {
		log.Printf("[Migration] Warning: SeedDefaultSLAPolicies encountered error: %v", err)
	}

	mux := http.NewServeMux()

	// Static files
	fs := http.FileServer(http.Dir("static"))
	mux.Handle("/static/", http.StripPrefix("/static/", fs))

	// Register routes
	mux.Handle("/dashboard", middleware.AuthRequired(middleware.PortalUserRequired(http.HandlerFunc(dashboardHandler.ShowDashboard))))
	mux.Handle("/dashboard/", middleware.AuthRequired(middleware.PortalUserRequired(http.HandlerFunc(dashboardHandler.ShowDashboard))))
	mux.Handle("/tiket", middleware.AuthRequired(middleware.PortalUserRequired(http.HandlerFunc(ticketHandler.ShowMyTickets))))
	mux.Handle("/tiket/", middleware.AuthRequired(middleware.PortalUserRequired(http.HandlerFunc(ticketHandler.HandleTicketDetail))))
	
	// Apply custom wrappers to CreateTicket
	createTicketHandler := InputSanitizer(ValidateTicketInput(http.HandlerFunc(ticketHandler.HandleCreateTicket)))
	mux.Handle("/kirim-tiket", middleware.AuthRequired(middleware.PortalUserRequired(createTicketHandler.ServeHTTP)))
	mux.Handle("/kirim-tiket/", middleware.AuthRequired(middleware.PortalUserRequired(createTicketHandler.ServeHTTP)))
	
	mux.Handle("/tiket/sukses/", middleware.AuthRequired(middleware.PortalUserRequired(http.HandlerFunc(ticketHandler.ShowTicketSuccess))))
	mux.Handle("/rating/", middleware.AuthRequired(http.HandlerFunc(ticketHandler.HandleRating)))
	mux.Handle("/settings", middleware.AuthRequired(middleware.PortalUserRequired(http.HandlerFunc(settingsHandler.HandleSettings))))
	mux.Handle("/settings/", middleware.AuthRequired(middleware.PortalUserRequired(http.HandlerFunc(settingsHandler.HandleSettings))))
	mux.Handle("/knowledge-base", middleware.AuthRequired(middleware.PortalUserRequired(http.HandlerFunc(dashboardHandler.ShowKnowledgeBase))))
	mux.Handle("/knowledge-base/", middleware.AuthRequired(middleware.PortalUserRequired(http.HandlerFunc(dashboardHandler.ShowKnowledgeBase))))
	mux.Handle("/knowledge-base/article/", middleware.AuthRequired(middleware.PortalUserRequired(http.HandlerFunc(dashboardHandler.ShowKBArticle))))
	mux.Handle("/api/kb/article/view", middleware.AuthRequired(middleware.PortalUserRequired(http.HandlerFunc(dashboardHandler.RecordKBArticleView))))

	// Health check
	mux.HandleFunc("/health", HealthCheckHandler)

	// Apply logging middleware
	loggedMux := middleware.LoggingMiddleware(mux)
	handler := http.MaxBytesHandler(loggedMux, 32<<20)

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: handler,
	}

	go func() {
		log.Printf("Ticket Service listening on port %s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("ListenAndServe error: %v", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down Ticket Service...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server Shutdown Failed: %v", err)
	}
	log.Println("Ticket Service exited gracefully")
}
