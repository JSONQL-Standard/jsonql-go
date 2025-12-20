package jsonql

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

type TranspilationTestCase struct {
	ID           string                 `json:"id"`
	Description  string                 `json:"description"`
	TableName    string                 `json:"tableName"`
	Query        map[string]interface{} `json:"query"`
	ExpectedSQL  string                 `json:"expectedSQL"`
	ExpectedArgs []interface{}          `json:"expectedArgs"`
}

func TestTranspilation(t *testing.T) {
	// Try to find the spec file relative to the test file
	// Assuming we are running from jsonql-go root
	specPath := "../jsonql-spec/tests/transpilation/sql.json"
	
	// Check if file exists
	if _, err := os.Stat(specPath); os.IsNotExist(err) {
		t.Skipf("Spec file not found at %s, skipping tests", specPath)
	}

	data, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatalf("Failed to read transpilation tests: %v", err)
	}

	var tests []TranspilationTestCase
	if err := json.Unmarshal(data, &tests); err != nil {
		t.Fatalf("Failed to parse transpilation tests: %v", err)
	}

	parser := NewParser()
	transpiler := NewTranspiler("sqlite")

	for _, tc := range tests {
		t.Run(tc.ID, func(t *testing.T) {
			// Parse the raw query map into our struct
			query, err := parser.Parse(tc.Query)
			if err != nil {
				t.Fatalf("Parsing failed: %v", err)
			}

			result, err := transpiler.Transpile(query, tc.TableName)
			if err != nil {
				t.Fatalf("Transpilation failed: %v", err)
			}

			if result.SQL != tc.ExpectedSQL {
				t.Errorf("Expected SQL: '%s', got: '%s'", tc.ExpectedSQL, result.SQL)
			}
			
			if len(tc.ExpectedArgs) > 0 {
				if len(result.Args) != len(tc.ExpectedArgs) {
					t.Errorf("Expected %d args, got %d", len(tc.ExpectedArgs), len(result.Args))
				} else {
					// Use DeepEqual for slice comparison if needed, or simple loop
					// Note: JSON numbers are float64, so we might need loose comparison
					for i, arg := range result.Args {
						// Simple equality check might fail for int vs float64
						if !reflect.DeepEqual(arg, tc.ExpectedArgs[i]) {
							// Try converting to float64 for comparison if numbers
							val1 := toFloat(arg)
							val2 := toFloat(tc.ExpectedArgs[i])
							if val1 != nil && val2 != nil && *val1 == *val2 {
								continue
							}
							t.Errorf("Expected arg %d to be %v (%T), got %v (%T)", i, tc.ExpectedArgs[i], tc.ExpectedArgs[i], arg, arg)
						}
					}
				}
			}
		})
	}
}

func toFloat(v interface{}) *float64 {
	var f float64
	switch i := v.(type) {
	case float64:
		f = i
	case int:
		f = float64(i)
	case int64:
		f = float64(i)
	default:
		return nil
	}
	return &f
}
