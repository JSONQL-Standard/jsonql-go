package jsonql_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	jsonqlhttp "github.com/jsonql-standard/jsonql-go/adapters/http"
	"github.com/jsonql-standard/jsonql-go/drivers/sqlite"
)

func TestSecurity_Whitelisting(t *testing.T) {
	// Setup DB
	driver, err := sqlite.NewDriver(":memory:")
	if err != nil {
		t.Fatalf("Failed to create driver: %v", err)
	}
	defer driver.Close()

	// Setup Handler with Whitelist
	handler, err := jsonqlhttp.NewHandler(jsonqlhttp.HandlerOptions{
		Driver:        driver,
		AllowedTables: []string{"users"},
	})
	if err != nil {
		t.Fatalf("Failed to create handler: %v", err)
	}

	// Test Allowed Table
	t.Run("Allowed Table", func(t *testing.T) {
		body := map[string]interface{}{
			"users": map[string]interface{}{
				"fields": []string{"id"},
			},
		}
		bodyBytes, _ := json.Marshal(body)
		req := httptest.NewRequest("POST", "/jsonql", bytes.NewBuffer(bodyBytes))
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		// Should be 200 (or 500 if DB fails, but not 403)
		if rec.Code == http.StatusForbidden {
			t.Errorf("Expected allowed, got 403 Forbidden")
		}
	})

	// Test Forbidden Table
	t.Run("Forbidden Table", func(t *testing.T) {
		body := map[string]interface{}{
			"posts": map[string]interface{}{
				"fields": []string{"id"},
			},
		}
		bodyBytes, _ := json.Marshal(body)
		req := httptest.NewRequest("POST", "/jsonql", bytes.NewBuffer(bodyBytes))
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Errorf("Expected 403 Forbidden, got %d", rec.Code)
		}
	})
}

func TestSecurity_MappingAndWhitelisting(t *testing.T) {
	driver, _ := sqlite.NewDriver(":memory:")
	defer driver.Close()

	// We want to expose 'my-users' which maps to 'users'.
	// We want to BLOCK 'users' direct access.
	// So we whitelist 'my-users' ONLY.
	handler, _ := jsonqlhttp.NewHandler(jsonqlhttp.HandlerOptions{
		Driver:        driver,
		AllowedTables: []string{"my-users"},
		TableMap: map[string]string{
			"my-users": "users",
		},
	})

	t.Run("Mapped and Allowed", func(t *testing.T) {
		body := map[string]interface{}{
			"my-users": map[string]interface{}{
				"fields": []string{"id"},
			},
		}
		bodyBytes, _ := json.Marshal(body)
		req := httptest.NewRequest("POST", "/jsonql", bytes.NewBuffer(bodyBytes))
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code == http.StatusForbidden {
			t.Errorf("Expected allowed, got 403 Forbidden")
		}
	})

	t.Run("Direct Access Forbidden", func(t *testing.T) {
		body := map[string]interface{}{
			"users": map[string]interface{}{
				"fields": []string{"id"},
			},
		}
		bodyBytes, _ := json.Marshal(body)
		req := httptest.NewRequest("POST", "/jsonql", bytes.NewBuffer(bodyBytes))
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Errorf("Expected 403 Forbidden, got %d", rec.Code)
		}
	})
}
