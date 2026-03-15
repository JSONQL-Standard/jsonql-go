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

// MongoDriver is the interface for MongoDB operations.
// Unlike the SQL Driver, it operates on MongoResult descriptors
// rather than raw SQL strings.
type MongoDriver interface {
	// Execute dispatches a MongoResult to the appropriate MongoDB operation
	// (find, aggregate, insertOne, updateMany, deleteMany) and returns
	// the result documents.
	Execute(ctx context.Context, result *MongoResult) (interface{}, error)

	// Close disconnects from the database.
	Close() error
}
