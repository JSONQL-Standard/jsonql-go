package jsonql_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/jsonql-standard/jsonql-go"
)

func TestValidator(t *testing.T) {
	schemaJSON := `{
		"tables": {
			"users": {
				"fields": {
					"id":    { "type": "number", "allowSelect": true, "allowFilter": true, "allowSort": true },
					"name":  { "type": "string", "allowSelect": true, "allowFilter": true, "allowSort": true },
					"email": { "type": "string", "allowSelect": false, "allowFilter": false, "allowSort": false },
					"role":  { "type": "string", "allowSelect": true, "allowFilter": false, "allowSort": false }
				}
			}
		}
	}`

	var schema jsonql.JSONQLSchema
	if err := json.Unmarshal([]byte(schemaJSON), &schema); err != nil {
		t.Fatalf("Failed to parse schema: %v", err)
	}

	tests := []struct {
		name      string
		tableName string
		query     jsonql.JSONQLQuery
		wantErr   bool
		errMsg    string
	}{
		{
			name:      "Valid Query",
			tableName: "users",
			query: jsonql.JSONQLQuery{
				Fields: []string{"id", "name"},
				Where:  map[string]interface{}{"name": map[string]interface{}{"eq": "Alice"}},
				Sort:   []string{"-id"},
			},
			wantErr: false,
		},
		{
			name:      "Invalid Table",
			tableName: "products",
			query:     jsonql.JSONQLQuery{},
			wantErr:   true,
			errMsg:    "table 'products' not found",
		},
		{
			name:      "Field Not Allowed (Select)",
			tableName: "users",
			query: jsonql.JSONQLQuery{
				Fields: []string{"email"},
			},
			wantErr: true,
			errMsg:  "field 'email' not allowed",
		},
		{
			name:      "Field Not Allowed (Filter)",
			tableName: "users",
			query: jsonql.JSONQLQuery{
				Where: map[string]interface{}{"role": "admin"},
			},
			wantErr: true,
			errMsg:  "field 'role' not filterable",
		},
		{
			name:      "Field Not Allowed (Sort)",
			tableName: "users",
			query: jsonql.JSONQLQuery{
				Sort: []string{"role"},
			},
			wantErr: true,
			errMsg:  "field 'role' not sortable",
		},
		{
			name:      "Unknown Field",
			tableName: "users",
			query: jsonql.JSONQLQuery{
				Fields: []string{"unknown"},
			},
			wantErr: true,
			errMsg:  "field 'unknown' not allowed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := jsonql.NewValidator(&schema, tt.tableName)
			err := v.Validate(&tt.query)

			if tt.wantErr {
				if err == nil {
					t.Errorf("Validator.Validate() error = nil, wantErr %v", tt.wantErr)
					return
				}
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("Validator.Validate() error = %v, want errMsg containing %v", err, tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("Validator.Validate() unexpected error = %v", err)
				}
			}
		})
	}
}
