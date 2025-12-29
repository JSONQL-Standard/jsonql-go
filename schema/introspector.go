package schema

import (
	jsonql "github.com/jsonql-standard/jsonql-go"
)

// Introspector is the interface for database schema introspection
type Introspector interface {
	Introspect() (*jsonql.JSONQLSchema, error)
}
