package handlers

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ticketing/config"
	"ticketing/middleware"
	"ticketing/models"
	"ticketing/utils"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// TestParseIDFromPath verifies safe path parameter extraction across all URL variations.
func TestParseIDFromPath(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{"Microservice company edit", "/companies/edit/123", 123},
		{"Monolith company edit", "/admin/companies/edit/123", 123},
		{"Trailing slash", "/admin/companies/edit/123/", 123},
		{"Microservice toggle", "/companies/toggle/456", 456},
		{"Monolith toggle", "/admin/companies/toggle/456", 456},
		{"Non-numeric ID", "/admin/companies/edit/abc", 0},
		{"Negative ID", "/admin/companies/edit/-5", 0},
		{"Zero ID", "/admin/companies/edit/0", 0},
		{"Root path", "/", 0},
		{"Empty path", "", 0},
		{"Path with query parameters", "/admin/companies/edit/789?ref=sidebar", 789},
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

// findRepoRoot locates the directory containing go.mod.
func findRepoRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}
	for i := 0; i < 6; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "."
}

// setupHandlerTestDB establishes an isolated PostgreSQL test schema or skips if unavailable.
func setupHandlerTestDB(t *testing.T) (*gorm.DB, func()) {
	t.Helper()
	cfg := config.LoadConfig()

	candidates := []struct {
		host, user, password, dbname string
		port                         int
		sslmode                      string
	}{
		{cfg.DBHost, cfg.DBUser, cfg.DBPassword, cfg.DBName, cfg.DBPort, cfg.DBSSLMode},
		{"localhost", "postgres", "postgres", "ticketing_db", 5432, "disable"},
		{"localhost", "postgres", "postgres", "postgres", 5432, "disable"},
		{"localhost", "your_db_user", "your_strong_password_here", "ticketing_db", 5432, "disable"},
		{"localhost", "postgres", "", "postgres", 5432, "disable"},
	}

	var db *gorm.DB
	var err error
	var chosenDSN string

	for _, cand := range candidates {
		dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%d sslmode=%s TimeZone=Asia/Jakarta",
			cand.host, cand.user, cand.password, cand.dbname, cand.port, cand.sslmode,
		)
		db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{
			Logger: logger.Default.LogMode(logger.Silent),
		})
		if err == nil {
			sqlDB, sqlErr := db.DB()
			if sqlErr == nil && sqlDB.Ping() == nil {
				chosenDSN = dsn
				break
			}
		}
	}

	if chosenDSN == "" || db == nil {
		t.Skipf("Skipping handler DB tests: PostgreSQL connection could not be established: %v", err)
		return nil, nil
	}

	schemaName := fmt.Sprintf("test_m2_handlers_%d", time.Now().UnixNano())
	if err := db.Exec(fmt.Sprintf("CREATE SCHEMA %s", schemaName)).Error; err != nil {
		t.Fatalf("Failed to create isolated schema %s: %v", schemaName, err)
	}

	testDB, err := gorm.Open(postgres.Open(chosenDSN), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("Failed to open test session: %v", err)
	}
	testDB.Exec(fmt.Sprintf("SET search_path TO %s, public", schemaName))

	// Set global config.DB
	oldDB := config.DB
	config.DB = testDB

	_ = testDB.AutoMigrate(
		&models.Company{},
		&models.Department{},
		&models.User{},
		&models.Group{},
		&models.Ticket{},
	)
	_, _ = models.SeedDefaultCompanyAndMigrate(testDB)

	cleanup := func() {
		db.Exec(fmt.Sprintf("DROP SCHEMA %s CASCADE", schemaName))
		if sqlDB, _ := db.DB(); sqlDB != nil {
			_ = sqlDB.Close()
		}
		if testSQLDB, _ := testDB.DB(); testSQLDB != nil {
			_ = testSQLDB.Close()
		}
		config.DB = oldDB
	}

	return testDB, cleanup
}

// withSuperAdminContext attaches an authenticated Super Admin user to the request context.
func withSuperAdminContext(r *http.Request) *http.Request {
	adminUser := &models.User{
		ID:           1,
		Username:     "admin",
		IsStaff:      true,
		IsSuperAdmin: true,
	}
	ctx := context.WithValue(r.Context(), middleware.UserKey, adminUser)
	return r.WithContext(ctx)
}

// TestCreateCompanyValidation tests name required, code required, max length, and case-insensitive uniqueness.
func TestCreateCompanyValidation(t *testing.T) {
	db, cleanup := setupHandlerTestDB(t)
	if db == nil {
		return
	}
	defer cleanup()

	root := findRepoRoot()
	_ = os.Chdir(root)
	utils.InitTemplates()

	cfg := config.LoadConfig()
	handler := NewAdminHandler(cfg, nil, nil, nil)

	// Subtest 1: Empty name rejected
	{
		form := url.Values{
			"name": {""},
			"code": {"VALIDCODE"},
		}
		req := httptest.NewRequest(http.MethodPost, "/admin/companies/create", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req = withSuperAdminContext(req)
		w := httptest.NewRecorder()

		handler.CreateCompanyForm(w, req)
		resp := w.Result()
		if resp.StatusCode != http.StatusSeeOther && resp.StatusCode != http.StatusFound {
			t.Errorf("Expected redirect for empty name, got %d", resp.StatusCode)
		}
		loc := resp.Header.Get("Location")
		if !strings.Contains(loc, "error=") {
			t.Errorf("Expected error query param in redirect, got %s", loc)
		}
	}

	// Subtest 2: Code exceeding 20 characters rejected
	{
		form := url.Values{
			"name": {"PT Panjang"},
			"code": {"123456789012345678901"}, // 21 chars
		}
		req := httptest.NewRequest(http.MethodPost, "/admin/companies/create", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req = withSuperAdminContext(req)
		w := httptest.NewRecorder()

		handler.CreateCompanyForm(w, req)
		resp := w.Result()
		loc := resp.Header.Get("Location")
		if !strings.Contains(loc, "error=") {
			t.Errorf("Expected error query param for code > 20 chars, got %s", loc)
		}
	}

	// Subtest 3: Case-insensitive duplicate code rejected
	{
		db.Create(&models.Company{Name: "PT Solusi Awal", Code: "SOLUSI", IsActive: true})

		form := url.Values{
			"name": {"PT Solusi Duplikat"},
			"code": {"solusi"},
		}
		req := httptest.NewRequest(http.MethodPost, "/admin/companies/create", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req = withSuperAdminContext(req)
		w := httptest.NewRecorder()

		handler.CreateCompanyForm(w, req)
		resp := w.Result()
		loc := resp.Header.Get("Location")
		if !strings.Contains(loc, "error=") {
			t.Errorf("Expected error for duplicate lowercase code, got %s", loc)
		}

		var count int64
		db.Model(&models.Company{}).Where("LOWER(code) = LOWER(?)", "solusi").Count(&count)
		if count != 1 {
			t.Errorf("Expected exactly 1 company with code SOLUSI, got %d", count)
		}
	}

	// Subtest 4: Whitespace trimming and uppercase conversion
	{
		form := url.Values{
			"name":    {"  PT Spasi Indah  "},
			"code":    {"  spasi20  "},
			"address": {"  Jl. Merdeka  "},
			"phone":   {"  021-12345  "},
		}
		req := httptest.NewRequest(http.MethodPost, "/admin/companies/create", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req = withSuperAdminContext(req)
		w := httptest.NewRecorder()

		handler.CreateCompanyForm(w, req)
		resp := w.Result()
		if resp.StatusCode != http.StatusSeeOther && resp.StatusCode != http.StatusFound {
			t.Errorf("Expected redirect after successful create, got %d", resp.StatusCode)
		}

		var created models.Company
		if err := db.Where("code = ?", "SPASI20").First(&created).Error; err != nil {
			t.Fatalf("Expected company with trimmed/uppercase code SPASI20 to be saved: %v", err)
		}
		if created.Name != "PT Spasi Indah" {
			t.Errorf("Expected trimmed name 'PT Spasi Indah', got '%s'", created.Name)
		}
	}
}

// TestEditCompanySelfExclusion verifies edit succeeds when retaining existing code.
func TestEditCompanySelfExclusion(t *testing.T) {
	db, cleanup := setupHandlerTestDB(t)
	if db == nil {
		return
	}
	defer cleanup()

	root := findRepoRoot()
	_ = os.Chdir(root)
	utils.InitTemplates()

	cfg := config.LoadConfig()
	handler := NewAdminHandler(cfg, nil, nil, nil)

	comp := models.Company{Name: "PT Asli", Code: "KODEASLI", IsActive: true}
	db.Create(&comp)

	// Edit retaining same code (in lowercase)
	form := url.Values{
		"name": {"PT Asli Baru"},
		"code": {"kodeasli"},
	}
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/admin/companies/edit/%d", comp.ID), strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = withSuperAdminContext(req)
	w := httptest.NewRecorder()

	handler.EditCompanyForm(w, req)
	resp := w.Result()
	loc := resp.Header.Get("Location")
	if strings.Contains(loc, "error=") {
		t.Fatalf("Self-exclusion failed: edit with self code produced error redirect: %s", loc)
	}

	var updated models.Company
	db.First(&updated, comp.ID)
	if updated.Name != "PT Asli Baru" {
		t.Errorf("Expected updated name 'PT Asli Baru', got '%s'", updated.Name)
	}

	// Subtest: Trying to use another company's code should be rejected
	otherComp := models.Company{Name: "PT Lain", Code: "KODELAIN", IsActive: true}
	db.Create(&otherComp)

	formOther := url.Values{
		"name": {"PT Asli Ubah Ke Lain"},
		"code": {"kodelain"},
	}
	reqOther := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/admin/companies/edit/%d", comp.ID), strings.NewReader(formOther.Encode()))
	reqOther.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reqOther = withSuperAdminContext(reqOther)
	wOther := httptest.NewRecorder()

	handler.EditCompanyForm(wOther, reqOther)
	respOther := wOther.Result()
	locOther := respOther.Header.Get("Location")
	if !strings.Contains(locOther, "error=") {
		t.Errorf("Expected duplicate code rejection on edit, got: %s", locOther)
	}
}

// TestToggleCompanyStatusMethods verifies both GET and POST work for status toggling.
func TestToggleCompanyStatusMethods(t *testing.T) {
	db, cleanup := setupHandlerTestDB(t)
	if db == nil {
		return
	}
	defer cleanup()

	cfg := config.LoadConfig()
	handler := NewAdminHandler(cfg, nil, nil, nil)

	comp := models.Company{Name: "PT Toggle", Code: "TOGGLEPT", IsActive: true}
	db.Create(&comp)

	// Test GET (table link)
	{
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/admin/companies/toggle/%d", comp.ID), nil)
		req = withSuperAdminContext(req)
		w := httptest.NewRecorder()

		handler.ToggleCompanyStatus(w, req)
		resp := w.Result()
		if resp.StatusCode != http.StatusSeeOther && resp.StatusCode != http.StatusFound {
			t.Errorf("Expected redirect on GET toggle, got %d", resp.StatusCode)
		}

		var toggled models.Company
		db.First(&toggled, comp.ID)
		if toggled.IsActive != false {
			t.Errorf("Expected IsActive to be false after GET toggle")
		}
	}

	// Test POST (API/test caller)
	{
		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/admin/companies/toggle/%d", comp.ID), strings.NewReader(""))
		req = withSuperAdminContext(req)
		w := httptest.NewRecorder()

		handler.ToggleCompanyStatus(w, req)
		resp := w.Result()
		if resp.StatusCode != http.StatusSeeOther && resp.StatusCode != http.StatusFound {
			t.Errorf("Expected redirect on POST toggle, got %d", resp.StatusCode)
		}

		var toggled models.Company
		db.First(&toggled, comp.ID)
		if toggled.IsActive != true {
			t.Errorf("Expected IsActive to be true after POST toggle")
		}
	}
}

// TestCreateDepartmentWithCompanyValidation tests company_id requirements.
func TestCreateDepartmentWithCompanyValidation(t *testing.T) {
	db, cleanup := setupHandlerTestDB(t)
	if db == nil {
		return
	}
	defer cleanup()

	root := findRepoRoot()
	_ = os.Chdir(root)
	utils.InitTemplates()

	cfg := config.LoadConfig()
	handler := NewAdminHandler(cfg, nil, nil, nil)

	activeComp := models.Company{Name: "PT Aktif", Code: "AKTIF", IsActive: true}
	db.Create(&activeComp)

	inactiveComp := models.Company{Name: "PT Nonaktif", Code: "NONAKTIF", IsActive: false}
	db.Create(&inactiveComp)

	// Subtest 1: Missing company_id
	{
		form := url.Values{"name": {"Dept Tanpa PT"}}
		req := httptest.NewRequest(http.MethodPost, "/admin/departments/create", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req = withSuperAdminContext(req)
		w := httptest.NewRecorder()

		handler.CreateDepartmentForm(w, req)
		resp := w.Result()
		loc := resp.Header.Get("Location")
		if !strings.Contains(loc, "error=") {
			t.Errorf("Expected error for missing company_id, got %s", loc)
		}
	}

	// Subtest 2: Inactive company_id
	{
		form := url.Values{
			"name":       {"Dept PT Nonaktif"},
			"company_id": {fmt.Sprintf("%d", inactiveComp.ID)},
		}
		req := httptest.NewRequest(http.MethodPost, "/admin/departments/create", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req = withSuperAdminContext(req)
		w := httptest.NewRecorder()

		handler.CreateDepartmentForm(w, req)
		resp := w.Result()
		loc := resp.Header.Get("Location")
		if !strings.Contains(loc, "error=") {
			t.Errorf("Expected error for inactive company_id, got %s", loc)
		}
	}

	// Subtest 3: Valid company_id succeeds
	{
		form := url.Values{
			"name":       {"Dept Sukses"},
			"company_id": {fmt.Sprintf("%d", activeComp.ID)},
		}
		req := httptest.NewRequest(http.MethodPost, "/admin/departments/create", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req = withSuperAdminContext(req)
		w := httptest.NewRecorder()

		handler.CreateDepartmentForm(w, req)
		resp := w.Result()
		if resp.StatusCode != http.StatusSeeOther && resp.StatusCode != http.StatusFound {
			t.Errorf("Expected redirect for valid department creation, got %d", resp.StatusCode)
		}

		var createdDept models.Department
		if err := db.Where("name = ?", "Dept Sukses").First(&createdDept).Error; err != nil {
			t.Fatalf("Expected Dept Sukses to exist in DB: %v", err)
		}
		if createdDept.CompanyID == nil || *createdDept.CompanyID != activeComp.ID {
			t.Errorf("Expected CompanyID %d, got %v", activeComp.ID, createdDept.CompanyID)
		}
	}
}
