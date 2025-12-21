package jsonql_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/jsonql-standard/jsonql-go"
	"github.com/jsonql-standard/jsonql-go/drivers/sqlite"
)

type ExecutionTestCase struct {
	ID             string                 `json:"id"`
	Description    string                 `json:"description"`
	TableName      string                 `json:"tableName"`
	Query          map[string]interface{} `json:"query"`
	ExpectedResult []map[string]interface{} `json:"expectedResult"`
}

func TestExecution(t *testing.T) {
	// 1. Load Data
	dataPath := "fixtures/suites/standard/data.json"
	
	dataBytes, err := os.ReadFile(dataPath)
	if err != nil {
		t.Skipf("Data file not found at %s, skipping execution tests", dataPath)
	}

	var dataset map[string][]map[string]interface{}
	if err := json.Unmarshal(dataBytes, &dataset); err != nil {
		t.Fatalf("Failed to parse data.json: %v", err)
	}

	// 2. Setup DB
	driver, err := sqlite.NewDriver(":memory:")
	if err != nil {
		t.Fatalf("Failed to open in-memory DB: %v", err)
	}
	defer driver.Close()

	ctx := context.Background()

	// 3. Create Tables and Insert Data
	for tableName, rows := range dataset {
		if len(rows) == 0 {
			continue
		}

		// Infer schema from first row
		firstRow := rows[0]
		var colDefs []string
		var colNames []string
		
		for col, val := range firstRow {
			colType := "TEXT"
			switch val.(type) {
			case float64:
				// JSON numbers are float64. Check if it's actually an int.
				if val == float64(int(val.(float64))) {
					colType = "INTEGER"
				} else {
					colType = "REAL"
				}
			case bool:
				colType = "BOOLEAN"
			case string:
				colType = "TEXT"
			case nil:
				colType = "TEXT" // Default to text for nulls
			}
			colDefs = append(colDefs, fmt.Sprintf("%s %s", col, colType))
			colNames = append(colNames, col)
		}

		createSQL := fmt.Sprintf("CREATE TABLE %s (%s)", tableName, strings.Join(colDefs, ", "))
		if _, err := driver.Execute(ctx, createSQL, nil); err != nil {
			t.Fatalf("Failed to create table %s: %v", tableName, err)
		}

		// Insert data
		placeholders := make([]string, len(colNames))
		for i := range placeholders {
			placeholders[i] = "?"
		}
		insertSQL := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", 
			tableName, strings.Join(colNames, ", "), strings.Join(placeholders, ", "))

		for _, row := range rows {
			var args []interface{}
			for _, col := range colNames {
				val := row[col]
				if val != nil {
					switch val.(type) {
					case map[string]interface{}, []interface{}:
						b, _ := json.Marshal(val)
						args = append(args, string(b))
					default:
						args = append(args, val)
					}
				} else {
					args = append(args, nil)
				}
			}
			if _, err := driver.Execute(ctx, insertSQL, args); err != nil {
				t.Fatalf("Failed to insert row into %s: %v", tableName, err)
			}
		}
	}

	// 4. Run Tests
	execTestPath := "fixtures/suites/standard/tests/execution.json"
	execBytes, err := os.ReadFile(execTestPath)
	if err != nil {
		t.Fatalf("Execution tests not found at %s", execTestPath)
	}

	var tests []ExecutionTestCase
	if err := json.Unmarshal(execBytes, &tests); err != nil {
		t.Fatalf("Failed to parse execution tests: %v", err)
	}

	parser := jsonql.NewParser()
	transpiler := jsonql.NewTranspiler("sqlite")

	for _, tc := range tests {
		t.Run(tc.ID, func(t *testing.T) {
			// Parse
			query, err := parser.Parse(tc.Query, nil, tc.TableName)
			if err != nil {
				t.Fatalf("Parsing failed: %v", err)
			}

			// Transpile
			result, err := transpiler.Transpile(query, tc.TableName, nil)
			if err != nil {
				t.Fatalf("Transpilation failed: %v", err)
			}

			// Execute
			rows, err := driver.Query(ctx, result.SQL, result.Args)
			if err != nil {
				t.Fatalf("Execution failed: %v\nSQL: %s\nArgs: %v", err, result.SQL, result.Args)
			}
			defer rows.Close()

			// Parse Results
			cols, _ := rows.Columns()
			var results []map[string]interface{}

			for rows.Next() {
				// Create a slice of interface{} to hold pointers to values
				columns := make([]interface{}, len(cols))
				columnPointers := make([]interface{}, len(cols))
				for i := range columns {
					columnPointers[i] = &columns[i]
				}

				if err := rows.Scan(columnPointers...); err != nil {
					t.Fatalf("Failed to scan row: %v", err)
				}

				rowMap := make(map[string]interface{})
				for i, colName := range cols {
					val := columns[i]
					// Convert []byte to string if necessary (SQLite often returns text as bytes)
					if b, ok := val.([]byte); ok {
						rowMap[colName] = string(b)
					} else {
						rowMap[colName] = val
					}
				}
				results = append(results, rowMap)
			}

			// Compare Results
			// We need to be careful with types (int vs float) and order
			if len(results) != len(tc.ExpectedResult) {
				t.Errorf("Expected %d rows, got %d", len(tc.ExpectedResult), len(results))
				return
			}

			for i, expected := range tc.ExpectedResult {
				actual := results[i]
				for k, v := range expected {
					actualVal, ok := actual[k]
					if !ok {
						t.Errorf("Row %d: missing key %s", i, k)
						continue
					}

					// Normalize numbers for comparison
					if !areEqual(v, actualVal) {
						t.Errorf("Row %d key %s: expected %v (%T), got %v (%T)", 
							i, k, v, v, actualVal, actualVal)
					}
				}
			}
		})
	}
}

func areEqual(expected, actual interface{}) bool {
	if reflect.DeepEqual(expected, actual) {
		return true
	}
	
	// Handle numeric mismatches (json unmarshals to float64, db might be int64)
	v1 := reflect.ValueOf(expected)
	v2 := reflect.ValueOf(actual)

	if isNumber(v1.Kind()) && isNumber(v2.Kind()) {
		f1 := toFloat(v1)
		f2 := toFloat(v2)
		return f1 == f2
	}

	return false
}

func isNumber(k reflect.Kind) bool {
	return k >= reflect.Int && k <= reflect.Float64
}

func toFloat(v reflect.Value) float64 {
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(v.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return float64(v.Uint())
	case reflect.Float32, reflect.Float64:
		return v.Float()
	}
	return 0
}
