package jsonql

import "fmt"

type SQLDialect interface {
	Name() string
	Placeholder(index int) string
	QuoteIdentifier(id string) string
}

type PostgresDialect struct{}
func (PostgresDialect) Name() string { return "postgres" }
func (PostgresDialect) Placeholder(i int) string { return fmt.Sprintf("$%d", i+1) }
func (PostgresDialect) QuoteIdentifier(id string) string { return fmt.Sprintf("\"%s\"", id) }

type SQLiteDialect struct{}
func (SQLiteDialect) Name() string { return "sqlite" }
func (SQLiteDialect) Placeholder(int) string { return "?" }
func (SQLiteDialect) QuoteIdentifier(id string) string { return id }

func NewSQLDialect(name string) SQLDialect {
	switch name {
	case "postgres": return PostgresDialect{}
	default: return SQLiteDialect{}
	}
}