package handlers

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"ticketing/config"
	"ticketing/services"
	"ticketing/utils"
)

func TestShowReports_MethodNotAllowed(t *testing.T) {
	cfg := config.LoadConfig()
	handler := NewAdminHandler(cfg, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/admin/reports", nil)
	w := httptest.NewRecorder()

	handler.ShowReports(w, req)
	resp := w.Result()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405 Method Not Allowed, got %d", resp.StatusCode)
	}
}

func TestShowReports_QueryParsing(t *testing.T) {
	// Memastikan query string parsing aman dan tidak panic saat config.DB bernilai nil / tidak terhubung
	cfg := config.LoadConfig()
	reportSvc := services.NewAdminReportService()
	handler := NewAdminHandler(cfg, nil, nil, nil, reportSvc)

	req := httptest.NewRequest(http.MethodGet, "/admin/reports?period=2026-09&company_id=5", nil)
	w := httptest.NewRecorder()

	// Hanya memverifikasi tidak ada runtime panic saat routing & parsing
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("ShowReports panicked: %v", r)
		}
	}()

	handler.ShowReports(w, req)
}

func TestReportTemplate_GrandTotalVisibility(t *testing.T) {
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get wd: %v", err)
	}
	defer os.Chdir(origDir)

	if err := os.Chdir(".."); err != nil {
		t.Fatalf("Failed to chdir: %v", err)
	}

	utils.InitTemplates()

	mockCompanyList := []services.CompanyReportItem{
		{
			CompanyID:   1,
			CompanyName: "PT Utama",
			CompanyCode: "DEFAULT",
			Departments: []services.DepartmentReportItem{
				{
					DepartmentID:   1,
					DepartmentName: "Billing",
					TotalTickets:   5,
					OpenTickets:    3,
					ClosedTickets:  2,
				},
			},
			TotalTickets:  5,
			OpenTickets:   3,
			ClosedTickets: 2,
		},
	}

	grandTotal := services.GrandTotalSummary{
		TotalTickets:  5,
		OpenTickets:   3,
		ClosedTickets: 2,
	}

	// Case 1: Filtered to specific company (SelectedCompanyID = 1) -> Grand total must NOT appear
	{
		reportData := &services.MonthlyReportData{
			PeriodLabel:          "September 2026",
			CompanyList:          mockCompanyList,
			GrandTotal:           grandTotal,
			SelectedCompanyID:    1,
			SelectedCompanyName:  "PT Utama (DEFAULT)",
			GeneratedAtFormatted: "14 Sep 2026, 15:50 WIB",
		}
		data := map[string]interface{}{
			"report":        reportData,
			"template_name": "admin/reports",
		}

		w := httptest.NewRecorder()
		utils.RenderTemplate(w, "admin/reports", data)
		body := w.Body.String()

		if strings.Contains(body, "GRAND TOTAL KESELURUHAN:") {
			t.Errorf("Grand total should NOT appear when filtered to a specific company (SelectedCompanyID = 1)")
		}
		if !strings.Contains(body, "Subtotal PT Utama:") {
			t.Errorf("Subtotal PT Utama should appear when filtered to PT Utama")
		}
		if !strings.Contains(body, "print-meta-table") {
			t.Errorf("print-meta-table should be present in rendered HTML")
		}
		if !strings.Contains(body, "PT Utama (DEFAULT)") {
			t.Errorf("Selected company name should be present in print metadata")
		}
	}

	// Case 2: Filtered to Semua Perusahaan (SelectedCompanyID = 0) -> Grand total MUST appear
	{
		reportData := &services.MonthlyReportData{
			PeriodLabel:          "September 2026",
			CompanyList:          mockCompanyList,
			GrandTotal:           grandTotal,
			SelectedCompanyID:    0,
			SelectedCompanyName:  "Semua Perusahaan",
			GeneratedAtFormatted: "14 Sep 2026, 15:50 WIB",
		}
		data := map[string]interface{}{
			"report":        reportData,
			"template_name": "admin/reports",
		}

		w := httptest.NewRecorder()
		utils.RenderTemplate(w, "admin/reports", data)
		body := w.Body.String()

		if !strings.Contains(body, "GRAND TOTAL KESELURUHAN:") {
			t.Errorf("Grand total MUST appear when filter Semua Perusahaan is active (SelectedCompanyID = 0)")
		}
		if !strings.Contains(body, "Subtotal PT Utama:") {
			t.Errorf("Subtotal PT Utama should appear")
		}
		if !strings.Contains(body, "Semua Perusahaan") {
			t.Errorf("Semua Perusahaan should be present in print metadata")
		}
	}
}

func TestShowDepartmentReport_MethodNotAllowed(t *testing.T) {
	cfg := config.LoadConfig()
	handler := NewAdminHandler(cfg, nil, nil, nil, nil)

	req := httptest.NewRequest(http.MethodPost, "/admin/reports/department", nil)
	w := httptest.NewRecorder()

	handler.ShowDepartmentReport(w, req)
	resp := w.Result()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405 Method Not Allowed, got %d", resp.StatusCode)
	}
}

func TestShowDepartmentReport_MissingDeptID(t *testing.T) {
	cfg := config.LoadConfig()
	reportSvc := services.NewAdminReportService()
	handler := NewAdminHandler(cfg, nil, nil, nil, reportSvc)

	req := httptest.NewRequest(http.MethodGet, "/admin/reports/department?period=2026-09", nil)
	w := httptest.NewRecorder()

	handler.ShowDepartmentReport(w, req)
	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status 400 Bad Request when department_id is missing, got %d", resp.StatusCode)
	}
}

/* [Feature Hidden / Stashed for later release]
func TestDepartmentReportTemplate_Rendering(t *testing.T) {
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get wd: %v", err)
	}
	defer os.Chdir(origDir)

	if err := os.Chdir(".."); err != nil {
		t.Fatalf("Failed to chdir: %v", err)
	}

	utils.InitTemplates()

	mockStaffList := []services.StaffReportItem{
		{
			StaffID:               10,
			StaffName:             "Budi Santoso",
			Username:              "budi",
			TotalTickets:          8,
			OpenTickets:           3,
			ClosedTickets:         5,
			FirstResponseBreached: 0,
			ResolutionBreached:    1,
			SLAMetCount:           7,
			SLAMetRate:            87.5,
			RatedCount:            4,
			AvgRating:             4.8,
		},
	}

	unassigned := &services.StaffReportItem{
		StaffID:               0,
		StaffName:             "Belum Diklaim / Pool Departemen",
		Username:              "-",
		TotalTickets:          2,
		OpenTickets:           2,
		ClosedTickets:         0,
		FirstResponseBreached: 1,
		ResolutionBreached:    0,
		SLAMetCount:           1,
		SLAMetRate:            50.0,
	}

	deptSummary := services.DepartmentReportItem{
		DepartmentID:          5,
		DepartmentName:        "IT Support",
		TotalTickets:          10,
		OpenTickets:           5,
		ClosedTickets:         5,
		FirstResponseBreached: 1,
		ResolutionBreached:    1,
		SLAMetCount:           8,
		SLAMetRate:            80.0,
		RatedCount:            4,
		AvgRating:             4.8,
	}

	reportData := &services.DepartmentDetailReportData{
		DepartmentID:         5,
		DepartmentName:       "IT Support",
		CompanyID:            1,
		CompanyName:          "PT Prima Solusi",
		CompanyCode:          "PPS",
		PeriodLabel:          "September 2026",
		Month:                9,
		Year:                 2026,
		GeneratedAtFormatted: "14 Sep 2026, 16:30 WIB",
		StaffList:            mockStaffList,
		UnassignedItem:       unassigned,
		Summary:              deptSummary,
		AutoPrint:            true,
	}

	data := map[string]interface{}{
		"report":        reportData,
		"template_name": "admin/department_report",
	}

	w := httptest.NewRecorder()
	utils.RenderTemplate(w, "admin/department_report", data)
	body := w.Body.String()

	if !strings.Contains(body, "Budi Santoso") {
		t.Errorf("Rendered department report should contain staff name 'Budi Santoso'")
	}
	if !strings.Contains(body, "@budi") {
		t.Errorf("Rendered department report should contain username '@budi'")
	}
	if !strings.Contains(body, "Belum Diklaim (Pool Departemen)") {
		t.Errorf("Rendered department report should contain unassigned pool item")
	}
	if !strings.Contains(body, "TOTAL DEPARTEMEN IT Support:") {
		t.Errorf("Rendered department report should contain subtotal label for IT Support")
	}
	if !strings.Contains(body, "window.print()") {
		t.Errorf("Rendered department report should contain window.print() when AutoPrint is true")
	}
}

func TestReportsTemplate_AccordionAndButtons(t *testing.T) {
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get wd: %v", err)
	}
	defer os.Chdir(origDir)

	if err := os.Chdir(".."); err != nil {
		t.Fatalf("Failed to chdir: %v", err)
	}

	utils.InitTemplates()

	mockCompanyList := []services.CompanyReportItem{
		{
			CompanyID:   1,
			CompanyName: "PT Prima Solusi",
			CompanyCode: "PPS",
			Departments: []services.DepartmentReportItem{
				{
					DepartmentID:   101,
					DepartmentName: "Network Operations",
					TotalTickets:   4,
					OpenTickets:    1,
					ClosedTickets:  3,
				},
			},
			TotalTickets:  4,
			OpenTickets:   1,
			ClosedTickets: 3,
		},
	}

	reportData := &services.MonthlyReportData{
		Filter:               services.MonthlyReportFilter{Month: 9, Year: 2026},
		MonthName:            "September",
		PeriodLabel:          "September 2026",
		CompanyList:          mockCompanyList,
		SelectedCompanyID:    0,
		SelectedCompanyName:  "Semua Perusahaan",
		GeneratedAtFormatted: "14 Sep 2026, 16:30 WIB",
		Year:                 2026,
	}

	data := map[string]interface{}{
		"report":        reportData,
		"template_name": "admin/reports",
	}

	w := httptest.NewRecorder()
	utils.RenderTemplate(w, "admin/reports", data)
	body := w.Body.String()

	if !strings.Contains(body, "company-chevron") {
		t.Errorf("Expected reports table to contain accordion chevron 'company-chevron'")
	}
	if !strings.Contains(body, "btn-action-print") {
		t.Errorf("Expected reports table to contain company print button 'btn-action-print'")
	}
	if !strings.Contains(body, "btn-action-print-dept") {
		t.Errorf("Expected reports table to contain department staff print button 'btn-action-print-dept'")
	}
	if !strings.Contains(body, "dept-company-1") {
		t.Errorf("Expected department row to have class 'dept-company-1'")
	}
}
*/


