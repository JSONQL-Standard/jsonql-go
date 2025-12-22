package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jsonql-standard/jsonql-go"
	_ "github.com/lib/pq"
)

// Driver implements jsonql.Driver for PostgreSQL
type Driver struct {
	db *sql.DB
}

// Ensure Driver implements jsonql.Driver
var _ jsonql.Driver = (*Driver)(nil)

// NewDriver creates a new PostgreSQL driver
// dsn example: "postgres://user:password@localhost/dbname?sslmode=disable"
func NewDriver(dsn string) (*Driver, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open postgres db: %w", err)
	}

	// Verify connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping postgres db: %w", err)
	}

	return &Driver{db: db}, nil
}

// Query executes a query and returns the rows
func (d *Driver) Query(ctx context.Context, query string, args []interface{}) (*sql.Rows, error) {
	return d.db.QueryContext(ctx, query, args...)
}

// Execute executes a statement and returns the result
func (d *Driver) Execute(ctx context.Context, query string, args []interface{}) (sql.Result, error) {
	return d.db.ExecContext(ctx, query, args...)
}

// Close closes the connection
func (d *Driver) Close() error {
	return d.db.Close()
}

// Dialect returns the SQL dialect name
func (d *Driver) Dialect() string {
	return "postgres"
}
