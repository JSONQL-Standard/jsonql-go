package jsonqlecho_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	jsonqlecho "github.com/jsonql-standard/jsonql-go/adapters/echo"
	"github.com/jsonql-standard/jsonql-go/drivers/sqlite"
	"github.com/labstack/echo/v4"
	_ "modernc.org/sqlite" // Register sqlite driver
)

func TestEchoHandler(t *testing.T) {
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
	handler, err := jsonqlecho.NewHandler(jsonqlecho.HandlerOptions{
		Driver: driver,
	})
	if err != nil {
		t.Fatalf("Failed to create handler: %v", err)
	}

	// 3. Setup Echo
	e := echo.New()
	e.POST("/jsonql", handler)

	// 4. Create Request
	reqBody := map[string]interface{}{
		"users": map[string]interface{}{
			"fields": []string{"name", "email"},
			"where": map[string]interface{}{
				"name": map[string]interface{}{"eq": "Alice"},
			},
		},
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/jsonql", bytes.NewBuffer(bodyBytes))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	// c := e.NewContext(req, rec) // Unused

	// 5. Serve (Call handler directly or via router)
	// Since we registered it, we can serve via ServeHTTP if we want full integration,
	// or just call the handler. Let's use ServeHTTP to test routing too.
	e.ServeHTTP(rec, req)

	// 6. Assert
	if rec.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d. Body: %s", rec.Code, rec.Body.String())
	}

	var resp []map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if len(resp) != 1 {
		t.Errorf("Expected 1 user, got %d", len(resp))
	}

	user := resp[0]
	if user["name"] != "Alice" {
		t.Errorf("Expected name Alice, got %v", user["name"])
	}
}
