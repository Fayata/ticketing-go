package utils

import (
	"os"
	"testing"
)

func TestInitTemplates(t *testing.T) {
	// Change directory to project root so "templates" folder can be found
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get wd: %v", err)
	}
	defer os.Chdir(origDir)

	// Move up one level from utils to project root
	if err := os.Chdir(".."); err != nil {
		t.Fatalf("Failed to chdir: %v", err)
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("InitTemplates panicked: %v", r)
		}
	}()

	InitTemplates()
}
