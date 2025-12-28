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
func (t *Transpiler) Transpile(query *JSONQLQuery, tableName string, schema *JSONQLSchema) (*TranspileResult, error) {
	if !isValidIdentifier(tableName) {
		return nil, fmt.Errorf("Invalid table name: %s", tableName)
	}

	var args []interface{}
	var selectParts []string
	var joinParts []string
	var whereConditions []string

	// 1. SELECT clause (Main Table)
	if len(query.Fields) > 0 {
		for _, f := range query.Fields {
			if !isValidIdentifier(f) {
				return nil, fmt.Errorf("Invalid field name: %s", f)
			}
			selectParts = append(selectParts, fmt.Sprintf("%s.%s", t.quoteIdentifier(tableName), t.quoteIdentifier(f)))
		}
	}

	// Handle Aggregates (Main Table)
	if len(query.Aggregate) > 0 {
		for alias, aggDef := range query.Aggregate {
			if !isValidIdentifier(alias) {
				return nil, fmt.Errorf("Invalid aggregate alias: %s", alias)
			}

			aggMap, ok := aggDef.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("Invalid aggregate definition for %s", alias)
			}

			for funcName, field := range aggMap {
				fieldName, ok := field.(string)
				if !ok {
					return nil, fmt.Errorf("Invalid aggregate field for %s", alias)
				}

				// Validate funcName
				validFuncs := map[string]bool{"sum": true, "count": true, "avg": true, "min": true, "max": true}
				if !validFuncs[funcName] {
					return nil, fmt.Errorf("Invalid aggregate function: %s", funcName)
				}

				if fieldName == "*" && funcName == "count" {
					selectParts = append(selectParts, fmt.Sprintf("COUNT(*) AS %s", t.quoteIdentifier(alias)))
				} else {
					if !isValidIdentifier(fieldName) {
						return nil, fmt.Errorf("Invalid aggregate field: %s", fieldName)
					}
					selectParts = append(selectParts, fmt.Sprintf("%s(%s.%s) AS %s", strings.ToUpper(funcName), t.quoteIdentifier(tableName), t.quoteIdentifier(fieldName), t.quoteIdentifier(alias)))
				}
			}
		}
	}

	// 2. Process Joins (Recursive)
	if len(query.Include) > 0 {
		if schema == nil {
			return nil, fmt.Errorf("Schema is required for relationships")
		}

		// Start recursion with root table info
		err := t.processJoin(query.Include, tableName, tableName, "", schema, &selectParts, &joinParts, &whereConditions, &args)
		if err != nil {
			return nil, err
		}
	}

	selectClause := "*"
	if len(selectParts) > 0 {
		selectClause = strings.Join(selectParts, ", ")
	}

	// 3. FROM clause
	fromClause := t.quoteIdentifier(tableName)
	if len(joinParts) > 0 {
		fromClause += " " + strings.Join(joinParts, " ")
	}

	sqlStr := fmt.Sprintf("SELECT %s FROM %s", selectClause, fromClause)

	// 4. WHERE clause (Main Table)
	if query.Where != nil {
		conds, newArgs, err := t.processWhere(query.Where, tableName)
		if err != nil {
			return nil, err
		}
		whereConditions = append(whereConditions, conds...)
		args = append(args, newArgs...)
	}

	if len(whereConditions) > 0 {
		sqlStr += " WHERE " + strings.Join(whereConditions, " AND ")
	}

	// 5. GROUP BY clause
	if len(query.GroupBy) > 0 {
		var groups []string
		for _, g := range query.GroupBy {
			if !isValidIdentifier(g) {
				return nil, fmt.Errorf("Invalid group by field: %s", g)
			}
			groups = append(groups, fmt.Sprintf("%s.%s", t.quoteIdentifier(tableName), t.quoteIdentifier(g)))
		}
		sqlStr += " GROUP BY " + strings.Join(groups, ", ")
	}

	// 6. SORT clause
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
			sortParts = append(sortParts, fmt.Sprintf("%s.%s %s", t.quoteIdentifier(tableName), t.quoteIdentifier(field), order))
		}
		sqlStr += " ORDER BY " + strings.Join(sortParts, ", ")
	}

	// 7. LIMIT / OFFSET
	if query.Limit != nil && *query.Limit > 0 {
		sqlStr += fmt.Sprintf(" LIMIT %d", *query.Limit)
	}
	if query.Offset != nil && *query.Offset > 0 {
		sqlStr += fmt.Sprintf(" OFFSET %d", *query.Offset)
	}

	return &TranspileResult{
		SQL:  sqlStr,
		Args: args,
	}, nil
}

func (t *Transpiler) processJoin(
	include map[string]interface{},
	parentTable string,
	parentAlias string,
	hydratorPath string, // e.g. "items", "items___product"
	schema *JSONQLSchema,
	selectParts *[]string,
	joinParts *[]string,
	whereConditions *[]string,
	args *[]interface{},
) error {
	tableDef, ok := schema.Tables[parentTable]
	if !ok {
		return fmt.Errorf("Table definition not found for %s", parentTable)
	}

	for relName, relConfig := range include {
		relation, ok := tableDef.Relations[relName]
		if !ok {
			return fmt.Errorf("Relation %s not found on table %s", relName, parentTable)
		}

		targetTable := relName
		if relation.Table != "" {
			targetTable = relation.Table
		}

		// Generate unique alias for this join
		// If parentAlias is root (tableName), alias is relName
		// Else alias is parentAlias_relName
		var currentTableAlias string
		var currentHydratorPath string

		if hydratorPath == "" {
			currentTableAlias = relName
			currentHydratorPath = relName
		} else {
			currentTableAlias = parentAlias + "_" + relName
			currentHydratorPath = hydratorPath + "___" + relName
		}

		relMap, ok := relConfig.(map[string]interface{})
		if !ok {
			return fmt.Errorf("Invalid include configuration for %s", relName)
		}

		// Parse Limit/Skip/Sort for Pagination
		var limit int
		var offset int
		var hasLimit, hasOffset bool

		if l, ok := relMap["limit"].(float64); ok {
			limit = int(l)
			hasLimit = true
		}
		if s, ok := relMap["skip"].(float64); ok {
			offset = int(s)
			hasOffset = true
		}

		var sortParts []string
		if sortVal, ok := relMap["sort"]; ok {
			parseSort := func(s string) string {
				desc := false
				field := s
				if strings.HasPrefix(s, "-") {
					desc = true
					field = s[1:]
				}
				order := "ASC"
				if desc {
					order = "DESC"
				}
				return fmt.Sprintf("%s %s", t.quoteIdentifier(field), order)
			}

			if s, ok := sortVal.(string); ok {
				sortParts = append(sortParts, parseSort(s))
			} else if sArr, ok := sortVal.([]interface{}); ok {
				for _, item := range sArr {
					if s, ok := item.(string); ok {
						sortParts = append(sortParts, parseSort(s))
					}
				}
			}
		}

		// Parse included fields
		if fields, ok := relMap["fields"].([]interface{}); ok {
			for _, f := range fields {
				fieldName, ok := f.(string)
				if ok {
					// Alias: currentHydratorPath___fieldName
					alias := fmt.Sprintf("%s___%s", currentHydratorPath, fieldName)
					*selectParts = append(*selectParts, fmt.Sprintf("%s.%s AS %s", t.quoteIdentifier(currentTableAlias), t.quoteIdentifier(fieldName), t.quoteIdentifier(alias)))
				}
			}
		}

		// Handle Aggregates (Subqueries)
		if aggMap, ok := relMap["aggregate"].(map[string]interface{}); ok {
			for alias, aggDef := range aggMap {
				defMap, ok := aggDef.(map[string]interface{})
				if !ok {
					continue // Skip invalid
				}
				for funcName, field := range defMap {
					fieldName, ok := field.(string)
					if !ok {
						continue
					}

					// Build Subquery
					// SELECT func(field) FROM targetTable WHERE joinCond AND whereCond

					// We need a unique alias for the subquery table to avoid collision with the main join
					subAlias := fmt.Sprintf("%s_agg_%s", currentTableAlias, alias)

					var subSelect string
					if fieldName == "*" && funcName == "count" {
						subSelect = "COUNT(*)"
					} else {
						subSelect = fmt.Sprintf("%s(%s.%s)", strings.ToUpper(funcName), t.quoteIdentifier(subAlias), t.quoteIdentifier(fieldName))
					}

					// Join Condition for Subquery
					var subOnClause string
					if relation.Type == "hasOne" {
						// parent.field = sub.id
						subOnClause = fmt.Sprintf("%s.%s = %s.id", t.quoteIdentifier(parentAlias), t.quoteIdentifier(relation.Field), t.quoteIdentifier(subAlias))
					} else {
						// sub.field = parent.id
						subOnClause = fmt.Sprintf("%s.%s = %s.id", t.quoteIdentifier(subAlias), t.quoteIdentifier(relation.Field), t.quoteIdentifier(parentAlias))
					}

					// Add filters from 'where' in include
					var subWhere []string
					subWhere = append(subWhere, subOnClause)

					if whereMap, ok := relMap["where"].(map[string]interface{}); ok {
						// We need to process where clauses but using subAlias
						conds, newArgs, err := t.processWhere(whereMap, subAlias)
						if err == nil {
							subWhere = append(subWhere, conds...)
							*args = append(*args, newArgs...)
						}
					}

					fullSubQuery := fmt.Sprintf("(SELECT %s FROM %s AS %s WHERE %s)",
						subSelect,
						t.quoteIdentifier(targetTable),
						t.quoteIdentifier(subAlias),
						strings.Join(subWhere, " AND "))

					// Alias: currentHydratorPath___alias
					finalAlias := fmt.Sprintf("%s___%s", currentHydratorPath, alias)
					*selectParts = append(*selectParts, fmt.Sprintf("%s AS %s", fullSubQuery, t.quoteIdentifier(finalAlias)))
				}
			}
		}

		// Construct JOIN ON clause
		var onClause string
		if relation.Type == "hasOne" {
			onClause = fmt.Sprintf("%s.%s = %s.id", t.quoteIdentifier(parentAlias), t.quoteIdentifier(relation.Field), t.quoteIdentifier(currentTableAlias))
		} else if relation.Type == "hasMany" {
			onClause = fmt.Sprintf("%s.%s = %s.id", t.quoteIdentifier(currentTableAlias), t.quoteIdentifier(relation.Field), t.quoteIdentifier(parentAlias))
		} else {
			onClause = fmt.Sprintf("%s.%s = %s.id", t.quoteIdentifier(parentAlias), t.quoteIdentifier(relation.Field), t.quoteIdentifier(currentTableAlias))
		}

		// Handle Nested Where (add to ON clause or WHERE clause?)
		// If we add to WHERE, it filters the parent row if child is missing/filtered.
		// If we add to ON, it filters the child (child becomes NULL) but keeps parent.
		// Standard JSONQL usually implies filtering the result set.
		// However, for "include", usually we want the parent.
		// Let's put it in ON clause for now to support "filter included items".
		if whereMap, ok := relMap["where"].(map[string]interface{}); ok {
			conds, newArgs, err := t.processWhere(whereMap, currentTableAlias)
			if err != nil {
				return err
			}
			if len(conds) > 0 {
				onClause += " AND " + strings.Join(conds, " AND ")
				*args = append(*args, newArgs...)
			}
		}

		// Handle Pagination (Window Functions)
		targetTableSQL := t.quoteIdentifier(targetTable)
		if hasLimit || hasOffset {
			// Add filters to ON clause
			if hasLimit {
				onClause += fmt.Sprintf(" AND %s.rn <= %d", t.quoteIdentifier(currentTableAlias), limit+offset)
			}
			if hasOffset {
				onClause += fmt.Sprintf(" AND %s.rn > %d", t.quoteIdentifier(currentTableAlias), offset)
			}

			// Build Subquery with Window Function
			orderBy := "id ASC" // Default sort
			if len(sortParts) > 0 {
				orderBy = strings.Join(sortParts, ", ")
			}

			partitionKey := "id"
			if relation.Type == "hasMany" || relation.Type == "hasOne" {
				partitionKey = relation.Field
			}

			targetTableSQL = fmt.Sprintf("(SELECT *, ROW_NUMBER() OVER (PARTITION BY %s ORDER BY %s) as rn FROM %s)",
				t.quoteIdentifier(partitionKey),
				orderBy,
				t.quoteIdentifier(targetTable))
		}

		*joinParts = append(*joinParts, fmt.Sprintf("LEFT JOIN %s AS %s ON %s", targetTableSQL, t.quoteIdentifier(currentTableAlias), onClause))

		// Recursive call for nested includes
		if nestedInclude, ok := relMap["include"].(map[string]interface{}); ok {
			err := t.processJoin(nestedInclude, targetTable, currentTableAlias, currentHydratorPath, schema, selectParts, joinParts, whereConditions, args)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (t *Transpiler) processWhere(where map[string]interface{}, tableAlias string) ([]string, []interface{}, error) {
	var conditions []string
	var args []interface{}

	for field, cond := range where {
		if field == "or" {
			if orList, ok := cond.([]interface{}); ok {
				var orConditions []string
				for _, item := range orList {
					if itemMap, ok := item.(map[string]interface{}); ok {
						for k, v := range itemMap {
							if !isValidIdentifier(k) {
								return nil, nil, fmt.Errorf("Invalid field name in OR clause: %s", k)
							}
							orConditions = append(orConditions, fmt.Sprintf("%s.%s = ?", t.quoteIdentifier(tableAlias), t.quoteIdentifier(k)))
							args = append(args, v)
						}
					}
				}
				if len(orConditions) > 0 {
					conditions = append(conditions, "("+strings.Join(orConditions, " OR ")+")")
				}
			}
			continue
		}

		if !isValidIdentifier(field) {
			return nil, nil, fmt.Errorf("Invalid field name in where clause: %s", field)
		}

		if valMap, ok := cond.(map[string]interface{}); ok {
			if v, ok := valMap["eq"]; ok {
				conditions = append(conditions, fmt.Sprintf("%s.%s = ?", t.quoteIdentifier(tableAlias), t.quoteIdentifier(field)))
				args = append(args, v)
			}
			if v, ok := valMap["neq"]; ok {
				conditions = append(conditions, fmt.Sprintf("%s.%s != ?", t.quoteIdentifier(tableAlias), t.quoteIdentifier(field)))
				args = append(args, v)
			}
			if v, ok := valMap["gt"]; ok {
				conditions = append(conditions, fmt.Sprintf("%s.%s > ?", t.quoteIdentifier(tableAlias), t.quoteIdentifier(field)))
				args = append(args, v)
			}
			if v, ok := valMap["gte"]; ok {
				conditions = append(conditions, fmt.Sprintf("%s.%s >= ?", t.quoteIdentifier(tableAlias), t.quoteIdentifier(field)))
				args = append(args, v)
			}
			if v, ok := valMap["lt"]; ok {
				conditions = append(conditions, fmt.Sprintf("%s.%s < ?", t.quoteIdentifier(tableAlias), t.quoteIdentifier(field)))
				args = append(args, v)
			}
			if v, ok := valMap["lte"]; ok {
				conditions = append(conditions, fmt.Sprintf("%s.%s <= ?", t.quoteIdentifier(tableAlias), t.quoteIdentifier(field)))
				args = append(args, v)
			}
			if v, ok := valMap["like"]; ok {
				conditions = append(conditions, fmt.Sprintf("%s.%s LIKE ?", t.quoteIdentifier(tableAlias), t.quoteIdentifier(field)))
				args = append(args, v)
			}
			if v, ok := valMap["in"]; ok {
				if slice, ok := v.([]interface{}); ok && len(slice) > 0 {
					placeholders := make([]string, len(slice))
					for i := range slice {
						placeholders[i] = "?"
						args = append(args, slice[i])
					}
					conditions = append(conditions, fmt.Sprintf("%s.%s IN (%s)", t.quoteIdentifier(tableAlias), t.quoteIdentifier(field), strings.Join(placeholders, ", ")))
				}
			}
		} else {
			conditions = append(conditions, fmt.Sprintf("%s.%s = ?", t.quoteIdentifier(tableAlias), t.quoteIdentifier(field)))
			args = append(args, cond)
		}
	}
	return conditions, args, nil
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
