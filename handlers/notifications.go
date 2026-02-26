package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"ticketing/config"
	"ticketing/models"
	"ticketing/services"
	"ticketing/utils"
)

type NotificationHandler struct {
	cfg                 *config.Config
	notificationService *services.NotificationService
}

func NewNotificationHandler(cfg *config.Config, notificationService *services.NotificationService) *NotificationHandler {
	return &NotificationHandler{cfg: cfg, notificationService: notificationService}
}

// List notifikasi user. Query param filter: semua | belum | tiket.
func (h *NotificationHandler) GetNotifications(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	user := GetUserFromContext(r).(*models.User)
	filter := r.URL.Query().Get("filter")

	result, err := h.notificationService.GetNotificationsWithCounts(user.ID, filter)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"notifications": result.Notifications,
		"counts": map[string]int64{
			"all":    result.Counts.All,
			"unread": result.Counts.Unread,
			"ticket": result.Counts.Ticket,
		},
	})
}

// Tandai satu notif sebagai dibaca (dipanggil pas user klik notif).
func (h *NotificationHandler) MarkAsRead(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	user := GetUserFromContext(r).(*models.User)
	r.ParseForm()
	notifIDStr := r.FormValue("id")
	if notifIDStr == "" {
		http.Error(w, "Notification ID required", http.StatusBadRequest)
		return
	}
	notifID, err := strconv.ParseUint(notifIDStr, 10, 32)
	if err != nil {
		http.Error(w, "Invalid notification ID", http.StatusBadRequest)
		return
	}
	ok, err := h.notificationService.MarkOneAsRead(user.ID, uint(notifID))
	if err != nil || !ok {
		http.Error(w, "Failed to mark as read", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
}

// Tandai semua notif user sebagai dibaca.
func (h *NotificationHandler) MarkAllAsRead(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	user := GetUserFromContext(r).(*models.User)
	if err := h.notificationService.MarkAllAsReadForUser(user.ID); err != nil {
		http.Error(w, "Failed to mark all as read", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
}

// Jumlah notif belum dibaca (buat badge di navbar).
func (h *NotificationHandler) GetUnreadCount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	user := GetUserFromContext(r).(*models.User)
	count, err := h.notificationService.GetUnreadCountForUser(user.ID)
	if err != nil {
		http.Error(w, "Failed to get count", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"count": count})
}

// Buat notif ke pemilik tiket pas ada balasan.
func CreateNotificationForTicketReply(ticket *models.Ticket, reply *models.TicketReply) {
	// Notify ticket owner if reply is from staff
	if reply.User.IsStaff && ticket.CreatedByID != reply.UserID {
		models.CreateNotification(
			config.DB,
			ticket.CreatedByID,
			models.NotificationTypeReply,
			"Balasan dari tim support",
			reply.User.GetFullName()+" membalas tiket "+ticket.GetTicketNumber()+": "+utils.TruncateString(reply.Message, 80),
			&ticket.ID,
		)
	}
	// Notify staff if reply is from ticket owner
	if !reply.User.IsStaff {
		// Notify assigned staff if any
		if ticket.AssignedToID != nil {
			models.CreateNotification(
				config.DB,
				*ticket.AssignedToID,
				models.NotificationTypeReply,
				"Balasan dari pengguna",
				ticket.CreatedBy.GetFullName()+" membalas tiket "+ticket.GetTicketNumber()+": "+utils.TruncateString(reply.Message, 80),
				&ticket.ID,
			)
		}
	}
}

// Buat notif pas status tiket berubah (misal ditutup).
func CreateNotificationForTicketStatusChange(ticket *models.Ticket, oldStatus models.TicketStatus) {
	if ticket.Status == models.StatusClosed && oldStatus != models.StatusClosed {
		models.CreateNotification(
			config.DB,
			ticket.CreatedByID,
			models.NotificationTypeStatusChange,
			"Tiket "+ticket.GetTicketNumber()+" selesai",
			"Tiket Anda telah ditandai selesai oleh tim support.",
			&ticket.ID,
		)
	}
}
