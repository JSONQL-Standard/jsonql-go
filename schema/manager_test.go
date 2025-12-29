package schema

import (
	"encoding/json"
	"testing"

	jsonql "github.com/jsonql-standard/jsonql-go"
)

// MockIntrospector for testing
type MockIntrospector struct {
	schema *jsonql.JSONQLSchema
}

func (m *MockIntrospector) Introspect() (*jsonql.JSONQLSchema, error) {
	return m.schema, nil
}

func TestManager_Load_IntrospectionOnly(t *testing.T) {
	mockSchemaJSON := `{
		"tables": {
			"users": {
				"fields": {
					"id": { "type": "integer" }
				}
			}
		}
	}`
	var mockSchema jsonql.JSONQLSchema
	if err := json.Unmarshal([]byte(mockSchemaJSON), &mockSchema); err != nil {
		t.Fatalf("Failed to unmarshal mock schema: %v", err)
	}

	manager := NewManager(ManagerOptions{
		Introspector: &MockIntrospector{schema: &mockSchema},
	})

	schema, err := manager.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if len(schema.Tables) != 1 {
		t.Errorf("Expected 1 table, got %d", len(schema.Tables))
	}
	if _, ok := schema.Tables["users"]; !ok {
		t.Error("Expected table 'users'")
	}
}

func TestManager_Load_Merge(t *testing.T) {
	// Base schema (simulating introspection)
	baseJSON := `{
		"tables": {
			"users": {
				"fields": {
					"id":   { "type": "integer" },
					"name": { "type": "string" }
				}
			}
		}
	}`
	var base jsonql.JSONQLSchema
	if err := json.Unmarshal([]byte(baseJSON), &base); err != nil {
		t.Fatalf("Failed to unmarshal base schema: %v", err)
	}

	// Override schema (simulating file load)
	overrideJSON := `{
		"tables": {
			"users": {
				"fields": {
					"email": { "type": "string" }
				},
				"relations": {
					"posts": { "type": "hasMany" }
				}
			},
			"posts": {
				"fields": {
					"id": { "type": "integer" }
				}
			}
		}
	}`
	var override jsonql.JSONQLSchema
	if err := json.Unmarshal([]byte(overrideJSON), &override); err != nil {
		t.Fatalf("Failed to unmarshal override schema: %v", err)
	}

	manager := &Manager{}
	manager.mergeSchemas(&base, &override)

	// Check users table merged
	users := base.Tables["users"]
	if len(users.Fields) != 3 {
		t.Errorf("Expected 3 fields in users, got %d", len(users.Fields))
	}
	if _, ok := users.Fields["email"]; !ok {
		t.Error("Expected email field in users")
	}
	if len(users.Relations) != 1 {
		t.Errorf("Expected 1 relation in users, got %d", len(users.Relations))
	}

	// Check posts table added
	if _, ok := base.Tables["posts"]; !ok {
		t.Error("Expected posts table")
	}
}

func TestManager_Load_FromJSON(t *testing.T) {
	// Demonstrating loading from JSON string instead of struct literal
	jsonSchema := `{
		"tables": {
			"products": {
				"fields": {
					"id": { "type": "integer" },
					"name": { "type": "string" }
				}
			}
		}
	}`

	var schema jsonql.JSONQLSchema
	if err := json.Unmarshal([]byte(jsonSchema), &schema); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	manager := NewManager(ManagerOptions{
		Introspector: &MockIntrospector{schema: &schema},
	})

	loaded, err := manager.Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if _, ok := loaded.Tables["products"]; !ok {
		t.Error("Expected table 'products' from JSON")
	}
}
