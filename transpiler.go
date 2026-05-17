package jsonql

import (
	"fmt"
	"sort"
	"strings"
)

// Transpiler converts JSONQL queries to SQL
type Transpiler struct {
	Dialect SQLDialect
	Logger  Logger
}

// NewTranspiler creates a new SQL transpiler
func NewTranspiler(name string) *Transpiler {
	return &Transpiler{Dialect: NewSQLDialect(name), Logger: NoOpLogger{}}
}

// NewTranspilerWithLogger creates a new SQL transpiler with a logger
func NewTranspilerWithLogger(name string, logger Logger) *Transpiler {
	if logger == nil {
		logger = NoOpLogger{}
	}
	return &Transpiler{Dialect: NewSQLDialect(name), Logger: logger}
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
			if f == "*" {
				selectParts = append(selectParts, fmt.Sprintf("%s.*", t.quoteIdentifier(tableName)))
				continue
			}
			if !isValidIdentifier(f) {
				return nil, fmt.Errorf("Invalid field name: %s", f)
			}
			selectParts = append(selectParts, fmt.Sprintf("%s.%s", t.quoteIdentifier(tableName), t.quoteIdentifier(f)))
		}
	} else if len(query.GroupBy) > 0 {
		// If fields are empty but GroupBy is present, automatically select the group keys
		for _, g := range query.GroupBy {
			if !isValidIdentifier(g) {
				return nil, fmt.Errorf("Invalid group by field: %s", g)
			}
			selectParts = append(selectParts, fmt.Sprintf("%s.%s", t.quoteIdentifier(tableName), t.quoteIdentifier(g)))
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
		err := t.processJoin(map[string]interface{}(query.Include), tableName, tableName, "", schema, &selectParts, &joinParts, &whereConditions, &args)
		if err != nil {
			return nil, err
		}
	}

	selectClause := "*"
	if len(selectParts) > 0 {
		selectClause = strings.Join(selectParts, ", ")
	}

	// Handle DISTINCT
	distinctKeyword := ""
	if query.Distinct != nil {
		if query.Distinct.All {
			distinctKeyword = "DISTINCT "
		} else if len(query.Distinct.Fields) > 0 {
			distinctKeyword = "DISTINCT "
			// Override SELECT clause when distinct is a field array and no explicit fields set
			if selectClause == "*" {
				var parts []string
				for _, f := range query.Distinct.Fields {
					if !isValidIdentifier(f) {
						return nil, fmt.Errorf("Invalid distinct field: %s", f)
					}
					parts = append(parts, fmt.Sprintf("%s.%s", t.quoteIdentifier(tableName), t.quoteIdentifier(f)))
				}
				selectClause = strings.Join(parts, ", ")
			}
		}
	}

	// 3. FROM clause
	fromClause := t.quoteIdentifier(tableName)
	if len(joinParts) > 0 {
		fromClause += " " + strings.Join(joinParts, " ")
	}

	sqlStr := fmt.Sprintf("SELECT %s%s FROM %s", distinctKeyword, selectClause, fromClause)

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
	{
		limit := 0
		offset := 0
		hasLimit := query.Limit != nil
		hasOffset := query.Offset != nil
		if query.Limit != nil && *query.Limit >= 0 {
			limit = *query.Limit
		}
		if query.Offset != nil && *query.Offset > 0 {
			offset = *query.Offset
		}
		if hasLimit || hasOffset {
			// MSSQL requires ORDER BY for OFFSET/FETCH; add default if missing
			if t.Dialect.Name() == "mssql" && len(query.Sort) == 0 {
				sqlStr += " ORDER BY (SELECT NULL)"
			}
			if clause := t.Dialect.GetLimitOffset(limit, offset); clause != "" {
				sqlStr += " " + clause
			}
		}
	}

	result := &TranspileResult{
		SQL:  t.replacePlaceholders(sqlStr),
		Args: args,
	}
	t.Logger.Debug("[JSONQL] SQL: %s", result.SQL)
	t.Logger.Debug("[JSONQL] Args: %v", result.Args)
	return result, nil
}

func (t *Transpiler) processJoin(
	include map[string]interface{},
	parentTable string,
	parentAlias string,
	hydratorPath string, // e.g. "items", "items__product"
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
			currentHydratorPath = hydratorPath + "__" + relName
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
					// Alias: currentHydratorPath__fieldName
					alias := fmt.Sprintf("%s__%s", currentHydratorPath, fieldName)
					*selectParts = append(*selectParts, fmt.Sprintf("%s.%s AS %s", t.quoteIdentifier(currentTableAlias), t.quoteIdentifier(fieldName), t.quoteIdentifier(alias)))
				}
			}
		} else {
			// No explicit fields — select all columns from the target table schema
			targetDef, found := schema.Tables[targetTable]
			if found && len(targetDef.Fields) > 0 {
				// Sort field names for deterministic output
				fieldNames := make([]string, 0, len(targetDef.Fields))
				for fname := range targetDef.Fields {
					fieldNames = append(fieldNames, fname)
				}
				sort.Strings(fieldNames)
				for _, fname := range fieldNames {
					alias := fmt.Sprintf("%s__%s", currentHydratorPath, fname)
					*selectParts = append(*selectParts, fmt.Sprintf("%s.%s AS %s", t.quoteIdentifier(currentTableAlias), t.quoteIdentifier(fname), t.quoteIdentifier(alias)))
				}
			} else {
				*selectParts = append(*selectParts, fmt.Sprintf("%s.*", t.quoteIdentifier(currentTableAlias)))
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

					// Alias: currentHydratorPath__alias
					finalAlias := fmt.Sprintf("%s__%s", currentHydratorPath, alias)
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

// comparisonOps maps JSONQL comparison operator keys → SQL operator symbols.
var comparisonOps = map[string]string{
	"eq":  "=",
	"ne":  "!=",
	"neq": "!=",
	"gt":  ">",
	"gte": ">=",
	"lt":  "<",
	"lte": "<=",
}

// likeOps maps JSONQL LIKE-pattern operator keys → SQL LIKE pattern builders.
var likeOps = map[string]func(interface{}) string{
	"contains": func(v interface{}) string { return fmt.Sprintf("%%%v%%", v) },
	"starts":   func(v interface{}) string { return fmt.Sprintf("%v%%", v) },
	"ends":     func(v interface{}) string { return fmt.Sprintf("%%%v", v) },
}

func (t *Transpiler) processWhere(where map[string]interface{}, tableAlias string) ([]string, []interface{}, error) {
	var conditions []string
	var args []interface{}

	for field, cond := range where {
		logicalConds, logicalArgs, handled, err := t.processLogical(field, cond, tableAlias)
		if err != nil {
			return nil, nil, err
		}
		if handled {
			conditions = append(conditions, logicalConds...)
			args = append(args, logicalArgs...)
			continue
		}

		if !isValidIdentifier(field) {
			return nil, nil, fmt.Errorf("Invalid field name in where clause: %s", field)
		}

		fieldConds, fieldArgs, err := t.processFieldCondition(field, cond, tableAlias)
		if err != nil {
			return nil, nil, err
		}
		conditions = append(conditions, fieldConds...)
		args = append(args, fieldArgs...)
	}
	return conditions, args, nil
}

// processLogical handles or/and/not. Returns handled=true when the key was
// recognised as a logical operator.
func (t *Transpiler) processLogical(field string, cond interface{}, tableAlias string) ([]string, []interface{}, bool, error) {
	switch strings.ToLower(field) {
	case "or":
		conds, args, err := t.processLogicalList(cond, tableAlias, " OR ", true)
		return conds, args, true, err
	case "and":
		conds, args, err := t.processLogicalList(cond, tableAlias, " AND ", false)
		return conds, args, true, err
	case "not":
		notMap, ok := cond.(map[string]interface{})
		if !ok {
			return nil, nil, true, nil
		}
		subConds, subArgs, err := t.processWhere(notMap, tableAlias)
		if err != nil {
			return nil, nil, true, err
		}
		if len(subConds) == 0 {
			return nil, nil, true, nil
		}
		return []string{"NOT (" + strings.Join(subConds, " AND ") + ")"}, subArgs, true, nil
	}
	return nil, nil, false, nil
}

// processLogicalList handles the array body of an `or` / `and` clause.
// When `wrap` is true the result is wrapped as a single OR group; otherwise
// each sub-where becomes its own AND-group condition.
func (t *Transpiler) processLogicalList(cond interface{}, tableAlias, sep string, wrap bool) ([]string, []interface{}, error) {
	list, ok := cond.([]interface{})
	if !ok {
		return nil, nil, nil
	}
	var groups []string
	var args []interface{}
	for _, item := range list {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		subConds, subArgs, err := t.processWhere(itemMap, tableAlias)
		if err != nil {
			return nil, nil, err
		}
		if len(subConds) == 0 {
			continue
		}
		groups = append(groups, "("+strings.Join(subConds, " AND ")+")")
		args = append(args, subArgs...)
	}
	if len(groups) == 0 {
		return nil, nil, nil
	}
	if wrap {
		return []string{"(" + strings.Join(groups, sep) + ")"}, args, nil
	}
	return groups, args, nil
}

// processFieldCondition builds conditions for a single field key, handling
// both operator-map values and implicit equality.
func (t *Transpiler) processFieldCondition(field string, cond interface{}, tableAlias string) ([]string, []interface{}, error) {
	quotedField := fmt.Sprintf("%s.%s", t.quoteIdentifier(tableAlias), t.quoteIdentifier(field))

	valMap, ok := cond.(map[string]interface{})
	if !ok {
		sql, args := t.implicitEq(quotedField, cond)
		return []string{sql}, args, nil
	}

	var conditions []string
	var args []interface{}
	handled := false
	for op, v := range valMap {
		sql, opArgs, ok, err := t.buildOperator(quotedField, op, v, tableAlias)
		if err != nil {
			return nil, nil, err
		}
		if !ok {
			continue
		}
		handled = true
		if sql != "" {
			conditions = append(conditions, sql)
		}
		args = append(args, opArgs...)
	}
	if !handled && len(valMap) > 0 {
		for op := range valMap {
			return nil, nil, fmt.Errorf("Unknown operator \"%s\" for field \"%s\"", op, field)
		}
	}
	return conditions, args, nil
}

// buildOperator dispatches a single operator. Returns ok=false if op is unknown.
// sql=="" with ok=true means the operator was recognised but produced no clause
// (e.g. empty `in` array).
func (t *Transpiler) buildOperator(quotedField, op string, value interface{}, tableAlias string) (string, []interface{}, bool, error) {
	if sqlOp, isCmp := comparisonOps[op]; isCmp {
		sql, args, err := t.buildComparison(quotedField, sqlOp, value, tableAlias)
		return sql, args, true, err
	}
	if op == "in" || op == "nin" {
		sql, args := t.buildInClause(quotedField, value, op == "nin")
		return sql, args, true, nil
	}
	if patternFn, isLike := likeOps[op]; isLike {
		sql, args := t.buildLikeClause(quotedField, patternFn(value))
		return sql, args, true, nil
	}
	if op == "like" {
		sql, args := t.buildLikeClause(quotedField, value)
		return sql, args, true, nil
	}
	return "", nil, false, nil
}

// buildComparison emits a comparison clause, handling NULL, field-refs and
// parameterised values uniformly.
func (t *Transpiler) buildComparison(quotedField, sqlOp string, value interface{}, tableAlias string) (string, []interface{}, error) {
	rhs, isRef, err := t.resolveFieldRef(value, tableAlias)
	if err != nil {
		return "", nil, err
	}
	if isRef {
		return fmt.Sprintf("%s %s %s", quotedField, sqlOp, rhs), nil, nil
	}
	if value == nil {
		if sqlOp == "!=" {
			return fmt.Sprintf("%s IS NOT NULL", quotedField), nil, nil
		}
		return fmt.Sprintf("%s IS NULL", quotedField), nil, nil
	}
	return fmt.Sprintf("%s %s ?", quotedField, sqlOp), []interface{}{value}, nil
}

func (t *Transpiler) buildInClause(quotedField string, value interface{}, negated bool) (string, []interface{}) {
	slice, ok := value.([]interface{})
	if !ok || len(slice) == 0 {
		return "", nil
	}
	placeholders := make([]string, len(slice))
	args := make([]interface{}, len(slice))
	for i, v := range slice {
		placeholders[i] = "?"
		args[i] = v
	}
	op := "IN"
	if negated {
		op = "NOT IN"
	}
	return fmt.Sprintf("%s %s (%s)", quotedField, op, strings.Join(placeholders, ", ")), args
}

func (t *Transpiler) buildLikeClause(quotedField string, pattern interface{}) (string, []interface{}) {
	return fmt.Sprintf("%s LIKE ?", quotedField), []interface{}{pattern}
}

func (t *Transpiler) implicitEq(quotedField string, value interface{}) (string, []interface{}) {
	if value == nil {
		return fmt.Sprintf("%s IS NULL", quotedField), nil
	}
	return fmt.Sprintf("%s = ?", quotedField), []interface{}{value}
}

// resolveFieldRef detects { "field": "name" } RHS values referring to another
// column. Returns (sqlExpr, isRef, error).
func (t *Transpiler) resolveFieldRef(value interface{}, tableAlias string) (string, bool, error) {
	refMap, ok := value.(map[string]interface{})
	if !ok {
		return "", false, nil
	}
	rawField, ok := refMap["field"]
	if !ok {
		return "", false, nil
	}
	fieldName, ok := rawField.(string)
	if !ok || fieldName == "" {
		return "", false, fmt.Errorf("Invalid field reference")
	}
	parts := strings.Split(fieldName, ".")
	switch len(parts) {
	case 1:
		if !isValidIdentifier(parts[0]) {
			return "", false, fmt.Errorf("Invalid field reference: %s", fieldName)
		}
		return fmt.Sprintf("%s.%s", t.quoteIdentifier(tableAlias), t.quoteIdentifier(parts[0])), true, nil
	case 2:
		if !isValidIdentifier(parts[0]) || !isValidIdentifier(parts[1]) {
			return "", false, fmt.Errorf("Invalid field reference: %s", fieldName)
		}
		return "NULL", true, nil
	}
	return "", false, fmt.Errorf("Invalid field reference: %s", fieldName)
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
		sb.WriteString(sql[:i])
		sb.WriteString(t.Dialect.Placeholder(count))
		count++
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

// TranspileInsert generates an INSERT SQL statement from a table name and data map
func (t *Transpiler) TranspileInsert(tableName string, data map[string]interface{}) (*TranspileResult, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("insert data cannot be empty")
	}
	if !isValidIdentifier(tableName) {
		return nil, fmt.Errorf("invalid table name: %s", tableName)
	}

	// Sort keys for deterministic output
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	columns := make([]string, len(keys))
	placeholders := make([]string, len(keys))
	args := make([]interface{}, len(keys))
	for i, k := range keys {
		if !isValidIdentifier(k) {
			return nil, fmt.Errorf("invalid column name: %s", k)
		}
		columns[i] = t.quoteIdentifier(k)
		placeholders[i] = "?"
		args[i] = data[k]
	}

	sqlStr := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		t.quoteIdentifier(tableName),
		strings.Join(columns, ", "),
		strings.Join(placeholders, ", "))

	if t.Dialect.SupportsReturning() {
		sqlStr += " RETURNING *"
	}

	return &TranspileResult{
		SQL:  t.replacePlaceholders(sqlStr),
		Args: args,
	}, nil
}

// TranspileUpdate generates an UPDATE SQL statement from a table name, patch data, and where clause
func (t *Transpiler) TranspileUpdate(tableName string, patch map[string]interface{}, where map[string]interface{}) (*TranspileResult, error) {
	if len(patch) == 0 {
		return nil, fmt.Errorf("update patch cannot be empty")
	}
	if !isValidIdentifier(tableName) {
		return nil, fmt.Errorf("invalid table name: %s", tableName)
	}

	// Sort keys for deterministic output
	keys := make([]string, 0, len(patch))
	for k := range patch {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	setParts := make([]string, len(keys))
	args := make([]interface{}, len(keys))
	for i, k := range keys {
		if !isValidIdentifier(k) {
			return nil, fmt.Errorf("invalid column name: %s", k)
		}
		setParts[i] = fmt.Sprintf("%s = ?", t.quoteIdentifier(k))
		args[i] = patch[k]
	}

	sqlStr := fmt.Sprintf("UPDATE %s SET %s", t.quoteIdentifier(tableName), strings.Join(setParts, ", "))

	if where != nil && len(where) > 0 {
		conds, whereArgs, err := t.processWhere(where, tableName)
		if err != nil {
			return nil, err
		}
		if len(conds) > 0 {
			// Remove table prefix from conditions for simple UPDATE (no alias)
			simpleConds := make([]string, len(conds))
			for i, c := range conds {
				simpleConds[i] = strings.Replace(c, t.quoteIdentifier(tableName)+".", "", 1)
			}
			sqlStr += " WHERE " + strings.Join(simpleConds, " AND ")
			args = append(args, whereArgs...)
		}
	}

	if t.Dialect.SupportsReturning() {
		sqlStr += " RETURNING *"
	}

	return &TranspileResult{
		SQL:  t.replacePlaceholders(sqlStr),
		Args: args,
	}, nil
}

// TranspileDelete generates a DELETE SQL statement from a table name and where clause
func (t *Transpiler) TranspileDelete(tableName string, where map[string]interface{}) (*TranspileResult, error) {
	if !isValidIdentifier(tableName) {
		return nil, fmt.Errorf("invalid table name: %s", tableName)
	}

	sqlStr := fmt.Sprintf("DELETE FROM %s", t.quoteIdentifier(tableName))

	if where != nil && len(where) > 0 {
		conds, args, err := t.processWhere(where, tableName)
		if err != nil {
			return nil, err
		}
		if len(conds) > 0 {
			simpleConds := make([]string, len(conds))
			for i, c := range conds {
				simpleConds[i] = strings.Replace(c, t.quoteIdentifier(tableName)+".", "", 1)
			}
			sqlStr += " WHERE " + strings.Join(simpleConds, " AND ")
			return &TranspileResult{
				SQL:  t.replacePlaceholders(sqlStr),
				Args: args,
			}, nil
		}
	}

	return &TranspileResult{
		SQL:  t.replacePlaceholders(sqlStr),
		Args: nil,
	}, nil
}
