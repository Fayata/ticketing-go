package services_test

import (
	"strings"
	"testing"

	"ticketing/config"
	"ticketing/models"
	"ticketing/services"
	"ticketing/utils"
)

// normalizePriorityMirror replicates the priority normalization logic from TicketService.CreateTicketWithAttachments.
func normalizePriorityMirror(priority string) models.TicketPriority {
	ticketPriority := models.TicketPriority(strings.ToUpper(strings.TrimSpace(priority)))
	if ticketPriority != models.PriorityHigh && ticketPriority != models.PriorityLow && ticketPriority != models.PriorityMedium {
		ticketPriority = models.PriorityMedium
	}
	return ticketPriority
}

// TestChallenger_TicketService_PriorityNormalizationStress tests priority string normalization,
// whitespace trimming, case-insensitivity, and fallback behavior for invalid inputs.
func TestChallenger_TicketService_PriorityNormalizationStress(t *testing.T) {
	testCases := []struct {
		input            string
		expectedPriority models.TicketPriority
		description      string
	}{
		// HIGH variations
		{"HIGH", models.PriorityHigh, "Exact uppercase HIGH"},
		{"high", models.PriorityHigh, "Lowercase high"},
		{"High", models.PriorityHigh, "Title case High"},
		{"hIgH", models.PriorityHigh, "Mixed case hIgH"},
		{"  high  ", models.PriorityHigh, "Whitespace padded high"},
		{"\thigh\n", models.PriorityHigh, "Tab/newline padded high"},

		// MEDIUM variations
		{"MEDIUM", models.PriorityMedium, "Exact uppercase MEDIUM"},
		{"medium", models.PriorityMedium, "Lowercase medium"},
		{"Medium", models.PriorityMedium, "Titlecase Medium"},
		{"  medium  ", models.PriorityMedium, "Whitespace padded medium"},

		// LOW variations
		{"LOW", models.PriorityLow, "Exact uppercase LOW"},
		{"low", models.PriorityLow, "Lowercase low"},
		{"Low", models.PriorityLow, "Titlecase Low"},
		{"  low  ", models.PriorityLow, "Whitespace padded low"},

		// Fallbacks to MEDIUM for unmapped / invalid / edge inputs
		{"", models.PriorityMedium, "Empty string fallback to MEDIUM"},
		{"   ", models.PriorityMedium, "Whitespace only fallback to MEDIUM"},
		{"URGENT", models.PriorityMedium, "URGENT unrecognized fallback to MEDIUM"},
		{"CRITICAL", models.PriorityMedium, "CRITICAL unrecognized fallback to MEDIUM"},
		{"NORMAL", models.PriorityMedium, "NORMAL unrecognized fallback to MEDIUM"},
		{"123", models.PriorityMedium, "Numeric string fallback to MEDIUM"},
		{"!@#$%", models.PriorityMedium, "Special chars fallback to MEDIUM"},
		{"VERY_HIGH", models.PriorityMedium, "Compound word fallback to MEDIUM"},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			got := normalizePriorityMirror(tc.input)
			if got != tc.expectedPriority {
				t.Errorf("normalizePriority(%q) = %q; want %q", tc.input, got, tc.expectedPriority)
			}
		})
	}
}

// TestChallenger_TicketService_ResolveSLAPolicyDelegation verifies ResolveSLAPolicy method on TicketService.
func TestChallenger_TicketService_ResolveSLAPolicyDelegation(t *testing.T) {
	// Build a minimal config for NewJWTService (current signature expects *config.Config)
	cfg := &config.Config{
		JWTSecret: "test_secret_that_is_32_chars_long!",
	}
	jwtSvc := utils.NewJWTService(cfg)
	svc := services.NewTicketService(jwtSvc)

	// In offline/nil-DB mode, should safely return Tier 4 built-in constants
	policy, respDur, resDur := svc.ResolveSLAPolicy(nil, nil, models.PriorityHigh)
	if policy != nil {
		t.Errorf("Expected nil policy pointer on nil DB, got %+v", policy)
	}
	if respDur != models.FallbackResponseHigh {
		t.Errorf("Expected fallback response duration %v, got %v", models.FallbackResponseHigh, respDur)
	}
	if resDur != models.FallbackResolutionHigh {
		t.Errorf("Expected fallback resolution duration %v, got %v", models.FallbackResolutionHigh, resDur)
	}
}
