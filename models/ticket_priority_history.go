package models

import (
	"time"
)

// TicketPriorityHistory records changes to a ticket's priority made by staff.
type TicketPriorityHistory struct {
	ID          uint           `gorm:"primarykey" json:"id"`
	TicketID    uint           `gorm:"not null;index" json:"ticket_id"`
	OldPriority TicketPriority `gorm:"not null" json:"old_priority"`
	NewPriority TicketPriority `gorm:"not null" json:"new_priority"`
	ChangedByID uint           `gorm:"not null;index" json:"changed_by_id"`
	Reason      string         `gorm:"type:text;not null" json:"reason"`
	CreatedAt   time.Time      `json:"created_at"`

	// Relations
	Ticket    Ticket `gorm:"foreignKey:TicketID" json:"ticket"`
	ChangedBy User   `gorm:"foreignKey:ChangedByID" json:"changed_by"`
}

// TableName returns the table name for TicketPriorityHistory.
func (TicketPriorityHistory) TableName() string {
	return "ticket_priority_histories"
}
