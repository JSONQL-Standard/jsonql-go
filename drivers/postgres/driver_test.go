package postgres_test

import (
	"context"
	"testing"

	"github.com/jsonql-standard/jsonql-go/drivers/postgres"
)

// TestPostgresDriver is a placeholder test.
// Real integration tests require a running Postgres instance.
// We can skip if connection fails.
func TestPostgresDriver(t *testing.T) {
	dsn := "postgres://postgres:postgres@localhost:5432/jsonql_test?sslmode=disable"
	driver, err := postgres.NewDriver(dsn)
	if err != nil {
		t.Skipf("Skipping postgres test: %v", err)
	}
	defer driver.Close()

	ctx := context.Background()

	// Create table
	_, err = driver.Execute(ctx, `
		CREATE TABLE IF NOT EXISTS users (
			id SERIAL PRIMARY KEY,
			name TEXT,
			email TEXT
		)
	`, nil)
	if err != nil {
		t.Fatalf("Failed to create table: %v", err)
	}

	// Clean up
	defer driver.Execute(ctx, "DROP TABLE users", nil)

	// Insert
	_, err = driver.Execute(ctx, "INSERT INTO users (name, email) VALUES ($1, $2)", []interface{}{"Bob", "bob@example.com"})
	if err != nil {
		t.Fatalf("Failed to insert: %v", err)
	}

	// Query
	rows, err := driver.Query(ctx, "SELECT name, email FROM users WHERE name = $1", []interface{}{"Bob"})
	if err != nil {
		t.Fatalf("Failed to query: %v", err)
	}
	defer rows.Close()

	if !rows.Next() {
		t.Fatal("Expected result, got none")
	}

	var name, email string
	if err := rows.Scan(&name, &email); err != nil {
		t.Fatal(err)
	}

	if name != "Bob" {
		t.Errorf("Expected Bob, got %s", name)
	}
}
