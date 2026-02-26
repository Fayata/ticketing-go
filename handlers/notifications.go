package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"ticketing/config"
	"ticketing/models"
	"ticketing/utils"
)

type NotificationHandler struct {
	cfg *config.Config
}

func NewNotificationHandler(cfg *config.Config) *NotificationHandler {
	return &NotificationHandler{cfg: cfg}
}

// List notifikasi user. Query param filter: semua | belum | tiket.
func (h *NotificationHandler) GetNotifications(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	user := GetUserFromContext(r).(*models.User)
	filter := r.URL.Query().Get("filter")
	
	var notifications []models.Notification
	query := config.DB.Where("user_id = ?", user.ID)
	
	if filter == "belum" {
		query = query.Where("is_read = ?", false)
	} else if filter == "tiket" {
		query = query.Where("type IN ?", []models.NotificationType{
			models.NotificationTypeTicket,
			models.NotificationTypeReply,
			models.NotificationTypeStatusChange,
		})
	}
	
	query.Preload("Ticket").
		Order("created_at DESC").
		Limit(50).
		Find(&notifications)
	
	type NotificationResponse struct {
		ID       uint   `json:"id"`
		Type     string `json:"type"`
		Icon     string `json:"icon"`
		Color    string `json:"color"`
		Title    string `json:"title"`
		Desc     string `json:"desc"`
		Time     string `json:"time"`
		Unread   bool   `json:"unread"`
		TicketID *uint  `json:"ticket_id,omitempty"`
	}
	
	responses := make([]NotificationResponse, 0, len(notifications))
	for _, n := range notifications {
		icon, color := getNotificationIconAndColor(n.Type)
		responses = append(responses, NotificationResponse{
			ID:       n.ID,
			Type:     string(n.Type),
			Icon:     icon,
			Color:    color,
			Title:    n.Title,
			Desc:     n.Message,
			Time:     formatTimeAgo(n.CreatedAt),
			Unread:   !n.IsRead,
			TicketID: n.TicketID,
		})
	}
	
	var allCount, unreadCount, ticketCount int64
	config.DB.Model(&models.Notification{}).Where("user_id = ?", user.ID).Count(&allCount)
	config.DB.Model(&models.Notification{}).Where("user_id = ? AND is_read = ?", user.ID, false).Count(&unreadCount)
	config.DB.Model(&models.Notification{}).Where("user_id = ? AND type IN ?", user.ID, []models.NotificationType{
		models.NotificationTypeTicket,
		models.NotificationTypeReply,
		models.NotificationTypeStatusChange,
	}).Count(&ticketCount)
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"notifications": responses,
		"counts": map[string]int64{
			"all":    allCount,
			"unread": unreadCount,
			"ticket": ticketCount,
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
	var notif models.Notification
	if err := config.DB.Where("id = ? AND user_id = ?", notifID, user.ID).First(&notif).Error; err != nil {
		http.Error(w, "Notification not found", http.StatusNotFound)
		return
	}
	if err := notif.MarkAsRead(config.DB); err != nil {
		http.Error(w, "Failed to mark as read", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
	})
}

// Tandai semua notif user sebagai dibaca.
func (h *NotificationHandler) MarkAllAsRead(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user := GetUserFromContext(r).(*models.User)
	
	if err := models.MarkAllAsRead(config.DB, user.ID); err != nil {
		http.Error(w, "Failed to mark all as read", http.StatusInternalServerError)
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
	})
}

// Jumlah notif belum dibaca (buat badge di navbar).
func (h *NotificationHandler) GetUnreadCount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	user := GetUserFromContext(r).(*models.User)
	
	count, err := models.GetUnreadCount(config.DB, user.ID)
	if err != nil {
		http.Error(w, "Failed to get count", http.StatusInternalServerError)
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"count": count,
	})
}

// icon & warna per tipe notif (buat tampilan di panel)
func getNotificationIconAndColor(notifType models.NotificationType) (icon, color string) {
	switch notifType {
	case models.NotificationTypeTicket:
		return "ticket", "ic-red"
	case models.NotificationTypeReply:
		return "reply", "ic-blue"
	case models.NotificationTypeStatusChange:
		return "check", "ic-green"
	case models.NotificationTypeSystem:
		return "alert", "ic-amber"
	default:
		return "info", "ic-blue"
	}
}

func formatTimeAgo(t time.Time) string {
	now := time.Now()
	diff := now.Sub(t)
	
	if diff < time.Minute {
		return "Baru saja"
	}
	if diff < time.Hour {
		minutes := int(diff.Minutes())
		return strconv.Itoa(minutes) + " menit lalu"
	}
	if diff < 24*time.Hour {
		hours := int(diff.Hours())
		return strconv.Itoa(hours) + " jam lalu"
	}
	if diff < 7*24*time.Hour {
		days := int(diff.Hours() / 24)
		return strconv.Itoa(days) + " hari lalu"
	}
	return t.Format("02 Jan 2006")
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
