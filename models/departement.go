package models

import "time"

type Department struct {
	ID        uint      `gorm:"primarykey" json:"id"`
	Name      string    `gorm:"not null" json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Relations
	Tickets []Ticket `gorm:"foreignKey:DepartmentID" json:"-"`
}
