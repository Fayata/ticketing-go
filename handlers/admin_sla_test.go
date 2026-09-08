package handlers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"ticketing/config"
	"ticketing/models"
	"ticketing/utils"
)

func TestParseIDFromPath_SLA(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{"SLA edit microservice", "/sla-policies/edit/42", 42},
		{"SLA edit monolith", "/admin/sla-policies/edit/42", 42},
		{"SLA toggle microservice", "/sla-policies/toggle/99", 99},
		{"SLA toggle with query", "/admin/sla-policies/toggle/99?source=list", 99},
		{"Invalid SLA ID", "/admin/sla-policies/edit/invalid", 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseIDFromPath(tc.input)
			if got != tc.expected {
				t.Errorf("parseIDFromPath(%q) = %d; want %d", tc.input, got, tc.expected)
			}
		})
	}
}

func TestListSLAPolicies_MethodNotAllowed(t *testing.T) {
	cfg := config.LoadConfig()
	handler := NewAdminHandler(cfg, nil, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/admin/sla-policies", nil)
	req = withSuperAdminContext(req)
	w := httptest.NewRecorder()

	handler.ListSLAPolicies(w, req)
	resp := w.Result()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405 Method Not Allowed, got %d", resp.StatusCode)
	}
}

func TestCreateSLAPolicy_ValidationErrors(t *testing.T) {
	db, cleanup := setupHandlerTestDB(t)
	if db == nil {
		return
	}
	defer cleanup()

	_ = db.AutoMigrate(&models.SLAPolicy{})

	root := findRepoRoot()
	_ = os.Chdir(root)
	utils.InitTemplates()

	cfg := config.LoadConfig()
	handler := NewAdminHandler(cfg, nil, nil, nil)

	// Subtest 1: Empty Name
	{
		form := url.Values{
			"name":              {""},
			"resp_high_hours":   {"1"},
			"resp_high_minutes": {"0"},
			"resp_med_hours":    {"4"},
			"resp_med_minutes":  {"0"},
			"resp_low_hours":    {"8"},
			"resp_low_minutes":  {"0"},
			"resol_high_days":   {"0"},
			"resol_high_hours":  {"4"},
			"resol_med_days":    {"1"},
			"resol_med_hours":   {"0"},
			"resol_low_days":    {"3"},
			"resol_low_hours":   {"0"},
		}
		req := httptest.NewRequest(http.MethodPost, "/admin/sla-policies/create", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req = withSuperAdminContext(req)
		w := httptest.NewRecorder()

		handler.CreateSLAPolicyForm(w, req)
		resp := w.Result()
		loc := resp.Header.Get("Location")
		if !strings.Contains(loc, "error=") {
			t.Errorf("Expected redirect error for empty name, got %s", loc)
		}
	}

	// Subtest 2: Zero response duration
	{
		form := url.Values{
			"name":              {"SLA Zero Duration"},
			"resp_high_hours":   {"0"},
			"resp_high_minutes": {"0"}, // total 0
			"resp_med_hours":    {"4"},
			"resp_med_minutes":  {"0"},
			"resp_low_hours":    {"8"},
			"resp_low_minutes":  {"0"},
			"resol_high_days":   {"0"},
			"resol_high_hours":  {"4"},
			"resol_med_days":    {"1"},
			"resol_med_hours":   {"0"},
			"resol_low_days":    {"3"},
			"resol_low_hours":   {"0"},
		}
		req := httptest.NewRequest(http.MethodPost, "/admin/sla-policies/create", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req = withSuperAdminContext(req)
		w := httptest.NewRecorder()

		handler.CreateSLAPolicyForm(w, req)
		resp := w.Result()
		loc := resp.Header.Get("Location")
		if !strings.Contains(loc, "error=") {
			t.Errorf("Expected redirect error for zero response duration, got %s", loc)
		}
	}
}

func TestCreateSLAPolicy_DualUnitMath_Success(t *testing.T) {
	db, cleanup := setupHandlerTestDB(t)
	if db == nil {
		return
	}
	defer cleanup()

	_ = db.AutoMigrate(&models.SLAPolicy{})

	root := findRepoRoot()
	_ = os.Chdir(root)
	utils.InitTemplates()

	cfg := config.LoadConfig()
	handler := NewAdminHandler(cfg, nil, nil, nil)

	// High: 1 Jam 30 Menit (90 min), 1 Hari 12 Jam (36 hours)
	form := url.Values{
		"name":              {"SLA Dual Unit Test"},
		"description":       {"Testing dual unit conversion"},
		"resp_high_hours":   {"1"},
		"resp_high_minutes": {"30"},
		"resp_med_hours":    {"3"},
		"resp_med_minutes":  {"15"},
		"resp_low_hours":    {"6"},
		"resp_low_minutes":  {"45"},
		"resol_high_days":   {"1"},
		"resol_high_hours":  {"12"},
		"resol_med_days":    {"2"},
		"resol_med_hours":   {"6"},
		"resol_low_days":    {"4"},
		"resol_low_hours":   {"0"},
	}
	req := httptest.NewRequest(http.MethodPost, "/admin/sla-policies/create", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = withSuperAdminContext(req)
	w := httptest.NewRecorder()

	handler.CreateSLAPolicyForm(w, req)
	resp := w.Result()
	loc := resp.Header.Get("Location")
	if !strings.Contains(loc, "success=") {
		t.Fatalf("Expected success redirect, got %s", loc)
	}

	// Verify database record
	var policy models.SLAPolicy
	if err := db.Where("name = ?", "SLA Dual Unit Test").First(&policy).Error; err != nil {
		t.Fatalf("Failed to find created policy: %v", err)
	}

	if policy.ResponseTimeHighMinutes != 90 {
		t.Errorf("Expected ResponseTimeHighMinutes = 90 (1h30m), got %d", policy.ResponseTimeHighMinutes)
	}
	if policy.ResponseTimeMedMinutes != 195 {
		t.Errorf("Expected ResponseTimeMedMinutes = 195 (3h15m), got %d", policy.ResponseTimeMedMinutes)
	}
	if policy.ResolutionTimeHighHours != 36 {
		t.Errorf("Expected ResolutionTimeHighHours = 36 (1d12h), got %d", policy.ResolutionTimeHighHours)
	}
	if policy.ResolutionTimeMedHours != 54 {
		t.Errorf("Expected ResolutionTimeMedHours = 54 (2d6h), got %d", policy.ResolutionTimeMedHours)
	}
}

func TestToggleSLAPolicyStatus(t *testing.T) {
	db, cleanup := setupHandlerTestDB(t)
	if db == nil {
		return
	}
	defer cleanup()

	_ = db.AutoMigrate(&models.SLAPolicy{})

	policy := models.SLAPolicy{
		Name:                    "Toggle Test Policy",
		IsActive:                true,
		ResponseTimeHighMinutes: 60,
		ResponseTimeMedMinutes:  240,
		ResponseTimeLowMinutes:  480,
		ResolutionTimeHighHours: 4,
		ResolutionTimeMedHours:  24,
		ResolutionTimeLowHours:  72,
	}
	db.Create(&policy)

	cfg := config.LoadConfig()
	handler := NewAdminHandler(cfg, nil, nil, nil)

	// Toggle to false
	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/admin/sla-policies/toggle/%d", policy.ID), nil)
	req = withSuperAdminContext(req)
	w := httptest.NewRecorder()

	handler.ToggleSLAPolicyStatus(w, req)
	resp := w.Result()
	loc := resp.Header.Get("Location")
	if !strings.Contains(loc, "success=") {
		t.Errorf("Expected success redirect, got %s", loc)
	}

	var updated models.SLAPolicy
	db.First(&updated, policy.ID)
	if updated.IsActive {
		t.Errorf("Expected policy IsActive to be false after toggle, got true")
	}
}
