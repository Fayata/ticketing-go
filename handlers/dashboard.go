package handlers

import (
	"net/http"

	"ticketing/config"
	"ticketing/models"
	"ticketing/services"
)

type DashboardHandler struct {
	cfg              *config.Config
	dashboardService  *services.DashboardService
	kbService        *services.KBService
}

func NewDashboardHandler(cfg *config.Config, dashboardService *services.DashboardService, kbService *services.KBService) *DashboardHandler {
	return &DashboardHandler{cfg: cfg, dashboardService: dashboardService, kbService: kbService}
}

func (h *DashboardHandler) ShowDashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user := GetUserFromContext(r).(*models.User)
	activeTicketsCount := GetActiveTicketsCount(r)

	dataOut, err := h.dashboardService.GetDashboardData(user.ID)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	data := AddBaseData(r, map[string]interface{}{
		"title":                "Dashboard - Portal Ticketing",
		"page_title":           "Dashboard",
		"page_subtitle":        "Selamat datang kembali, " + user.GetFullName() + "!",
		"nav_active":           "dashboard",
		"template_name":        "tickets/dashboard",
		"user":                 user,
		"active_tickets_count": activeTicketsCount,
		"waiting_tickets":      dataOut.WaitingCount,
		"in_progress_tickets":  dataOut.InProgressCount,
		"closed_tickets":       dataOut.ClosedCount,
		"total_tickets":        dataOut.TotalCount,
		"recent_tickets":       dataOut.RecentTickets,
		"announcements":        []interface{}{},
		"popular_articles":     dataOut.PopularArticles,
		"unread_count":         dataOut.UnreadCount,
	})

	RenderTemplate(w, "tickets/dashboard", data)
}
