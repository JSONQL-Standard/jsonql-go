package jsonql

import (
	"context"
	"database/sql"
)

// Driver is the interface that must be implemented by database drivers
type Driver interface {
	// Query executes a query and returns the rows
	Query(ctx context.Context, sql string, args []interface{}) (*sql.Rows, error)

	// Execute executes a statement and returns the result
	Execute(ctx context.Context, sql string, args []interface{}) (sql.Result, error)

	// Close closes the connection
	Close() error

	// Dialect returns the SQL dialect name (e.g., "sqlite", "postgres", "mysql")
	Dialect() string
}
