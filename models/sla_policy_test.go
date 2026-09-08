package models_test

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"ticketing/config"
	"ticketing/models"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestSLAPolicyModel_StructFieldsAndTags(t *testing.T) {
	p := models.SLAPolicy{}
	if p.TableName() != "sla_policies" {
		t.Errorf("expected TableName() to be 'sla_policies', got '%s'", p.TableName())
	}

	policyType := reflect.TypeOf(p)
	requiredFields := map[string]string{
		"ID":                      "uint",
		"Name":                    "string",
		"Description":             "string",
		"CompanyID":               "*uint",
		"DepartmentID":            "*uint",
		"IsActive":                "bool",
		"IsDefault":               "bool",
		"ResponseTimeHighMinutes": "int",
		"ResponseTimeMedMinutes":  "int",
		"ResponseTimeLowMinutes":  "int",
		"ResolutionTimeHighHours": "int",
		"ResolutionTimeMedHours":  "int",
		"ResolutionTimeLowHours":  "int",
		"CreatedAt":               "time.Time",
		"UpdatedAt":               "time.Time",
		"Company":                 "*models.Company",
		"Department":              "*models.Department",
	}

	for fieldName, expectedType := range requiredFields {
		f, ok := policyType.FieldByName(fieldName)
		if !ok {
			t.Errorf("SLAPolicy struct missing field '%s'", fieldName)
			continue
		}
		if f.Type.String() != expectedType {
			t.Errorf("SLAPolicy field '%s' expected type '%s', got '%s'", fieldName, expectedType, f.Type.String())
		}
	}
}

func TestTicketPriorityHistoryModel_StructFieldsAndTags(t *testing.T) {
	h := models.TicketPriorityHistory{}
	if h.TableName() != "ticket_priority_histories" {
		t.Errorf("expected TableName() to be 'ticket_priority_histories', got '%s'", h.TableName())
	}

	histType := reflect.TypeOf(h)
	requiredFields := map[string]string{
		"ID":          "uint",
		"TicketID":    "uint",
		"OldPriority": "models.TicketPriority",
		"NewPriority": "models.TicketPriority",
		"ChangedByID": "uint",
		"Reason":      "string",
		"CreatedAt":   "time.Time",
		"Ticket":      "models.Ticket",
		"ChangedBy":   "models.User",
	}

	for fieldName, expectedType := range requiredFields {
		f, ok := histType.FieldByName(fieldName)
		if !ok {
			t.Errorf("TicketPriorityHistory struct missing field '%s'", fieldName)
			continue
		}
		if f.Type.String() != expectedType {
			t.Errorf("TicketPriorityHistory field '%s' expected type '%s', got '%s'", fieldName, expectedType, f.Type.String())
		}
	}
}

func TestTicketModel_SLAFields(t *testing.T) {
	ticketType := reflect.TypeOf(models.Ticket{})
	requiredFields := map[string]string{
		"SLAPolicyID":           "*uint",
		"FirstResponseDeadline": "*time.Time",
		"FirstResponseAt":       "*time.Time",
		"FirstResponseMet":      "*bool",
		"ResolutionDeadline":    "*time.Time",
		"EstimatedResolutionAt": "*time.Time",
		"SLAWarningSent":        "bool",
		"SLABreachSent":         "bool",
		"SLAPolicy":             "*models.SLAPolicy",
		"PriorityHistories":     "[]models.TicketPriorityHistory",
	}

	for fieldName, expectedType := range requiredFields {
		f, ok := ticketType.FieldByName(fieldName)
		if !ok {
			t.Errorf("Ticket struct missing SLA field '%s'", fieldName)
			continue
		}
		if f.Type.String() != expectedType {
			t.Errorf("Ticket SLA field '%s' expected type '%s', got '%s'", fieldName, expectedType, f.Type.String())
		}
	}
}

func TestSLAPolicy_GetFirstResponseDuration_NilAndValues(t *testing.T) {
	// 1. Nil policy fallback
	var nilPolicy *models.SLAPolicy
	if d := nilPolicy.GetFirstResponseDuration(models.PriorityHigh); d != 60*time.Minute {
		t.Errorf("expected 60m for High on nil policy, got %v", d)
	}
	if d := nilPolicy.GetFirstResponseDuration(models.PriorityMedium); d != 240*time.Minute {
		t.Errorf("expected 240m for Medium on nil policy, got %v", d)
	}
	if d := nilPolicy.GetFirstResponseDuration(models.PriorityLow); d != 480*time.Minute {
		t.Errorf("expected 480m for Low on nil policy, got %v", d)
	}

	// 2. Custom values
	p := &models.SLAPolicy{
		ResponseTimeHighMinutes: 30,
		ResponseTimeMedMinutes:  120,
		ResponseTimeLowMinutes:  360,
	}
	if d := p.GetFirstResponseDuration(models.PriorityHigh); d != 30*time.Minute {
		t.Errorf("expected 30m for High, got %v", d)
	}
	if d := p.GetFirstResponseDuration(models.PriorityMedium); d != 120*time.Minute {
		t.Errorf("expected 120m for Medium, got %v", d)
	}
	if d := p.GetFirstResponseDuration(models.PriorityLow); d != 360*time.Minute {
		t.Errorf("expected 360m for Low, got %v", d)
	}

	// 3. Fallback when value <= 0
	zeroPolicy := &models.SLAPolicy{}
	if d := zeroPolicy.GetFirstResponseDuration(models.PriorityHigh); d != 60*time.Minute {
		t.Errorf("expected 60m fallback for zero High, got %v", d)
	}
}

func TestSLAPolicy_GetResolutionDuration_NilAndValues(t *testing.T) {
	// 1. Nil policy fallback
	var nilPolicy *models.SLAPolicy
	if d := nilPolicy.GetResolutionDuration(models.PriorityHigh); d != 4*time.Hour {
		t.Errorf("expected 4h for High on nil policy, got %v", d)
	}
	if d := nilPolicy.GetResolutionDuration(models.PriorityMedium); d != 24*time.Hour {
		t.Errorf("expected 24h for Medium on nil policy, got %v", d)
	}
	if d := nilPolicy.GetResolutionDuration(models.PriorityLow); d != 72*time.Hour {
		t.Errorf("expected 72h for Low on nil policy, got %v", d)
	}

	// 2. Custom values
	p := &models.SLAPolicy{
		ResolutionTimeHighHours: 2,
		ResolutionTimeMedHours:  12,
		ResolutionTimeLowHours:  48,
	}
	if d := p.GetResolutionDuration(models.PriorityHigh); d != 2*time.Hour {
		t.Errorf("expected 2h for High, got %v", d)
	}
	if d := p.GetResolutionDuration(models.PriorityMedium); d != 12*time.Hour {
		t.Errorf("expected 12h for Medium, got %v", d)
	}
	if d := p.GetResolutionDuration(models.PriorityLow); d != 48*time.Hour {
		t.Errorf("expected 48h for Low, got %v", d)
	}
}

func TestResolveSLAPolicy_NilDB(t *testing.T) {
	policy, respDur, resDur := models.ResolveSLAPolicy(nil, nil, nil, models.PriorityHigh)
	if policy != nil {
		t.Errorf("expected nil policy when db is nil, got %+v", policy)
	}
	if respDur != 60*time.Minute {
		t.Errorf("expected 60m response duration, got %v", respDur)
	}
	if resDur != 4*time.Hour {
		t.Errorf("expected 4h resolution duration, got %v", resDur)
	}
}

func TestResolveSLAPolicy_Continuous247Calendar(t *testing.T) {
	// Test Friday 23:45 UTC + 60m -> Saturday 00:45 UTC without interruption
	fridayNight := time.Date(2026, 9, 11, 23, 45, 0, 0, time.UTC)
	_, respDur, resDur := models.ResolveSLAPolicy(nil, nil, nil, models.PriorityHigh)

	responseDeadline := fridayNight.Add(respDur)
	expectedResponse := time.Date(2026, 9, 12, 0, 45, 0, 0, time.UTC)
	if !responseDeadline.Equal(expectedResponse) {
		t.Errorf("24/7 calendar response failed across midnight: expected %v, got %v", expectedResponse, responseDeadline)
	}

	resolutionDeadline := fridayNight.Add(resDur)
	expectedResolution := time.Date(2026, 9, 12, 3, 45, 0, 0, time.UTC)
	if !resolutionDeadline.Equal(expectedResolution) {
		t.Errorf("24/7 calendar resolution failed across midnight: expected %v, got %v", expectedResolution, resolutionDeadline)
	}
}

func TestTicket_GetSLABadgeInfo(t *testing.T) {
	now := time.Now()
	metTrue := true
	metFalse := false

	// Case 1: Closed ticket met
	closedMet := &models.Ticket{
		Status:           models.StatusClosed,
		FirstResponseMet: &metTrue,
	}
	b := closedMet.GetSLABadgeInfo()
	if b.Class != "sla-badge-green" || b.IsBreached {
		t.Errorf("closed ticket met: expected green, not breached, got %+v", b)
	}

	// Case 2: Closed ticket breached
	closedBreached := &models.Ticket{
		Status:           models.StatusClosed,
		FirstResponseMet: &metFalse,
	}
	b = closedBreached.GetSLABadgeInfo()
	if b.Class != "sla-badge-red" || !b.IsBreached {
		t.Errorf("closed ticket breached: expected red, breached, got %+v", b)
	}

	// Case 3: Open ticket awaiting first response, deadline far in future
	future := now.Add(2 * time.Hour)
	openSafe := &models.Ticket{
		Status:                models.StatusWaiting,
		FirstResponseDeadline: &future,
	}
	b = openSafe.GetSLABadgeInfo()
	if b.Class != "sla-badge-green" || b.IsBreached || b.IsWarning {
		t.Errorf("open ticket safe: expected green, got %+v", b)
	}

	// Case 4: Open ticket awaiting first response, deadline in 15 mins (warning)
	nearDeadline := now.Add(15 * time.Minute)
	openWarning := &models.Ticket{
		Status:                models.StatusWaiting,
		FirstResponseDeadline: &nearDeadline,
	}
	b = openWarning.GetSLABadgeInfo()
	if b.Class != "sla-badge-yellow" || !b.IsWarning {
		t.Errorf("open ticket warning: expected yellow, got %+v", b)
	}

	// Case 5: Open ticket awaiting first response, deadline in past (breach)
	pastDeadline := now.Add(-10 * time.Minute)
	openBreached := &models.Ticket{
		Status:                models.StatusWaiting,
		FirstResponseDeadline: &pastDeadline,
	}
	b = openBreached.GetSLABadgeInfo()
	if b.Class != "sla-badge-red" || !b.IsBreached {
		t.Errorf("open ticket breached: expected red, got %+v", b)
	}

	// Case 6: Open ticket first response met, resolution estimate in future
	respTime := now.Add(-30 * time.Minute)
	estFuture := now.Add(3 * time.Hour)
	estTicket := &models.Ticket{
		Status:                models.StatusInProgress,
		FirstResponseAt:       &respTime,
		FirstResponseMet:      &metTrue,
		EstimatedResolutionAt: &estFuture,
	}
	b = estTicket.GetSLABadgeInfo()
	if b.Class != "sla-badge-green" || b.IsBreached {
		t.Errorf("estimated ticket safe: expected green, got %+v", b)
	}
}

func setupSLATestDB(t *testing.T) (*gorm.DB, func()) {
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
		t.Skipf("Skipping integration test: PostgreSQL database connection could not be established: %v", err)
		return nil, nil
	}

	schemaName := fmt.Sprintf("test_sla_%d", time.Now().UnixNano())
	if err := db.Exec(fmt.Sprintf("CREATE SCHEMA %s", schemaName)).Error; err != nil {
		t.Fatalf("Failed to create isolated test schema %s: %v", schemaName, err)
	}

	testDB, err := gorm.Open(postgres.Open(chosenDSN), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("Failed to create isolated session: %v", err)
	}
	testDB.Exec(fmt.Sprintf("SET search_path TO %s, public", schemaName))

	cleanup := func() {
		testDB.Exec(fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", schemaName))
	}

	return testDB, cleanup
}

func TestResolveSLAPolicy_Hierarchy_WithDB(t *testing.T) {
	db, cleanup := setupSLATestDB(t)
	if db == nil {
		return
	}
	defer cleanup()

	// Migrate models
	if err := db.AutoMigrate(
		&models.Company{},
		&models.Department{},
		&models.SLAPolicy{},
		&models.Ticket{},
	); err != nil {
		t.Fatalf("AutoMigrate failed: %v", err)
	}

	// 1. Setup Company & Department
	comp := models.Company{Name: "PT Maju Terus", Code: "PMT", IsActive: true}
	db.Create(&comp)

	dept := models.Department{Name: "IT Support", CompanyID: &comp.ID}
	db.Create(&dept)

	// 2. Global Default Policy (High: 60m)
	globalPolicy := models.SLAPolicy{
		Name:                    "Global Default",
		IsDefault:               true,
		IsActive:                true,
		ResponseTimeHighMinutes: 60,
		ResolutionTimeHighHours: 4,
	}
	db.Create(&globalPolicy)

	// Hierarchy Level 3: Should resolve Global Default
	resolved, respDur, _ := models.ResolveSLAPolicy(db, nil, nil, models.PriorityHigh)
	if resolved == nil || resolved.ID != globalPolicy.ID || respDur != 60*time.Minute {
		t.Errorf("Level 3 check failed: expected global default (ID %d, 60m), got %+v, %v", globalPolicy.ID, resolved, respDur)
	}

	// 3. Company Default Policy (High: 45m)
	compPolicy := models.SLAPolicy{
		Name:                    "Company PT Default",
		CompanyID:               &comp.ID,
		IsActive:                true,
		ResponseTimeHighMinutes: 45,
		ResolutionTimeHighHours: 3,
	}
	db.Create(&compPolicy)

	// Hierarchy Level 2: Should resolve Company Default over Global Default
	resolved, respDur, _ = models.ResolveSLAPolicy(db, &comp.ID, nil, models.PriorityHigh)
	if resolved == nil || resolved.ID != compPolicy.ID || respDur != 45*time.Minute {
		t.Errorf("Level 2 check failed: expected company policy (ID %d, 45m), got %+v, %v", compPolicy.ID, resolved, respDur)
	}

	// 4. Department Override Policy (High: 15m)
	deptPolicy := models.SLAPolicy{
		Name:                    "Dept Override",
		CompanyID:               &comp.ID,
		DepartmentID:            &dept.ID,
		IsActive:                true,
		ResponseTimeHighMinutes: 15,
		ResolutionTimeHighHours: 1,
	}
	db.Create(&deptPolicy)

	// Hierarchy Level 1: Should resolve Department Override over Company and Global
	resolved, respDur, _ = models.ResolveSLAPolicy(db, &comp.ID, &dept.ID, models.PriorityHigh)
	if resolved == nil || resolved.ID != deptPolicy.ID || respDur != 15*time.Minute {
		t.Errorf("Level 1 check failed: expected dept override (ID %d, 15m), got %+v, %v", deptPolicy.ID, resolved, respDur)
	}

	// Inactive check: If dept policy is inactive, should fallback to company policy
	db.Model(&deptPolicy).Update("is_active", false)
	resolved, respDur, _ = models.ResolveSLAPolicy(db, &comp.ID, &dept.ID, models.PriorityHigh)
	if resolved == nil || resolved.ID != compPolicy.ID || respDur != 45*time.Minute {
		t.Errorf("Inactive dept policy should fallback to company policy, got %+v, %v", resolved, respDur)
	}
}

func TestSeedDefaultSLAPolicies_Idempotent_WithDB(t *testing.T) {
	db, cleanup := setupSLATestDB(t)
	if db == nil {
		return
	}
	defer cleanup()

	// Migrate models
	if err := db.AutoMigrate(
		&models.Company{},
		&models.Department{},
		&models.SLAPolicy{},
		&models.Ticket{},
	); err != nil {
		t.Fatalf("AutoMigrate failed: %v", err)
	}

	// Create legacy ticket without SLA fields
	legacyTicket := models.Ticket{
		Title:       "Legacy Ticket",
		Description: "Created before SLA module",
		Status:      models.StatusWaiting,
		Priority:    models.PriorityHigh,
		CreatedAt:   time.Now().Add(-1 * time.Hour),
	}
	db.Create(&legacyTicket)

	// 1. First seeding run
	if err := models.SeedDefaultSLAPolicies(db); err != nil {
		t.Fatalf("First SeedDefaultSLAPolicies failed: %v", err)
	}

	var defaultPolicy models.SLAPolicy
	if err := db.Where("is_default = ?", true).First(&defaultPolicy).Error; err != nil {
		t.Fatalf("Expected default SLA policy to exist: %v", err)
	}

	// Verify legacy ticket was backfilled
	var reloadedTicket models.Ticket
	db.First(&reloadedTicket, legacyTicket.ID)
	if reloadedTicket.SLAPolicyID == nil || *reloadedTicket.SLAPolicyID != defaultPolicy.ID {
		t.Errorf("Expected legacy ticket SLA policy ID to be %d, got %v", defaultPolicy.ID, reloadedTicket.SLAPolicyID)
	}
	if reloadedTicket.FirstResponseDeadline == nil {
		t.Errorf("Expected legacy ticket to have FirstResponseDeadline calculated")
	}

	// 2. Second seeding run (idempotency check)
	if err := models.SeedDefaultSLAPolicies(db); err != nil {
		t.Fatalf("Second SeedDefaultSLAPolicies failed: %v", err)
	}

	var count int64
	db.Model(&models.SLAPolicy{}).Where("is_default = ?", true).Count(&count)
	if count != 1 {
		t.Errorf("Expected exactly 1 default policy after second run, got %d", count)
	}
}
