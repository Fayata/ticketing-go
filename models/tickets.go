package models

import (
	"time"

	"gorm.io/gorm"
)

type TicketStatus string
type TicketPriority string

const (
	StatusWaiting    TicketStatus = "WAITING"
	StatusInProgress TicketStatus = "IN_PROGRESS"
	StatusClosed     TicketStatus = "CLOSED"

	PriorityLow    TicketPriority = "LOW"
	PriorityMedium TicketPriority = "MEDIUM"
	PriorityHigh   TicketPriority = "HIGH"
)

type Ticket struct {
	ID           uint           `gorm:"primarykey" json:"id"`
	Title        string         `gorm:"not null" json:"title"`
	Description  string         `gorm:"type:text;not null" json:"description"`
	Status       TicketStatus   `gorm:"default:'WAITING'" json:"status"`
	Priority     TicketPriority `gorm:"default:'MEDIUM'" json:"priority"`
	ReplyToEmail string         `json:"reply_to_email"`
	CreatedByID  uint           `gorm:"not null" json:"created_by_id"`
	DepartmentID *uint          `json:"department_id"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`

	// Relations
	CreatedBy  User          `gorm:"foreignKey:CreatedByID" json:"created_by"`
	Department *Department   `gorm:"foreignKey:DepartmentID" json:"department"`
	Replies    []TicketReply `gorm:"foreignKey:TicketID" json:"replies"`
}

func (t *Ticket) GetStatusDisplay() string {
	switch t.Status {
	case StatusWaiting:
		return "Menunggu Balasan"
	case StatusInProgress:
		return "In Progress"
	case StatusClosed:
		return "Closed"
	default:
		return string(t.Status)
	}
}

func (t *Ticket) GetPriorityDisplay() string {
	switch t.Priority {
	case PriorityLow:
		return "Low"
	case PriorityMedium:
		return "Medium"
	case PriorityHigh:
		return "High"
	default:
		return string(t.Priority)
	}
}

func (t *Ticket) GetReplyCount() int {
	return len(t.Replies)
}
