package models

import (
	"time"

	"gorm.io/gorm"
)

type User struct {
	ID        uint   `gorm:"primarykey" json:"id"`
	Username  string `gorm:"uniqueIndex;not null" json:"username"`
	// Email is nullable: admin-created accounts start without email.
	// NULL = not yet set. Non-null but IsVerified=false = pending verification.
	Email    *string        `gorm:"uniqueIndex" json:"email"`
	Password string         `gorm:"not null" json:"-"`
	FirstName string        `json:"first_name"`
	LastName  string        `json:"last_name"`

	IsStaff      bool `gorm:"default:false" json:"is_staff"`
	IsSuperAdmin bool `gorm:"default:false" json:"is_super_admin"`

	IsActive     bool           `gorm:"default:true" json:"is_active"`
	LastLogin    *time.Time     `json:"last_login"`
	LastActiveAt *time.Time     `gorm:"index" json:"last_active_at"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
	IsVerified   bool           `gorm:"default:false" json:"is_verified"`

	DepartmentID *uint       `json:"department_id"`
	Department   *Department `gorm:"foreignKey:DepartmentID" json:"department"`

	// Relations
	Tickets []Ticket      `gorm:"foreignKey:CreatedByID" json:"-"`
	Replies []TicketReply `gorm:"foreignKey:UserID" json:"-"`
	Groups  []Group       `gorm:"many2many:user_groups;" json:"-"`
}

// GetEmail returns the user's email or "" if not set yet.
func (u *User) GetEmail() string {
	if u == nil || u.Email == nil {
		return ""
	}
	return *u.Email
}

// NeedsEmailSetup returns true when the user must set/verify their email before proceeding.
func (u *User) NeedsEmailSetup() bool {
	if u == nil {
		return false
	}
	return u.Email == nil || *u.Email == ""
}
type Group struct {
	ID        uint      `gorm:"primarykey" json:"id"`
	Name      string    `gorm:"uniqueIndex;not null" json:"name"`
	CreatedAt time.Time `json:"created_at"`
	Users     []User    `gorm:"many2many:user_groups;" json:"-"`
}

func (u *User) GetFullName() string {
	if u.FirstName != "" || u.LastName != "" {
		return u.FirstName + " " + u.LastName
	}
	return u.Username
}

func (u *User) HasPortalAccess() bool {
	if u.IsStaff || u.IsSuperAdmin {
		return true
	}
	for _, group := range u.Groups {
		if group.Name == "Portal Users" {
			return true
		}
	}
	return false
}

// IsOnline returns true if the user was active within the last 2 minutes.
func (u *User) IsOnline() bool {
	if u == nil || u.LastActiveAt == nil {
		return false
	}
	return time.Since(*u.LastActiveAt) < 2*time.Minute
}

