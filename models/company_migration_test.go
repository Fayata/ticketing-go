package models_test

import (
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"ticketing/config"
	"ticketing/models"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// TestCompanyModel_StructFieldsAndTags verifies the Company model specification
// without requiring an active database connection.
func TestCompanyModel_StructFieldsAndTags(t *testing.T) {
	c := models.Company{}

	// Test TableName
	if c.TableName() != "companies" {
		t.Errorf("expected TableName() to be 'companies', got '%s'", c.TableName())
	}

	companyType := reflect.TypeOf(c)

	// Verify required fields exist
	requiredFields := map[string]string{
		"ID":          "uint",
		"Name":        "string",
		"Code":        "string",
		"Address":     "string",
		"Phone":       "string",
		"IsActive":    "bool",
		"CreatedAt":   "time.Time",
		"UpdatedAt":   "time.Time",
		"DeletedAt":   "gorm.DeletedAt",
		"Departments": "[]models.Department",
		"Tickets":     "[]models.Ticket",
	}

	for fieldName, expectedType := range requiredFields {
		f, ok := companyType.FieldByName(fieldName)
		if !ok {
			t.Errorf("Company struct missing field '%s'", fieldName)
			continue
		}
		actualType := f.Type.String()
		if actualType != expectedType {
			t.Errorf("Company field '%s' expected type '%s', got '%s'", fieldName, expectedType, actualType)
		}
	}

	// Verify Code GORM tag
	codeField, _ := companyType.FieldByName("Code")
	gormTag := codeField.Tag.Get("gorm")
	if gormTag != "size:20;uniqueIndex;not null" {
		t.Errorf("expected Code gorm tag 'size:20;uniqueIndex;not null', got '%s'", gormTag)
	}

	// Verify Department CompanyID and Company relation
	deptType := reflect.TypeOf(models.Department{})
	compIDField, ok := deptType.FieldByName("CompanyID")
	if !ok || compIDField.Type.String() != "*uint" {
		t.Errorf("Department struct must have 'CompanyID *uint' field")
	}
	compField, ok := deptType.FieldByName("Company")
	if !ok || compField.Type.String() != "*models.Company" {
		t.Errorf("Department struct must have 'Company *Company' field")
	}

	// Verify Ticket CompanyID and Company relation
	ticketType := reflect.TypeOf(models.Ticket{})
	tCompIDField, ok := ticketType.FieldByName("CompanyID")
	if !ok || tCompIDField.Type.String() != "*uint" {
		t.Errorf("Ticket struct must have 'CompanyID *uint' field")
	}
	tCompField, ok := ticketType.FieldByName("Company")
	if !ok || tCompField.Type.String() != "*models.Company" {
		t.Errorf("Ticket struct must have 'Company *Company' field")
	}
}

// setupIsolatedTestDB attempts to connect to PostgreSQL and create an isolated schema.
// If PostgreSQL is not reachable, the test is skipped gracefully.
func setupIsolatedTestDB(t *testing.T) (*gorm.DB, func()) {
	t.Helper()
	cfg := config.LoadConfig()

	// Try candidate connection strings
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
		t.Skipf("Skipping integration test: PostgreSQL database connection could not be established: %v", err)
		return nil, nil
	}

	// Create unique isolated schema
	schemaName := fmt.Sprintf("test_m1_%d", time.Now().UnixNano())
	if err := db.Exec(fmt.Sprintf("CREATE SCHEMA %s", schemaName)).Error; err != nil {
		t.Fatalf("Failed to create isolated test schema %s: %v", schemaName, err)
	}

	// Connect test session to the isolated schema
	testDB, err := gorm.Open(postgres.Open(chosenDSN), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("Failed to create isolated session: %v", err)
	}
	testDB.Exec(fmt.Sprintf("SET search_path TO %s, public", schemaName))

	cleanup := func() {
		db.Exec(fmt.Sprintf("DROP SCHEMA %s CASCADE", schemaName))
		if sqlDB, _ := db.DB(); sqlDB != nil {
			_ = sqlDB.Close()
		}
		if testSQLDB, _ := testDB.DB(); testSQLDB != nil {
			_ = testSQLDB.Close()
		}
	}

	return testDB, cleanup
}

// TestAutoMigrateAndForeignKeys tests table creation, column creation, and foreign keys.
func TestAutoMigrateAndForeignKeys(t *testing.T) {
	db, cleanup := setupIsolatedTestDB(t)
	if db == nil {
		return
	}
	defer cleanup()

	// Migrate in proper dependency order: Company leading
	err := db.AutoMigrate(
		&models.Company{},
		&models.Department{},
		&models.Ticket{},
	)
	if err != nil {
		t.Fatalf("AutoMigrate failed: %v", err)
	}

	// Verify tables exist
	if !db.Migrator().HasTable(&models.Company{}) {
		t.Error("expected table 'companies' to exist")
	}
	if !db.Migrator().HasTable(&models.Department{}) {
		t.Error("expected table 'departments' to exist")
	}
	if !db.Migrator().HasTable(&models.Ticket{}) {
		t.Error("expected table 'tickets' to exist")
	}

	// Verify columns exist
	if !db.Migrator().HasColumn(&models.Department{}, "company_id") {
		t.Error("expected column 'company_id' in departments")
	}
	if !db.Migrator().HasColumn(&models.Ticket{}, "company_id") {
		t.Error("expected column 'company_id' in tickets")
	}

	// Verify Foreign Key enforcement: inserting invalid company ID fails
	invalidCompanyID := uint(999999)
	invalidDept := models.Department{
		Name:      "Orphan Dept",
		CompanyID: &invalidCompanyID,
	}
	if err := db.Create(&invalidDept).Error; err == nil {
		t.Error("expected foreign key error when inserting department with non-existent company_id, got nil")
	}

	// Verify inserting with valid company succeeds
	validCompany := models.Company{
		Name:     "Valid PT",
		Code:     "VALID_PT",
		IsActive: true,
	}
	if err := db.Create(&validCompany).Error; err != nil {
		t.Fatalf("failed to create valid company: %v", err)
	}

	validDept := models.Department{
		Name:      "Valid Dept",
		CompanyID: &validCompany.ID,
	}
	if err := db.Create(&validDept).Error; err != nil {
		t.Errorf("failed to create department with valid company_id: %v", err)
	}

	validTicket := models.Ticket{
		Title:        "Valid Ticket",
		Description:  "Valid Ticket Description",
		CompanyID:    &validCompany.ID,
		DepartmentID: &validDept.ID,
		CreatedByID:  1,
	}
	if err := db.Create(&validTicket).Error; err != nil {
		t.Errorf("failed to create ticket with valid company_id: %v", err)
	}
}

// TestSeedDefaultCompany_Idempotency tests that seeding default company runs idempotently.
func TestSeedDefaultCompany_Idempotency(t *testing.T) {
	db, cleanup := setupIsolatedTestDB(t)
	if db == nil {
		return
	}
	defer cleanup()

	_ = db.AutoMigrate(&models.Company{}, &models.Department{}, &models.Ticket{})

	// First execution
	comp1, err := models.SeedDefaultCompanyAndMigrate(db)
	if err != nil {
		t.Fatalf("First SeedDefaultCompanyAndMigrate failed: %v", err)
	}
	if comp1 == nil || comp1.ID == 0 {
		t.Fatalf("Expected valid company, got %+v", comp1)
	}
	if comp1.Code != models.DefaultCompanyCode || comp1.Name != models.DefaultCompanyName {
		t.Errorf("Unexpected company data: %+v", comp1)
	}

	// Second execution (Idempotency test)
	comp2, err := models.SeedDefaultCompanyAndMigrate(db)
	if err != nil {
		t.Fatalf("Second SeedDefaultCompanyAndMigrate failed: %v", err)
	}
	if comp2.ID != comp1.ID {
		t.Errorf("Expected identical company ID on second run, got %d vs %d", comp2.ID, comp1.ID)
	}

	// Check total count in DB
	var count int64
	db.Model(&models.Company{}).Where("code = ?", models.DefaultCompanyCode).Count(&count)
	if count != 1 {
		t.Errorf("Expected exactly 1 company with code %s, got %d", models.DefaultCompanyCode, count)
	}
}

// TestSafeBackfill_DepartmentsAndTickets tests that pre-existing departments and tickets
// (including soft-deleted tickets) are safely backfilled without touching other companies.
func TestSafeBackfill_DepartmentsAndTickets(t *testing.T) {
	db, cleanup := setupIsolatedTestDB(t)
	if db == nil {
		return
	}
	defer cleanup()

	_ = db.AutoMigrate(&models.Company{}, &models.Department{}, &models.Ticket{})

	// Create a separate company to prove non-target companies are NOT touched
	otherCompany := models.Company{
		Name:     "Other PT",
		Code:     "OTHER_PT",
		IsActive: true,
	}
	if err := db.Create(&otherCompany).Error; err != nil {
		t.Fatalf("failed to create other company: %v", err)
	}

	// Seed pre-existing records:
	// 1. Dept with nil company_id
	deptNil := models.Department{Name: "Legacy Nil Dept"}
	db.Create(&deptNil)

	// 2. Dept belonging to Other PT (should remain otherCompany.ID)
	deptOther := models.Department{Name: "Other Dept", CompanyID: &otherCompany.ID}
	db.Create(&deptOther)

	// 3. Active ticket with nil company_id
	ticketActive := models.Ticket{
		Title:       "Legacy Active Ticket",
		Description: "Active ticket test",
		CreatedByID: 1,
	}
	db.Create(&ticketActive)

	// 4. Soft-deleted ticket with nil company_id
	ticketSoftDeleted := models.Ticket{
		Title:       "Legacy Deleted Ticket",
		Description: "Deleted ticket test",
		CreatedByID: 1,
		DeletedAt:   gorm.DeletedAt{Time: time.Now(), Valid: true},
	}
	db.Create(&ticketSoftDeleted)

	// 5. Ticket belonging to Other PT
	ticketOther := models.Ticket{
		Title:       "Other Ticket",
		Description: "Other ticket test",
		CreatedByID: 1,
		CompanyID:   &otherCompany.ID,
	}
	db.Create(&ticketOther)

	// Run Seed and Backfill
	defaultCompany, err := models.SeedDefaultCompanyAndMigrate(db)
	if err != nil {
		t.Fatalf("SeedDefaultCompanyAndMigrate failed: %v", err)
	}

	// Verification 1: Legacy Dept was backfilled
	var verifiedDept models.Department
	db.First(&verifiedDept, deptNil.ID)
	if verifiedDept.CompanyID == nil || *verifiedDept.CompanyID != defaultCompany.ID {
		t.Errorf("Expected deptNil CompanyID=%d, got %v", defaultCompany.ID, verifiedDept.CompanyID)
	}

	// Verification 2: Other Dept was NOT overwritten
	var verifiedOtherDept models.Department
	db.First(&verifiedOtherDept, deptOther.ID)
	if verifiedOtherDept.CompanyID == nil || *verifiedOtherDept.CompanyID != otherCompany.ID {
		t.Errorf("Expected deptOther CompanyID=%d, got %v", otherCompany.ID, verifiedOtherDept.CompanyID)
	}

	// Verification 3: Legacy Active Ticket was backfilled
	var verifiedActive models.Ticket
	db.First(&verifiedActive, ticketActive.ID)
	if verifiedActive.CompanyID == nil || *verifiedActive.CompanyID != defaultCompany.ID {
		t.Errorf("Expected ticketActive CompanyID=%d, got %v", defaultCompany.ID, verifiedActive.CompanyID)
	}

	// Verification 4: Legacy Soft-Deleted Ticket was backfilled via Unscoped()
	var verifiedDeleted models.Ticket
	err = db.Unscoped().First(&verifiedDeleted, ticketSoftDeleted.ID).Error
	if err != nil {
		t.Fatalf("Failed to query soft-deleted ticket: %v", err)
	}
	if verifiedDeleted.CompanyID == nil || *verifiedDeleted.CompanyID != defaultCompany.ID {
		t.Errorf("Expected soft-deleted ticket CompanyID=%d, got %v", defaultCompany.ID, verifiedDeleted.CompanyID)
	}
	if !verifiedDeleted.DeletedAt.Valid {
		t.Error("Expected soft-deleted ticket to retain its DeletedAt status")
	}

	// Verification 5: Other Ticket was NOT overwritten
	var verifiedOtherTicket models.Ticket
	db.First(&verifiedOtherTicket, ticketOther.ID)
	if verifiedOtherTicket.CompanyID == nil || *verifiedOtherTicket.CompanyID != otherCompany.ID {
		t.Errorf("Expected ticketOther CompanyID=%d, got %v", otherCompany.ID, verifiedOtherTicket.CompanyID)
	}
}

// TestMultipleRunsAndConcurrency tests that concurrent execution across multiple services is safe.
func TestMultipleRunsAndConcurrency(t *testing.T) {
	db, cleanup := setupIsolatedTestDB(t)
	if db == nil {
		return
	}
	defer cleanup()

	_ = db.AutoMigrate(&models.Company{}, &models.Department{}, &models.Ticket{})

	// Simulate concurrent multi-service startup
	const concurrency = 10
	var wg sync.WaitGroup
	errs := make([]error, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, errs[idx] = models.SeedDefaultCompanyAndMigrate(db)
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("Goroutine %d failed: %v", i, err)
		}
	}

	// Verify exactly 1 default company exists
	var count int64
	db.Model(&models.Company{}).Where("code = ?", models.DefaultCompanyCode).Count(&count)
	if count != 1 {
		t.Errorf("Expected exactly 1 default company record under concurrent execution, got %d", count)
	}
}
