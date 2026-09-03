package models

import (
	"errors"
	"fmt"
	"log"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	// DefaultCompanyCode is the unique identifier code for the default company
	DefaultCompanyCode = "DEFAULT"
	// DefaultCompanyName is the display name for the default company
	DefaultCompanyName = "PT Utama"
)

// SeedDefaultCompanyAndMigrate idempotently ensures the default company ('PT Utama', code: 'DEFAULT')
// exists and safely backfills any pre-existing departments and tickets (including soft-deleted tickets)
// where company_id IS NULL OR company_id = 0.
func SeedDefaultCompanyAndMigrate(db *gorm.DB) (*Company, error) {
	if db == nil {
		return nil, errors.New("db connection cannot be nil")
	}

	var company Company

	err := db.Transaction(func(tx *gorm.DB) error {
		// 1. Idempotently find or create default company
		err := tx.Where("code = ?", DefaultCompanyCode).First(&company).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				company = Company{
					Name:     DefaultCompanyName,
					Code:     DefaultCompanyCode,
					Address:  "Kantor Pusat",
					Phone:    "-",
					IsActive: true,
				}
				// Use ON CONFLICT DO NOTHING to prevent PostgreSQL transaction abort on concurrent insert
				if createErr := tx.Clauses(clause.OnConflict{
					Columns:   []clause.Column{{Name: "code"}},
					DoNothing: true,
				}).Create(&company).Error; createErr != nil {
					return fmt.Errorf("failed to create default company: %w", createErr)
				}

				// If ON CONFLICT did nothing, fetch the existing record created by the concurrent process
				if company.ID == 0 {
					if fetchErr := tx.Where("code = ?", DefaultCompanyCode).First(&company).Error; fetchErr != nil {
						return fmt.Errorf("failed to retrieve default company after concurrent insert: %w", fetchErr)
					}
				}
			} else {
				return fmt.Errorf("failed to query default company: %w", err)
			}
		}

		if company.ID == 0 {
			return errors.New("default company ID is zero after creation/lookup")
		}

		// 2. Safely backfill pre-existing departments where company_id IS NULL OR company_id = 0
		if tx.Migrator().HasTable(&Department{}) {
			resDept := tx.Model(&Department{}).
				Where("company_id IS NULL OR company_id = 0").
				Update("company_id", company.ID)
			if resDept.Error != nil {
				return fmt.Errorf("failed to backfill legacy departments: %w", resDept.Error)
			}
			if resDept.RowsAffected > 0 {
				log.Printf("[Migration] Associated %d legacy departments with default company ID=%d (%s)",
					resDept.RowsAffected, company.ID, company.Code)
			}
		}

		// 3. Safely backfill pre-existing tickets (including soft-deleted records via Unscoped)
		// where company_id IS NULL OR company_id = 0
		if tx.Migrator().HasTable(&Ticket{}) {
			resTicket := tx.Unscoped().Model(&Ticket{}).
				Where("company_id IS NULL OR company_id = 0").
				Update("company_id", company.ID)
			if resTicket.Error != nil {
				return fmt.Errorf("failed to backfill legacy tickets: %w", resTicket.Error)
			}
			if resTicket.RowsAffected > 0 {
				log.Printf("[Migration] Associated %d legacy tickets (including soft-deleted) with default company ID=%d (%s)",
					resTicket.RowsAffected, company.ID, company.Code)
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return &company, nil
}
