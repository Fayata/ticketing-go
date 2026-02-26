package services

import (
	"strconv"
	"time"

	"ticketing/config"
	"ticketing/models"
)

// FormatTimeAgo returns human-readable time ago string.
func FormatTimeAgo(t time.Time) string {
	now := time.Now()
	diff := now.Sub(t)
	if diff < time.Minute {
		return "Baru saja"
	}
	if diff < time.Hour {
		return strconv.Itoa(int(diff.Minutes())) + " menit lalu"
	}
	if diff < 24*time.Hour {
		return strconv.Itoa(int(diff.Hours())) + " jam lalu"
	}
	if diff < 7*24*time.Hour {
		days := int(diff.Hours() / 24)
		return strconv.Itoa(days) + " hari lalu"
	}
	return t.Format("02 Jan 2006")
}

type NotificationService struct{}

func NewNotificationService() *NotificationService {
	return &NotificationService{}
}

// NotificationItem for API response.
type NotificationItem struct {
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

// GetNotificationIconAndColor returns icon and color class for notification type.
func GetNotificationIconAndColor(notifType models.NotificationType) (icon, color string) {
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

// NotificationsResult for list API.
type NotificationsResult struct {
	Notifications []NotificationItem
	Counts        struct {
		All    int64 `json:"all"`
		Unread int64 `json:"unread"`
		Ticket int64 `json:"ticket"`
	}
}

// GetNotificationsWithCounts returns notifications and counts for user.
func (s *NotificationService) GetNotificationsWithCounts(userID uint, filter string) (*NotificationsResult, error) {
	var notifications []models.Notification
	query := config.DB.Where("user_id = ?", userID)
	if filter == "belum" {
		query = query.Where("is_read = ?", false)
	} else if filter == "tiket" {
		query = query.Where("type IN ?", []models.NotificationType{
			models.NotificationTypeTicket,
			models.NotificationTypeReply,
			models.NotificationTypeStatusChange,
		})
	}
	query.Preload("Ticket").Order("created_at DESC").Limit(50).Find(&notifications)

	responses := make([]NotificationItem, 0, len(notifications))
	for _, n := range notifications {
		icon, color := GetNotificationIconAndColor(n.Type)
		responses = append(responses, NotificationItem{
			ID:       n.ID,
			Type:     string(n.Type),
			Icon:     icon,
			Color:    color,
			Title:    n.Title,
			Desc:     n.Message,
			Time:     FormatTimeAgo(n.CreatedAt),
			Unread:   !n.IsRead,
			TicketID: n.TicketID,
		})
	}

	var allCount, unreadCount, ticketCount int64
	config.DB.Model(&models.Notification{}).Where("user_id = ?", userID).Count(&allCount)
	config.DB.Model(&models.Notification{}).Where("user_id = ? AND is_read = ?", userID, false).Count(&unreadCount)
	config.DB.Model(&models.Notification{}).Where("user_id = ? AND type IN ?", userID, []models.NotificationType{
		models.NotificationTypeTicket,
		models.NotificationTypeReply,
		models.NotificationTypeStatusChange,
	}).Count(&ticketCount)

	result := &NotificationsResult{Notifications: responses}
	result.Counts.All = allCount
	result.Counts.Unread = unreadCount
	result.Counts.Ticket = ticketCount
	return result, nil
}

// MarkOneAsRead marks a single notification as read. Returns true if found and updated.
func (s *NotificationService) MarkOneAsRead(userID uint, notifID uint) (bool, error) {
	var notif models.Notification
	if err := config.DB.Where("id = ? AND user_id = ?", notifID, userID).First(&notif).Error; err != nil {
		return false, err
	}
	return true, notif.MarkAsRead(config.DB)
}

// MarkAllAsReadForUser marks all notifications for user as read.
func (s *NotificationService) MarkAllAsReadForUser(userID uint) error {
	return models.MarkAllAsRead(config.DB, userID)
}

// GetUnreadCountForUser returns unread notification count.
func (s *NotificationService) GetUnreadCountForUser(userID uint) (int64, error) {
	return models.GetUnreadCount(config.DB, userID)
}
