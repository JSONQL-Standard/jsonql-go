package builder

import (
	jsonql "github.com/jsonql-standard/jsonql-go"
)

// QueryBuilder provides a fluent API for constructing JSONQL queries
type QueryBuilder struct {
	query *jsonql.JSONQLQuery
}

// New creates a new QueryBuilder instance
func New() *QueryBuilder {
	return &QueryBuilder{
		query: &jsonql.JSONQLQuery{
			Version: "1.0",
		},
	}
}

// From sets the source table
func (b *QueryBuilder) From(table string) *QueryBuilder {
	b.query.From = table
	return b
}

// Select adds fields to the selection
func (b *QueryBuilder) Select(fields ...string) *QueryBuilder {
	b.query.Fields = append(b.query.Fields, fields...)
	return b
}

// Where sets the initial WHERE clause
func (b *QueryBuilder) Where(condition map[string]interface{}) *QueryBuilder {
	b.query.Where = condition
	return b
}

// AndWhere adds an AND condition to the WHERE clause
func (b *QueryBuilder) AndWhere(condition map[string]interface{}) *QueryBuilder {
	if b.query.Where == nil {
		b.query.Where = condition
		return b
	}

	// Check if existing is already an AND
	if ands, ok := b.query.Where["and"].([]interface{}); ok {
		b.query.Where["and"] = append(ands, condition)
	} else {
		// Wrap existing + new in AND
		b.query.Where = map[string]interface{}{
			"and": []interface{}{
				b.query.Where,
				condition,
			},
		}
	}
	return b
}

// OrWhere adds an OR condition to the WHERE clause
func (b *QueryBuilder) OrWhere(condition map[string]interface{}) *QueryBuilder {
	if b.query.Where == nil {
		b.query.Where = condition
		return b
	}

	// Check if existing is already an OR
	if ors, ok := b.query.Where["or"].([]interface{}); ok {
		b.query.Where["or"] = append(ors, condition)
	} else {
		// Wrap existing + new in OR
		b.query.Where = map[string]interface{}{
			"or": []interface{}{
				b.query.Where,
				condition,
			},
		}
	}
	return b
}

// OrderBy adds fields to the sort order
func (b *QueryBuilder) OrderBy(fields ...string) *QueryBuilder {
	b.query.Sort = append(b.query.Sort, fields...)
	return b
}

// Limit sets the maximum number of records to return
func (b *QueryBuilder) Limit(limit int) *QueryBuilder {
	b.query.Limit = &limit
	return b
}

// Offset sets the number of records to skip
func (b *QueryBuilder) Offset(offset int) *QueryBuilder {
	b.query.Offset = &offset
	return b
}

// Build returns the constructed JSONQLQuery
func (b *QueryBuilder) Build() *jsonql.JSONQLQuery {
	return b.query
}
