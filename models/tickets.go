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
	CompanyID    *uint          `gorm:"index" json:"company_id"`
	DepartmentID *uint          `json:"department_id"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`

	AssignedToID *uint `json:"assigned_to_id"`
	AssignedTo   *User `gorm:"foreignKey:AssignedToID" json:"assigned_to"`

	// SLA Tracking Fields
	SLAPolicyID           *uint      `gorm:"index" json:"sla_policy_id"`
	FirstResponseDeadline *time.Time `gorm:"index" json:"first_response_deadline"`
	FirstResponseAt       *time.Time `json:"first_response_at"`
	FirstResponseMet      *bool      `json:"first_response_met"` // nil = pending, true = met, false = breached
	ResolutionDeadline    *time.Time `json:"resolution_deadline"`
	EstimatedResolutionAt *time.Time `json:"estimated_resolution_at"`
	SLAWarningSent        bool       `gorm:"default:false;index" json:"sla_warning_sent"`
	SLABreachSent         bool       `gorm:"default:false;index" json:"sla_breach_sent"`

	// Relations
	CreatedBy         User                    `gorm:"foreignKey:CreatedByID" json:"created_by"`
	Company           *Company                `gorm:"foreignKey:CompanyID" json:"company,omitempty"`
	Department        *Department             `gorm:"foreignKey:DepartmentID" json:"department"`
	Replies           []TicketReply           `gorm:"foreignKey:TicketID" json:"replies"`
	Attachments       []TicketAttachment      `gorm:"foreignKey:TicketID;constraint:OnDelete:CASCADE;" json:"attachments"`
	SLAPolicy         *SLAPolicy              `gorm:"foreignKey:SLAPolicyID" json:"sla_policy,omitempty"`
	PriorityHistories []TicketPriorityHistory `gorm:"foreignKey:TicketID" json:"priority_histories,omitempty"`

	// Transient UI fields
	UnreadCount int `gorm:"-" json:"unread_count"`
}

func (t *Ticket) GetUnreadCount() int {
	if t == nil {
		return 0
	}
	return t.UnreadCount
}

func (t *Ticket) GetStatusDisplay() string {
	if t == nil {
		return ""
	}
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
	if t == nil {
		return ""
	}
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
	if t == nil {
		return 0
	}
	return len(t.Replies)
}

func (t *Ticket) GetTicketNumber() string {
	if t == nil {
		return "T00-0000"
	}
	year := t.CreatedAt.Format("06")
	if t.CreatedAt.IsZero() {
		year = time.Now().Format("06")
	}

	return fmt.Sprintf("T%s-%04d", year, t.ID)
}

func (t *Ticket) GetInitialAttachments() []TicketAttachment {
	if t == nil {
		return nil
	}
	var list []TicketAttachment
	for _, a := range t.Attachments {
		if a.ReplyID == nil || *a.ReplyID == 0 {
			list = append(list, a)
		}
	}
	return list
}

// IsFirstResponseBreached returns true if the first response was evaluated and breached the SLA deadline.
func (t *Ticket) IsFirstResponseBreached() bool {
	if t == nil || t.FirstResponseMet == nil {
		return false
	}
	return !*t.FirstResponseMet
}

// IsFirstResponseMetStatus returns true if the first response was evaluated and met the SLA deadline.
func (t *Ticket) IsFirstResponseMetStatus() bool {
	if t == nil || t.FirstResponseMet == nil {
		return false
	}
	return *t.FirstResponseMet
}

// SLABadgeInfo holds UI presentation data for ticket SLA status.
type SLABadgeInfo struct {
	Class      string `json:"class"`       // "sla-badge-green", "sla-badge-yellow", "sla-badge-red", "sla-badge-gray"
	Label      string `json:"label"`       // "SLA Aman", "Mendekati Batas", "SLA Breached", "Sesuai Estimasi", etc.
	Detail     string `json:"detail"`      // "Sisa 25m", "Terlambat 1j 10m", etc.
	IsBreached bool   `json:"is_breached"`
	IsWarning  bool   `json:"is_warning"`
}

// GetSLABadgeInfo determines the visual SLA badge status for a ticket.
func (t *Ticket) GetSLABadgeInfo() SLABadgeInfo {
	if t == nil {
		return SLABadgeInfo{Class: "sla-badge-gray", Label: "Tidak Ada Data"}
	}

	// 1. Closed Ticket
	if t.Status == StatusClosed {
		if t.FirstResponseMet != nil && !*t.FirstResponseMet {
			return SLABadgeInfo{
				Class:      "sla-badge-red",
				Label:      "Selesai (SLA Breached)",
				Detail:     "Terlambat Respon",
				IsBreached: true,
			}
		}
		return SLABadgeInfo{
			Class:  "sla-badge-green",
			Label:  "Selesai (SLA Terpenuhi)",
			Detail: "Tepat Waktu",
		}
	}

	now := time.Now()

	// 2. First Response Milestone already reached
	if t.FirstResponseAt != nil || t.FirstResponseMet != nil {
		if t.FirstResponseMet != nil && !*t.FirstResponseMet {
			return SLABadgeInfo{
				Class:      "sla-badge-red",
				Label:      "First Response Terlewat",
				Detail:     "SLA Breached",
				IsBreached: true,
			}
		}

		// First response was met; check resolution estimation if ticket is still active
		if t.EstimatedResolutionAt != nil {
			if now.After(*t.EstimatedResolutionAt) {
				overdue := now.Sub(*t.EstimatedResolutionAt)
				return SLABadgeInfo{
					Class:      "sla-badge-red",
					Label:      "Melewati Estimasi",
					Detail:     formatDurationText(overdue) + " lalu",
					IsBreached: true,
				}
			}
			remaining := t.EstimatedResolutionAt.Sub(now)
			if remaining <= 1*time.Hour {
				return SLABadgeInfo{
					Class:     "sla-badge-yellow",
					Label:     "Mendekati Estimasi",
					Detail:    "Sisa " + formatDurationText(remaining),
					IsWarning: true,
				}
			}
			return SLABadgeInfo{
				Class:  "sla-badge-green",
				Label:  "Sesuai Estimasi",
				Detail: "Sisa " + formatDurationText(remaining),
			}
		}

		return SLABadgeInfo{
			Class:  "sla-badge-green",
			Label:  "Respon Terpenuhi",
			Detail: "Menunggu Estimasi",
		}
	}

	// 3. First Response Milestone Pending
	if t.FirstResponseDeadline == nil {
		return SLABadgeInfo{
			Class:  "sla-badge-gray",
			Label:  "Tanpa SLA",
			Detail: "-",
		}
	}

	if now.After(*t.FirstResponseDeadline) {
		overdue := now.Sub(*t.FirstResponseDeadline)
		return SLABadgeInfo{
			Class:      "sla-badge-red",
			Label:      "SLA Breached",
			Detail:     "Terlambat " + formatDurationText(overdue),
			IsBreached: true,
		}
	}

	remaining := t.FirstResponseDeadline.Sub(now)
	if remaining <= 30*time.Minute {
		return SLABadgeInfo{
			Class:     "sla-badge-yellow",
			Label:     "Mendekati Batas",
			Detail:    "Sisa " + formatDurationText(remaining),
			IsWarning: true,
		}
	}

	return SLABadgeInfo{
		Class:  "sla-badge-green",
		Label:  "SLA Aman",
		Detail: "Sisa " + formatDurationText(remaining),
	}
}

func formatDurationText(d time.Duration) string {
	if d < 0 {
		d = -d
	}
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	if hours > 24 {
		days := hours / 24
		remHours := hours % 24
		if remHours > 0 {
			return fmt.Sprintf("%dh %dj", days, remHours)
		}
		return fmt.Sprintf("%dh", days)
	}
	if hours > 0 {
		if minutes > 0 {
			return fmt.Sprintf("%dj %dm", hours, minutes)
		}
		return fmt.Sprintf("%dj", hours)
	}
	if minutes > 0 {
		return fmt.Sprintf("%dm", minutes)
	}
	return "<1m"
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