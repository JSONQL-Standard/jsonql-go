package jsonql_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/jsonql-standard/jsonql-go"
	"github.com/jsonql-standard/jsonql-go/drivers/sqlite"
)

// ---------- Builder Tests ----------

func TestEngineBuilder_DefaultsSQLite(t *testing.T) {
	engine := jsonql.NewEngineBuilder().
		Executor(func(ctx context.Context, sql string, args []interface{}) (*sql.Rows, error) {
			return nil, nil
		}).
		Build()
	if engine == nil {
		t.Fatal("Build() returned nil")
	}
}

func TestEngineBuilder_Postgres(t *testing.T) {
	engine := jsonql.NewEngineBuilder().
		Postgres().
		Executor(func(ctx context.Context, sql string, args []interface{}) (*sql.Rows, error) {
			return nil, nil
		}).
		Build()
	if engine == nil {
		t.Fatal("Build() returned nil")
	}
}

func TestEngineBuilder_MySQL(t *testing.T) {
	engine := jsonql.NewEngineBuilder().
		MySQL().
		Executor(func(ctx context.Context, sql string, args []interface{}) (*sql.Rows, error) {
			return nil, nil
		}).
		Build()
	if engine == nil {
		t.Fatal("Build() returned nil")
	}
}

func TestEngineBuilder_MSSQL(t *testing.T) {
	engine := jsonql.NewEngineBuilder().
		MSSQL().
		Executor(func(ctx context.Context, sql string, args []interface{}) (*sql.Rows, error) {
			return nil, nil
		}).
		Build()
	if engine == nil {
		t.Fatal("Build() returned nil")
	}
}

func TestEngineBuilder_WithDriver(t *testing.T) {
	driver, err := sqlite.NewDriver(":memory:")
	if err != nil {
		t.Fatalf("Failed to create driver: %v", err)
	}
	defer driver.Close()

	engine := jsonql.NewEngineBuilder().
		WithDriver(driver).
		Build()
	if engine == nil {
		t.Fatal("Build() returned nil")
	}
}

func TestEngineBuilder_DebugMode(t *testing.T) {
	engine := jsonql.NewEngineBuilder().
		SQLite().
		Debug(true).
		Executor(func(ctx context.Context, sql string, args []interface{}) (*sql.Rows, error) {
			return nil, nil
		}).
		Build()
	if engine == nil {
		t.Fatal("Build() returned nil")
	}
}

func TestEngineBuilder_WithSchema(t *testing.T) {
	schema := &jsonql.JSONQLSchema{
		Tables: map[string]*jsonql.JSONQLTable{
			"users": {
				Fields: map[string]*jsonql.JSONQLField{
					"id":   {Type: "integer"},
					"name": {Type: "string"},
				},
			},
		},
	}

	engine := jsonql.NewEngineBuilder().
		SQLite().
		Schema(schema).
		Executor(func(ctx context.Context, sql string, args []interface{}) (*sql.Rows, error) {
			return nil, nil
		}).
		Build()
	if engine == nil {
		t.Fatal("Build() returned nil")
	}
}

// ---------- Execution Tests (using real SQLite) ----------

func setupTestDB(t *testing.T) *sqlite.Driver {
	t.Helper()
	driver, err := sqlite.NewDriver(":memory:")
	if err != nil {
		t.Fatalf("Failed to open in-memory DB: %v", err)
	}

	ctx := context.Background()

	// Create users table
	_, err = driver.Execute(ctx,
		"CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT, email TEXT, age INTEGER)", nil)
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	// Insert test data
	for _, row := range []struct {
		id    int
		name  string
		email string
		age   int
	}{
		{1, "Alice", "alice@example.com", 30},
		{2, "Bob", "bob@example.com", 25},
		{3, "Charlie", "charlie@example.com", 35},
	} {
		_, err = driver.Execute(ctx,
			"INSERT INTO users (id, name, email, age) VALUES (?, ?, ?, ?)",
			[]interface{}{row.id, row.name, row.email, row.age})
		if err != nil {
			t.Fatalf("Failed to insert: %v", err)
		}
	}

	return driver
}

func TestEngine_SelectAll(t *testing.T) {
	driver := setupTestDB(t)
	defer driver.Close()

	engine := jsonql.NewEngineBuilder().
		WithDriver(driver).
		Build()

	raw := map[string]interface{}{}
	result, err := engine.Execute(context.Background(), raw, "users")
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result.IsMutation {
		t.Error("Expected IsMutation=false for a select query")
	}
	if len(result.Data) != 3 {
		t.Errorf("Expected 3 rows, got %d", len(result.Data))
	}
}

func TestEngine_SelectWithFilter(t *testing.T) {
	driver := setupTestDB(t)
	defer driver.Close()

	engine := jsonql.NewEngineBuilder().
		WithDriver(driver).
		Build()

	raw := map[string]interface{}{
		"where": map[string]interface{}{
			"name": "Alice",
		},
	}
	result, err := engine.Execute(context.Background(), raw, "users")
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if len(result.Data) != 1 {
		t.Errorf("Expected 1 row, got %d", len(result.Data))
	}
}

func TestEngine_SelectWithSort(t *testing.T) {
	driver := setupTestDB(t)
	defer driver.Close()

	engine := jsonql.NewEngineBuilder().
		WithDriver(driver).
		Build()

	raw := map[string]interface{}{
		"sort":  "-age",
		"limit": float64(2),
	}
	result, err := engine.Execute(context.Background(), raw, "users")
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if len(result.Data) != 2 {
		t.Errorf("Expected 2 rows, got %d", len(result.Data))
	}
}

func TestEngine_SelectWithFields(t *testing.T) {
	driver := setupTestDB(t)
	defer driver.Close()

	engine := jsonql.NewEngineBuilder().
		WithDriver(driver).
		Build()

	raw := map[string]interface{}{
		"fields": []interface{}{"name", "email"},
	}
	result, err := engine.Execute(context.Background(), raw, "users")
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if len(result.Data) != 3 {
		t.Errorf("Expected 3 rows, got %d", len(result.Data))
	}
}

func TestEngine_InsertMutation(t *testing.T) {
	driver := setupTestDB(t)
	defer driver.Close()

	engine := jsonql.NewEngineBuilder().
		WithDriver(driver).
		Build()

	raw := map[string]interface{}{
		"data": map[string]interface{}{
			"id":    float64(4),
			"name":  "Diana",
			"email": "diana@example.com",
			"age":   float64(28),
		},
	}
	result, err := engine.Execute(context.Background(), raw, "users")
	if err != nil {
		t.Fatalf("Insert failed: %v", err)
	}
	if !result.IsMutation {
		t.Error("Expected IsMutation=true for an insert")
	}

	// Verify insert by querying
	selectResult, err := engine.Execute(context.Background(), map[string]interface{}{
		"where": map[string]interface{}{"name": "Diana"},
	}, "users")
	if err != nil {
		t.Fatalf("Select after insert failed: %v", err)
	}
	if len(selectResult.Data) != 1 {
		t.Errorf("Expected 1 row after insert, got %d", len(selectResult.Data))
	}
}

func TestEngine_UpdateMutation(t *testing.T) {
	driver := setupTestDB(t)
	defer driver.Close()

	engine := jsonql.NewEngineBuilder().
		WithDriver(driver).
		Build()

	raw := map[string]interface{}{
		"patch": map[string]interface{}{
			"age": float64(31),
		},
		"where": map[string]interface{}{
			"name": "Alice",
		},
	}
	result, err := engine.Execute(context.Background(), raw, "users")
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	if !result.IsMutation {
		t.Error("Expected IsMutation=true for an update")
	}
}

func TestEngine_DeleteMutation(t *testing.T) {
	driver := setupTestDB(t)
	defer driver.Close()

	engine := jsonql.NewEngineBuilder().
		WithDriver(driver).
		Build()

	raw := map[string]interface{}{
		"delete": true,
		"where": map[string]interface{}{
			"name": "Bob",
		},
	}
	result, err := engine.Execute(context.Background(), raw, "users")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	if !result.IsMutation {
		t.Error("Expected IsMutation=true for a delete")
	}

	// Verify deletion
	selectResult, err := engine.Execute(context.Background(), map[string]interface{}{}, "users")
	if err != nil {
		t.Fatalf("Select after delete failed: %v", err)
	}
	if len(selectResult.Data) != 2 {
		t.Errorf("Expected 2 rows after deletion, got %d", len(selectResult.Data))
	}
}

// ---------- Error Handling Tests ----------

func TestEngine_NoDriverOrExecutor(t *testing.T) {
	engine := jsonql.NewEngineBuilder().
		SQLite().
		Build()

	_, err := engine.Execute(context.Background(), map[string]interface{}{}, "users")
	if err == nil {
		t.Fatal("Expected error when no driver or executor configured")
	}
}

func TestEngine_ValidationWithSchema(t *testing.T) {
	driver := setupTestDB(t)
	defer driver.Close()

	schema := &jsonql.JSONQLSchema{
		Tables: map[string]*jsonql.JSONQLTable{
			"users": {
				Fields: map[string]*jsonql.JSONQLField{
					"id":    {Type: "integer"},
					"name":  {Type: "string"},
					"email": {Type: "string"},
					"age":   {Type: "integer"},
				},
			},
		},
	}

	engine := jsonql.NewEngineBuilder().
		WithDriver(driver).
		Schema(schema).
		Build()

	// Valid query should work
	result, err := engine.Execute(context.Background(), map[string]interface{}{
		"fields": []interface{}{"name"},
	}, "users")
	if err != nil {
		t.Fatalf("Valid query with schema failed: %v", err)
	}
	if len(result.Data) != 3 {
		t.Errorf("Expected 3 rows, got %d", len(result.Data))
	}
}

func TestEngine_ExecutorCallback(t *testing.T) {
	var capturedSQL string
	var capturedArgs []interface{}

	driver := setupTestDB(t)
	defer driver.Close()

	engine := jsonql.NewEngineBuilder().
		SQLite().
		Executor(func(ctx context.Context, sqlStr string, args []interface{}) (*sql.Rows, error) {
			capturedSQL = sqlStr
			capturedArgs = args
			// Use the actual driver to execute
			return driver.Query(ctx, sqlStr, args)
		}).
		Build()

	raw := map[string]interface{}{
		"where": map[string]interface{}{
			"name": "Alice",
		},
	}
	result, err := engine.Execute(context.Background(), raw, "users")
	if err != nil {
		t.Fatalf("Execute with executor failed: %v", err)
	}

	if capturedSQL == "" {
		t.Error("Executor callback was not invoked")
	}
	if !strings.Contains(strings.ToLower(capturedSQL), "select") {
		t.Errorf("Expected SELECT query, got: %s", capturedSQL)
	}
	if capturedArgs == nil {
		t.Error("Expected args to be non-nil")
	}
	if len(result.Data) != 1 {
		t.Errorf("Expected 1 row, got %d", len(result.Data))
	}
}

// ---------- Fixture-based Execution Tests ----------

func TestEngine_ExecutionFixtures(t *testing.T) {
	dataPath := "fixtures/suites/standard/data.json"
	dataBytes, err := os.ReadFile(dataPath)
	if err != nil {
		t.Skipf("Data file not found at %s, skipping fixture-based engine tests", dataPath)
	}

	var dataset map[string][]map[string]interface{}
	if err := json.Unmarshal(dataBytes, &dataset); err != nil {
		t.Fatalf("Failed to parse data.json: %v", err)
	}

	driver, err := sqlite.NewDriver(":memory:")
	if err != nil {
		t.Fatalf("Failed to open in-memory DB: %v", err)
	}
	defer driver.Close()

	ctx := context.Background()

	// Create and populate tables
	for tableName, rows := range dataset {
		if len(rows) == 0 {
			continue
		}
		firstRow := rows[0]
		var colDefs, colNames []string
		for col, val := range firstRow {
			colType := "TEXT"
			switch val.(type) {
			case float64:
				if val == float64(int(val.(float64))) {
					colType = "INTEGER"
				} else {
					colType = "REAL"
				}
			case bool:
				colType = "BOOLEAN"
			}
			colDefs = append(colDefs, fmt.Sprintf("%s %s", col, colType))
			colNames = append(colNames, col)
		}

		createSQL := fmt.Sprintf("CREATE TABLE %s (%s)", tableName, strings.Join(colDefs, ", "))
		if _, err := driver.Execute(ctx, createSQL, nil); err != nil {
			t.Fatalf("Create %s failed: %v", tableName, err)
		}

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
				t.Fatalf("Insert into %s failed: %v", tableName, err)
			}
		}
	}

	// Build engine
	engine := jsonql.NewEngineBuilder().
		WithDriver(driver).
		Build()

	// Load and run execution tests
	execBytes, err := os.ReadFile("fixtures/suites/standard/tests/execution.json")
	if err != nil {
		t.Skipf("Execution tests not found, skipping")
	}

	var tests []ExecutionTestCase
	if err := json.Unmarshal(execBytes, &tests); err != nil {
		t.Fatalf("Failed to parse execution tests: %v", err)
	}

	for _, tc := range tests {
		t.Run("Engine_"+tc.ID, func(t *testing.T) {
			result, err := engine.Execute(ctx, tc.Query, tc.TableName)
			if err != nil {
				t.Fatalf("Engine.Execute failed: %v", err)
			}

			if len(result.Data) != len(tc.ExpectedResult) {
				t.Errorf("Expected %d rows, got %d", len(tc.ExpectedResult), len(result.Data))
				return
			}

			for i, expected := range tc.ExpectedResult {
				actual := result.Data[i]
				for k, v := range expected {
					actualVal, ok := actual[k]
					if !ok {
						t.Errorf("Row %d: missing key %s", i, k)
						continue
					}
					if !areEqual(v, actualVal) {
						t.Errorf("Row %d key %s: expected %v (%T), got %v (%T)",
							i, k, v, v, actualVal, actualVal)
					}
				}
			}
		})
	}
}
