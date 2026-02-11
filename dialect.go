package jsonql

import "fmt"

// SQLDialect defines how SQL is generated for a specific database engine
type SQLDialect interface {
	Name() string
	Placeholder(index int) string
	QuoteIdentifier(id string) string
	// SupportsReturning indicates if the dialect supports RETURNING clause for mutations
	SupportsReturning() bool
}

// PostgresDialect generates SQL for PostgreSQL
type PostgresDialect struct{}

func (PostgresDialect) Name() string                     { return "postgres" }
func (PostgresDialect) Placeholder(i int) string         { return fmt.Sprintf("$%d", i+1) }
func (PostgresDialect) QuoteIdentifier(id string) string { return fmt.Sprintf("\"%s\"", id) }
func (PostgresDialect) SupportsReturning() bool          { return true }

// SQLiteDialect generates SQL for SQLite
type SQLiteDialect struct{}

func (SQLiteDialect) Name() string                     { return "sqlite" }
func (SQLiteDialect) Placeholder(int) string           { return "?" }
func (SQLiteDialect) QuoteIdentifier(id string) string { return fmt.Sprintf("\"%s\"", id) }
func (SQLiteDialect) SupportsReturning() bool          { return false }

// MySQLDialect generates SQL for MySQL/MariaDB
type MySQLDialect struct{}

func (MySQLDialect) Name() string                     { return "mysql" }
func (MySQLDialect) Placeholder(int) string           { return "?" }
func (MySQLDialect) QuoteIdentifier(id string) string { return fmt.Sprintf("`%s`", id) }
func (MySQLDialect) SupportsReturning() bool          { return false }

// NewSQLDialect creates a dialect from a name string
func NewSQLDialect(name string) SQLDialect {
	switch name {
	case "postgres":
		return PostgresDialect{}
	case "mysql":
		return MySQLDialect{}
	default:
		return SQLiteDialect{}
	}
}
