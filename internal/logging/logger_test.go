package logging

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoggingInitAndCategorizedLogs(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "logging_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Initialize logging for test
	Init("test-service", tempDir)
	defer Close()

	// Write logs to various categories
	HTTPAccess.Info("HTTP request processed", "status", 200, "method", "GET", "path", "/api/test")
	HTTPTemplate.Info("Template rendered", "template", "test.html", "duration_ms", 12)
	DBQueries.Debug("SQL executed", "query", "SELECT 1", "elapsed_ms", 5)
	DBSlowQueries.Warn("Slow query detected", "query", "SELECT * FROM tickets", "elapsed_ms", 250)
	AuthLogin.Info("User logged in", "user_id", 42, "ip", "127.0.0.1")
	TicketLifecycle.Info("Ticket created", "ticket_id", 101, "ticket_number", "T26-0001")
	SLACalculations.Info("SLA deadline calculated", "ticket_id", 101, "hours", 2)
	NotificationInApp.Info("Notification sent", "user_id", 42, "title", "Test")
	SystemLifecycle.Info("Service startup complete", "port", 8080)

	// Verify categories and directories were created
	expectedDirs := []string{
		filepath.Join(tempDir, "http_ui"),
		filepath.Join(tempDir, "db"),
		filepath.Join(tempDir, "auth"),
		filepath.Join(tempDir, "tickets"),
		filepath.Join(tempDir, "sla"),
		filepath.Join(tempDir, "notifications"),
		filepath.Join(tempDir, "system"),
	}

	for _, dir := range expectedDirs {
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			t.Errorf("expected directory %s to exist as a directory, err: %v", dir, err)
		}
	}

	// Verify log files exist and contain valid JSON lines
	testCases := []struct {
		category    string
		filename    string
		expectedSub string
		expectedMsg string
	}{
		{"http_ui", "access.json.log", "access", "HTTP request processed"},
		{"tickets", "lifecycle.json.log", "lifecycle", "Ticket created"},
		{"auth", "login.json.log", "login", "User logged in"},
		{"sla", "calculations.json.log", "calculations", "SLA deadline calculated"},
	}

	for _, tc := range testCases {
		filePath := filepath.Join(tempDir, tc.category, tc.filename)
		f, err := os.Open(filePath)
		if err != nil {
			t.Errorf("failed to open expected log file %s: %v", filePath, err)
			continue
		}

		scanner := bufio.NewScanner(f)
		var foundMatch bool
		for scanner.Scan() {
			line := scanner.Text()
			var logEntry map[string]interface{}
			if err := json.Unmarshal([]byte(line), &logEntry); err != nil {
				t.Errorf("log line in %s is not valid JSON: %s (err: %v)", filePath, line, err)
				continue
			}

			// Validate standard JSON fields
			if logEntry["service"] != "test-service" {
				t.Errorf("expected service 'test-service', got '%v'", logEntry["service"])
			}
			if logEntry["category"] != tc.category {
				t.Errorf("expected category '%s', got '%v'", tc.category, logEntry["category"])
			}
			if logEntry["subcategory"] != tc.expectedSub {
				t.Errorf("expected subcategory '%s', got '%v'", tc.expectedSub, logEntry["subcategory"])
			}
			if logEntry["msg"] == tc.expectedMsg {
				foundMatch = true
			}
			if _, ok := logEntry["time"]; !ok {
				t.Errorf("expected 'time' field in log entry")
			}
			if _, ok := logEntry["level"]; !ok {
				t.Errorf("expected 'level' field in log entry")
			}
		}
		f.Close()

		if !foundMatch {
			t.Errorf("did not find expected message '%s' in %s", tc.expectedMsg, filePath)
		}
	}
}
