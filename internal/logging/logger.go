package logging

import (
	"context"
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

var (
	once sync.Once

	// Base directories and writers
	logWriters []io.Closer

	// HTTP & UI sub-loggers
	HTTPAccess   *slog.Logger
	HTTPTemplate *slog.Logger

	// Database sub-loggers
	DBQueries     *slog.Logger
	DBSlowQueries *slog.Logger
	DBMigrations  *slog.Logger
	DBErrors      *slog.Logger

	// Auth sub-loggers
	AuthLogin         *slog.Logger
	AuthRegister      *slog.Logger
	AuthPasswordReset *slog.Logger
	AuthEmailVerify   *slog.Logger
	AuthSecurity      *slog.Logger

	// Tickets sub-loggers
	TicketLifecycle   *slog.Logger
	TicketChat        *slog.Logger
	TicketAttachments *slog.Logger
	TicketRatings     *slog.Logger

	// SLA sub-loggers
	SLACalculations *slog.Logger
	SLAResponses    *slog.Logger
	SLABreaches     *slog.Logger
	SLAEscalations  *slog.Logger

	// Notifications sub-loggers
	NotificationInApp *slog.Logger
	NotificationEmail *slog.Logger
	NotificationWS    *slog.Logger

	// System sub-loggers
	SystemGateway   *slog.Logger
	SystemPanic     *slog.Logger
	SystemLifecycle *slog.Logger

	// Admin sub-loggers
	AdminReports *slog.Logger
	AdminAudit   *slog.Logger

	// Group loggers (shortcuts)
	HTTP          *slog.Logger
	DB            *slog.Logger
	Auth          *slog.Logger
	Tickets       *slog.Logger
	SLA           *slog.Logger
	Notifications *slog.Logger
	Admin         *slog.Logger
	System        *slog.Logger
)

func init() {
	// Initialize default fallback loggers to stdout before Init() is explicitly called
	initFallbackLoggers("app")
}

func initFallbackLoggers(serviceName string) {
	h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})
	base := slog.New(h).With("service", serviceName)

	HTTPAccess = base.With("category", "http_ui", "subcategory", "access")
	HTTPTemplate = base.With("category", "http_ui", "subcategory", "template")

	DBQueries = base.With("category", "db", "subcategory", "queries")
	DBSlowQueries = base.With("category", "db", "subcategory", "slow_queries")
	DBMigrations = base.With("category", "db", "subcategory", "migrations")
	DBErrors = base.With("category", "db", "subcategory", "errors")

	AuthLogin = base.With("category", "auth", "subcategory", "login")
	AuthRegister = base.With("category", "auth", "subcategory", "register")
	AuthPasswordReset = base.With("category", "auth", "subcategory", "password_reset")
	AuthEmailVerify = base.With("category", "auth", "subcategory", "email_verify")
	AuthSecurity = base.With("category", "auth", "subcategory", "security")

	TicketLifecycle = base.With("category", "tickets", "subcategory", "lifecycle")
	TicketChat = base.With("category", "tickets", "subcategory", "chat")
	TicketAttachments = base.With("category", "tickets", "subcategory", "attachments")
	TicketRatings = base.With("category", "tickets", "subcategory", "ratings")

	SLACalculations = base.With("category", "sla", "subcategory", "calculations")
	SLAResponses = base.With("category", "sla", "subcategory", "responses")
	SLABreaches = base.With("category", "sla", "subcategory", "breaches")
	SLAEscalations = base.With("category", "sla", "subcategory", "escalations")

	NotificationInApp = base.With("category", "notifications", "subcategory", "in_app")
	NotificationEmail = base.With("category", "notifications", "subcategory", "email")
	NotificationWS = base.With("category", "notifications", "subcategory", "websocket")

	SystemGateway = base.With("category", "system", "subcategory", "gateway")
	SystemPanic = base.With("category", "system", "subcategory", "panic")
	SystemLifecycle = base.With("category", "system", "subcategory", "lifecycle")

	AdminReports = base.With("category", "admin", "subcategory", "reports")
	AdminAudit = base.With("category", "admin", "subcategory", "audit")

	HTTP = HTTPAccess
	DB = DBQueries
	Auth = AuthLogin
	Tickets = TicketLifecycle
	SLA = SLACalculations
	Notifications = NotificationInApp
	Admin = AdminReports
	System = SystemLifecycle

	slog.SetDefault(SystemLifecycle)
}

// Init initializes the logging system with dedicated JSON rotating log files under baseDir.
// If baseDir is omitted, it looks up the LOG_DIR env variable, defaulting to "./logs".
func Init(serviceName string, customBaseDir ...string) {
	once.Do(func() {
		baseDir := os.Getenv("LOG_DIR")
		if len(customBaseDir) > 0 && customBaseDir[0] != "" {
			baseDir = customBaseDir[0]
		}
		if baseDir == "" {
			baseDir = "./logs"
		}

		makeLogger := func(category, subcategory, filename string) *slog.Logger {
			filePath := filepath.Join(baseDir, category, filename)
			rotater, err := NewRotatingFileWriter(filePath, 10*1024*1024, 5)
			var writer io.Writer = os.Stdout
			if err == nil {
				logWriters = append(logWriters, rotater)
				writer = io.MultiWriter(os.Stdout, rotater)
			} else {
				fmt.Printf("[Logging] Warning: could not create rotating writer for %s: %v\n", filePath, err)
			}

			handler := slog.NewJSONHandler(writer, &slog.HandlerOptions{
				Level: slog.LevelDebug,
			})
			return slog.New(handler).With(
				"service", serviceName,
				"category", category,
				"subcategory", subcategory,
			)
		}

		// HTTP & UI
		HTTPAccess = makeLogger("http_ui", "access", "access.json.log")
		HTTPTemplate = makeLogger("http_ui", "template", "template.json.log")

		// Database
		DBQueries = makeLogger("db", "queries", "queries.json.log")
		DBSlowQueries = makeLogger("db", "slow_queries", "slow_queries.json.log")
		DBMigrations = makeLogger("db", "migrations", "migrations.json.log")
		DBErrors = makeLogger("db", "errors", "errors.json.log")

		// Auth
		AuthLogin = makeLogger("auth", "login", "login.json.log")
		AuthRegister = makeLogger("auth", "register", "register.json.log")
		AuthPasswordReset = makeLogger("auth", "password_reset", "password_reset.json.log")
		AuthEmailVerify = makeLogger("auth", "email_verify", "email_verify.json.log")
		AuthSecurity = makeLogger("auth", "security", "security.json.log")

		// Tickets
		TicketLifecycle = makeLogger("tickets", "lifecycle", "lifecycle.json.log")
		TicketChat = makeLogger("tickets", "chat", "chat.json.log")
		TicketAttachments = makeLogger("tickets", "attachments", "attachments.json.log")
		TicketRatings = makeLogger("tickets", "ratings", "ratings.json.log")

		// SLA
		SLACalculations = makeLogger("sla", "calculations", "calculations.json.log")
		SLAResponses = makeLogger("sla", "responses", "responses.json.log")
		SLABreaches = makeLogger("sla", "breaches", "breaches.json.log")
		SLAEscalations = makeLogger("sla", "escalations", "escalations.json.log")

		// Notifications
		NotificationInApp = makeLogger("notifications", "in_app", "in_app.json.log")
		NotificationEmail = makeLogger("notifications", "email", "email.json.log")
		NotificationWS = makeLogger("notifications", "websocket", "websocket.json.log")

		// System
		SystemGateway = makeLogger("system", "gateway", "gateway.json.log")
		SystemPanic = makeLogger("system", "panic", "panic.json.log")
		SystemLifecycle = makeLogger("system", "lifecycle", "lifecycle.json.log")

		// Admin
		AdminReports = makeLogger("admin", "reports", "reports.json.log")
		AdminAudit = makeLogger("admin", "audit", "audit.json.log")

		// Group shortcuts
		HTTP = HTTPAccess
		DB = DBQueries
		Auth = AuthLogin
		Tickets = TicketLifecycle
		SLA = SLACalculations
		Notifications = NotificationInApp
		Admin = AdminReports
		System = SystemLifecycle

		slog.SetDefault(SystemLifecycle)

		// Bridge Go's standard log package output into SystemLifecycle
		log.SetOutput(&stdLogBridge{logger: SystemLifecycle})
		log.SetFlags(0) // Timestamp already handled by slog JSON handler
	})
}

// Close closes all open log file writers.
func Close() {
	for _, w := range logWriters {
		_ = w.Close()
	}
}

type stdLogBridge struct {
	logger *slog.Logger
}

func (b *stdLogBridge) Write(p []byte) (n int, err error) {
	msg := string(p)
	if len(msg) > 0 && msg[len(msg)-1] == '\n' {
		msg = msg[:len(msg)-1]
	}
	b.logger.Info(msg, "source", "std_log")
	return len(p), nil
}

// CorrelationContext creates or returns a context containing correlation ID.
type correlationKey struct{}

func WithCorrelationID(ctx context.Context, corrID string) context.Context {
	return context.WithValue(ctx, correlationKey{}, corrID)
}

func GetCorrelationID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if val, ok := ctx.Value(correlationKey{}).(string); ok {
		return val
	}
	return ""
}
