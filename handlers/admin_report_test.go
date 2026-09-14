package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"ticketing/config"
	"ticketing/services"
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
