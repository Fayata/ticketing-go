package models

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gorm.io/gorm"
)

// TicketAttachment stores metadata and storage path for uploaded images on tickets and replies.
type TicketAttachment struct {
	ID        uint      `gorm:"primarykey" json:"id"`
	TicketID  uint      `gorm:"not null;index" json:"ticket_id"`
	ReplyID   *uint     `gorm:"index" json:"reply_id,omitempty"`
	FileName  string    `gorm:"not null" json:"file_name"`
	FilePath  string    `gorm:"not null" json:"file_path"`
	FileSize  int64     `gorm:"not null" json:"file_size"`
	MimeType  string    `gorm:"size:100;not null" json:"mime_type"`
	CreatedAt time.Time `json:"created_at"`

	// Relations
	Ticket Ticket       `gorm:"foreignKey:TicketID;constraint:OnDelete:CASCADE;" json:"-"`
	Reply  *TicketReply `gorm:"foreignKey:ReplyID;constraint:OnDelete:CASCADE;" json:"-"`
}

func (TicketAttachment) TableName() string {
	return "ticket_attachments"
}

// GetFormattedSize returns human-readable file size.
func (a *TicketAttachment) GetFormattedSize() string {
	if a.FileSize < 1024 {
		return fmt.Sprintf("%d B", a.FileSize)
	} else if a.FileSize < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(a.FileSize)/1024.0)
	}
	return fmt.Sprintf("%.1f MB", float64(a.FileSize)/(1024.0*1024.0))
}

// AfterDelete hook cleans up the file on disk when an attachment record is deleted.
func (a *TicketAttachment) AfterDelete(tx *gorm.DB) (err error) {
	if a.FilePath != "" {
		_ = os.Remove(filepath.FromSlash(a.FilePath))
	}
	return nil
}
