package models

import (
	"time"

	"gorm.io/gorm"
)

type Company struct {
	ID          uint           `gorm:"primarykey" json:"id"`
	Name        string         `gorm:"size:255;not null" json:"name"`
	Code        string         `gorm:"size:20;uniqueIndex;not null" json:"code"`
	Address     string         `gorm:"type:text" json:"address"`
	Phone       string         `gorm:"size:50" json:"phone"`
	IsActive    bool           `gorm:"default:true" json:"is_active"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`
	Departments []Department   `gorm:"foreignKey:CompanyID" json:"departments,omitempty"`
	Tickets     []Ticket       `gorm:"foreignKey:CompanyID" json:"tickets,omitempty"`
}

func (Company) TableName() string {
	return "companies"
}
