package logging

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRotatingFileWriter(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "log_rotate_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	logPath := filepath.Join(tempDir, "test.log")
	// Max 100 bytes, max 3 backups
	writer, err := NewRotatingFileWriter(logPath, 100, 3)
	if err != nil {
		t.Fatalf("failed to create writer: %v", err)
	}
	defer writer.Close()

	// Write 40 bytes
	data40 := []byte("1234567890123456789012345678901234567890")
	_, err = writer.Write(data40)
	if err != nil {
		t.Fatalf("write 1 failed: %v", err)
	}

	// Write another 40 bytes (total 80)
	_, err = writer.Write(data40)
	if err != nil {
		t.Fatalf("write 2 failed: %v", err)
	}

	// Write another 40 bytes (exceeds 100 bytes -> trigger rotate)
	_, err = writer.Write(data40)
	if err != nil {
		t.Fatalf("write 3 failed: %v", err)
	}

	// Check that test.log exists and test.log.1 exists
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		t.Errorf("expected %s to exist", logPath)
	}

	backup1 := logPath + ".1"
	if _, err := os.Stat(backup1); os.IsNotExist(err) {
		t.Errorf("expected backup file %s to exist", backup1)
	}
}
