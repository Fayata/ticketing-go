package handlers

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"ticketing/config"
	"ticketing/middleware"
	"ticketing/models"
)

// withStaffContext attaches a staff user to the request context.
func withStaffContext(r *http.Request, user *models.User) *http.Request {
	ctx := context.WithValue(r.Context(), middleware.UserKey, user)
	return r.WithContext(ctx)
}

// --------------------------------------------------------------------------
// SUITE A: PURE UNIT TESTS (Fast, In-Memory, No External Dependencies)
// --------------------------------------------------------------------------

// TestCalculateFirstResponseMet_Boundaries tests the 3-state boundary calculation for SLA first response.
func TestCalculateFirstResponseMet_Boundaries(t *testing.T) {
	now := time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC)

	// 1. Response achieved well before deadline -> Met
	futureDeadline := now.Add(30 * time.Minute)
	met := CalculateFirstResponseMet(now, &futureDeadline)
	if met == nil || *met != true {
		t.Fatalf("expected FirstResponseMet == true when now is before deadline, got %v", met)
	}

	// 2. Response achieved exactly on the deadline (!now.After(deadline)) -> Met (boundary equality)
	exactDeadline := now
	metExact := CalculateFirstResponseMet(now, &exactDeadline)
	if metExact == nil || *metExact != true {
		t.Fatalf("expected FirstResponseMet == true when now equals deadline, got %v", metExact)
	}

	// 3. Response achieved after deadline -> Breached
	pastDeadline := now.Add(-1 * time.Nanosecond)
	breached := CalculateFirstResponseMet(now, &pastDeadline)
	if breached == nil || *breached != false {
		t.Fatalf("expected FirstResponseMet == false when now is after deadline, got %v", breached)
	}

	// 4. No deadline specified -> nil (untracked/legacy)
	nilDeadlineResult := CalculateFirstResponseMet(now, nil)
	if nilDeadlineResult != nil {
		t.Fatalf("expected nil when deadline is nil, got %v", nilDeadlineResult)
	}
}

// TestParseEstimatedResolution_Presets tests standard duration presets.
func TestParseEstimatedResolution_Presets(t *testing.T) {
	baseTime := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		preset   string
		expected time.Duration
	}{
		{"1h", 1 * time.Hour},
		{"2h", 2 * time.Hour},
		{"4h", 4 * time.Hour},
		{"8h", 8 * time.Hour},
		{"1d", 24 * time.Hour},
		{"3d", 72 * time.Hour},
		{" 1H ", 1 * time.Hour},   // Case & whitespace insensitivity
		{"2H", 2 * time.Hour},
		{" 1D ", 24 * time.Hour},
	}

	for _, c := range cases {
		t.Run(c.preset, func(t *testing.T) {
			res, err := ParseEstimatedResolution(c.preset, "", baseTime)
			if err != nil {
				t.Fatalf("unexpected error for preset %q: %v", c.preset, err)
			}
			expectedTime := baseTime.Add(c.expected)
			if !res.Equal(expectedTime) {
				t.Errorf("preset %q: expected %v, got %v", c.preset, expectedTime, *res)
			}
		})
	}
}

// TestParseEstimatedResolution_CustomDate tests multi-layout parsing and validation.
func TestParseEstimatedResolution_CustomDate(t *testing.T) {
	baseTime := time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC)

	// Valid future date formats
	validDates := []string{
		"2026-09-08T15:30:00Z", // RFC3339
		"2026-09-08T15:30:00",  // HTML5 datetime-local with seconds
		"2026-09-08T15:30",     // HTML5 datetime-local without seconds
		"2026-09-08 15:30:00",  // Standard datetime
		"2026-09-08 15:30",     // Standard datetime no seconds
		"2026-09-09",           // Date only (tomorrow)
	}

	for _, dateStr := range validDates {
		t.Run(dateStr, func(t *testing.T) {
			res, err := ParseEstimatedResolution("custom", dateStr, baseTime)
			if err != nil {
				t.Fatalf("failed to parse valid date %q: %v", dateStr, err)
			}
			if !res.After(baseTime) {
				t.Errorf("parsed date %v should be strictly after baseTime %v", *res, baseTime)
			}
		})
	}

	// Rejection of past date
	pastDate := "2026-09-08T09:00:00Z"
	if _, err := ParseEstimatedResolution("custom", pastDate, baseTime); err == nil {
		t.Errorf("expected error when estimating date in the past, got nil")
	}

	// Rejection of empty custom date
	if _, err := ParseEstimatedResolution("custom", "", baseTime); err == nil {
		t.Errorf("expected error when custom date is empty, got nil")
	}

	// Rejection of malformed date string
	if _, err := ParseEstimatedResolution("custom", "not-a-date", baseTime); err == nil {
		t.Errorf("expected error for malformed date string, got nil")
	}

	// Rejection of invalid preset
	if _, err := ParseEstimatedResolution("100d", "", baseTime); err == nil {
		t.Errorf("expected error for unknown preset, got nil")
	}
}

// TestValidatePriorityAdjustment tests priority change constraints.
func TestValidatePriorityAdjustment(t *testing.T) {
	// Valid changes
	if err := ValidatePriorityAdjustment(models.PriorityLow, models.PriorityHigh, "Kebutuhan mendesak produksi"); err != nil {
		t.Errorf("unexpected error for valid priority adjustment: %v", err)
	}
	if err := ValidatePriorityAdjustment(models.PriorityHigh, models.PriorityMedium, "Tingkat keparahan diturunkan"); err != nil {
		t.Errorf("unexpected error for valid priority adjustment: %v", err)
	}

	// Identical priority rejection
	if err := ValidatePriorityAdjustment(models.PriorityHigh, models.PriorityHigh, "Tidak ada perubahan"); err == nil {
		t.Errorf("expected error when new priority equals old priority, got nil")
	}

	// Reason < 5 characters rejection
	if err := ValidatePriorityAdjustment(models.PriorityLow, models.PriorityHigh, "abc"); err == nil {
		t.Errorf("expected error when reason is < 5 chars, got nil")
	}

	// Whitespace-only reason rejection
	if err := ValidatePriorityAdjustment(models.PriorityLow, models.PriorityHigh, "     "); err == nil {
		t.Errorf("expected error when reason is whitespace, got nil")
	}

	// Invalid priority string rejection
	if err := ValidatePriorityAdjustment(models.PriorityLow, models.TicketPriority("CRITICAL"), "Alasan valid tapi prioritas salah"); err == nil {
		t.Errorf("expected error for invalid priority name, got nil")
	}
}

// TestRecalculateSLADeadlines_Logic tests recalculation rules when priority changes.
func TestRecalculateSLADeadlines_Logic(t *testing.T) {
	createdAt := time.Date(2026, 9, 8, 8, 0, 0, 0, time.UTC)
	respDuration := 60 * time.Minute
	resDuration := 4 * time.Hour

	// Case 1: First response is still pending (firstResponseAt == nil)
	// Both FirstResponseDeadline and ResolutionDeadline must be recalculated
	newResp, newRes := RecalculateSLADeadlines(createdAt, nil, respDuration, resDuration)
	if newResp == nil {
		t.Fatalf("expected newRespDeadline to be non-nil when firstResponseAt is nil")
	}
	expectedResp := createdAt.Add(respDuration)
	if !newResp.Equal(expectedResp) {
		t.Errorf("expected newRespDeadline %v, got %v", expectedResp, *newResp)
	}
	expectedRes := createdAt.Add(resDuration)
	if newRes == nil || !newRes.Equal(expectedRes) {
		t.Errorf("expected newResDeadline %v, got %v", expectedRes, newRes)
	}

	// Case 2: First response has already occurred (firstResponseAt != nil)
	// FirstResponseDeadline MUST NOT be recalculated (returns nil to indicate preserve)
	firstResponseAt := createdAt.Add(25 * time.Minute)
	newResp2, newRes2 := RecalculateSLADeadlines(createdAt, &firstResponseAt, respDuration, resDuration)
	if newResp2 != nil {
		t.Errorf("expected newRespDeadline to be nil (preserved) when firstResponseAt != nil, got %v", *newResp2)
	}
	if newRes2 == nil || !newRes2.Equal(expectedRes) {
		t.Errorf("expected newResDeadline %v, got %v", expectedRes, newRes2)
	}
}

// --------------------------------------------------------------------------
// SUITE B: INTEGRATION / HTTP HANDLER TESTS (Using setupHandlerTestDB)
// --------------------------------------------------------------------------

// TestClaimTicket_SLAFirstResponseMet tests that claiming before the deadline stamps FirstResponseMet = true.
func TestClaimTicket_SLAFirstResponseMet(t *testing.T) {
	db, cleanup := setupHandlerTestDB(t)
	if db == nil {
		return
	}
	defer cleanup()

	cfg := config.LoadConfig()
	handler := NewDepartmentHandler(cfg, nil, nil)

	// Seed Company & Department
	company := models.Company{Name: "PT SLA Corp", Code: "SLACORP", IsActive: true}
	db.Create(&company)
	dept := models.Department{Name: "IT Support", CompanyID: &company.ID}
	db.Create(&dept)

	// Seed Staff & Creator
	staff := models.User{Username: "staff_sla", DepartmentID: &dept.ID, IsStaff: true}
	db.Create(&staff)
	creator := models.User{Username: "user_client", IsStaff: false}
	db.Create(&creator)

	// Seed Ticket with future deadline
	futureDeadline := time.Now().Add(2 * time.Hour)
	ticket := models.Ticket{
		Title:                 "Gangguan Internet",
		Description:           "Koneksi lambat",
		Status:                models.StatusWaiting,
		Priority:              models.PriorityMedium,
		DepartmentID:          &dept.ID,
		CompanyID:             &company.ID,
		CreatedByID:           creator.ID,
		FirstResponseDeadline: &futureDeadline,
	}
	db.Create(&ticket)

	// Execute Claim
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/departement/tiket/claim/%d", ticket.ID), nil)
	req = withStaffContext(req, &staff)
	w := httptest.NewRecorder()

	handler.ClaimTicket(w, req)

	// Verify Ticket Updated in DB
	var updated models.Ticket
	if err := db.First(&updated, ticket.ID).Error; err != nil {
		t.Fatalf("failed to reload ticket: %v", err)
	}

	if updated.AssignedToID == nil || *updated.AssignedToID != staff.ID {
		t.Errorf("expected ticket to be assigned to staff %d, got %v", staff.ID, updated.AssignedToID)
	}
	if updated.Status != models.StatusInProgress {
		t.Errorf("expected status IN_PROGRESS, got %s", updated.Status)
	}
	if updated.FirstResponseAt == nil {
		t.Fatalf("expected FirstResponseAt to be stamped, got nil")
	}
	if updated.FirstResponseMet == nil || *updated.FirstResponseMet != true {
		t.Errorf("expected FirstResponseMet to be true, got %v", updated.FirstResponseMet)
	}
}

// TestClaimTicket_SLAFirstResponseBreached tests that claiming after the deadline stamps FirstResponseMet = false.
func TestClaimTicket_SLAFirstResponseBreached(t *testing.T) {
	db, cleanup := setupHandlerTestDB(t)
	if db == nil {
		return
	}
	defer cleanup()

	cfg := config.LoadConfig()
	handler := NewDepartmentHandler(cfg, nil, nil)

	company := models.Company{Name: "PT Breach Corp", Code: "BREACHCORP", IsActive: true}
	db.Create(&company)
	dept := models.Department{Name: "Support", CompanyID: &company.ID}
	db.Create(&dept)

	staff := models.User{Username: "staff_late", DepartmentID: &dept.ID, IsStaff: true}
	db.Create(&staff)
	creator := models.User{Username: "user_victim", IsStaff: false}
	db.Create(&creator)

	// Seed ticket with deadline in the past
	pastDeadline := time.Now().Add(-2 * time.Hour)
	ticket := models.Ticket{
		Title:                 "Aplikasi Error",
		Description:           "Critical issue",
		Status:                models.StatusWaiting,
		Priority:              models.PriorityHigh,
		DepartmentID:          &dept.ID,
		CompanyID:             &company.ID,
		CreatedByID:           creator.ID,
		FirstResponseDeadline: &pastDeadline,
	}
	db.Create(&ticket)

	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/departement/tiket/claim/%d", ticket.ID), nil)
	req = withStaffContext(req, &staff)
	w := httptest.NewRecorder()

	handler.ClaimTicket(w, req)

	var updated models.Ticket
	if err := db.First(&updated, ticket.ID).Error; err != nil {
		t.Fatalf("failed to reload ticket: %v", err)
	}

	if updated.FirstResponseAt == nil {
		t.Fatalf("expected FirstResponseAt to be stamped, got nil")
	}
	if updated.FirstResponseMet == nil || *updated.FirstResponseMet != false {
		t.Errorf("expected FirstResponseMet to be false (breached), got %v", updated.FirstResponseMet)
	}
}

// TestClaimTicket_IdempotentFirstResponse tests that subsequent claims do not overwrite first response timestamps.
func TestClaimTicket_IdempotentFirstResponse(t *testing.T) {
	db, cleanup := setupHandlerTestDB(t)
	if db == nil {
		return
	}
	defer cleanup()

	cfg := config.LoadConfig()
	handler := NewDepartmentHandler(cfg, nil, nil)

	dept := models.Department{Name: "Tech"}
	db.Create(&dept)
	staff := models.User{Username: "staff_idem", DepartmentID: &dept.ID, IsStaff: true}
	db.Create(&staff)
	creator := models.User{Username: "creator_idem"}
	db.Create(&creator)

	// Ticket already has FirstResponseAt stamped 1 hour ago
	originalResponseAt := time.Now().Add(-1 * time.Hour).Truncate(time.Second)
	metTrue := true
	deadline := time.Now().Add(1 * time.Hour)
	ticket := models.Ticket{
		Title:                 "Idempotent Test",
		Description:           "Desc",
		Status:                models.StatusInProgress,
		DepartmentID:          &dept.ID,
		CreatedByID:           creator.ID,
		FirstResponseDeadline: &deadline,
		FirstResponseAt:       &originalResponseAt,
		FirstResponseMet:      &metTrue,
	}
	db.Create(&ticket)

	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/departement/tiket/claim/%d", ticket.ID), nil)
	req = withStaffContext(req, &staff)
	w := httptest.NewRecorder()

	handler.ClaimTicket(w, req)

	var updated models.Ticket
	db.First(&updated, ticket.ID)
	if updated.FirstResponseAt == nil || !updated.FirstResponseAt.Truncate(time.Second).Equal(originalResponseAt) {
		t.Errorf("FirstResponseAt was overwritten! Expected %v, got %v", originalResponseAt, updated.FirstResponseAt)
	}
}

// TestClaimTicket_WithOptionalEstimation tests setting initial resolution estimate upon claim.
func TestClaimTicket_WithOptionalEstimation(t *testing.T) {
	db, cleanup := setupHandlerTestDB(t)
	if db == nil {
		return
	}
	defer cleanup()

	cfg := config.LoadConfig()
	handler := NewDepartmentHandler(cfg, nil, nil)

	dept := models.Department{Name: "Hardware"}
	db.Create(&dept)
	staff := models.User{Username: "staff_opt", DepartmentID: &dept.ID, IsStaff: true}
	db.Create(&staff)
	creator := models.User{Username: "user_opt"}
	db.Create(&creator)

	ticket := models.Ticket{
		Title:        "Claim with preset estimate",
		Description:  "Testing preset",
		Status:       models.StatusWaiting,
		DepartmentID: &dept.ID,
		CreatedByID:  creator.ID,
	}
	db.Create(&ticket)

	formData := url.Values{}
	formData.Set("preset", "4h")
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/departement/tiket/claim/%d", ticket.ID), strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = withStaffContext(req, &staff)
	w := httptest.NewRecorder()

	handler.ClaimTicket(w, req)

	var updated models.Ticket
	db.First(&updated, ticket.ID)
	if updated.EstimatedResolutionAt == nil {
		t.Fatalf("expected EstimatedResolutionAt to be populated from claim preset, got nil")
	}
	expectedAfter := time.Now().Add(3 * time.Hour)
	if !updated.EstimatedResolutionAt.After(expectedAfter) {
		t.Errorf("expected EstimatedResolutionAt > %v, got %v", expectedAfter, *updated.EstimatedResolutionAt)
	}
}

// TestClaimTicket_IDOR_CrossDepartment tests that cross-department staff cannot claim foreign tickets.
func TestClaimTicket_IDOR_CrossDepartment(t *testing.T) {
	db, cleanup := setupHandlerTestDB(t)
	if db == nil {
		return
	}
	defer cleanup()

	cfg := config.LoadConfig()
	handler := NewDepartmentHandler(cfg, nil, nil)

	deptA := models.Department{Name: "Dept Alpha"}
	db.Create(&deptA)
	deptB := models.Department{Name: "Dept Beta"}
	db.Create(&deptB)

	staffB := models.User{Username: "staff_beta", DepartmentID: &deptB.ID, IsStaff: true}
	db.Create(&staffB)
	creator := models.User{Username: "user_claim_idor"}
	db.Create(&creator)

	ticketA := models.Ticket{
		Title:        "Dept Alpha Ticket",
		Description:  "Protected",
		Status:       models.StatusWaiting,
		DepartmentID: &deptA.ID,
		CreatedByID:  creator.ID,
	}
	db.Create(&ticketA)

	// Staff B attempts to claim Ticket A
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/departement/tiket/claim/%d", ticketA.ID), nil)
	req = withStaffContext(req, &staffB)
	w := httptest.NewRecorder()

	handler.ClaimTicket(w, req)

	var check models.Ticket
	db.First(&check, ticketA.ID)
	if check.AssignedToID != nil {
		t.Errorf("IDOR violation: ticket from Dept Alpha was assigned to staff from Dept Beta!")
	}
}

// TestDepartmentReply_FirstResponseSafetyNet verifies replying stamps FirstResponseAt if not already stamped.
func TestDepartmentReply_FirstResponseSafetyNet(t *testing.T) {
	db, cleanup := setupHandlerTestDB(t)
	if db == nil {
		return
	}
	defer cleanup()

	cfg := config.LoadConfig()
	handler := NewDepartmentHandler(cfg, nil, nil)

	dept := models.Department{Name: "Network"}
	db.Create(&dept)
	staff := models.User{Username: "staff_net", DepartmentID: &dept.ID, IsStaff: true}
	db.Create(&staff)
	creator := models.User{Username: "user_net"}
	db.Create(&creator)

	deadline := time.Now().Add(1 * time.Hour)
	ticket := models.Ticket{
		Title:                 "Network Outage",
		Description:           "Router reset",
		Status:                models.StatusWaiting,
		DepartmentID:          &dept.ID,
		CreatedByID:           creator.ID,
		FirstResponseDeadline: &deadline,
		FirstResponseAt:       nil,
	}
	db.Create(&ticket)

	formData := url.Values{}
	formData.Set("message", "Sedang kami periksa perangkat router Anda.")
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/departement/tiket/%d", ticket.ID), strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = withStaffContext(req, &staff)
	w := httptest.NewRecorder()

	handler.DepartmentReply(w, req)

	var updated models.Ticket
	db.First(&updated, ticket.ID)
	if updated.FirstResponseAt == nil {
		t.Fatalf("expected FirstResponseAt to be stamped via reply safety net, got nil")
	}
	if updated.FirstResponseMet == nil || *updated.FirstResponseMet != true {
		t.Errorf("expected FirstResponseMet == true on timely reply, got %v", updated.FirstResponseMet)
	}

	// Verify reply record exists
	var replyCount int64
	db.Model(&models.TicketReply{}).Where("ticket_id = ?", ticket.ID).Count(&replyCount)
	if replyCount == 0 {
		t.Errorf("expected ticket reply to be saved")
	}
}

// TestReleaseTicket_PreservesSLA tests that releasing a ticket preserves stamped SLA fields.
func TestReleaseTicket_PreservesSLA(t *testing.T) {
	db, cleanup := setupHandlerTestDB(t)
	if db == nil {
		return
	}
	defer cleanup()

	cfg := config.LoadConfig()
	handler := NewDepartmentHandler(cfg, nil, nil)

	dept := models.Department{Name: "DevOps"}
	db.Create(&dept)
	staff := models.User{Username: "staff_devops", DepartmentID: &dept.ID, IsStaff: true}
	db.Create(&staff)
	creator := models.User{Username: "client_devops"}
	db.Create(&creator)

	respAt := time.Now().Add(-30 * time.Minute).Truncate(time.Second)
	metTrue := true
	ticket := models.Ticket{
		Title:            "Release preservation test",
		Description:      "Desc",
		Status:           models.StatusInProgress,
		DepartmentID:     &dept.ID,
		AssignedToID:     &staff.ID,
		CreatedByID:      creator.ID,
		FirstResponseAt:  &respAt,
		FirstResponseMet: &metTrue,
	}
	db.Create(&ticket)

	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/departement/tiket/release/%d", ticket.ID), nil)
	req = withStaffContext(req, &staff)
	w := httptest.NewRecorder()

	handler.ReleaseTicket(w, req)

	var released models.Ticket
	db.First(&released, ticket.ID)
	if released.AssignedToID != nil {
		t.Errorf("expected assigned_to_id to be nil after release, got %v", released.AssignedToID)
	}
	if released.Status != models.StatusWaiting {
		t.Errorf("expected status WAITING after release, got %s", released.Status)
	}
	if released.FirstResponseAt == nil || !released.FirstResponseAt.Truncate(time.Second).Equal(respAt) {
		t.Errorf("FirstResponseAt was corrupted during release: expected %v, got %v", respAt, released.FirstResponseAt)
	}
	if released.FirstResponseMet == nil || *released.FirstResponseMet != true {
		t.Errorf("FirstResponseMet was altered during release: got %v", released.FirstResponseMet)
	}
}

// TestSetTicketEstimate_Handler tests resolution estimation via POST endpoint.
func TestSetTicketEstimate_Handler(t *testing.T) {
	db, cleanup := setupHandlerTestDB(t)
	if db == nil {
		return
	}
	defer cleanup()

	cfg := config.LoadConfig()
	handler := NewDepartmentHandler(cfg, nil, nil)

	dept := models.Department{Name: "Security"}
	db.Create(&dept)
	staff := models.User{Username: "staff_sec", DepartmentID: &dept.ID, IsStaff: true}
	db.Create(&staff)
	creator := models.User{Username: "client_sec"}
	db.Create(&creator)

	ticket := models.Ticket{
		Title:        "Firewall config",
		Description:  "Desc",
		Status:       models.StatusInProgress,
		DepartmentID: &dept.ID,
		CreatedByID:  creator.ID,
	}
	db.Create(&ticket)

	// 1. Valid preset update
	formData := url.Values{}
	formData.Set("preset", "2h")
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/departement/tiket/estimate/%d", ticket.ID), strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = withStaffContext(req, &staff)
	w := httptest.NewRecorder()

	handler.SetTicketEstimate(w, req)

	var updated models.Ticket
	db.First(&updated, ticket.ID)
	if updated.EstimatedResolutionAt == nil {
		t.Fatalf("expected EstimatedResolutionAt to be set, got nil")
	}

	// 2. Reject closed ticket
	updated.Status = models.StatusClosed
	db.Save(&updated)

	reqClosed := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/departement/tiket/estimate/%d", ticket.ID), strings.NewReader(formData.Encode()))
	reqClosed.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqClosed = withStaffContext(reqClosed, &staff)
	wClosed := httptest.NewRecorder()

	handler.SetTicketEstimate(wClosed, reqClosed)
	if !strings.Contains(wClosed.Header().Get("Location"), "error") {
		t.Errorf("expected error redirect when setting estimate on closed ticket, got %v", wClosed.Header().Get("Location"))
	}
}

// TestSetTicketPriority_Handler tests staff priority adjustment, audit history, timeline reply, and notification.
func TestSetTicketPriority_Handler(t *testing.T) {
	db, cleanup := setupHandlerTestDB(t)
	if db == nil {
		return
	}
	defer cleanup()

	cfg := config.LoadConfig()
	handler := NewDepartmentHandler(cfg, nil, nil)

	company := models.Company{Name: "Priority Test PT", Code: "PRIOPT", IsActive: true}
	db.Create(&company)
	dept := models.Department{Name: "Operations", CompanyID: &company.ID}
	db.Create(&dept)

	staff := models.User{Username: "staff_ops", DepartmentID: &dept.ID, IsStaff: true}
	db.Create(&staff)
	creator := models.User{Username: "user_ops"}
	db.Create(&creator)

	createdAt := time.Now().Add(-10 * time.Minute)
	ticket := models.Ticket{
		Title:        "Priority Adjust Test",
		Description:  "Original priority is LOW",
		Status:       models.StatusWaiting,
		Priority:     models.PriorityLow,
		CompanyID:    &company.ID,
		DepartmentID: &dept.ID,
		CreatedByID:  creator.ID,
		CreatedAt:    createdAt,
	}
	db.Create(&ticket)

	// Escalate from LOW to HIGH
	formData := url.Values{}
	formData.Set("priority", "HIGH")
	formData.Set("reason", "Dampak meluas ke seluruh departemen produksi")
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/departement/tiket/priority/%d", ticket.ID), strings.NewReader(formData.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = withStaffContext(req, &staff)
	w := httptest.NewRecorder()

	handler.SetTicketPriority(w, req)

	// 1. Verify Ticket Priority Updated
	var updated models.Ticket
	db.First(&updated, ticket.ID)
	if updated.Priority != models.PriorityHigh {
		t.Errorf("expected Priority HIGH, got %s", updated.Priority)
	}

	// 2. Verify TicketPriorityHistory Record Created
	var history models.TicketPriorityHistory
	if err := db.Where("ticket_id = ?", ticket.ID).First(&history).Error; err != nil {
		t.Fatalf("failed to find TicketPriorityHistory: %v", err)
	}
	if history.OldPriority != models.PriorityLow || history.NewPriority != models.PriorityHigh {
		t.Errorf("expected history %s -> %s, got %s -> %s", models.PriorityLow, models.PriorityHigh, history.OldPriority, history.NewPriority)
	}
	if history.ChangedByID != staff.ID {
		t.Errorf("expected history ChangedByID %d, got %d", staff.ID, history.ChangedByID)
	}
	if history.Reason != "Dampak meluas ke seluruh departemen produksi" {
		t.Errorf("expected history reason preserved, got %q", history.Reason)
	}

	// 3. Verify Timeline System Reply Created
	var replies []models.TicketReply
	db.Where("ticket_id = ?", ticket.ID).Find(&replies)
	foundSystemReply := false
	for _, r := range replies {
		if strings.Contains(r.Message, "[Sistem] Prioritas tiket diubah dari LOW ke HIGH") {
			foundSystemReply = true
			break
		}
	}
	if !foundSystemReply {
		t.Errorf("expected timeline system reply recording priority adjustment")
	}

	// 4. Verify Deadlines Recalculated (firstResponseAt is nil)
	if updated.FirstResponseDeadline == nil {
		t.Errorf("expected FirstResponseDeadline to be recalculated and populated")
	}

	// 5. Test Identical Priority Rejection
	formDataSame := url.Values{}
	formDataSame.Set("priority", "HIGH")
	formDataSame.Set("reason", "Alasan sama")
	reqSame := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/departement/tiket/priority/%d", ticket.ID), strings.NewReader(formDataSame.Encode()))
	reqSame.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqSame = withStaffContext(reqSame, &staff)
	wSame := httptest.NewRecorder()

	handler.SetTicketPriority(wSame, reqSame)
	if !strings.Contains(wSame.Header().Get("Location"), "error") {
		t.Errorf("expected error redirect when setting identical priority")
	}

	// 6. Test Short Reason (<5 chars) Rejection
	formDataShort := url.Values{}
	formDataShort.Set("priority", "LOW")
	formDataShort.Set("reason", "pend") // 4 chars
	reqShort := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/departement/tiket/priority/%d", ticket.ID), strings.NewReader(formDataShort.Encode()))
	reqShort.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqShort = withStaffContext(reqShort, &staff)
	wShort := httptest.NewRecorder()

	handler.SetTicketPriority(wShort, reqShort)
	if !strings.Contains(wShort.Header().Get("Location"), "error") {
		t.Errorf("expected error redirect when reason is < 5 chars")
	}
}
