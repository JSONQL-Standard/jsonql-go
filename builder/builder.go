package builder

import (
	"fmt"

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

// GroupBy sets the group by fields
func (b *QueryBuilder) GroupBy(fields ...string) *QueryBuilder {
	b.query.GroupBy = append(b.query.GroupBy, fields...)
	return b
}

// Aggregate adds aggregate definitions
func (b *QueryBuilder) Aggregate(aggregate map[string]interface{}) *QueryBuilder {
	b.query.Aggregate = aggregate
	return b
}

// Include adds relation includes
func (b *QueryBuilder) Include(include map[string]interface{}) *QueryBuilder {
	b.query.Include = include
	return b
}

// Distinct enables SELECT DISTINCT. Pass true for all fields.
func (b *QueryBuilder) Distinct(all bool) *QueryBuilder {
	b.query.Distinct = &jsonql.DistinctOption{All: all}
	return b
}

// DistinctFields enables SELECT DISTINCT on specific fields.
func (b *QueryBuilder) DistinctFields(fields ...string) *QueryBuilder {
	b.query.Distinct = &jsonql.DistinctOption{Fields: fields}
	return b
}

// Build returns the constructed JSONQLQuery
func (b *QueryBuilder) Build() *jsonql.JSONQLQuery {
	return b.query
}

// Reset resets the builder to a clean state
func (b *QueryBuilder) Reset() *QueryBuilder {
	b.query = &jsonql.JSONQLQuery{Version: "1.0"}
	return b
}

// ---------- Mutation Builder ----------

// MutationBuilder provides a fluent API for constructing JSONQL mutations
type MutationBuilder struct {
	mutation *jsonql.JSONQLMutation
}

// NewMutation creates a new MutationBuilder instance
func NewMutation() *MutationBuilder {
	return &MutationBuilder{}
}

// Create initializes a create (INSERT) mutation
func (b *MutationBuilder) Create(from string, data map[string]interface{}) *MutationBuilder {
	b.mutation = &jsonql.JSONQLMutation{
		Op:   "create",
		Data: data,
	}
	return b
}

// Update initializes an update mutation
func (b *MutationBuilder) Update(from string, patch map[string]interface{}) *MutationBuilder {
	b.mutation = &jsonql.JSONQLMutation{
		Op:    "update",
		Patch: patch,
	}
	return b
}

// Delete initializes a delete mutation
func (b *MutationBuilder) Delete(from string) *MutationBuilder {
	b.mutation = &jsonql.JSONQLMutation{
		Op: "delete",
	}
	return b
}

// Where sets the where clause for the mutation
func (b *MutationBuilder) Where(where map[string]interface{}) *MutationBuilder {
	if b.mutation == nil {
		return b
	}
	b.mutation.Where = where
	return b
}

// Build returns the constructed JSONQLMutation
func (b *MutationBuilder) Build() (*jsonql.JSONQLMutation, error) {
	if b.mutation == nil {
		return nil, fmt.Errorf("mutation not initialized: call Create, Update, or Delete first")
	}
	return b.mutation, nil
}

// Reset resets the builder to a clean state
func (b *MutationBuilder) Reset() *MutationBuilder {
	b.mutation = nil
	return b
}

// ---------- Condition Helpers ----------
// These functions create WHERE condition maps compatible with JSONQL query syntax.

// Eq creates an equality condition: {"eq": value}
func Eq(value interface{}) map[string]interface{} {
	return map[string]interface{}{"eq": value}
}

// Neq creates a not-equal condition: {"neq": value}
func Neq(value interface{}) map[string]interface{} {
	return map[string]interface{}{"neq": value}
}

// Gt creates a greater-than condition: {"gt": value}
func Gt(value interface{}) map[string]interface{} {
	return map[string]interface{}{"gt": value}
}

// Gte creates a greater-than-or-equal condition: {"gte": value}
func Gte(value interface{}) map[string]interface{} {
	return map[string]interface{}{"gte": value}
}

// Lt creates a less-than condition: {"lt": value}
func Lt(value interface{}) map[string]interface{} {
	return map[string]interface{}{"lt": value}
}

// Lte creates a less-than-or-equal condition: {"lte": value}
func Lte(value interface{}) map[string]interface{} {
	return map[string]interface{}{"lte": value}
}

// In creates an IN condition: {"in": values}
func In(values ...interface{}) map[string]interface{} {
	return map[string]interface{}{"in": values}
}

// Nin creates a NOT IN condition: {"nin": values}
func Nin(values ...interface{}) map[string]interface{} {
	return map[string]interface{}{"nin": values}
}

// Like creates a LIKE condition: {"like": pattern}
func Like(pattern string) map[string]interface{} {
	return map[string]interface{}{"like": pattern}
}

// Contains creates a contains condition: {"like": "%value%"}
func Contains(value string) map[string]interface{} {
	return map[string]interface{}{"like": "%" + value + "%"}
}

// StartsWith creates a starts-with condition: {"like": "value%"}
func StartsWith(value string) map[string]interface{} {
	return map[string]interface{}{"like": value + "%"}
}

// EndsWith creates an ends-with condition: {"like": "%value"}
func EndsWith(value string) map[string]interface{} {
	return map[string]interface{}{"like": "%" + value}
}

// Field creates a field condition: {fieldName: condition}
func Field(fieldName string, condition map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{fieldName: condition}
}

// And creates an AND logical condition
func And(conditions ...map[string]interface{}) map[string]interface{} {
	items := make([]interface{}, len(conditions))
	for i, c := range conditions {
		items[i] = c
	}
	return map[string]interface{}{"and": items}
}

// Or creates an OR logical condition
func Or(conditions ...map[string]interface{}) map[string]interface{} {
	items := make([]interface{}, len(conditions))
	for i, c := range conditions {
		items[i] = c
	}
	return map[string]interface{}{"or": items}
}

// Not creates a NOT logical condition
func Not(condition map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{"not": condition}
}

// FieldRef creates a field reference for field-to-field comparisons: {"field": "columnName"}
func FieldRef(fieldName string) map[string]interface{} {
	return map[string]interface{}{"field": fieldName}
}
