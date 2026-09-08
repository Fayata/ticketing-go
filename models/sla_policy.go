package models

import (
	"errors"
	"fmt"
	"log"
	"time"

	"gorm.io/gorm"
)

// Fallback SLA Duration Constants (applied when no active policy exists in DB)
const (
	FallbackResponseHigh = 60 * time.Minute  // 1 hour
	FallbackResponseMed  = 240 * time.Minute // 4 hours
	FallbackResponseLow  = 480 * time.Minute // 8 hours

	FallbackResolutionHigh = 4 * time.Hour  // 4 hours
	FallbackResolutionMed  = 24 * time.Hour // 24 hours (1 day)
	FallbackResolutionLow  = 72 * time.Hour // 72 hours (3 days)

	DefaultSLAPolicyName = "Standar Default Sistem"
)

// SLAPolicy represents an SLA policy matrix for High, Medium, and Low priorities.
type SLAPolicy struct {
	ID                      uint           `gorm:"primarykey" json:"id"`
	Name                    string         `gorm:"size:255;not null" json:"name"`
	Description             string         `gorm:"type:text" json:"description"`
	CompanyID               *uint          `gorm:"index" json:"company_id"`
	DepartmentID            *uint          `gorm:"index" json:"department_id"`
	IsActive                bool           `gorm:"default:true" json:"is_active"`
	IsDefault               bool           `gorm:"default:false" json:"is_default"`
	ResponseTimeHighMinutes int            `gorm:"default:60;not null" json:"response_time_high_minutes"`
	ResponseTimeMedMinutes  int            `gorm:"default:240;not null" json:"response_time_med_minutes"`
	ResponseTimeLowMinutes  int            `gorm:"default:480;not null" json:"response_time_low_minutes"`
	ResolutionTimeHighHours int            `gorm:"default:4;not null" json:"resolution_time_high_hours"`
	ResolutionTimeMedHours  int            `gorm:"default:24;not null" json:"resolution_time_med_hours"`
	ResolutionTimeLowHours  int            `gorm:"default:72;not null" json:"resolution_time_low_hours"`
	CreatedAt               time.Time      `json:"created_at"`
	UpdatedAt               time.Time      `json:"updated_at"`
	DeletedAt               gorm.DeletedAt `gorm:"index" json:"-"`

	// Relations
	Company    *Company    `gorm:"foreignKey:CompanyID" json:"company,omitempty"`
	Department *Department `gorm:"foreignKey:DepartmentID" json:"department,omitempty"`
}

// TableName returns the table name for SLAPolicy.
func (SLAPolicy) TableName() string {
	return "sla_policies"
}

// GetFirstResponseDuration returns target first response duration for given priority. Nil-safe.
func (p *SLAPolicy) GetFirstResponseDuration(priority TicketPriority) time.Duration {
	if p == nil {
		switch priority {
		case PriorityHigh:
			return FallbackResponseHigh
		case PriorityLow:
			return FallbackResponseLow
		default:
			return FallbackResponseMed
		}
	}
	switch priority {
	case PriorityHigh:
		if p.ResponseTimeHighMinutes > 0 {
			return time.Duration(p.ResponseTimeHighMinutes) * time.Minute
		}
		return FallbackResponseHigh
	case PriorityLow:
		if p.ResponseTimeLowMinutes > 0 {
			return time.Duration(p.ResponseTimeLowMinutes) * time.Minute
		}
		return FallbackResponseLow
	default:
		if p.ResponseTimeMedMinutes > 0 {
			return time.Duration(p.ResponseTimeMedMinutes) * time.Minute
		}
		return FallbackResponseMed
	}
}

// GetResolutionDuration returns target resolution duration for given priority. Nil-safe.
func (p *SLAPolicy) GetResolutionDuration(priority TicketPriority) time.Duration {
	if p == nil {
		switch priority {
		case PriorityHigh:
			return FallbackResolutionHigh
		case PriorityLow:
			return FallbackResolutionLow
		default:
			return FallbackResolutionMed
		}
	}
	switch priority {
	case PriorityHigh:
		if p.ResolutionTimeHighHours > 0 {
			return time.Duration(p.ResolutionTimeHighHours) * time.Hour
		}
		return FallbackResolutionHigh
	case PriorityLow:
		if p.ResolutionTimeLowHours > 0 {
			return time.Duration(p.ResolutionTimeLowHours) * time.Hour
		}
		return FallbackResolutionLow
	default:
		if p.ResolutionTimeMedHours > 0 {
			return time.Duration(p.ResolutionTimeMedHours) * time.Hour
		}
		return FallbackResolutionMed
	}
}

// GetResponseDuration is an alias for GetFirstResponseDuration.
func (p *SLAPolicy) GetResponseDuration(priority TicketPriority) time.Duration {
	return p.GetFirstResponseDuration(priority)
}

// ResolveSLAPolicy evaluates the 4-tier SLA policy hierarchy:
// 1. Department Override (department_id = ? AND is_active = true)
// 2. Company PT Default (company_id = ? AND (department_id IS NULL OR department_id = 0) AND is_active = true)
// 3. System Default Policy (is_default = true AND is_active = true)
// 4. Built-in constants fallback (High: 60m/4h, Med: 240m/24h, Low: 480m/72h)
// Returns the resolved *SLAPolicy (nil if using fallback constants), firstResponseDuration, and resolutionDuration.
func ResolveSLAPolicy(db *gorm.DB, companyID, departmentID *uint, priority ...TicketPriority) (*SLAPolicy, time.Duration, time.Duration) {
	p := PriorityMedium
	if len(priority) > 0 && priority[0] != "" {
		p = priority[0]
	}

	if db == nil {
		return nil, (*SLAPolicy)(nil).GetFirstResponseDuration(p), (*SLAPolicy)(nil).GetResolutionDuration(p)
	}

	var policy SLAPolicy

	// 1. Department Override
	if departmentID != nil && *departmentID > 0 {
		if err := db.Where("department_id = ? AND is_active = ?", *departmentID, true).
			Order("id DESC").First(&policy).Error; err == nil {
			return &policy, policy.GetFirstResponseDuration(p), policy.GetResolutionDuration(p)
		}
	}

	// 2. Company PT Default
	if companyID != nil && *companyID > 0 {
		if err := db.Where("company_id = ? AND (department_id IS NULL OR department_id = 0) AND is_active = ?", *companyID, true).
			Order("id DESC").First(&policy).Error; err == nil {
			return &policy, policy.GetFirstResponseDuration(p), policy.GetResolutionDuration(p)
		}
	}

	// 3. System Default Policy
	if err := db.Where("is_default = ? AND is_active = ?", true, true).
		Order("id DESC").First(&policy).Error; err == nil {
		return &policy, policy.GetFirstResponseDuration(p), policy.GetResolutionDuration(p)
	}

	// 4. Built-in In-Code Constants Fallback
	return nil, (*SLAPolicy)(nil).GetFirstResponseDuration(p), (*SLAPolicy)(nil).GetResolutionDuration(p)
}

// ResolveSLAPolicyWithDurations is a convenience helper explicitly returning durations.
func ResolveSLAPolicyWithDurations(db *gorm.DB, companyID, departmentID *uint, priority TicketPriority) (*SLAPolicy, time.Duration, time.Duration) {
	return ResolveSLAPolicy(db, companyID, departmentID, priority)
}

// SeedDefaultSLAPolicies ensures a global default SLA policy exists and safely backfills legacy tickets.
func SeedDefaultSLAPolicies(db *gorm.DB) error {
	if db == nil {
		return errors.New("db connection cannot be nil")
	}

	return db.Transaction(func(tx *gorm.DB) error {
		var defaultPolicy SLAPolicy

		// 1. Idempotently find or create default system policy
		err := tx.Where("is_default = ?", true).First(&defaultPolicy).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				defaultPolicy = SLAPolicy{
					Name:                    DefaultSLAPolicyName,
					Description:             "Kebijakan SLA standar sistem default (High: 60m/4h, Med: 240m/24h, Low: 480m/72h)",
					IsActive:                true,
					IsDefault:               true,
					ResponseTimeHighMinutes: 60,
					ResponseTimeMedMinutes:  240,
					ResponseTimeLowMinutes:  480,
					ResolutionTimeHighHours: 4,
					ResolutionTimeMedHours:  24,
					ResolutionTimeLowHours:  72,
				}
				if createErr := tx.Create(&defaultPolicy).Error; createErr != nil {
					// Retry fetch in case of concurrent insert
					if fetchErr := tx.Where("is_default = ?", true).First(&defaultPolicy).Error; fetchErr != nil {
						return fmt.Errorf("failed to create default SLA policy: %w", createErr)
					}
				}
			} else {
				return fmt.Errorf("failed to query default SLA policy: %w", err)
			}
		}

		if defaultPolicy.ID == 0 {
			return errors.New("default SLA policy ID is zero after creation/lookup")
		}

		// 2. Safely backfill pre-existing tickets where sla_policy_id IS NULL OR sla_policy_id = 0
		if tx.Migrator().HasTable(&Ticket{}) {
			resPolicy := tx.Unscoped().Model(&Ticket{}).
				Where("sla_policy_id IS NULL OR sla_policy_id = 0").
				Update("sla_policy_id", defaultPolicy.ID)
			if resPolicy.Error != nil {
				return fmt.Errorf("failed to backfill legacy ticket sla_policy_id: %w", resPolicy.Error)
			}
			if resPolicy.RowsAffected > 0 {
				log.Printf("[Migration] Associated %d legacy tickets with default SLA policy ID=%d",
					resPolicy.RowsAffected, defaultPolicy.ID)
			}

			// 3. Backfill first_response_deadline and resolution_deadline for tickets where deadline is null
			var legacyTickets []Ticket
			if err := tx.Unscoped().Where("first_response_deadline IS NULL").Find(&legacyTickets).Error; err == nil && len(legacyTickets) > 0 {
				metTrue := true
				for _, t := range legacyTickets {
					respDur := defaultPolicy.GetFirstResponseDuration(t.Priority)
					resDur := defaultPolicy.GetResolutionDuration(t.Priority)
					baseTime := t.CreatedAt
					if baseTime.IsZero() {
						baseTime = time.Now()
					}
					deadline := baseTime.Add(respDur)
					resDeadline := baseTime.Add(resDur)

					updates := map[string]interface{}{
						"first_response_deadline": deadline,
						"resolution_deadline":     resDeadline,
					}

					// If legacy ticket was already closed, mark SLA as met gracefully so it is not treated as a retro-breach
					if t.Status == StatusClosed {
						updates["first_response_met"] = &metTrue
						if t.FirstResponseAt == nil {
							updates["first_response_at"] = t.UpdatedAt
						}
					}

					tx.Unscoped().Model(&Ticket{}).Where("id = ?", t.ID).Updates(updates)
				}
				log.Printf("[Migration] Calculated deadlines for %d legacy tickets", len(legacyTickets))
			}
		}

		return nil
	})
}
