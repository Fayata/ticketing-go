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
			PeriodLabel:       "September 2026",
			CompanyList:       mockCompanyList,
			GrandTotal:        grandTotal,
			SelectedCompanyID: 1,
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
	}

	// Case 2: Filtered to Semua Perusahaan (SelectedCompanyID = 0) -> Grand total MUST appear
	{
		reportData := &services.MonthlyReportData{
			PeriodLabel:       "September 2026",
			CompanyList:       mockCompanyList,
			GrandTotal:        grandTotal,
			SelectedCompanyID: 0,
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
	}
}

