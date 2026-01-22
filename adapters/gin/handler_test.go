package jsonqlgin_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	jsonqlgin "github.com/jsonql-standard/jsonql-go/adapters/gin"
	jsonqlhttp "github.com/jsonql-standard/jsonql-go/adapters/http"
	"github.com/jsonql-standard/jsonql-go/drivers/sqlite"
	_ "modernc.org/sqlite" // Register sqlite driver
)

func TestGinHandler(t *testing.T) {
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
	handler, err := jsonqlgin.NewHandler(jsonqlgin.HandlerOptions{
		AdapterOptions: jsonqlhttp.AdapterOptions{
			Driver: driver,
		},
	})
	if err != nil {
		t.Fatalf("Failed to create handler: %v", err)
	}

	// 3. Setup Gin
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/jsonql", handler)

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

	req, _ := http.NewRequest("POST", "/jsonql", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// 5. Serve
	r.ServeHTTP(w, req)

	// 6. Assert
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	var resp []map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
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
