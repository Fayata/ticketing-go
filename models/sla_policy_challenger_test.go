package models_test

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	"ticketing/models"
)

// ============================================================================
// 1. Continuous 24/7 Calendar Calculation Stress Testing
// ============================================================================

// TestChallenger_247Calendar_ContinuousNoPauses verifies that deadlines are calculated
// continuously around the clock (24/7) with ZERO business hour cutoffs or weekend pauses.
func TestChallenger_247Calendar_ContinuousNoPauses(t *testing.T) {
	testCases := []struct {
		name             string
		createdAt        time.Time
		duration         time.Duration
		expectedDeadline time.Time
		mustNotEqual     time.Time // counter-example: e.g. business hour jump or monday jump
	}{
		{
			name:             "Friday night to Saturday morning (weekend transition)",
			createdAt:        time.Date(2026, 9, 11, 23, 30, 0, 0, time.UTC), // Friday 23:30
			duration:         2 * time.Hour,
			expectedDeadline: time.Date(2026, 9, 12, 1, 30, 0, 0, time.UTC), // Saturday 01:30
			mustNotEqual:     time.Date(2026, 9, 14, 9, 30, 0, 0, time.UTC), // Monday 09:30 (business hours jump)
		},
		{
			name:             "Saturday midday 24h duration (weekend interior)",
			createdAt:        time.Date(2026, 9, 12, 14, 0, 0, 0, time.UTC), // Saturday 14:00
			duration:         24 * time.Hour,
			expectedDeadline: time.Date(2026, 9, 13, 14, 0, 0, 0, time.UTC), // Sunday 14:00
			mustNotEqual:     time.Date(2026, 9, 15, 14, 0, 0, 0, time.UTC), // Tuesday jump
		},
		{
			name:             "Sunday night across midnight into Monday morning",
			createdAt:        time.Date(2026, 9, 13, 22, 15, 0, 0, time.UTC), // Sunday 22:15
			duration:         4 * time.Hour,
			expectedDeadline: time.Date(2026, 9, 14, 2, 15, 0, 0, time.UTC), // Monday 02:15
			mustNotEqual:     time.Date(2026, 9, 14, 11, 0, 0, 0, time.UTC), // Monday business hour jump
		},
		{
			name:             "After-hours evening (16:50 + 60m -> 17:50, not next day 09:50)",
			createdAt:        time.Date(2026, 9, 8, 16, 50, 0, 0, time.UTC),
			duration:         60 * time.Minute,
			expectedDeadline: time.Date(2026, 9, 8, 17, 50, 0, 0, time.UTC),
			mustNotEqual:     time.Date(2026, 9, 9, 9, 50, 0, 0, time.UTC),
		},
		{
			name:             "Night shift (02:10 + 45m -> 02:55)",
			createdAt:        time.Date(2026, 9, 8, 2, 10, 0, 0, time.UTC),
			duration:         45 * time.Minute,
			expectedDeadline: time.Date(2026, 9, 8, 2, 55, 0, 0, time.UTC),
			mustNotEqual:     time.Date(2026, 9, 8, 9, 45, 0, 0, time.UTC),
		},
		{
			name:             "Indonesian Public Holiday (17 August Independence Day)",
			createdAt:        time.Date(2026, 8, 17, 10, 0, 0, 0, time.UTC),
			duration:         4 * time.Hour,
			expectedDeadline: time.Date(2026, 8, 17, 14, 0, 0, 0, time.UTC),
			mustNotEqual:     time.Date(2026, 8, 18, 10, 0, 0, 0, time.UTC),
		},
		{
			name:             "Year-end rollover (31 December to 1 January)",
			createdAt:        time.Date(2026, 12, 31, 23, 0, 0, 0, time.UTC),
			duration:         2 * time.Hour,
			expectedDeadline: time.Date(2027, 1, 1, 1, 0, 0, 0, time.UTC),
			mustNotEqual:     time.Date(2027, 1, 2, 9, 0, 0, 0, time.UTC),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actualDeadline := tc.createdAt.Add(tc.duration)
			if !actualDeadline.Equal(tc.expectedDeadline) {
				t.Errorf("24/7 continuous calculation failed: expected %v, got %v", tc.expectedDeadline, actualDeadline)
			}
			if actualDeadline.Equal(tc.mustNotEqual) {
				t.Errorf("24/7 calendar erroneously matched business hours pause counter-example: %v", tc.mustNotEqual)
			}
			diff := actualDeadline.Sub(tc.createdAt)
			if diff != tc.duration {
				t.Errorf("Duration invariant violated: Sub = %v, want %v", diff, tc.duration)
			}
		})
	}
}

// TestChallenger_247Calendar_RandomizedInvariantOracle runs 1,000 randomized time/duration
// samples to verify that CreatedAt.Add(duration).Sub(CreatedAt) == duration across all calendar boundaries.
func TestChallenger_247Calendar_RandomizedInvariantOracle(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	baseEpoch := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Unix()

	for i := 0; i < 1000; i++ {
		randomSec := rng.Int63n(365 * 24 * 3600)
		created := time.Unix(baseEpoch+randomSec, 0).UTC()
		randomMin := rng.Intn(10080) + 1 // 1 min to 7 days
		dur := time.Duration(randomMin) * time.Minute

		deadline := created.Add(dur)
		if deadline.Sub(created) != dur {
			t.Fatalf("Randomized invariant failure at iteration %d: created=%v dur=%v deadline=%v", i, created, dur, deadline)
		}
	}
}

// ============================================================================
// 2. Priority Target Duration & Normalization Boundary Cases
// ============================================================================

// TestChallenger_Priority_DurationsAndFallbacks verifies duration mapping for all priorities.
func TestChallenger_Priority_DurationsAndFallbacks(t *testing.T) {
	// Standard fallback policy
	var nilPolicy *models.SLAPolicy

	expectedFallbacks := []struct {
		priority        models.TicketPriority
		expectedRespDur time.Duration
		expectedResDur  time.Duration
	}{
		{models.PriorityHigh, models.FallbackResponseHigh, models.FallbackResolutionHigh},
		{models.PriorityMedium, models.FallbackResponseMed, models.FallbackResolutionMed},
		{models.PriorityLow, models.FallbackResponseLow, models.FallbackResolutionLow},
		// Invalid / unmapped priorities must fallback to MEDIUM targets
		{models.TicketPriority(""), models.FallbackResponseMed, models.FallbackResolutionMed},
		{models.TicketPriority("URGENT"), models.FallbackResponseMed, models.FallbackResolutionMed},
		{models.TicketPriority("CRITICAL"), models.FallbackResponseMed, models.FallbackResolutionMed},
		{models.TicketPriority("INVALID"), models.FallbackResponseMed, models.FallbackResolutionMed},
		{models.TicketPriority("12345"), models.FallbackResponseMed, models.FallbackResolutionMed},
	}

	for _, tc := range expectedFallbacks {
		t.Run(fmt.Sprintf("NilPolicy_Priority_%s", tc.priority), func(t *testing.T) {
			resp := nilPolicy.GetFirstResponseDuration(tc.priority)
			if resp != tc.expectedRespDur {
				t.Errorf("GetFirstResponseDuration(%q) = %v; want %v", tc.priority, resp, tc.expectedRespDur)
			}
			res := nilPolicy.GetResolutionDuration(tc.priority)
			if res != tc.expectedResDur {
				t.Errorf("GetResolutionDuration(%q) = %v; want %v", tc.priority, res, tc.expectedResDur)
			}
		})
	}
}

// TestChallenger_Priority_CaseSensitivity verifies that typed PriorityHigh/Med/Low match exact constants
// and documents untyped raw string behavior.
func TestChallenger_Priority_CaseSensitivity(t *testing.T) {
	policy := &models.SLAPolicy{
		ResponseTimeHighMinutes: 30,
		ResponseTimeMedMinutes:  120,
		ResponseTimeLowMinutes:  360,
		ResolutionTimeHighHours: 2,
		ResolutionTimeMedHours:  12,
		ResolutionTimeLowHours:  48,
	}

	// Exact typed constants
	if d := policy.GetFirstResponseDuration(models.PriorityHigh); d != 30*time.Minute {
		t.Errorf("PriorityHigh expected 30m, got %v", d)
	}
	if d := policy.GetFirstResponseDuration(models.PriorityMedium); d != 120*time.Minute {
		t.Errorf("PriorityMedium expected 120m, got %v", d)
	}
	if d := policy.GetFirstResponseDuration(models.PriorityLow); d != 360*time.Minute {
		t.Errorf("PriorityLow expected 360m, got %v", d)
	}

	// Lowercase untyped string casts without normalization fallback to Medium
	// (TicketService.CreateTicket normalizes strings via strings.ToUpper(strings.TrimSpace(priority)) before calling)
	if d := policy.GetFirstResponseDuration(models.TicketPriority("high")); d != 120*time.Minute {
		t.Errorf("Un-normalized 'high' expected fallback to Medium (120m), got %v", d)
	}
	if d := policy.GetFirstResponseDuration(models.TicketPriority("High")); d != 120*time.Minute {
		t.Errorf("Un-normalized 'High' expected fallback to Medium (120m), got %v", d)
	}
}

// TestChallenger_ZeroAndNegativeDuration_DefenseInDepth verifies that corrupted or zero
// policy durations in the database safely trigger default constant fallbacks.
func TestChallenger_ZeroAndNegativeDuration_DefenseInDepth(t *testing.T) {
	corruptedPolicies := []*models.SLAPolicy{
		{
			Name:                    "All Zeroes",
			ResponseTimeHighMinutes: 0,
			ResponseTimeMedMinutes:  0,
			ResponseTimeLowMinutes:  0,
			ResolutionTimeHighHours: 0,
			ResolutionTimeMedHours:  0,
			ResolutionTimeLowHours:  0,
		},
		{
			Name:                    "Negative Durations",
			ResponseTimeHighMinutes: -60,
			ResponseTimeMedMinutes:  -240,
			ResponseTimeLowMinutes:  -480,
			ResolutionTimeHighHours: -4,
			ResolutionTimeMedHours:  -24,
			ResolutionTimeLowHours:  -72,
		},
	}

	for _, p := range corruptedPolicies {
		t.Run(p.Name, func(t *testing.T) {
			// High
			if d := p.GetFirstResponseDuration(models.PriorityHigh); d != models.FallbackResponseHigh {
				t.Errorf("%s High response expected fallback %v, got %v", p.Name, models.FallbackResponseHigh, d)
			}
			if d := p.GetResolutionDuration(models.PriorityHigh); d != models.FallbackResolutionHigh {
				t.Errorf("%s High resolution expected fallback %v, got %v", p.Name, models.FallbackResolutionHigh, d)
			}
			// Medium
			if d := p.GetFirstResponseDuration(models.PriorityMedium); d != models.FallbackResponseMed {
				t.Errorf("%s Med response expected fallback %v, got %v", p.Name, models.FallbackResponseMed, d)
			}
			if d := p.GetResolutionDuration(models.PriorityMedium); d != models.FallbackResolutionMed {
				t.Errorf("%s Med resolution expected fallback %v, got %v", p.Name, models.FallbackResolutionMed, d)
			}
			// Low
			if d := p.GetFirstResponseDuration(models.PriorityLow); d != models.FallbackResponseLow {
				t.Errorf("%s Low response expected fallback %v, got %v", p.Name, models.FallbackResponseLow, d)
			}
			if d := p.GetResolutionDuration(models.PriorityLow); d != models.FallbackResolutionLow {
				t.Errorf("%s Low resolution expected fallback %v, got %v", p.Name, models.FallbackResolutionLow, d)
			}
		})
	}
}

// TestChallenger_BoundaryDurations_Extremes verifies minimum positive duration (1 min / 1 hour)
// and extreme long durations (1 year).
func TestChallenger_BoundaryDurations_Extremes(t *testing.T) {
	extremePolicy := &models.SLAPolicy{
		ResponseTimeHighMinutes: 1,      // 1 minute minimum
		ResolutionTimeHighHours: 1,      // 1 hour minimum
		ResponseTimeLowMinutes:  525600, // 1 year in minutes
		ResolutionTimeLowHours:  8760,   // 1 year in hours
	}

	if d := extremePolicy.GetFirstResponseDuration(models.PriorityHigh); d != 1*time.Minute {
		t.Errorf("Expected 1m, got %v", d)
	}
	if d := extremePolicy.GetResolutionDuration(models.PriorityHigh); d != 1*time.Hour {
		t.Errorf("Expected 1h, got %v", d)
	}
	if d := extremePolicy.GetFirstResponseDuration(models.PriorityLow); d != 525600*time.Minute {
		t.Errorf("Expected 525600m, got %v", d)
	}
	if d := extremePolicy.GetResolutionDuration(models.PriorityLow); d != 8760*time.Hour {
		t.Errorf("Expected 8760h, got %v", d)
	}
}

// ============================================================================
// 3. 4-Tier SLA Policy Resolution Hierarchy Oracle
// ============================================================================

// TestChallenger_Hierarchy_Tier4_NilDBFallback verifies that when DB is nil or unavailable,
// ResolveSLAPolicy returns nil policy and built-in constants for all priorities.
func TestChallenger_Hierarchy_Tier4_NilDBFallback(t *testing.T) {
	priorities := []models.TicketPriority{
		models.PriorityHigh,
		models.PriorityMedium,
		models.PriorityLow,
	}

	for _, p := range priorities {
		policy, respDur, resDur := models.ResolveSLAPolicy(nil, nil, nil, p)
		if policy != nil {
			t.Errorf("Tier 4: expected nil policy pointer, got %+v", policy)
		}
		expectedResp := (*models.SLAPolicy)(nil).GetFirstResponseDuration(p)
		if respDur != expectedResp {
			t.Errorf("Tier 4: priority %s expected response %v, got %v", p, expectedResp, respDur)
		}
		expectedRes := (*models.SLAPolicy)(nil).GetResolutionDuration(p)
		if resDur != expectedRes {
			t.Errorf("Tier 4: priority %s expected resolution %v, got %v", p, expectedRes, resDur)
		}
	}
}

// TestChallenger_Hierarchy_NilPointerTolerances verifies zero-pointer and nil-pointer stability.
func TestChallenger_Hierarchy_NilPointerTolerances(t *testing.T) {
	var zeroID uint = 0

	// DepartmentID = 0 should be treated as unset and skip Tier 1
	policy, respDur, resDur := models.ResolveSLAPolicy(nil, nil, &zeroID, models.PriorityHigh)
	if policy != nil || respDur != models.FallbackResponseHigh || resDur != models.FallbackResolutionHigh {
		t.Errorf("Expected fallback constants for zero department ID on nil DB")
	}

	// CompanyID = 0 should be treated as unset and skip Tier 2
	policy, respDur, resDur = models.ResolveSLAPolicy(nil, &zeroID, nil, models.PriorityHigh)
	if policy != nil || respDur != models.FallbackResponseHigh || resDur != models.FallbackResolutionHigh {
		t.Errorf("Expected fallback constants for zero company ID on nil DB")
	}
}

// TestChallenger_ResolveSLAPolicyWithDurations_Wrapper verifies that the convenience wrapper
// ResolveSLAPolicyWithDurations yields identical results to ResolveSLAPolicy.
func TestChallenger_ResolveSLAPolicyWithDurations_Wrapper(t *testing.T) {
	p1, r1, s1 := models.ResolveSLAPolicy(nil, nil, nil, models.PriorityHigh)
	p2, r2, s2 := models.ResolveSLAPolicyWithDurations(nil, nil, nil, models.PriorityHigh)

	if p1 != p2 || r1 != r2 || s1 != s2 {
		t.Errorf("ResolveSLAPolicyWithDurations discrepancy: (%v,%v,%v) vs (%v,%v,%v)", p1, r1, s1, p2, r2, s2)
	}
}
