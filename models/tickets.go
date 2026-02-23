package models

import (
	"fmt"
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

	AssignedToID *uint `json:"assigned_to_id"`
	AssignedTo   *User `gorm:"foreignKey:AssignedToID" json:"assigned_to"`

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

func (t *Ticket) GetTicketNumber() string {
	year := t.CreatedAt.Format("06")
	if t.CreatedAt.IsZero() {
		year = time.Now().Format("06")
	}

	return fmt.Sprintf("T%s-%04d ", year, t.ID)
}

// TicketAssignmentHistory tracks which staff members have worked on a ticket
type TicketAssignmentHistory struct {
	ID           uint      `gorm:"primarykey" json:"id"`
	TicketID     uint      `gorm:"not null;index" json:"ticket_id"`
	StaffID      uint      `gorm:"not null;index" json:"staff_id"`
	AssignedAt   time.Time `gorm:"not null" json:"assigned_at"`
	ReleasedAt   *time.Time `json:"released_at"`
	IsCompleted  bool      `gorm:"default:false" json:"is_completed"` // True if ticket was closed while this staff had it
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`

	// Relations
	Ticket Ticket `gorm:"foreignKey:TicketID" json:"ticket"`
	Staff  User   `gorm:"foreignKey:StaffID" json:"staff"`
}

// TicketRating stores user ratings for closed tickets
type TicketRating struct {
	ID          uint      `gorm:"primarykey" json:"id"`
	TicketID    uint      `gorm:"not null;uniqueIndex" json:"ticket_id"` // One rating per ticket
	Rating      int       `gorm:"not null;check:rating >= 1 AND rating <= 5" json:"rating"` // 1-5 stars
	Comment     string    `gorm:"type:text" json:"comment"`
	RatedByID   uint      `gorm:"not null" json:"rated_by_id"` // User who created the ticket
	RatedAt     time.Time `gorm:"not null" json:"rated_at"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`

	// Relations
	Ticket Ticket `gorm:"foreignKey:TicketID" json:"ticket"`
	RatedBy User  `gorm:"foreignKey:RatedByID" json:"rated_by"`
}