package models

import (
	"time"

	"gorm.io/gorm"
)

// KBCategory untuk kategori Knowledge Base
type KBCategory struct {
	ID          uint           `gorm:"primarykey" json:"id"`
	Name        string         `gorm:"size:120;not null" json:"name"`
	Slug        string         `gorm:"size:120;uniqueIndex;not null" json:"slug"`
	Description string         `gorm:"type:text" json:"description"`
	Icon        string         `gorm:"size:20" json:"icon"`        // emoji atau nama icon
	ColorClass  string         `gorm:"size:30" json:"color_class"` // green, cyan, indigo, amber, rose, violet
	SortOrder   int            `gorm:"default:0" json:"sort_order"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	DeletedAt   gorm.DeletedAt `gorm:"index" json:"-"`

	Articles []KBArticle `gorm:"foreignKey:CategoryID" json:"articles"`
}

// TableName untuk KBCategory
func (KBCategory) TableName() string {
	return "kb_categories"
}

// KBArticle untuk artikel Knowledge Base
type KBArticle struct {
	ID               uint           `gorm:"primarykey" json:"id"`
	CategoryID       uint           `gorm:"not null;index" json:"category_id"`
	Title            string         `gorm:"size:255;not null" json:"title"`
	Slug             string         `gorm:"size:255;index;not null" json:"slug"`
	Content          string         `gorm:"type:text;not null" json:"content"`
	Views            int            `gorm:"default:0" json:"views"`
	ReadTimeMinutes  int            `gorm:"default:0" json:"read_time_minutes"`
	Published        bool           `gorm:"default:true" json:"published"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
	DeletedAt        gorm.DeletedAt `gorm:"index" json:"-"`

	Category *KBCategory `gorm:"foreignKey:CategoryID" json:"category"`
}

// TableName untuk KBArticle
func (KBArticle) TableName() string {
	return "kb_articles"
}
