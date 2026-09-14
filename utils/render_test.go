package utils

import (
	"bytes"
	"fmt"
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

func TestTemplateLenHelper(t *testing.T) {
	// Move up one level from utils to project root
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get wd: %v", err)
	}
	defer os.Chdir(origDir)

	if err := os.Chdir(".."); err != nil {
		t.Fatalf("Failed to chdir: %v", err)
	}

	InitTemplates()

	type CustomItem struct {
		Name string
	}

	testCases := []struct {
		name     string
		input    interface{}
		expected int
	}{
		{"nil slice", nil, 0},
		{"empty slice", []CustomItem{}, 0},
		{"custom struct slice", []CustomItem{{Name: "A"}, {Name: "B"}, {Name: "C"}}, 3},
		{"string slice", []string{"x", "y"}, 2},
		{"int slice", []int{10, 20, 30, 40}, 4},
		{"string", "hello", 5},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tClone, err := templates.Clone()
			if err != nil {
				t.Fatalf("Failed to clone templates: %v", err)
			}
			tmpl, err := tClone.New("test-len").Parse("{{len .}}")
			if err != nil {
				t.Fatalf("Failed to parse test template: %v", err)
			}

			var buf bytes.Buffer
			if err := tmpl.Execute(&buf, tc.input); err != nil {
				t.Fatalf("Failed to execute template: %v", err)
			}

			expectedStr := fmt.Sprintf("%d", tc.expected)
			if buf.String() != expectedStr {
				t.Errorf("Expected len %s, got %s", expectedStr, buf.String())
			}
		})
	}
}

