package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

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

// GetLiveDashboardAPI returns current user dashboard data (KPIs, recent tickets) as JSON for live updates.
func (h *DashboardHandler) GetLiveDashboardAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user, ok := GetUserFromContext(r).(*models.User)
	if !ok || user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	dataOut, err := h.dashboardService.GetDashboardData(user.ID)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	type recentTicketJSON struct {
		ID              uint   `json:"id"`
		TicketNumber    string `json:"ticket_number"`
		Title           string `json:"title"`
		Status          string `json:"status"`
		StatusDisplay   string `json:"status_display"`
		StatusClass     string `json:"status_class"`
		Priority        string `json:"priority"`
		PriorityDisplay string `json:"priority_display"`
		PriorityClass   string `json:"priority_class"`
		DepartmentName  string `json:"department_name"`
		ReplyCount      int    `json:"reply_count"`
		CreatedAt       string `json:"created_at"`
		CreatedAtAgo    string `json:"created_at_ago"`
	}

	recent := make([]recentTicketJSON, 0, len(dataOut.RecentTickets))
	for _, t := range dataOut.RecentTickets {
		deptName := "Umum"
		if t.Department != nil {
			deptName = t.Department.Name
		}
		statusClass := strings.ToLower(string(t.Status))
		priorityClass := strings.ToLower(string(t.Priority))

		recent = append(recent, recentTicketJSON{
			ID:              t.ID,
			TicketNumber:    t.GetTicketNumber(),
			Title:           t.Title,
			Status:          string(t.Status),
			StatusDisplay:   t.GetStatusDisplay(),
			StatusClass:     statusClass,
			Priority:        string(t.Priority),
			PriorityDisplay: t.GetPriorityDisplay(),
			PriorityClass:   priorityClass,
			DepartmentName:  deptName,
			ReplyCount:      t.GetReplyCount(),
			CreatedAt:       t.CreatedAt.Format("02 Jan 2006, 15:04"),
			CreatedAtAgo:    timeSinceShort(t.CreatedAt),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"waiting_tickets":     dataOut.WaitingCount,
		"in_progress_tickets": dataOut.InProgressCount,
		"closed_tickets":      dataOut.ClosedCount,
		"total_tickets":       dataOut.TotalCount,
		"recent_tickets":      recent,
	})
}

func timeSinceShort(t time.Time) string {
	diff := time.Since(t)
	days := int(diff.Hours() / 24)
	hours := int(diff.Hours())
	minutes := int(diff.Minutes())
	if days > 0 {
		return fmt.Sprintf("%d hari lalu", days)
	}
	if hours > 0 {
		return fmt.Sprintf("%d jam lalu", hours)
	}
	if minutes > 0 {
		return fmt.Sprintf("%d menit lalu", minutes)
	}
	return "Baru saja"
}
