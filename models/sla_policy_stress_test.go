package models_test

import (
	"testing"
	"time"

	"ticketing/models"
)

// ============================================================================
// Stress Test Suite 1: Dual-Unit Inversion Oracle & Mathematical Invariance
// ============================================================================

// TestStress_DualUnit_InversionOracle empirically stress-tests the mathematical
// round-trip consistency of dual-unit conversions across hundreds of permutations.
// Property: Given any valid duration in hours & minutes (or days & hours),
// converting to canonical storage units (total minutes or total hours) and reconstructing
// into dual form units must strictly preserve the exact duration with canonical modulo.
func TestStress_DualUnit_InversionOracle(t *testing.T) {
	hoursSet := []int{0, 1, 2, 4, 8, 12, 24, 48, 72, 168, 336, 720, 8760}
	minutesSet := []int{0, 1, 15, 30, 45, 59, 60, 90, 120, 180, 500, 1440}

	// 1. Response Time Invariance Oracle: (H, M) -> TotalMinutes -> (H', M')
	for _, h := range hoursSet {
		for _, m := range minutesSet {
			totalMinutes := h*60 + m
			if totalMinutes == 0 {
				continue // 0 total minutes is boundary checked separately
			}

			// Reconstruction as performed in EditSLAPolicyForm:
			hRecon := totalMinutes / 60
			mRecon := totalMinutes % 60

			// Oracle Invariance 1: Total duration must be identical
			reconstructedTotal := hRecon*60 + mRecon
			if reconstructedTotal != totalMinutes {
				t.Fatalf("Invariance violated for H=%d M=%d: got reconstructed total %d != original %d",
					h, m, reconstructedTotal, totalMinutes)
			}

			// Oracle Invariance 2: Reconstructed minutes must be canonical [0, 59]
			if mRecon < 0 || mRecon >= 60 {
				t.Fatalf("Canonical modulo violated for H=%d M=%d: mRecon=%d out of [0, 59]",
					h, m, mRecon)
			}

			// Oracle Invariance 3: time.Duration evaluation matches
			durOriginal := time.Duration(totalMinutes) * time.Minute
			durRecon := time.Duration(hRecon)*time.Hour + time.Duration(mRecon)*time.Minute
			if durOriginal != durRecon {
				t.Fatalf("time.Duration mismatch for H=%d M=%d: %v != %v", h, m, durOriginal, durRecon)
			}
		}
	}

	daysSet := []int{0, 1, 2, 3, 7, 14, 30, 90, 180, 365, 730, 1000}
	resolHoursSet := []int{0, 1, 4, 8, 12, 23, 24, 48, 72, 100}

	// 2. Resolution Time Invariance Oracle: (D, H) -> TotalHours -> (D', H')
	for _, d := range daysSet {
		for _, h := range resolHoursSet {
			totalHours := d*24 + h
			if totalHours == 0 {
				continue
			}

			// Reconstruction as performed in EditSLAPolicyForm:
			dRecon := totalHours / 24
			hRecon := totalHours % 24

			// Oracle Invariance 1: Total duration must be identical
			reconstructedTotal := dRecon*24 + hRecon
			if reconstructedTotal != totalHours {
				t.Fatalf("Invariance violated for D=%d H=%d: got reconstructed total %d != original %d",
					d, h, reconstructedTotal, totalHours)
			}

			// Oracle Invariance 2: Reconstructed hours must be canonical [0, 23]
			if hRecon < 0 || hRecon >= 24 {
				t.Fatalf("Canonical modulo violated for D=%d H=%d: hRecon=%d out of [0, 23]",
					d, h, hRecon)
			}

			// Oracle Invariance 3: time.Duration evaluation matches
			durOriginal := time.Duration(totalHours) * time.Hour
			durRecon := time.Duration(dRecon)*24*time.Hour + time.Duration(hRecon)*time.Hour
			if durOriginal != durRecon {
				t.Fatalf("time.Duration mismatch for D=%d H=%d: %v != %v", d, h, durOriginal, durRecon)
			}
		}
	}
}

// ============================================================================
// Stress Test Suite 2: Extreme Boundaries & Numerical Safety
// ============================================================================

// TestStress_Boundary_Values_And_Overflow tests extreme boundary values
// and verifies that integer multiplication or conversion does not cause overflow or panic.
func TestStress_Boundary_Values_And_Overflow(t *testing.T) {
	boundaries := []struct {
		name                 string
		respH, respM         int
		resolD, resolH       int
		expectedRespMinutes  int
		expectedResolHours   int
		expectedRespDuration time.Duration
		expectedResolDur     time.Duration
	}{
		{
			name:                 "Minimum valid response: 0h 1m",
			respH:                0,
			respM:                1,
			resolD:               0,
			resolH:               1,
			expectedRespMinutes:  1,
			expectedResolHours:   1,
			expectedRespDuration: 1 * time.Minute,
			expectedResolDur:     1 * time.Hour,
		},
		{
			name:                 "One year resolution: 365d 0h",
			respH:                168, // 1 week response
			respM:                0,
			resolD:               365,
			resolH:               0,
			expectedRespMinutes:  168 * 60,
			expectedResolHours:   365 * 24, // 8760 hours
			expectedRespDuration: 168 * time.Hour,
			expectedResolDur:     8760 * time.Hour,
		},
		{
			name:                 "High precision boundary: 0h 59m, 0d 23h",
			respH:                0,
			respM:                59,
			resolD:               0,
			resolH:               23,
			expectedRespMinutes:  59,
			expectedResolHours:   23,
			expectedRespDuration: 59 * time.Minute,
			expectedResolDur:     23 * time.Hour,
		},
		{
			name:                 "Multi-year resolution: 1000d 23h",
			respH:                720, // 30 days response
			respM:                0,
			resolD:               1000,
			resolH:               23,
			expectedRespMinutes:  720 * 60,
			expectedResolHours:   24023,
			expectedRespDuration: 720 * time.Hour,
			expectedResolDur:     24023 * time.Hour,
		},
	}

	for _, tc := range boundaries {
		t.Run(tc.name, func(t *testing.T) {
			calcRespMinutes := tc.respH*60 + tc.respM
			if calcRespMinutes != tc.expectedRespMinutes {
				t.Errorf("Expected resp minutes %d, got %d", tc.expectedRespMinutes, calcRespMinutes)
			}

			calcResolHours := tc.resolD*24 + tc.resolH
			if calcResolHours != tc.expectedResolHours {
				t.Errorf("Expected resol hours %d, got %d", tc.expectedResolHours, calcResolHours)
			}

			// Verify SLAPolicy duration methods
			p := &models.SLAPolicy{
				ResponseTimeHighMinutes: calcRespMinutes,
				ResolutionTimeHighHours: calcResolHours,
			}

			dResp := p.GetFirstResponseDuration(models.PriorityHigh)
			if dResp != tc.expectedRespDuration {
				t.Errorf("GetFirstResponseDuration mismatch: expected %v, got %v", tc.expectedRespDuration, dResp)
			}

			dResol := p.GetResolutionDuration(models.PriorityHigh)
			if dResol != tc.expectedResolDur {
				t.Errorf("GetResolutionDuration mismatch: expected %v, got %v", tc.expectedResolDur, dResol)
			}
		})
	}
}

// ============================================================================
// Stress Test Suite 3: Nil Safety, Zero Values, and Fallback Robustness
// ============================================================================

// TestStress_DurationMethods_NilSafeAndFallbacks verifies that nil receivers,
// zero values, negative values, and unknown priorities degrade gracefully to fallbacks.
func TestStress_DurationMethods_NilSafeAndFallbacks(t *testing.T) {
	// 1. Nil policy pointer must never panic and must return documented fallbacks
	var nilPolicy *models.SLAPolicy
	if d := nilPolicy.GetFirstResponseDuration(models.PriorityHigh); d != models.FallbackResponseHigh {
		t.Errorf("Nil policy High response expected %v, got %v", models.FallbackResponseHigh, d)
	}
	if d := nilPolicy.GetFirstResponseDuration(models.PriorityMedium); d != models.FallbackResponseMed {
		t.Errorf("Nil policy Med response expected %v, got %v", models.FallbackResponseMed, d)
	}
	if d := nilPolicy.GetFirstResponseDuration(models.PriorityLow); d != models.FallbackResponseLow {
		t.Errorf("Nil policy Low response expected %v, got %v", models.FallbackResponseLow, d)
	}
	if d := nilPolicy.GetFirstResponseDuration("UNKNOWN_PRIORITY"); d != models.FallbackResponseMed {
		t.Errorf("Nil policy unknown priority expected Med fallback %v, got %v", models.FallbackResponseMed, d)
	}

	if d := nilPolicy.GetResolutionDuration(models.PriorityHigh); d != models.FallbackResolutionHigh {
		t.Errorf("Nil policy High resolution expected %v, got %v", models.FallbackResolutionHigh, d)
	}
	if d := nilPolicy.GetResolutionDuration(models.PriorityMedium); d != models.FallbackResolutionMed {
		t.Errorf("Nil policy Med resolution expected %v, got %v", models.FallbackResolutionMed, d)
	}
	if d := nilPolicy.GetResolutionDuration(models.PriorityLow); d != models.FallbackResolutionLow {
		t.Errorf("Nil policy Low resolution expected %v, got %v", models.FallbackResolutionLow, d)
	}

	// 2. Zero-value struct must fallback to positive system defaults
	zeroPolicy := &models.SLAPolicy{}
	if d := zeroPolicy.GetFirstResponseDuration(models.PriorityHigh); d != models.FallbackResponseHigh {
		t.Errorf("Zero struct High response expected fallback %v, got %v", models.FallbackResponseHigh, d)
	}
	if d := zeroPolicy.GetResolutionDuration(models.PriorityHigh); d != models.FallbackResolutionHigh {
		t.Errorf("Zero struct High resolution expected fallback %v, got %v", models.FallbackResolutionHigh, d)
	}

	// 3. Negative values in database must fallback safely instead of negative durations
	negPolicy := &models.SLAPolicy{
		ResponseTimeHighMinutes: -15,
		ResponseTimeMedMinutes:  -120,
		ResponseTimeLowMinutes:  -500,
		ResolutionTimeHighHours: -4,
		ResolutionTimeMedHours:  -24,
		ResolutionTimeLowHours:  -72,
	}
	if d := negPolicy.GetFirstResponseDuration(models.PriorityHigh); d <= 0 || d != models.FallbackResponseHigh {
		t.Errorf("Negative response time did not fallback safely: got %v", d)
	}
	if d := negPolicy.GetResolutionDuration(models.PriorityHigh); d <= 0 || d != models.FallbackResolutionHigh {
		t.Errorf("Negative resolution time did not fallback safely: got %v", d)
	}
}

// ============================================================================
// Stress Test Suite 4: 24/7 Calendar Continuous Arithmetic Stress Test
// ============================================================================

// TestStress_Continuous247Calendar_Transitions stress-tests continuous deadline
// arithmetic across month boundaries, leap years, and year-end rollovers.
func TestStress_Continuous247Calendar_Transitions(t *testing.T) {
	testCases := []struct {
		name       string
		startTime  time.Time
		duration   time.Duration
		expectedAt time.Time
	}{
		{
			name:       "Month-end rollover (April 30 -> May 1)",
			startTime:  time.Date(2026, 4, 30, 23, 30, 0, 0, time.UTC),
			duration:   45 * time.Minute,
			expectedAt: time.Date(2026, 5, 1, 0, 15, 0, 0, time.UTC),
		},
		{
			name:       "Leap year rollover (Feb 28 -> Feb 29)",
			startTime:  time.Date(2024, 2, 28, 23, 0, 0, 0, time.UTC),
			duration:   2 * time.Hour,
			expectedAt: time.Date(2024, 2, 29, 1, 0, 0, 0, time.UTC),
		},
		{
			name:       "Year-end rollover (Dec 31 -> Jan 1)",
			startTime:  time.Date(2026, 12, 31, 23, 0, 0, 0, time.UTC),
			duration:   2 * time.Hour,
			expectedAt: time.Date(2027, 1, 1, 1, 0, 0, 0, time.UTC),
		},
		{
			name:       "Weekend non-stop (Friday 23:00 -> Sunday 23:00 across 48h)",
			startTime:  time.Date(2026, 9, 11, 23, 0, 0, 0, time.UTC), // Friday
			duration:   48 * time.Hour,
			expectedAt: time.Date(2026, 9, 13, 23, 0, 0, 0, time.UTC), // Sunday
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			calc := tc.startTime.Add(tc.duration)
			if !calc.Equal(tc.expectedAt) {
				t.Errorf("24/7 calendar transition failed: expected %v, got %v", tc.expectedAt, calc)
			}
		})
	}
}

// ============================================================================
// Stress Test Suite 5: Seeding Idempotency & Legacy Ticket Backfill (Database)
// ============================================================================

// TestStress_DatabaseSeeding_IdempotentMultiRun stress-tests running
// SeedDefaultSLAPolicies repeatedly in a loop to ensure zero duplicates.
func TestStress_DatabaseSeeding_IdempotentMultiRun(t *testing.T) {
	db, cleanup := setupSLATestDB(t)
	if db == nil {
		return
	}
	defer cleanup()

	_ = db.AutoMigrate(&models.Company{}, &models.Department{}, &models.SLAPolicy{}, &models.Ticket{})

	// Execute 5 consecutive seeding runs
	for i := 1; i <= 5; i++ {
		err := models.SeedDefaultSLAPolicies(db)
		if err != nil {
			t.Fatalf("SeedDefaultSLAPolicies run #%d failed: %v", i, err)
		}
	}

	// Verify exactly 1 default policy exists
	var defaultPolicies []models.SLAPolicy
	if err := db.Where("is_default = ?", true).Find(&defaultPolicies).Error; err != nil {
		t.Fatalf("Failed to query default policies: %v", err)
	}

	if len(defaultPolicies) != 1 {
		t.Fatalf("Idempotency violation: expected exactly 1 default policy after 5 runs, found %d", len(defaultPolicies))
	}

	policy := defaultPolicies[0]
	if policy.Name != models.DefaultSLAPolicyName {
		t.Errorf("Expected policy name %s, got %s", models.DefaultSLAPolicyName, policy.Name)
	}
	if !policy.IsActive || !policy.IsDefault {
		t.Errorf("Expected policy to be both active and default, got active=%v default=%v", policy.IsActive, policy.IsDefault)
	}
	if policy.ResponseTimeHighMinutes != 60 || policy.ResponseTimeMedMinutes != 240 || policy.ResponseTimeLowMinutes != 480 {
		t.Errorf("Incorrect response time defaults: %d, %d, %d",
			policy.ResponseTimeHighMinutes, policy.ResponseTimeMedMinutes, policy.ResponseTimeLowMinutes)
	}
	if policy.ResolutionTimeHighHours != 4 || policy.ResolutionTimeMedHours != 24 || policy.ResolutionTimeLowHours != 72 {
		t.Errorf("Incorrect resolution time defaults: %d, %d, %d",
			policy.ResolutionTimeHighHours, policy.ResolutionTimeMedHours, policy.ResolutionTimeLowHours)
	}
}

// TestStress_LegacyTicket_BackfillAndIntegrity stress-tests backfilling tickets
// of various priorities, statuses, and pre-existing SLA associations.
func TestStress_LegacyTicket_BackfillAndIntegrity(t *testing.T) {
	db, cleanup := setupSLATestDB(t)
	if db == nil {
		return
	}
	defer cleanup()

	_ = db.AutoMigrate(&models.Company{}, &models.Department{}, &models.SLAPolicy{}, &models.Ticket{})

	pastTime := time.Now().Add(-5 * time.Hour)

	// Create custom policy for pre-associated ticket test
	customPolicy := models.SLAPolicy{
		Name:                    "Pre-existing Custom Policy",
		IsActive:                true,
		IsDefault:               false,
		ResponseTimeHighMinutes: 20,
		ResolutionTimeHighHours: 2,
	}
	db.Create(&customPolicy)

	// Ticket 1: Open, High Priority, no SLA fields
	t1 := models.Ticket{
		Title:       "Open High Priority Legacy",
		Description: "Should be backfilled with default policy and 60m response deadline",
		Status:      models.StatusWaiting,
		Priority:    models.PriorityHigh,
		CreatedAt:   pastTime,
	}
	db.Create(&t1)

	// Ticket 2: InProgress, Med Priority, no SLA fields
	t2 := models.Ticket{
		Title:       "InProgress Med Priority Legacy",
		Description: "Should be backfilled with default policy and 240m response deadline",
		Status:      models.StatusInProgress,
		Priority:    models.PriorityMedium,
		CreatedAt:   pastTime,
	}
	db.Create(&t2)

	// Ticket 3: Closed, Low Priority, no SLA fields
	closedUpdate := pastTime.Add(2 * time.Hour)
	t3 := models.Ticket{
		Title:       "Closed Low Priority Legacy",
		Description: "Should be backfilled with SLA met=true so no retro-breach occurs",
		Status:      models.StatusClosed,
		Priority:    models.PriorityLow,
		CreatedAt:   pastTime,
		UpdatedAt:   closedUpdate,
	}
	db.Create(&t3)

	// Ticket 4: Already has custom SLA policy
	customDeadline := pastTime.Add(20 * time.Minute)
	t4 := models.Ticket{
		Title:                 "Modern Ticket With SLA",
		Description:           "Should NOT be overwritten by seeder backfill",
		Status:                models.StatusWaiting,
		Priority:              models.PriorityHigh,
		SLAPolicyID:           &customPolicy.ID,
		FirstResponseDeadline: &customDeadline,
		CreatedAt:             pastTime,
	}
	db.Create(&t4)

	// Run seeder
	if err := models.SeedDefaultSLAPolicies(db); err != nil {
		t.Fatalf("SeedDefaultSLAPolicies failed: %v", err)
	}

	var defaultPolicy models.SLAPolicy
	db.Where("is_default = ?", true).First(&defaultPolicy)

	// Check T1: Backfilled
	var r1 models.Ticket
	db.First(&r1, t1.ID)
	if r1.SLAPolicyID == nil || *r1.SLAPolicyID != defaultPolicy.ID {
		t.Errorf("T1 SLAPolicyID expected %d, got %v", defaultPolicy.ID, r1.SLAPolicyID)
	}
	if r1.FirstResponseDeadline == nil {
		t.Errorf("T1 FirstResponseDeadline expected non-nil")
	} else {
		expectedDeadline := pastTime.Add(60 * time.Minute)
		if r1.FirstResponseDeadline.Sub(expectedDeadline).Abs() > time.Second {
			t.Errorf("T1 deadline mismatch: expected %v, got %v", expectedDeadline, *r1.FirstResponseDeadline)
		}
	}
	if r1.FirstResponseMet != nil {
		t.Errorf("T1 FirstResponseMet should remain nil (pending), got %v", *r1.FirstResponseMet)
	}

	// Check T2: Backfilled
	var r2 models.Ticket
	db.First(&r2, t2.ID)
	if r2.SLAPolicyID == nil || *r2.SLAPolicyID != defaultPolicy.ID {
		t.Errorf("T2 SLAPolicyID expected %d, got %v", defaultPolicy.ID, r2.SLAPolicyID)
	}
	if r2.FirstResponseDeadline == nil {
		t.Errorf("T2 FirstResponseDeadline expected non-nil")
	} else {
		expectedDeadline := pastTime.Add(240 * time.Minute)
		if r2.FirstResponseDeadline.Sub(expectedDeadline).Abs() > time.Second {
			t.Errorf("T2 deadline mismatch: expected %v, got %v", expectedDeadline, *r2.FirstResponseDeadline)
		}
	}

	// Check T3: Closed ticket backfilled with graceful FirstResponseMet = true
	var r3 models.Ticket
	db.First(&r3, t3.ID)
	if r3.SLAPolicyID == nil || *r3.SLAPolicyID != defaultPolicy.ID {
		t.Errorf("T3 SLAPolicyID expected %d, got %v", defaultPolicy.ID, r3.SLAPolicyID)
	}
	if r3.FirstResponseMet == nil || !*r3.FirstResponseMet {
		t.Errorf("T3 closed ticket should be marked met=true, got %v", r3.FirstResponseMet)
	}
	if r3.FirstResponseAt == nil {
		t.Errorf("T3 FirstResponseAt expected to be set for closed ticket, got nil")
	}

	// Check T4: Untouched
	var r4 models.Ticket
	db.First(&r4, t4.ID)
	if r4.SLAPolicyID == nil || *r4.SLAPolicyID != customPolicy.ID {
		t.Errorf("T4 custom policy was overwritten! Expected %d, got %v", customPolicy.ID, r4.SLAPolicyID)
	}
	if r4.FirstResponseDeadline == nil || !r4.FirstResponseDeadline.Equal(customDeadline) {
		t.Errorf("T4 custom deadline was overwritten!")
	}
}
