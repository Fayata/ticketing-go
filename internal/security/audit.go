package security

import (
	"log/slog"
	"time"
)

// Action constants for audit events
const (
	ActionLogin         = "LOGIN"
	ActionLoginFailed   = "LOGIN_FAILED"
	ActionRegister      = "REGISTER"
	ActionPasswordReset = "PASSWORD_RESET"
	ActionUserToggle    = "USER_TOGGLE"
	ActionRoleChange    = "ROLE_CHANGE"
	ActionTicketCreate  = "TICKET_CREATE"
	ActionTicketClose   = "TICKET_CLOSE"
	ActionFileUpload    = "FILE_UPLOAD"
)

// AuditEvent represents a security-relevant event
type AuditEvent struct {
	Timestamp  time.Time `json:"timestamp"`
	UserID     uint      `json:"user_id"`
	Action     string    `json:"action"`
	Resource   string    `json:"resource"`
	ResourceID string    `json:"resource_id"`
	IP         string    `json:"ip"`
	Details    string    `json:"details"`
	Success    bool      `json:"success"`
}

// LogAudit logs an audit event as structured JSON using slog
func LogAudit(event AuditEvent) {
	slog.Info("Audit Event",
		slog.Time("timestamp", event.Timestamp),
		slog.Uint64("user_id", uint64(event.UserID)),
		slog.String("action", event.Action),
		slog.String("resource", event.Resource),
		slog.String("resource_id", event.ResourceID),
		slog.String("ip", event.IP),
		slog.String("details", event.Details),
		slog.Bool("success", event.Success),
	)
}

// LogSecurityEvent is a convenience function for logging simple security events
func LogSecurityEvent(eventType, message, ip string, userID uint) {
	event := AuditEvent{
		Timestamp: time.Now(),
		UserID:    userID,
		Action:    eventType,
		IP:        ip,
		Details:   message,
		Success:   true,
	}
	LogAudit(event)
}
