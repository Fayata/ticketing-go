package handlers

import (
	"net/http"

	"ticketing/config"
	"ticketing/models"
)

type DashboardHandler struct {
	cfg *config.Config
}

func NewDashboardHandler(cfg *config.Config) *DashboardHandler {
	return &DashboardHandler{cfg: cfg}
}

func (h *DashboardHandler) ShowDashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user := GetUserFromContext(r).(*models.User)
	activeTicketsCount := GetActiveTicketsCount(r)

	var waitingCount, inProgressCount, closedCount, totalCount int64

	config.DB.Model(&models.Ticket{}).
		Where("created_by_id = ? AND status = ?", user.ID, models.StatusWaiting).
		Count(&waitingCount)

	config.DB.Model(&models.Ticket{}).
		Where("created_by_id = ? AND status = ?", user.ID, models.StatusInProgress).
		Count(&inProgressCount)

	config.DB.Model(&models.Ticket{}).
		Where("created_by_id = ? AND status = ?", user.ID, models.StatusClosed).
		Count(&closedCount)

	config.DB.Model(&models.Ticket{}).
		Where("created_by_id = ?", user.ID).
		Count(&totalCount)

	// Get recent tickets
	var recentTickets []*models.Ticket
	config.DB.Preload("Department").
		Preload("Replies").
		Where("created_by_id = ?", user.ID).
		Order("created_at DESC").
		Limit(5).
		Find(&recentTickets)

	data := AddBaseData(r, map[string]interface{}{
		"title":                "Dashboard - Portal Ticketing",
		"page_title":           "Dashboard",
		"page_subtitle":        "Selamat datang kembali, " + user.GetFullName() + "!",
		"nav_active":           "dashboard",
		"template_name":        "tickets/dashboard",
		"user":                 user,
		"active_tickets_count": activeTicketsCount,
		"waiting_tickets":      waitingCount,
		"in_progress_tickets":  inProgressCount,
		"closed_tickets":       closedCount,
		"total_tickets":        totalCount,
		"recent_tickets":       recentTickets,
		"announcements":        []interface{}{},
		"popular_articles":     []interface{}{},
		"unread_count":         0,
	})

	RenderTemplate(w, "tickets/dashboard", data)
}
