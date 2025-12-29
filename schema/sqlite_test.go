package schema

import (
	"context"
	"testing"

	"github.com/jsonql-standard/jsonql-go/drivers/sqlite"
)

func TestSQLiteIntrospector(t *testing.T) {
	// 1. Setup In-Memory DB
	driver, err := sqlite.NewDriver(":memory:")
	if err != nil {
		t.Fatalf("Failed to create driver: %v", err)
	}
	defer driver.Close()

	ctx := context.Background()
	_, err = driver.Execute(ctx, `
		CREATE TABLE users (
			id INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			active BOOLEAN
		);
		CREATE TABLE posts (
			id INTEGER PRIMARY KEY,
			title TEXT,
			user_id INTEGER
		);
	`, nil)
	if err != nil {
		t.Fatalf("Failed to create tables: %v", err)
	}

	// 2. Introspect
	introspector := NewSQLiteIntrospector(driver)
	schema, err := introspector.Introspect()
	if err != nil {
		t.Fatalf("Introspection failed: %v", err)
	}

	// 3. Verify
	if len(schema.Tables) != 2 {
		t.Errorf("Expected 2 tables, got %d", len(schema.Tables))
	}

	// Verify Users
	users, ok := schema.Tables["users"]
	if !ok {
		t.Fatal("Expected users table")
	}
	if len(users.Fields) != 3 {
		t.Errorf("Expected 3 fields in users, got %d", len(users.Fields))
	}
	if users.Fields["id"].Type != "integer" {
		t.Errorf("Expected id type integer, got %s", users.Fields["id"].Type)
	}
	if users.Fields["name"].Type != "string" {
		t.Errorf("Expected name type string, got %s", users.Fields["name"].Type)
	}

	// Verify Posts
	posts, ok := schema.Tables["posts"]
	if !ok {
		t.Fatal("Expected posts table")
	}
	if len(posts.Fields) != 3 {
		t.Errorf("Expected 3 fields in posts, got %d", len(posts.Fields))
	}
}
