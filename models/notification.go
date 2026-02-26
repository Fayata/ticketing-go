package models

import (
	"time"
	"gorm.io/gorm"
)

type NotificationType string

const (
	NotificationTypeTicket      NotificationType = "tiket"
	NotificationTypeReply       NotificationType = "reply"
	NotificationTypeStatusChange NotificationType = "status"
	NotificationTypeSystem      NotificationType = "sistem"
)

type Notification struct {
	ID        uint             `gorm:"primaryKey" json:"id"`
	UserID    uint             `gorm:"not null;index" json:"user_id"`
	User      User             `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Type      NotificationType `gorm:"type:varchar(20);not null" json:"type"`
	Title     string           `gorm:"type:varchar(255);not null" json:"title"`
	Message   string           `gorm:"type:text" json:"message"`
	IsRead    bool             `gorm:"default:false;index" json:"is_read"`
	TicketID  *uint            `gorm:"index" json:"ticket_id,omitempty"`
	Ticket    *Ticket          `gorm:"foreignKey:TicketID" json:"ticket,omitempty"`
	CreatedAt time.Time        `json:"created_at"`
	ReadAt    *time.Time       `json:"read_at,omitempty"`
}

func (Notification) TableName() string {
	return "notifications"
}

// Buat notif baru (dipake pas ada tiket/balasan/status berubah)
func CreateNotification(db *gorm.DB, userID uint, notifType NotificationType, title, message string, ticketID *uint) error {
	notif := Notification{
		UserID:  userID,
		Type:    notifType,
		Title:   title,
		Message: message,
		TicketID: ticketID,
		IsRead:  false,
	}
	return db.Create(&notif).Error
}

// Tandai notif ini sudah dibaca
func (n *Notification) MarkAsRead(db *gorm.DB) error {
	now := time.Now()
	return db.Model(n).Updates(map[string]interface{}{
		"is_read": true,
		"read_at": &now,
	}).Error
}

// Tandai semua notif user sebagai dibaca
func MarkAllAsRead(db *gorm.DB, userID uint) error {
	now := time.Now()
	return db.Model(&Notification{}).
		Where("user_id = ? AND is_read = ?", userID, false).
		Updates(map[string]interface{}{
			"is_read": true,
			"read_at": &now,
		}).Error
}

// Hitung notif belum dibaca (buat badge)
func GetUnreadCount(db *gorm.DB, userID uint) (int64, error) {
	var count int64
	err := db.Model(&Notification{}).
		Where("user_id = ? AND is_read = ?", userID, false).
		Count(&count).Error
	return count, err
}

// Hitung notif balasan (reply) yang belum dibaca — untuk badge "Tiket Saya"
func GetUnreadRepliesCount(db *gorm.DB, userID uint) (int64, error) {
	var count int64
	err := db.Model(&Notification{}).
		Where("user_id = ? AND type = ? AND is_read = ?", userID, NotificationTypeReply, false).
		Count(&count).Error
	return count, err
}
