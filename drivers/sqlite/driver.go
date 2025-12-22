package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jsonql-standard/jsonql-go"
	_ "modernc.org/sqlite"
)

// Driver implements jsonql.Driver for SQLite
type Driver struct {
	db *sql.DB
}

// Ensure Driver implements jsonql.Driver
var _ jsonql.Driver = (*Driver)(nil)

// NewDriver creates a new SQLite driver
// dsn can be ":memory:" or a file path
func NewDriver(dsn string) (*Driver, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite db: %w", err)
	}

	// Verify connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping sqlite db: %w", err)
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
	return "sqlite"
}

// GetDB returns the underlying sql.DB object (useful for setup/teardown in tests)
func (d *Driver) GetDB() *sql.DB {
	return d.db
}
