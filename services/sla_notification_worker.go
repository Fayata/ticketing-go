package services

import (
	"fmt"
	"log"
	"time"

	"gorm.io/gorm"

	"ticketing/internal/logging"
	"ticketing/models"
)

// slaWarningWindow is how long before deadline a "mendekati batas" warning is sent.
const slaWarningWindow = 30 * time.Minute

// slaCheckInterval is how often the background worker polls the database.
const slaCheckInterval = 5 * time.Minute

// StartSLANotificationWorker runs a background goroutine that periodically
// checks for tickets approaching or past their SLA first-response deadline
// and sends notifications to all staff in the relevant department.
//
// It must be started after the database is initialized, e.g.:
//
//	go services.StartSLANotificationWorker(config.DB)
func StartSLANotificationWorker(db *gorm.DB) {
	logging.SystemLifecycle.Info("SLA Notification Worker started", "interval", slaCheckInterval.String())
	log.Println("[SLA Worker] Started – checking every", slaCheckInterval)
	ticker := time.NewTicker(slaCheckInterval)
	defer ticker.Stop()

	// Run once immediately on startup, then on every tick.
	runSLACheck(db)
	for range ticker.C {
		runSLACheck(db)
	}
}

// runSLACheck is the single iteration of the SLA check loop.
func runSLACheck(db *gorm.DB) {
	now := time.Now()

	// Find all open tickets that have a first-response deadline but no response yet.
	var tickets []models.Ticket
	if err := db.
		Preload("Department").
		Where("first_response_at IS NULL").
		Where("first_response_deadline IS NOT NULL").
		Where("status != ?", models.StatusClosed).
		Find(&tickets).Error; err != nil {
		log.Printf("[SLA Worker] Error querying tickets: %v", err)
		return
	}

	for i := range tickets {
		t := &tickets[i]

		if t.FirstResponseDeadline == nil || t.DepartmentID == nil {
			continue
		}

		deadline := *t.FirstResponseDeadline
		timeLeft := deadline.Sub(now)

		switch {
		case now.After(deadline):
			// SLA already breached
			if !t.SLABreachSent {
				logging.SLABreaches.Warn("SLA first response deadline breached",
					"ticket_id", t.ID,
					"ticket_number", t.GetTicketNumber(),
					"department_id", *t.DepartmentID,
					"deadline", deadline,
				)
				if err := sendSLANotification(db, t, true); err != nil {
					logging.SLABreaches.Error("Failed to send breach notification",
						"ticket_id", t.ID,
						"error", err.Error(),
					)
					continue
				}
				// Mark breach sent so we don't spam
				db.Model(t).Update("sla_breach_sent", true)
				log.Printf("[SLA Worker] Breach notification sent for ticket %d (dept %d)", t.ID, *t.DepartmentID)
			}

		case timeLeft <= slaWarningWindow:
			// Approaching deadline – warning
			if !t.SLAWarningSent {
				logging.SLAEscalations.Warn("SLA first response deadline approaching",
					"ticket_id", t.ID,
					"ticket_number", t.GetTicketNumber(),
					"department_id", *t.DepartmentID,
					"time_left", formatDuration(timeLeft),
				)
				if err := sendSLANotification(db, t, false); err != nil {
					logging.SLAEscalations.Error("Failed to send warning notification",
						"ticket_id", t.ID,
						"error", err.Error(),
					)
					continue
				}
				db.Model(t).Update("sla_warning_sent", true)
				log.Printf("[SLA Worker] Warning notification sent for ticket %d (dept %d, sisa %s)", t.ID, *t.DepartmentID, formatDuration(timeLeft))
			}
		}
	}
}

// sendSLANotification fetches all active staff in the ticket's department and
// creates a notification for each one.
func sendSLANotification(db *gorm.DB, ticket *models.Ticket, isBreached bool) error {
	if ticket.DepartmentID == nil {
		return nil
	}

	// Fetch all active staff members in this department
	var staffList []models.User
	if err := db.
		Where("department_id = ? AND is_staff = ? AND is_active = ?", *ticket.DepartmentID, true, true).
		Find(&staffList).Error; err != nil {
		return fmt.Errorf("query staff: %w", err)
	}

	if len(staffList) == 0 {
		return nil
	}

	ticketID := ticket.ID
	title, message := buildNotificationContent(ticket, isBreached)

	for _, staff := range staffList {
		if err := models.CreateNotification(
			db,
			staff.ID,
			models.NotificationTypeSystem,
			title,
			message,
			&ticketID,
		); err != nil {
			log.Printf("[SLA Worker] Failed to notify staff %d: %v", staff.ID, err)
		}
	}
	return nil
}

// buildNotificationContent returns the title and message for an SLA notification.
func buildNotificationContent(ticket *models.Ticket, isBreached bool) (title, message string) {
	deptName := "Departemen Anda"
	if ticket.Department != nil {
		deptName = ticket.Department.Name
	}

	ticketNum := ticket.GetTicketNumber()
	priority := ticket.GetPriorityDisplay()

	if isBreached {
		now := time.Now()
		overdue := now.Sub(*ticket.FirstResponseDeadline)
		title = fmt.Sprintf("⚠️ SLA Breached – %s", ticketNum)
		message = fmt.Sprintf(
			"Tiket [%s] \"%s\" (Prioritas: %s) di departemen %s telah melewati batas waktu respon pertama sebesar %s. Segera tangani tiket ini.",
			ticketNum, ticket.Title, priority, deptName, formatDuration(overdue),
		)
	} else {
		timeLeft := ticket.FirstResponseDeadline.Sub(time.Now())
		title = fmt.Sprintf("⏰ SLA Mendekati Batas – %s", ticketNum)
		message = fmt.Sprintf(
			"Tiket [%s] \"%s\" (Prioritas: %s) di departemen %s akan melewati batas waktu respon pertama dalam %s. Segera ambil tindakan.",
			ticketNum, ticket.Title, priority, deptName, formatDuration(timeLeft),
		)
	}
	return
}

// formatDuration formats a duration into a human-friendly Indonesian string.
func formatDuration(d time.Duration) string {
	if d < 0 {
		d = -d
	}
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60

	if hours >= 24 {
		days := hours / 24
		remH := hours % 24
		if remH > 0 {
			return fmt.Sprintf("%dh %dj", days, remH)
		}
		return fmt.Sprintf("%dh", days)
	}
	if hours > 0 {
		if minutes > 0 {
			return fmt.Sprintf("%dj %dm", hours, minutes)
		}
		return fmt.Sprintf("%dj", hours)
	}
	if minutes > 0 {
		return fmt.Sprintf("%dm", minutes)
	}
	return "<1m"
}
