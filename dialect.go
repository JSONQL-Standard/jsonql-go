package jsonql

import (
	"fmt"
	"strings"
)

// SQLDialect defines how SQL is generated for a specific database engine
type SQLDialect interface {
	Name() string
	Placeholder(index int) string
	QuoteIdentifier(id string) string
	// SupportsReturning indicates if the dialect supports RETURNING clause for mutations
	SupportsReturning() bool
	// GetLimitOffset returns the LIMIT/OFFSET clause for the dialect.
	// For MSSQL this returns OFFSET...FETCH NEXT syntax.
	GetLimitOffset(limit, offset int) string
}

// PostgresDialect generates SQL for PostgreSQL
type PostgresDialect struct{}

func (PostgresDialect) Name() string                     { return "postgres" }
func (PostgresDialect) Placeholder(i int) string         { return fmt.Sprintf("$%d", i+1) }
func (PostgresDialect) QuoteIdentifier(id string) string { return fmt.Sprintf("\"%s\"", id) }
func (PostgresDialect) SupportsReturning() bool          { return true }
func (PostgresDialect) GetLimitOffset(limit, offset int) string {
	if limit == 0 && offset == 0 {
		return "LIMIT 0"
	}
	var parts []string
	if limit > 0 {
		parts = append(parts, fmt.Sprintf("LIMIT %d", limit))
	}
	if offset > 0 {
		parts = append(parts, fmt.Sprintf("OFFSET %d", offset))
	}
	return strings.Join(parts, " ")
}

// SQLiteDialect generates SQL for SQLite
type SQLiteDialect struct{}

func (SQLiteDialect) Name() string                     { return "sqlite" }
func (SQLiteDialect) Placeholder(int) string           { return "?" }
func (SQLiteDialect) QuoteIdentifier(id string) string { return fmt.Sprintf("\"%s\"", id) }
func (SQLiteDialect) SupportsReturning() bool          { return false }
func (SQLiteDialect) GetLimitOffset(limit, offset int) string {
	if limit == 0 && offset == 0 {
		return "LIMIT 0"
	}
	var parts []string
	if limit > 0 {
		parts = append(parts, fmt.Sprintf("LIMIT %d", limit))
	} else if offset > 0 {
		// SQLite requires LIMIT before OFFSET; use -1 for unlimited
		parts = append(parts, "LIMIT -1")
	}
	if offset > 0 {
		parts = append(parts, fmt.Sprintf("OFFSET %d", offset))
	}
	return strings.Join(parts, " ")
}

// MySQLDialect generates SQL for MySQL/MariaDB
type MySQLDialect struct{}

func (MySQLDialect) Name() string                     { return "mysql" }
func (MySQLDialect) Placeholder(int) string           { return "?" }
func (MySQLDialect) QuoteIdentifier(id string) string { return fmt.Sprintf("`%s`", id) }
func (MySQLDialect) SupportsReturning() bool          { return false }
func (MySQLDialect) GetLimitOffset(limit, offset int) string {
	if limit == 0 && offset == 0 {
		return "LIMIT 0"
	}
	var parts []string
	if limit > 0 {
		parts = append(parts, fmt.Sprintf("LIMIT %d", limit))
	} else if offset > 0 {
		// MySQL requires LIMIT before OFFSET; use large number for unlimited
		parts = append(parts, "LIMIT 18446744073709551615")
	}
	if offset > 0 {
		parts = append(parts, fmt.Sprintf("OFFSET %d", offset))
	}
	return strings.Join(parts, " ")
}

// MSSQLDialect generates SQL for Microsoft SQL Server
type MSSQLDialect struct{}

func (MSSQLDialect) Name() string                     { return "mssql" }
func (MSSQLDialect) Placeholder(i int) string         { return fmt.Sprintf("@p%d", i+1) }
func (MSSQLDialect) QuoteIdentifier(id string) string { return fmt.Sprintf("[%s]", id) }
func (MSSQLDialect) SupportsReturning() bool          { return false }
func (MSSQLDialect) GetLimitOffset(limit, offset int) string {
	if limit == 0 && offset == 0 {
		return "OFFSET 0 ROWS FETCH NEXT 0 ROWS ONLY"
	}
	if limit > 0 {
		off := offset
		if off < 0 {
			off = 0
		}
		return fmt.Sprintf("OFFSET %d ROWS FETCH NEXT %d ROWS ONLY", off, limit)
	}
	if offset > 0 {
		return fmt.Sprintf("OFFSET %d ROWS", offset)
	}
	return ""
}

// NewSQLDialect creates a dialect from a name string
func NewSQLDialect(name string) SQLDialect {
	switch name {
	case "postgres":
		return PostgresDialect{}
	case "mysql":
		return MySQLDialect{}
	case "mssql":
		return MSSQLDialect{}
	default:
		return SQLiteDialect{}
	}
}
