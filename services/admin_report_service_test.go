package services

import (
	"fmt"
	"testing"
	"time"

	"ticketing/config"
	"ticketing/models"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestAdminReportService_GetMonthName(t *testing.T) {
	tests := []struct {
		m    int
		want string
	}{
		{1, "Januari"},
		{2, "Februari"},
		{5, "Mei"},
		{8, "Agustus"},
		{9, "September"},
		{12, "Desember"},
		{0, ""},
		{13, ""},
		{-1, ""},
	}

	for _, tc := range tests {
		got := GetMonthName(tc.m)
		if got != tc.want {
			t.Errorf("GetMonthName(%d) = %q; want %q", tc.m, got, tc.want)
		}
	}
}

func TestAdminReportService_GetMonthOptions(t *testing.T) {
	svc := NewAdminReportService()
	now := time.Now()
	curMonth := int(now.Month())
	curYear := now.Year()

	options := svc.GetMonthOptions(curMonth, curYear)
	if len(options) != 12 {
		t.Fatalf("Expected 12 month options, got %d", len(options))
	}

	// Option pertama harusnya bulan sekarang dan IsSelected = true
	if !options[0].IsSelected {
		t.Errorf("Expected first option to be selected, got false")
	}
	if options[0].Month != curMonth || options[0].Year != curYear {
		t.Errorf("Expected first option (%d, %d), got (%d, %d)", curMonth, curYear, options[0].Month, options[0].Year)
	}

	expectedLabel := fmt.Sprintf("%s %d", GetMonthName(curMonth), curYear)
	if options[0].Label != expectedLabel {
		t.Errorf("Expected label %q, got %q", expectedLabel, options[0].Label)
	}
}

func setupReportTestDB(t *testing.T) (*gorm.DB, func()) {
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
		t.Skipf("Skipping report DB tests: PostgreSQL connection could not be established: %v", err)
		return nil, nil
	}

	schemaName := fmt.Sprintf("test_report_%d", time.Now().UnixNano())
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

	oldDB := config.DB
	config.DB = testDB

	_ = testDB.AutoMigrate(
		&models.Company{},
		&models.Department{},
		&models.User{},
		&models.Ticket{},
		&models.TicketRating{},
	)

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

func TestAdminReportService_GetMonthlyCompanyReport(t *testing.T) {
	testDB, cleanup := setupReportTestDB(t)
	if testDB == nil {
		return
	}
	defer cleanup()

	// Seed Company & Department
	company := models.Company{
		Name:     "PT Nusantara Tech",
		Code:     "NT",
		IsActive: true,
	}
	if err := testDB.Create(&company).Error; err != nil {
		t.Fatalf("Failed to create company: %v", err)
	}

	dept1 := models.Department{
		Name:      "IT Support",
		CompanyID: &company.ID,
	}
	dept2 := models.Department{
		Name:      "Billing",
		CompanyID: &company.ID,
	}
	testDB.Create(&dept1)
	testDB.Create(&dept2)

	// Base time in current month
	now := time.Now()
	testMonth := int(now.Month())
	testYear := now.Year()
	ticketBaseDate := time.Date(testYear, time.Month(testMonth), 5, 10, 0, 0, 0, now.Location())

	trueVal := true
	falseVal := false
	pastDeadline := ticketBaseDate.Add(2 * time.Hour)
	futureDeadline := now.Add(48 * time.Hour)

	// Ticket 1: Dept 1, Closed, SLA Met, Rating 5
	t1 := models.Ticket{
		Title:               "Printer Error",
		Description:         "Printer tidak jalan",
		Status:              models.StatusClosed,
		Priority:            models.PriorityLow,
		CompanyID:           &company.ID,
		DepartmentID:        &dept1.ID,
		FirstResponseMet:    &trueVal,
		ResolutionDeadline:  &futureDeadline,
		CreatedAt:           ticketBaseDate,
		UpdatedAt:           ticketBaseDate.Add(1 * time.Hour),
	}
	testDB.Create(&t1)
	testDB.Create(&models.TicketRating{TicketID: t1.ID, Rating: 5})

	// Ticket 2: Dept 1, Closed, FirstResponse Breached, Resolution SLA Met, Rating 3
	t2 := models.Ticket{
		Title:               "Network Slow",
		Description:         "Koneksi lambat",
		Status:              models.StatusClosed,
		Priority:            models.PriorityHigh,
		CompanyID:           &company.ID,
		DepartmentID:        &dept1.ID,
		FirstResponseMet:    &falseVal,
		ResolutionDeadline:  &futureDeadline,
		CreatedAt:           ticketBaseDate,
		UpdatedAt:           ticketBaseDate.Add(3 * time.Hour),
	}
	testDB.Create(&t2)
	testDB.Create(&models.TicketRating{TicketID: t2.ID, Rating: 3})

	// Ticket 3: Dept 2, Open (Waiting), Resolution Breached
	t3 := models.Ticket{
		Title:               "Invoice Request",
		Description:         "Butuh invoice",
		Status:              models.StatusWaiting,
		Priority:            models.PriorityMedium,
		CompanyID:           &company.ID,
		DepartmentID:        &dept2.ID,
		FirstResponseMet:    &trueVal,
		ResolutionDeadline:  &pastDeadline, // Breached
		CreatedAt:           ticketBaseDate,
		UpdatedAt:           ticketBaseDate,
	}
	testDB.Create(&t3)

	// Jalankan service report
	svc := NewAdminReportService()
	report, err := svc.GetMonthlyCompanyReport(MonthlyReportFilter{
		Month: testMonth,
		Year:  testYear,
	})
	if err != nil {
		t.Fatalf("GetMonthlyCompanyReport failed: %v", err)
	}

	// Validasi Grand Total
	if report.GrandTotal.TotalTickets != 3 {
		t.Errorf("Expected TotalTickets = 3, got %d", report.GrandTotal.TotalTickets)
	}
	if report.GrandTotal.OpenTickets != 1 {
		t.Errorf("Expected OpenTickets = 1, got %d", report.GrandTotal.OpenTickets)
	}
	if report.GrandTotal.ClosedTickets != 2 {
		t.Errorf("Expected ClosedTickets = 2, got %d", report.GrandTotal.ClosedTickets)
	}
	if report.GrandTotal.FirstResponseBreached != 1 {
		t.Errorf("Expected FirstResponseBreached = 1, got %d", report.GrandTotal.FirstResponseBreached)
	}
	if report.GrandTotal.ResolutionBreached != 1 {
		t.Errorf("Expected ResolutionBreached = 1, got %d", report.GrandTotal.ResolutionBreached)
	}
	if report.GrandTotal.TotalRatedCount != 2 {
		t.Errorf("Expected TotalRatedCount = 2, got %d", report.GrandTotal.TotalRatedCount)
	}
	// (5 + 3) / 2 = 4.0
	if report.GrandTotal.OverallAvgRating != 4.0 {
		t.Errorf("Expected OverallAvgRating = 4.0, got %.1f", report.GrandTotal.OverallAvgRating)
	}

	// Validasi Struktur Perusahaan
	if len(report.CompanyList) == 0 {
		t.Fatalf("Expected at least 1 company item in report")
	}
	compReport := report.CompanyList[0]
	if compReport.CompanyName != "PT Nusantara Tech" {
		t.Errorf("Expected CompanyName 'PT Nusantara Tech', got %q", compReport.CompanyName)
	}
	if compReport.TotalTickets != 3 {
		t.Errorf("Expected company TotalTickets = 3, got %d", compReport.TotalTickets)
	}
	if len(compReport.Departments) != 2 {
		t.Errorf("Expected 2 departments under company, got %d", len(compReport.Departments))
	}
}
