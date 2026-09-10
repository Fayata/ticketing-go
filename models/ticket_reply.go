package models

import (
	"time"
)

type TicketReply struct {
	ID          uint               `gorm:"primarykey" json:"id"`
	TicketID    uint               `gorm:"not null;index" json:"ticket_id"`
	UserID      uint               `gorm:"not null;index" json:"user_id"`
	Message     string             `gorm:"type:text;not null" json:"message"`
	IsRead      bool               `gorm:"default:false;index" json:"is_read"`
	IsDelivered bool               `gorm:"default:false;index" json:"is_delivered"`
	ReadAt      *time.Time         `json:"read_at"`
	CreatedAt   time.Time          `json:"created_at"`

	// Relations
	Ticket      Ticket             `gorm:"foreignKey:TicketID" json:"ticket"`
	User        User               `gorm:"foreignKey:UserID" json:"user"`
	Attachments []TicketAttachment `gorm:"foreignKey:ReplyID;constraint:OnDelete:CASCADE;" json:"attachments"`
}

// GetReadStatusClass returns "read" (centang 2 biru), "delivered" (centang 2 abu), or "sent" (centang 1 abu)
func (r *TicketReply) GetReadStatusClass() string {
	if r == nil {
		return "sent"
	}
	if r.IsRead {
		return "read"
	}
	if r.IsDelivered {
		return "delivered"
	}
	return "sent"
}

// GetReadStatusCheck returns "✓✓" for read/delivered, or "✓" for sent
func (r *TicketReply) GetReadStatusCheck() string {
	if r == nil {
		return "✓"
	}
	if r.IsRead || r.IsDelivered {
		return "✓✓"
	}
	return "✓"
}

// GetReadStatusTitle returns user-friendly tooltip description
func (r *TicketReply) GetReadStatusTitle() string {
	if r == nil {
		return "Terkirim"
	}
	if r.IsRead {
		return "Sudah Dibaca"
	}
	if r.IsDelivered {
		return "Tersampaikan (Lawan Bicara Online)"
	}
	return "Terkirim (Lawan Bicara Offline)"
}
