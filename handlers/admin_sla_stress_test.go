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

// ============================================================================
// Stress Test Suite 1: Input Parsing & Conversion Oracles
// ============================================================================

// TestStress_AdminInputParsingOracles tests parseIntValue and parseUintPtr
// against malicious, malformed, negative, float, and boundary inputs.
func TestStress_AdminInputParsingOracles(t *testing.T) {
	intCases := []struct {
		input    string
		expected int
	}{
		{"", 0},
		{"   ", 0},
		{"\t\n", 0},
		{"0", 0},
		{"1", 1},
		{"59", 59},
		{"168", 168},
		{"365", 365},
		{"999999", 999999},
		{"-1", 0},
		{"-100", 0},
		{"-999999", 0},
		{"abc", 0},
		{"12px", 0},
		{"3.14", 0},
		{"1e6", 0},
		{"NaN", 0},
		{"null", 0},
		{"undefined", 0},
		{" 42 ", 42},
		{" 007 ", 7},
	}

	for _, tc := range intCases {
		got := parseIntValue(tc.input)
		if got != tc.expected {
			t.Errorf("parseIntValue(%q) = %d; want %d", tc.input, got, tc.expected)
		}
	}

	uintCases := []struct {
		input    string
		expected *uint
	}{
		{"", nil},
		{"   ", nil},
		{"0", nil}, // 0 should map to nil pointer for tenant foreign keys
		{"-1", nil},
		{"-999", nil},
		{"invalid", nil},
	}

	for _, tc := range uintCases {
		got := parseUintPtr(tc.input)
		if tc.expected == nil && got != nil {
			t.Errorf("parseUintPtr(%q) expected nil, got %v", tc.input, *got)
		}
	}

	// Valid positive uint
	val42 := parseUintPtr("42")
	if val42 == nil || *val42 != 42 {
		t.Errorf("parseUintPtr(\"42\") expected 42, got %v", val42)
	}
}

// ============================================================================
// Stress Test Suite 2: Form Validation Rejection Matrix (Offline / Unit)
// ============================================================================

// TestStress_CreateSLAPolicy_ValidationMatrix verifies that invalid, zero,
// negative, or empty form values are systematically rejected by the handler.
func TestStress_CreateSLAPolicy_ValidationMatrix(t *testing.T) {
	cfg := config.LoadConfig()
	handler := NewAdminHandler(cfg, nil, nil, nil)

	baseForm := func() url.Values {
		return url.Values{
			"name":              {"Valid Policy Name"},
			"description":       {"Valid Description"},
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
	}

	tests := []struct {
		name          string
		modifyForm    func(v url.Values)
		expectedError string
	}{
		{
			name: "Empty Name",
			modifyForm: func(v url.Values) {
				v.Set("name", "")
			},
			expectedError: "Nama",
		},
		{
			name: "Whitespace-Only Name",
			modifyForm: func(v url.Values) {
				v.Set("name", "    ")
			},
			expectedError: "Nama",
		},
		{
			name: "Zero High Response Duration (0h 0m)",
			modifyForm: func(v url.Values) {
				v.Set("resp_high_hours", "0")
				v.Set("resp_high_minutes", "0")
			},
			expectedError: "minimal 1 menit",
		},
		{
			name: "Zero Med Response Duration (0h 0m)",
			modifyForm: func(v url.Values) {
				v.Set("resp_med_hours", "0")
				v.Set("resp_med_minutes", "0")
			},
			expectedError: "minimal 1 menit",
		},
		{
			name: "Zero Low Response Duration (0h 0m)",
			modifyForm: func(v url.Values) {
				v.Set("resp_low_hours", "0")
				v.Set("resp_low_minutes", "0")
			},
			expectedError: "minimal 1 menit",
		},
		{
			name: "Negative Response Value (-1h 0m -> parsed as 0)",
			modifyForm: func(v url.Values) {
				v.Set("resp_high_hours", "-1")
				v.Set("resp_high_minutes", "0")
			},
			expectedError: "minimal 1 menit",
		},
		{
			name: "Zero High Resolution Duration (0d 0h)",
			modifyForm: func(v url.Values) {
				v.Set("resol_high_days", "0")
				v.Set("resol_high_hours", "0")
			},
			expectedError: "minimal 1 jam",
		},
		{
			name: "Zero Med Resolution Duration (0d 0h)",
			modifyForm: func(v url.Values) {
				v.Set("resol_med_days", "0")
				v.Set("resol_med_hours", "0")
			},
			expectedError: "minimal 1 jam",
		},
		{
			name: "Zero Low Resolution Duration (0d 0h)",
			modifyForm: func(v url.Values) {
				v.Set("resol_low_days", "0")
				v.Set("resol_low_hours", "0")
			},
			expectedError: "minimal 1 jam",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			form := baseForm()
			tc.modifyForm(form)

			req := httptest.NewRequest(http.MethodPost, "/admin/sla-policies/create", strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req = withSuperAdminContext(req)
			w := httptest.NewRecorder()

			handler.CreateSLAPolicyForm(w, req)
			resp := w.Result()
			loc := resp.Header.Get("Location")

			unescapedLoc, _ := url.QueryUnescape(loc)
			if !strings.Contains(unescapedLoc, tc.expectedError) {
				t.Errorf("Expected redirect error containing %q, got %q", tc.expectedError, unescapedLoc)
			}
		})
	}
}

// ============================================================================
// Stress Test Suite 3: Scope Validation & Cross-Tenant Isolation (Database)
// ============================================================================

// TestStress_ScopeValidation_And_Matching stress-tests company/department scope
// relations, cross-tenant mismatches, and system default isolation.
func TestStress_ScopeValidation_And_Matching(t *testing.T) {
	db, cleanup := setupHandlerTestDB(t)
	if db == nil {
		return
	}
	defer cleanup()

	_ = db.AutoMigrate(&models.Company{}, &models.Department{}, &models.SLAPolicy{})

	root := findRepoRoot()
	_ = os.Chdir(root)
	utils.InitTemplates()

	cfg := config.LoadConfig()
	handler := NewAdminHandler(cfg, nil, nil, nil)

	// Setup 2 Companies and Departments
	compA := models.Company{Name: "PT Alpha", Code: "ALPHA", IsActive: true}
	db.Create(&compA)
	deptA := models.Department{Name: "Dept Alpha", CompanyID: &compA.ID}
	db.Create(&deptA)

	compB := models.Company{Name: "PT Beta", Code: "BETA", IsActive: true}
	db.Create(&compB)
	deptB := models.Department{Name: "Dept Beta", CompanyID: &compB.ID}
	db.Create(&deptB)

	// Subtest 1: Mismatched company & department (Company A + Department B)
	{
		form := url.Values{
			"name":              {"Mismatched Scope Policy"},
			"company_id":        {fmt.Sprintf("%d", compA.ID)},
			"department_id":     {fmt.Sprintf("%d", deptB.ID)},
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
		unescaped, _ := url.QueryUnescape(resp.Header.Get("Location"))
		if !strings.Contains(unescaped, "tidak terdaftar pada perusahaan") {
			t.Errorf("Expected mismatch error, got %s", unescaped)
		}
	}

	// Subtest 2: Non-existent Department ID
	{
		form := url.Values{
			"name":              {"NonExistent Dept Policy"},
			"department_id":     {"999999"},
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
		unescaped, _ := url.QueryUnescape(resp.Header.Get("Location"))
		if !strings.Contains(unescaped, "Departemen tidak ditemukan") {
			t.Errorf("Expected not found error for invalid dept, got %s", unescaped)
		}
	}

	// Subtest 3: System default isolation (is_default overrides company & dept to nil)
	{
		form := url.Values{
			"name":              {"Global Default Isolation Test"},
			"is_default":        {"true"},
			"company_id":        {fmt.Sprintf("%d", compA.ID)},
			"department_id":     {fmt.Sprintf("%d", deptA.ID)},
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
		if !strings.Contains(resp.Header.Get("Location"), "success=") {
			t.Fatalf("Expected success for system default creation: %s", resp.Header.Get("Location"))
		}

		var created models.SLAPolicy
		db.Where("name = ?", "Global Default Isolation Test").First(&created)
		if !created.IsDefault {
			t.Errorf("Expected IsDefault to be true")
		}
		if created.CompanyID != nil {
			t.Errorf("Expected CompanyID to be nil for system default, got %v", *created.CompanyID)
		}
		if created.DepartmentID != nil {
			t.Errorf("Expected DepartmentID to be nil for system default, got %v", *created.DepartmentID)
		}
	}
}

// ============================================================================
// Stress Test Suite 4: Duplicate Active Policy Prevention (Database)
// ============================================================================

// TestStress_DuplicateActivePolicy_Prevention stress-tests the conflict detection
// preventing multiple active policies with identical scope.
func TestStress_DuplicateActivePolicy_Prevention(t *testing.T) {
	db, cleanup := setupHandlerTestDB(t)
	if db == nil {
		return
	}
	defer cleanup()

	_ = db.AutoMigrate(&models.Company{}, &models.Department{}, &models.SLAPolicy{})

	root := findRepoRoot()
	_ = os.Chdir(root)
	utils.InitTemplates()

	cfg := config.LoadConfig()
	handler := NewAdminHandler(cfg, nil, nil, nil)

	comp := models.Company{Name: "PT Duplication Test", Code: "DUP1", IsActive: true}
	db.Create(&comp)
	dept1 := models.Department{Name: "Dept One", CompanyID: &comp.ID}
	db.Create(&dept1)
	dept2 := models.Department{Name: "Dept Two", CompanyID: &comp.ID}
	db.Create(&dept2)

	createPolicy := func(name string, compID, deptID *uint, isDefault, isActive bool) *http.Response {
		form := url.Values{
			"name":              {name},
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
		if compID != nil {
			form.Set("company_id", fmt.Sprintf("%d", *compID))
		}
		if deptID != nil {
			form.Set("department_id", fmt.Sprintf("%d", *deptID))
		}
		if isDefault {
			form.Set("is_default", "true")
		}
		if !isActive {
			form.Set("is_active", "false")
		}

		req := httptest.NewRequest(http.MethodPost, "/admin/sla-policies/create", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req = withSuperAdminContext(req)
		w := httptest.NewRecorder()

		handler.CreateSLAPolicyForm(w, req)
		return w.Result()
	}

	// 1. Create first active company policy -> Success
	resp1 := createPolicy("PT Policy Active", &comp.ID, nil, false, true)
	if !strings.Contains(resp1.Header.Get("Location"), "success=") {
		t.Fatalf("Failed to create first company policy: %s", resp1.Header.Get("Location"))
	}

	// 2. Attempt to create second active policy for SAME company -> Must be blocked!
	resp2 := createPolicy("PT Policy Duplicate", &comp.ID, nil, false, true)
	unescaped2, _ := url.QueryUnescape(resp2.Header.Get("Location"))
	if !strings.Contains(unescaped2, "sudah ada") {
		t.Fatalf("Expected duplicate company policy to be rejected, got %s", unescaped2)
	}

	// 3. Create active department policy under same company -> Must be ALLOWED (hierarchy coexistence)
	resp3 := createPolicy("Dept 1 Policy Active", &comp.ID, &dept1.ID, false, true)
	if !strings.Contains(resp3.Header.Get("Location"), "success=") {
		t.Fatalf("Department override under existing company policy should be allowed: %s", resp3.Header.Get("Location"))
	}

	// 4. Attempt to create second active policy for SAME department -> Must be blocked!
	resp4 := createPolicy("Dept 1 Policy Duplicate", &comp.ID, &dept1.ID, false, true)
	unescaped4, _ := url.QueryUnescape(resp4.Header.Get("Location"))
	if !strings.Contains(unescaped4, "sudah ada") {
		t.Fatalf("Expected duplicate department policy to be rejected, got %s", unescaped4)
	}

	// 5. Create active policy for DIFFERENT department -> Must be ALLOWED
	resp5 := createPolicy("Dept 2 Policy Active", &comp.ID, &dept2.ID, false, true)
	if !strings.Contains(resp5.Header.Get("Location"), "success=") {
		t.Fatalf("Policy for different department should be allowed: %s", resp5.Header.Get("Location"))
	}

	// 6. Create INACTIVE policy for same department -> Must be ALLOWED
	resp6 := createPolicy("Dept 1 Inactive Draft", &comp.ID, &dept1.ID, false, false)
	if !strings.Contains(resp6.Header.Get("Location"), "success=") {
		t.Fatalf("Inactive policy for same scope should be allowed: %s", resp6.Header.Get("Location"))
	}
}
