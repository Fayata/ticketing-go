package models

import (
	"time"

	"gorm.io/gorm"
)

type User struct {
	ID        uint           `gorm:"primarykey" json:"id"`
	Username  string         `gorm:"uniqueIndex;not null" json:"username"`
	Email     string         `gorm:"uniqueIndex;not null" json:"email"`
	Password  string         `gorm:"not null" json:"-"`
	FirstName string         `json:"first_name"`
	LastName  string         `json:"last_name"`
	IsStaff   bool           `gorm:"default:false" json:"is_staff"`
	IsActive  bool           `gorm:"default:true" json:"is_active"`
	LastLogin *time.Time     `json:"last_login"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	// Relations
	Tickets []Ticket      `gorm:"foreignKey:CreatedByID" json:"-"`
	Replies []TicketReply `gorm:"foreignKey:UserID" json:"-"`
	Groups  []Group       `gorm:"many2many:user_groups;" json:"-"`
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
	if u.IsStaff {
		return true
	}
	for _, group := range u.Groups {
		if group.Name == "Portal Users" {
			return true
		}
	}
	return false
}
