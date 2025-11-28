package models

import (
	"time"
)

type TicketReply struct {
	ID        uint      `gorm:"primarykey" json:"id"`
	TicketID  uint      `gorm:"not null" json:"ticket_id"`
	UserID    uint      `gorm:"not null" json:"user_id"`
	Message   string    `gorm:"type:text;not null" json:"message"`
	CreatedAt time.Time `json:"created_at"`

	// Relations
	Ticket Ticket `gorm:"foreignKey:TicketID" json:"ticket"`
	User   User   `gorm:"foreignKey:UserID" json:"user"`
}
