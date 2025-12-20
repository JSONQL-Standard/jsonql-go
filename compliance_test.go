package jsonql

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCase matches the structure of the JSON compliance tests
type TestCase struct {
	ID             string                 `json:"id"`
	Description    string                 `json:"description"`
	Query          map[string]interface{} `json:"query"`
	Valid          *bool                  `json:"valid"`
	ExpectedError  string                 `json:"expectedError"`
	ExpectedResult interface{}            `json:"expectedResult"`
}

func TestCompliance(t *testing.T) {
	// Define the root path to the compliance suites
	// Allow overriding via environment variable for CI/CD
	suitesPath := os.Getenv("JSONQL_SPEC_PATH")
	if suitesPath == "" {
		suitesPath = "../jsonql-spec"
	}
	suitesPath = filepath.Join(suitesPath, "tests/suites")
	
	absPath, _ := filepath.Abs(suitesPath)

	if _, err := os.Stat(suitesPath); os.IsNotExist(err) {
		t.Skipf("Compliance suites not found at %s, skipping", absPath)
	}

	// Helper to run tests from a file
	runTestsFromFile := func(path string) {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("Failed to read test file %s: %v", path, err)
		}

		var tests []TestCase
		if err := json.Unmarshal(data, &tests); err != nil {
			t.Fatalf("Failed to parse test file %s: %v", path, err)
		}

		parser := NewParser()

		for _, tc := range tests {
			t.Run(tc.ID, func(t *testing.T) {
				err := parser.Parse(tc.Query)

				// Determine if the test expects validity
				expectValid := true
				if tc.Valid != nil {
					expectValid = *tc.Valid
				} else if tc.ExpectedResult == nil {
					// If valid is missing and no expected result, assume valid (default for most tests)
					// BUT wait, standard tests usually have "valid": true/false.
					// Issues tests might just have expectedResult.
					// If both are missing, it's ambiguous.
					// Let's assume true unless explicitly false?
					// In Go, unmarshal to bool gives false.
					// Let's stick to: if Valid is present, use it.
					// If Valid is missing, check ExpectedResult. If present -> true.
					// If neither, assume true?
					expectValid = true
				}

				if expectValid {
					if err != nil {
						t.Errorf("Expected query to be valid, but got error: %v", err)
					}
				} else {
					if err == nil {
						t.Errorf("Expected query to be invalid, but it passed validation")
					} else if tc.ExpectedError != "" {
						if !strings.Contains(err.Error(), tc.ExpectedError) {
							t.Errorf("Expected error containing '%s', got '%s'", tc.ExpectedError, err.Error())
						}
					}
				}
			})
		}
	}

	// 1. Standard Suite
	standardTestsPath := filepath.Join(suitesPath, "standard", "tests")
	files, _ := os.ReadDir(standardTestsPath)
	for _, f := range files {
		if strings.HasSuffix(f.Name(), ".json") {
			runTestsFromFile(filepath.Join(standardTestsPath, f.Name()))
		}
	}

	// 2. Issues Suite
	issuesPath := filepath.Join(suitesPath, "issues")
	issueDirs, _ := os.ReadDir(issuesPath)
	for _, d := range issueDirs {
		if d.IsDir() {
			testFile := filepath.Join(issuesPath, d.Name(), "test.json")
			if _, err := os.Stat(testFile); err == nil {
				runTestsFromFile(testFile)
			}
		}
	}

	// 3. Security Suite
	securityPath := filepath.Join(suitesPath, "security")
	secFiles, _ := os.ReadDir(securityPath)
	for _, f := range secFiles {
		if strings.HasSuffix(f.Name(), ".json") {
			runTestsFromFile(filepath.Join(securityPath, f.Name()))
		}
	}
}
