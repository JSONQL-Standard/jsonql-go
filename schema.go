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
	Fields    map[string]*JSONQLField    `json:"fields"`
	Relations map[string]*JSONQLRelation `json:"relations,omitempty"`
}

type JSONQLField struct {
	Type        string `json:"type"`
	AllowSelect bool   `json:"allowSelect,omitempty"`
	AllowFilter bool   `json:"allowFilter,omitempty"`
	AllowSort   bool   `json:"allowSort,omitempty"`
}

type JSONQLRelation struct {
	Type  string `json:"type"`            // "hasOne" | "hasMany"
	Field string `json:"field"`           // Foreign Key
	Table string `json:"table,omitempty"` // Target table name (optional, defaults to relation name)
}

type Validator struct {
	schema *JSONQLSchema
	table  string
}

func NewValidator(schema *JSONQLSchema, table string) *Validator {
	return &Validator{schema: schema, table: table}
}

func (v *Validator) Validate(query *JSONQLQuery) error {
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

	// Fields
	for _, f := range query.Fields {
		field, ok := table.Fields[f]
		if !ok || !field.AllowSelect {
			return fmt.Errorf("field '%s' not allowed on table '%s'", f, v.table)
		}
	}

	// Where fields (allowFilter)
	for field := range query.Where {
		if field == "or" {
			continue
		}
		fieldObj, ok := table.Fields[field]
		if !ok || !fieldObj.AllowFilter {
			return fmt.Errorf("field '%s' not filterable on table '%s'", field, v.table)
		}
	}

	// Sort fields (allowSort)
	for _, s := range query.Sort {
		field := s
		if len(s) > 0 && s[0] == '-' {
			field = s[1:]
		}
		fieldObj, ok := table.Fields[field]
		if !ok || !fieldObj.AllowSort {
			return fmt.Errorf("field '%s' not sortable on table '%s'", field, v.table)
		}
	}

	return nil
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
						dummy := &JSONQLQuery{Include: nestedMap}
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
