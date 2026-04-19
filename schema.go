package jsonql

import "fmt"

type JSONQLSchema struct {
	Tables   map[string]*JSONQLTable `json:"tables"`
	Settings *JSONQLSettings         `json:"settings,omitempty"`
}

type JSONQLSettings struct {
	AllowAggregate bool `json:"allowAggregate"`
	MaxDepth       int  `json:"maxDepth"`
}

type JSONQLTable struct {
	Fields     map[string]*JSONQLField    `json:"fields"`
	Relations  map[string]*JSONQLRelation `json:"relations,omitempty"`
	PrimaryKey string                     `json:"primaryKey,omitempty"`
}

type JSONQLField struct {
	Type           string `json:"type"`
	AllowSelect    *bool  `json:"allowSelect,omitempty"`
	AllowFilter    *bool  `json:"allowFilter,omitempty"`
	AllowSort      *bool  `json:"allowSort,omitempty"`
	AllowGroup     *bool  `json:"allowGroup,omitempty"`
	AllowAggregate *bool  `json:"allowAggregate,omitempty"`
	AllowSum       *bool  `json:"allowSum,omitempty"`
	AllowAvg       *bool  `json:"allowAvg,omitempty"`
	AllowMin       *bool  `json:"allowMin,omitempty"`
	AllowMax       *bool  `json:"allowMax,omitempty"`
	AllowCount     *bool  `json:"allowCount,omitempty"`
}

type JSONQLRelation struct {
	Type         string `json:"type"`             // "hasOne" | "hasMany" | "belongsTo"
	Field        string `json:"foreignKey"`       // Foreign Key
	Table        string `json:"target,omitempty"` // Target table name
	AllowInclude *bool  `json:"allowInclude,omitempty"`
}

type Validator struct {
	schema *JSONQLSchema
	table  string
}

func NewValidator(schema *JSONQLSchema, table string) *Validator {
	return &Validator{schema: schema, table: table}
}

func (v *Validator) Validate(query *JSONQLQuery) error {
	if err := v.doValidate(query); err != nil {
		return &JsonQLValidationError{
			Errors: []ValidationError{{Message: err.Error(), Code: "VALIDATION_ERROR"}},
		}
	}
	return nil
}

func (v *Validator) doValidate(query *JSONQLQuery) error {
	// Check global settings
	if v.schema.Settings != nil {
		if !v.schema.Settings.AllowAggregate && (len(query.Aggregate) > 0 || len(query.GroupBy) > 0) {
			return fmt.Errorf("aggregations are disabled in this schema")
		}
		if v.schema.Settings.MaxDepth > 0 {
			depth := v.calculateDepth(query)
			if depth > v.schema.Settings.MaxDepth {
				return fmt.Errorf("query depth %d exceeds maximum allowed depth of %d", depth, v.schema.Settings.MaxDepth)
			}
		}
	}

	table, ok := v.schema.Tables[v.table]
	if !ok {
		return fmt.Errorf("table '%s' not found in schema", v.table)
	}

	// Fields — only validate when the schema defines fields for this table
	if len(table.Fields) > 0 {
		for _, f := range query.Fields {
			field, ok := table.Fields[f]
			if !ok || (field.AllowSelect != nil && !*field.AllowSelect) {
				return fmt.Errorf("field '%s' not allowed on table '%s'", f, v.table)
			}
		}
	}

	// Where fields (allowFilter) — only validate when the schema defines fields
	if len(table.Fields) > 0 {
		for field := range query.Where {
			if field == "or" || field == "OR" || field == "and" || field == "AND" || field == "not" || field == "NOT" {
				continue
			}
			fieldObj, ok := table.Fields[field]
			if !ok || (fieldObj.AllowFilter != nil && !*fieldObj.AllowFilter) {
				return fmt.Errorf("field '%s' not filterable on table '%s'", field, v.table)
			}
		}
	}

	// Sort fields (allowSort) — only validate when schema defines fields
	if len(table.Fields) > 0 {
		for _, s := range query.Sort {
			field := s
			if len(s) > 0 && s[0] == '-' {
				field = s[1:]
			}
			fieldObj, ok := table.Fields[field]
			if !ok || (fieldObj.AllowSort != nil && !*fieldObj.AllowSort) {
				return fmt.Errorf("field '%s' not sortable on table '%s'", field, v.table)
			}
		}

		// Group By (allowGroup)
		for _, g := range query.GroupBy {
			field, ok := table.Fields[g]
			if !ok || (field.AllowGroup != nil && !*field.AllowGroup) {
				return fmt.Errorf("field '%s' not groupable on table '%s'", g, v.table)
			}
		}

		// Aggregates
		for _, aggDef := range query.Aggregate {
			if aggMap, ok := aggDef.(map[string]interface{}); ok {
				for funcName, fRaw := range aggMap {
					fieldName, ok := fRaw.(string)
					if !ok {
						continue
					}

					if fieldName == "*" && funcName == "count" {
						continue // COUNT(*) usually allowed if aggregation is allowed globally
					}

					field, ok := table.Fields[fieldName]
					if !ok {
						return fmt.Errorf("field '%s' not found on table '%s'", fieldName, v.table)
					}

					// Check specific function permission first
					allowed := true
					checked := false

					switch funcName {
					case "sum":
						if field.AllowSum != nil {
							allowed = *field.AllowSum
							checked = true
						}
					case "avg":
						if field.AllowAvg != nil {
							allowed = *field.AllowAvg
							checked = true
						}
					case "min":
						if field.AllowMin != nil {
							allowed = *field.AllowMin
							checked = true
						}
					case "max":
						if field.AllowMax != nil {
							allowed = *field.AllowMax
							checked = true
						}
					case "count":
						if field.AllowCount != nil {
							allowed = *field.AllowCount
							checked = true
						}
					}

					// If specific check was not performed (nil), fall back to AllowAggregate
					if !checked {
						if field.AllowAggregate != nil {
							allowed = *field.AllowAggregate
						}
					}

					if !allowed {
						return fmt.Errorf("aggregation '%s' not allowed on field '%s'", funcName, fieldName)
					}
				}
			}
		}
	} // end if len(table.Fields) > 0

	// Relations (allowInclude)
	for relName := range query.Include {
		rel, ok := table.Relations[relName]
		if !ok {
			return fmt.Errorf("relation '%s' not found on table '%s'", relName, v.table)
		}
		if rel.AllowInclude != nil && !*rel.AllowInclude {
			return fmt.Errorf("relation '%s' not allowed to be included", relName)
		}
	}

	return nil
}

// ValidateAll performs comprehensive validation and returns structured errors instead of a single error.
// This is the recommended validation method for providing detailed feedback to API consumers.
func (v *Validator) ValidateAll(query *JSONQLQuery) *ValidationResult {
	result := &ValidationResult{Valid: true}

	// Check global settings
	if v.schema.Settings != nil {
		if !v.schema.Settings.AllowAggregate && (len(query.Aggregate) > 0 || len(query.GroupBy) > 0) {
			result.Valid = false
			result.Errors = append(result.Errors, ValidationError{
				Code:    "AGGREGATION_DISABLED",
				Message: "aggregations are disabled in this schema",
			})
		}
		if v.schema.Settings.MaxDepth > 0 {
			depth := v.calculateDepth(query)
			if depth > v.schema.Settings.MaxDepth {
				result.Valid = false
				result.Errors = append(result.Errors, ValidationError{
					Code:    "MAX_DEPTH_EXCEEDED",
					Message: fmt.Sprintf("query depth %d exceeds maximum allowed depth of %d", depth, v.schema.Settings.MaxDepth),
				})
			}
		}
	}

	table, ok := v.schema.Tables[v.table]
	if !ok {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Code:    "TABLE_NOT_FOUND",
			Message: fmt.Sprintf("table '%s' not found in schema", v.table),
		})
		return result
	}

	// Fields
	for _, f := range query.Fields {
		field, ok := table.Fields[f]
		if !ok || (field.AllowSelect != nil && !*field.AllowSelect) {
			result.Valid = false
			result.Errors = append(result.Errors, ValidationError{
				Code:    "FIELD_NOT_ALLOWED",
				Message: fmt.Sprintf("field '%s' not allowed on table '%s'", f, v.table),
				Path:    "fields",
			})
		}
	}

	// Where fields
	for field := range query.Where {
		if field == "or" || field == "and" || field == "not" {
			continue
		}
		fieldObj, ok := table.Fields[field]
		if !ok || (fieldObj.AllowFilter != nil && !*fieldObj.AllowFilter) {
			result.Valid = false
			result.Errors = append(result.Errors, ValidationError{
				Code:    "FIELD_NOT_FILTERABLE",
				Message: fmt.Sprintf("field '%s' not filterable on table '%s'", field, v.table),
				Path:    "where",
			})
		}
	}

	// Sort fields
	for _, s := range query.Sort {
		field := s
		if len(s) > 0 && s[0] == '-' {
			field = s[1:]
		}
		fieldObj, ok := table.Fields[field]
		if !ok || (fieldObj.AllowSort != nil && !*fieldObj.AllowSort) {
			result.Valid = false
			result.Errors = append(result.Errors, ValidationError{
				Code:    "FIELD_NOT_SORTABLE",
				Message: fmt.Sprintf("field '%s' not sortable on table '%s'", field, v.table),
				Path:    "sort",
			})
		}
	}

	// Group By
	for _, g := range query.GroupBy {
		field, ok := table.Fields[g]
		if !ok || (field.AllowGroup != nil && !*field.AllowGroup) {
			result.Valid = false
			result.Errors = append(result.Errors, ValidationError{
				Code:    "FIELD_NOT_GROUPABLE",
				Message: fmt.Sprintf("field '%s' not groupable on table '%s'", g, v.table),
				Path:    "groupBy",
			})
		}
	}

	// Aggregates
	for alias, aggDef := range query.Aggregate {
		if aggMap, ok := aggDef.(map[string]interface{}); ok {
			for funcName, fRaw := range aggMap {
				fieldName, ok := fRaw.(string)
				if !ok {
					continue
				}
				if fieldName == "*" && funcName == "count" {
					continue
				}
				field, ok := table.Fields[fieldName]
				if !ok {
					result.Valid = false
					result.Errors = append(result.Errors, ValidationError{
						Code:    "FIELD_NOT_FOUND",
						Message: fmt.Sprintf("field '%s' not found on table '%s'", fieldName, v.table),
						Path:    "aggregate." + alias,
					})
					continue
				}
				allowed := true
				checked := false
				switch funcName {
				case "sum":
					if field.AllowSum != nil {
						allowed = *field.AllowSum
						checked = true
					}
				case "avg":
					if field.AllowAvg != nil {
						allowed = *field.AllowAvg
						checked = true
					}
				case "min":
					if field.AllowMin != nil {
						allowed = *field.AllowMin
						checked = true
					}
				case "max":
					if field.AllowMax != nil {
						allowed = *field.AllowMax
						checked = true
					}
				case "count":
					if field.AllowCount != nil {
						allowed = *field.AllowCount
						checked = true
					}
				}
				if !checked && field.AllowAggregate != nil {
					allowed = *field.AllowAggregate
				}
				if !allowed {
					result.Valid = false
					result.Errors = append(result.Errors, ValidationError{
						Code:    "AGGREGATION_NOT_ALLOWED",
						Message: fmt.Sprintf("aggregation '%s' not allowed on field '%s'", funcName, fieldName),
						Path:    "aggregate." + alias,
					})
				}
			}
		}
	}

	// Relations
	for relName := range query.Include {
		rel, ok := table.Relations[relName]
		if !ok {
			result.Valid = false
			result.Errors = append(result.Errors, ValidationError{
				Code:    "RELATION_NOT_FOUND",
				Message: fmt.Sprintf("relation '%s' not found on table '%s'", relName, v.table),
				Path:    "include",
			})
			continue
		}
		if rel.AllowInclude != nil && !*rel.AllowInclude {
			result.Valid = false
			result.Errors = append(result.Errors, ValidationError{
				Code:    "RELATION_NOT_ALLOWED",
				Message: fmt.Sprintf("relation '%s' not allowed to be included", relName),
				Path:    "include",
			})
		}
	}

	return result
}

func (v *Validator) calculateDepth(query *JSONQLQuery) int {
	if len(query.Include) == 0 {
		return 0
	}

	maxChildDepth := 0
	for _, subQueryRaw := range query.Include {
		// Handle sub-query if it's a map (complex include)
		if subQueryMap, ok := subQueryRaw.(map[string]interface{}); ok {
			// We need to parse this map back to a query to check its depth recursively
			// For simplicity in this MVP, we assume 1 level per include map entry if we can't fully parse it easily here without circular deps
			// But ideally we should cast to *JSONQLQuery if possible or traverse the map
			// Given the structure of Include map[string]interface{}, it might be just a boolean or a sub-query object.

			// If it has "fields" or "include", it's a sub-query
			if _, hasFields := subQueryMap["fields"]; hasFields {
				// It's a sub-query, but we don't have the struct here easily without unmarshalling again or manual traversal.
				// Let's do a rough estimation: if it has "include", recurse.
				if nestedInclude, hasInclude := subQueryMap["include"]; hasInclude {
					if nestedMap, ok := nestedInclude.(map[string]interface{}); ok {
						// Create a dummy query to recurse
						dummy := &JSONQLQuery{Include: IncludeMap(nestedMap)}
						d := v.calculateDepth(dummy)
						if d > maxChildDepth {
							maxChildDepth = d
						}
					}
				}
			}
		}
	}
	return 1 + maxChildDepth
}
