package jsonql

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
)

// genericDriver implements Driver using database/sql directly.
type genericDriver struct {
	db      *sql.DB
	dialect string
}

func (d *genericDriver) Query(ctx context.Context, q string, args []interface{}) (*sql.Rows, error) {
	return d.db.QueryContext(ctx, q, args...)
}

func (d *genericDriver) Execute(ctx context.Context, q string, args []interface{}) (sql.Result, error) {
	return d.db.ExecContext(ctx, q, args...)
}

func (d *genericDriver) Close() error { return d.db.Close() }

func (d *genericDriver) Dialect() string { return d.dialect }

func createGenericDriver(sqlDriverName, dialect, dsn string) (Driver, error) {
	db, err := sql.Open(sqlDriverName, dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open %s db: %w", dialect, err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to connect to %s db: %w", dialect, err)
	}
	return &genericDriver{db: db, dialect: dialect}, nil
}

// CreateDriver creates a Driver for the given dialect using standard environment
// variables for connection configuration.
//
// Supported dialects: "postgres", "mysql", "sqlite", "mssql"
//
// Environment variables read per dialect:
//   - postgres: DB_DSN (default: "postgresql://jsonql:password@localhost:5432/jsonql_test?sslmode=disable")
//   - mysql:    DB_DSN (default: "jsonql:password@tcp(localhost:3306)/jsonql_test")
//   - sqlite:   DB_FILENAME (default: ":memory:")
//   - mssql:    DB_DSN (default: "sqlserver://sa:Password@localhost:1433?database=jsonql_test")
//
// Important: You must import the appropriate driver package for side-effects:
//
//	import _ "github.com/jsonql-standard/jsonql-go/drivers/postgres" // registers "postgres"
//	import _ "github.com/jsonql-standard/jsonql-go/drivers/mysql"    // registers "mysql"
//	import _ "github.com/jsonql-standard/jsonql-go/drivers/sqlite"   // registers "sqlite"
//	import _ "github.com/jsonql-standard/jsonql-go/drivers/mssql"    // registers "sqlserver"
//
// Usage:
//
//	driver, err := jsonql.CreateDriver("postgres")
//	defer driver.Close()
func CreateDriver(dialect string) (Driver, error) {
	switch dialect {
	case "postgres":
		dsn := envOr("DB_DSN", "postgresql://jsonql:password@localhost:5432/jsonql_test?sslmode=disable")
		return createGenericDriver("postgres", "postgres", dsn)
	case "mysql":
		dsn := envOr("DB_DSN", "jsonql:password@tcp(localhost:3306)/jsonql_test")
		return createGenericDriver("mysql", "mysql", dsn)
	case "sqlite":
		filename := envOr("DB_FILENAME", ":memory:")
		return createGenericDriver("sqlite", "sqlite", filename)
	case "mssql":
		dsn := envOr("DB_DSN", "sqlserver://sa:Password@localhost:1433?database=jsonql_test")
		return createGenericDriver("sqlserver", "mssql", dsn)
	default:
		return nil, fmt.Errorf("unsupported dialect: %s", dialect)
	}
}

// CreateDriverWithDSN creates a Driver for the given dialect with an explicit DSN.
//
// Usage:
//
//	driver, err := jsonql.CreateDriverWithDSN("postgres", "postgresql://user:pass@host:5432/db")
func CreateDriverWithDSN(dialect, dsn string) (Driver, error) {
	switch dialect {
	case "postgres":
		return createGenericDriver("postgres", "postgres", dsn)
	case "mysql":
		return createGenericDriver("mysql", "mysql", dsn)
	case "sqlite":
		return createGenericDriver("sqlite", "sqlite", dsn)
	case "mssql":
		return createGenericDriver("sqlserver", "mssql", dsn)
	default:
		return nil, fmt.Errorf("unsupported dialect: %s", dialect)
	}
}

// LoadSchema reads and parses a JSONQL schema from a JSON file.
//
// Usage:
//
//	schema, err := jsonql.LoadSchema("path/to/schema.json")
func LoadSchema(path string) (*JSONQLSchema, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read schema file %s: %w", path, err)
	}
	schema := &JSONQLSchema{}
	if err := json.Unmarshal(data, schema); err != nil {
		return nil, fmt.Errorf("failed to parse schema JSON: %w", err)
	}
	return schema, nil
}

// MustLoadSchema is like LoadSchema but panics on error.
// Intended for use during startup when a missing schema is fatal.
//
// Usage:
//
//	schema := jsonql.MustLoadSchema(os.Getenv("JSONQL_SCHEMA_PATH"))
func MustLoadSchema(path string) *JSONQLSchema {
	schema, err := LoadSchema(path)
	if err != nil {
		panic(err)
	}
	return schema
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
