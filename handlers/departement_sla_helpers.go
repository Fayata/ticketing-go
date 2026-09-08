package handlers

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"ticketing/models"
)

// CalculateFirstResponseMet evaluates whether the current response timestamp met the SLA deadline.
// Returns nil if no deadline was set.
// If now <= deadline (!now.After(*deadline)), returns &true (met on time).
// If now > deadline, returns &false (breached).
func CalculateFirstResponseMet(now time.Time, deadline *time.Time) *bool {
	if deadline == nil {
		return nil
	}
	isMet := !now.After(*deadline)
	return &isMet
}

// ParseEstimatedResolution parses resolution presets (1h, 2h, 4h, 8h, 1d, 3d) or custom datetime strings.
// Returns an error if preset is unknown, custom date is malformed, or target time is not strictly in the future.
func ParseEstimatedResolution(preset, customDate string, baseTime time.Time) (*time.Time, error) {
	preset = strings.ToLower(strings.TrimSpace(preset))
	customDate = strings.TrimSpace(customDate)

	var target time.Time
	switch preset {
	case "1h":
		target = baseTime.Add(1 * time.Hour)
	case "2h":
		target = baseTime.Add(2 * time.Hour)
	case "4h":
		target = baseTime.Add(4 * time.Hour)
	case "8h":
		target = baseTime.Add(8 * time.Hour)
	case "1d":
		target = baseTime.Add(24 * time.Hour)
	case "3d":
		target = baseTime.Add(72 * time.Hour)
	case "custom", "":
		if customDate == "" {
			return nil, errors.New("waktu estimasi kustom harus diisi")
		}
		layouts := []string{
			time.RFC3339,
			"2006-01-02T15:04:05",
			"2006-01-02T15:04",
			"2006-01-02 15:04:05",
			"2006-01-02 15:04",
			"2006-01-02",
		}
		var parsed time.Time
		var parseErr error
		for _, layout := range layouts {
			parsed, parseErr = time.ParseInLocation(layout, customDate, baseTime.Location())
			if parseErr == nil {
				break
			}
		}
		if parseErr != nil {
			return nil, fmt.Errorf("format tanggal estimasi tidak valid: %s", customDate)
		}
		target = parsed
	default:
		return nil, fmt.Errorf("preset estimasi '%s' tidak valid", preset)
	}

	if !target.After(baseTime) {
		return nil, errors.New("waktu estimasi penyelesaian harus di masa depan")
	}
	return &target, nil
}

// ValidatePriorityAdjustment verifies the requested priority change is valid and reason satisfies constraints.
func ValidatePriorityAdjustment(oldPriority, newPriority models.TicketPriority, reason string) error {
	if newPriority != models.PriorityHigh && newPriority != models.PriorityMedium && newPriority != models.PriorityLow {
		return errors.New("prioritas baru tidak valid")
	}
	if newPriority == oldPriority {
		return errors.New("prioritas baru sama dengan prioritas saat ini")
	}
	trimmedReason := strings.TrimSpace(reason)
	if len(trimmedReason) < 5 {
		return errors.New("alasan perubahan prioritas minimal 5 karakter")
	}
	return nil
}

// RecalculateSLADeadlines computes updated SLA deadlines when a ticket's priority is modified.
// If firstResponseAt is nil, first response deadline is recalculated.
// If firstResponseAt is already set, first response deadline remains nil (unaltered).
// Resolution deadline is recalculated based on ticket creation time and resolution duration.
func RecalculateSLADeadlines(createdAt time.Time, firstResponseAt *time.Time, respDuration, resDuration time.Duration) (*time.Time, *time.Time) {
	var newRespDeadline *time.Time
	if firstResponseAt == nil {
		resp := createdAt.Add(respDuration)
		newRespDeadline = &resp
	}
	res := createdAt.Add(resDuration)
	return newRespDeadline, &res
}
