package jsonql

import (
	"fmt"
	"strings"
)

// Transpiler converts JSONQL queries to SQL
type Transpiler struct {
	Dialect string
}

// NewTranspiler creates a new SQL transpiler
func NewTranspiler(dialect string) *Transpiler {
	if dialect == "" {
		dialect = "sqlite"
	}
	return &Transpiler{Dialect: dialect}
}

// TranspileResult holds the generated SQL and arguments
type TranspileResult struct {
	SQL  string
	Args []interface{}
}

// Transpile converts a parsed query object to a SQL string and arguments
func (t *Transpiler) Transpile(query *JSONQLQuery, tableName string) (*TranspileResult, error) {
	if !isValidIdentifier(tableName) {
		return nil, fmt.Errorf("Invalid table name: %s", tableName)
	}

	var args []interface{}

	// 1. SELECT clause
	selectClause := "*"
	if len(query.Fields) > 0 {
		var cols []string
		for _, f := range query.Fields {
			if !isValidIdentifier(f) {
				return nil, fmt.Errorf("Invalid field name: %s", f)
			}
			cols = append(cols, t.quoteIdentifier(f))
		}
		selectClause = strings.Join(cols, ", ")
	}

	// 2. FROM clause
	sqlStr := fmt.Sprintf("SELECT %s FROM %s", selectClause, t.quoteIdentifier(tableName))

	// 3. WHERE clause
	if query.Where != nil {
		var conditions []string
		for field, cond := range query.Where {
			if !isValidIdentifier(field) {
				return nil, fmt.Errorf("Invalid field name in where clause: %s", field)
			}

			// Handle simple equality: "field": "value"
			if val, ok := cond.(map[string]interface{}); !ok {
				conditions = append(conditions, fmt.Sprintf("%s = ?", t.quoteIdentifier(field)))
				args = append(args, cond)
			} else {
				// Handle operators: "field": { "gt": 10 }
				if val, ok := cond.(map[string]interface{}); ok {
					if v, ok := val["eq"]; ok {
						conditions = append(conditions, fmt.Sprintf("%s = ?", t.quoteIdentifier(field)))
						args = append(args, v)
					}
					if v, ok := val["neq"]; ok {
						conditions = append(conditions, fmt.Sprintf("%s != ?", t.quoteIdentifier(field)))
						args = append(args, v)
					}
					if v, ok := val["gt"]; ok {
						conditions = append(conditions, fmt.Sprintf("%s > ?", t.quoteIdentifier(field)))
						args = append(args, v)
					}
					if v, ok := val["gte"]; ok {
						conditions = append(conditions, fmt.Sprintf("%s >= ?", t.quoteIdentifier(field)))
						args = append(args, v)
					}
					if v, ok := val["lt"]; ok {
						conditions = append(conditions, fmt.Sprintf("%s < ?", t.quoteIdentifier(field)))
						args = append(args, v)
					}
					if v, ok := val["lte"]; ok {
						conditions = append(conditions, fmt.Sprintf("%s <= ?", t.quoteIdentifier(field)))
						args = append(args, v)
					}
					if v, ok := val["like"]; ok {
						conditions = append(conditions, fmt.Sprintf("%s LIKE ?", t.quoteIdentifier(field)))
						args = append(args, v)
					}
					if v, ok := val["in"]; ok {
						if slice, ok := v.([]interface{}); ok && len(slice) > 0 {
							placeholders := make([]string, len(slice))
							for i := range slice {
								placeholders[i] = "?"
								args = append(args, slice[i])
							}
							conditions = append(conditions, fmt.Sprintf("%s IN (%s)", t.quoteIdentifier(field), strings.Join(placeholders, ", ")))
						}
					}
				}
			}
		}
		if len(conditions) > 0 {
			sqlStr += " WHERE " + strings.Join(conditions, " AND ")
		}
	}

	// 4. SORT clause
	if len(query.Sort) > 0 {
		var sortParts []string
		for _, s := range query.Sort {
			desc := false
			field := s
			if strings.HasPrefix(s, "-") {
				desc = true
				field = s[1:]
			}
			if !isValidIdentifier(field) {
				return nil, fmt.Errorf("Invalid sort field: %s", field)
			}
			
			order := "ASC"
			if desc {
				order = "DESC"
			}
			sortParts = append(sortParts, fmt.Sprintf("%s %s", t.quoteIdentifier(field), order))
		}
		sqlStr += " ORDER BY " + strings.Join(sortParts, ", ")
	}

	// 5. LIMIT / OFFSET
	if query.Limit != nil {
		sqlStr += fmt.Sprintf(" LIMIT %d", *query.Limit)
	}
	if query.Offset != nil {
		sqlStr += fmt.Sprintf(" OFFSET %d", *query.Offset)
	}

	return &TranspileResult{
		SQL:  sqlStr,
		Args: args,
	}, nil
}

func (t *Transpiler) quoteIdentifier(name string) string {
	// Basic quoting, can be improved based on dialect
	return fmt.Sprintf("\"%s\"", name)
}

