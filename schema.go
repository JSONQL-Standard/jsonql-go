package jsonql

import "fmt"

type JSONQLSchema struct {
	Tables map[string]*JSONQLTable `json:"tables"`
}

type JSONQLTable struct {
	Fields map[string]*JSONQLField `json:"fields"`
	Relations map[string]*JSONQLRelation `json:"relations,omitempty"`
}

type JSONQLField struct {
	Type string `json:"type"`
	AllowSelect bool `json:"allowSelect,omitempty"`
	AllowFilter bool `json:"allowFilter,omitempty"`
	AllowSort bool `json:"allowSort,omitempty"`
}

type JSONQLRelation struct {
	Type string `json:"type"` // "hasOne" | "hasMany"
	Field string `json:"field"`
}

type Validator struct {
	schema *JSONQLSchema
	table string
}

func NewValidator(schema *JSONQLSchema, table string) *Validator {
	return &Validator{schema: schema, table: table}
}

func (v *Validator) Validate(query *JSONQLQuery) error {
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