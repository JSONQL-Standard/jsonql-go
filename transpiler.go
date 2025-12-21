package jsonql

import (
	"fmt"
	"strings"
)

// Transpiler converts JSONQL queries to SQL
type Transpiler struct {
	Dialect SQLDialect
}

// NewTranspiler creates a new SQL transpiler
func NewTranspiler(name string) *Transpiler {
	return &Transpiler{Dialect: NewSQLDialect(name)}
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

			// Check if it's a map (operators) or value (equality)
			if valMap, ok := cond.(map[string]interface{}); ok {
				// Handle operators: "field": { "gt": 10 }
				if v, ok := valMap["eq"]; ok {
					conditions = append(conditions, fmt.Sprintf("%s = ?", t.quoteIdentifier(field)))
					args = append(args, v)
				}
				if v, ok := valMap["neq"]; ok {
					conditions = append(conditions, fmt.Sprintf("%s != ?", t.quoteIdentifier(field)))
					args = append(args, v)
				}
				if v, ok := valMap["gt"]; ok {
					conditions = append(conditions, fmt.Sprintf("%s > ?", t.quoteIdentifier(field)))
					args = append(args, v)
				}
				if v, ok := valMap["gte"]; ok {
					conditions = append(conditions, fmt.Sprintf("%s >= ?", t.quoteIdentifier(field)))
					args = append(args, v)
				}
				if v, ok := valMap["lt"]; ok {
					conditions = append(conditions, fmt.Sprintf("%s < ?", t.quoteIdentifier(field)))
					args = append(args, v)
				}
				if v, ok := valMap["lte"]; ok {
					conditions = append(conditions, fmt.Sprintf("%s <= ?", t.quoteIdentifier(field)))
					args = append(args, v)
				}
				if v, ok := valMap["like"]; ok {
					conditions = append(conditions, fmt.Sprintf("%s LIKE ?", t.quoteIdentifier(field)))
					args = append(args, v)
				}
				if v, ok := valMap["in"]; ok {
					if slice, ok := v.([]interface{}); ok && len(slice) > 0 {
						placeholders := make([]string, len(slice))
						for i := range slice {
							placeholders[i] = "?"
							args = append(args, slice[i])
						}
						conditions = append(conditions, fmt.Sprintf("%s IN (%s)", t.quoteIdentifier(field), strings.Join(placeholders, ", ")))
					}
				}
			} else {
				// Handle simple equality: "field": "value"
				conditions = append(conditions, fmt.Sprintf("%s = ?", t.quoteIdentifier(field)))
				args = append(args, cond)
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

	// Post-process for dialect-specific placeholders
	if t.Dialect.Name() == "postgres" {
		sqlStr = t.replacePlaceholders(sqlStr)
	}

	return &TranspileResult{
		SQL:  sqlStr,
		Args: args,
	}, nil
}

func (t *Transpiler) replacePlaceholders(sql string) string {
	count := 0
	var sb strings.Builder
	for {
		i := strings.Index(sql, "?")
		if i == -1 {
			sb.WriteString(sql)
			break
		}
		count++
		sb.WriteString(sql[:i])
		sb.WriteString(fmt.Sprintf("$%d", count))
		sql = sql[i+1:]
	}
	return sb.String()
}

func (t *Transpiler) placeholder(index int) string {
	return t.Dialect.Placeholder(index)
}

func (t *Transpiler) quoteIdentifier(name string) string {
	return t.Dialect.QuoteIdentifier(name)
}

