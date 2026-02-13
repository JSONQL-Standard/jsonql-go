package mssql

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"

	"github.com/jsonql-standard/jsonql-go"
	_ "github.com/microsoft/go-mssqldb"
)

// Driver implements the jsonql.Driver interface for Microsoft SQL Server.
type Driver struct {
	db *sql.DB
}

var _ jsonql.Driver = (*Driver)(nil)

// NewDriver creates a new MSSQL driver from a connection string.
// Example DSN: "sqlserver://sa:Password123@localhost:1433?database=jsonql_test"
func NewDriver(dsn string) (*Driver, error) {
	db, err := sql.Open("sqlserver", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open mssql db: %w", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping mssql db: %w", err)
	}
	return &Driver{db: db}, nil
}

func (d *Driver) Query(ctx context.Context, query string, args []interface{}) (*sql.Rows, error) {
	return d.db.QueryContext(ctx, query, args...)
}

var insertRegex = regexp.MustCompile(`(?i)^\s*INSERT\s+INTO\s+\[?(\w+)\]?`)

func (d *Driver) Execute(ctx context.Context, query string, args []interface{}) (sql.Result, error) {
	// For INSERT statements, wrap with IDENTITY_INSERT ON/OFF in same batch
	if matches := insertRegex.FindStringSubmatch(strings.TrimSpace(query)); len(matches) > 1 {
		table := matches[1]
		wrappedQuery := fmt.Sprintf(
			"SET IDENTITY_INSERT [%s] ON; %s; SET IDENTITY_INSERT [%s] OFF",
			table, query, table,
		)
		return d.db.ExecContext(ctx, wrappedQuery, args...)
	}
	return d.db.ExecContext(ctx, query, args...)
}

func (d *Driver) Close() error {
	return d.db.Close()
}

func (d *Driver) Dialect() string {
	return "mssql"
}

func (d *Driver) GetDB() *sql.DB {
	return d.db
}
