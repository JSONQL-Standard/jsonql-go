package jsonqlhttp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	jsonqlhttp "github.com/jsonql-standard/jsonql-go/adapters/http"
	"github.com/jsonql-standard/jsonql-go/drivers/sqlite"
)

func TestHandler(t *testing.T) {
	// 1. Setup DB
	driver, err := sqlite.NewDriver(":memory:")
	if err != nil {
		t.Fatalf("Failed to create driver: %v", err)
	}
	defer driver.Close()

	// Create table and data
	ctx := context.Background()
	_, err = driver.Execute(ctx, "CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT, email TEXT)", nil)
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}
	_, err = driver.Execute(ctx, "INSERT INTO users (name, email) VALUES (?, ?)", []interface{}{"Alice", "alice@example.com"})
	if err != nil {
		t.Fatalf("Failed to insert data: %v", err)
	}

	// 2. Setup Handler
	handler, err := jsonqlhttp.NewHandler(jsonqlhttp.HandlerOptions{
		Driver: driver,
	})
	if err != nil {
		t.Fatalf("Failed to create handler: %v", err)
	}

	// 3. Create Request
	// Query: { "users": { "fields": ["name"], "filter": { "name": "Alice" } } }
	reqBody := map[string]interface{}{
		"users": map[string]interface{}{
			"fields": []string{"name", "email"},
			"filter": map[string]interface{}{
				"name": "Alice",
			},
		},
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/jsonql", bytes.NewReader(bodyBytes))
	w := httptest.NewRecorder()

	// 4. Execute
	handler.ServeHTTP(w, req)

	// 5. Verify
	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var result []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(result))
	}

	if result[0]["name"] != "Alice" {
		t.Errorf("Expected name Alice, got %v", result[0]["name"])
	}
	if result[0]["email"] != "alice@example.com" {
		t.Errorf("Expected email alice@example.com, got %v", result[0]["email"])
	}
}
