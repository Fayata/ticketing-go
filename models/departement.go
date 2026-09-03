package models

import "time"

type Department struct {
	ID        uint      `gorm:"primarykey" json:"id"`
	CompanyID *uint     `gorm:"index" json:"company_id"`
	Name      string    `gorm:"not null" json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	

	// Relations
	Company *Company `gorm:"foreignKey:CompanyID" json:"company,omitempty"`
	Tickets []Ticket `gorm:"foreignKey:DepartmentID" json:"-"`
}
