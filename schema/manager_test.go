package schema

import (
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
	mockSchema := &jsonql.JSONQLSchema{
		Tables: map[string]*jsonql.JSONQLTable{
			"users": {
				Fields: map[string]*jsonql.JSONQLField{
					"id": {Type: "integer"},
				},
			},
		},
	}

	manager := NewManager(ManagerOptions{
		Introspector: &MockIntrospector{schema: mockSchema},
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
	base := &jsonql.JSONQLSchema{
		Tables: map[string]*jsonql.JSONQLTable{
			"users": {
				Fields: map[string]*jsonql.JSONQLField{
					"id":   {Type: "integer"},
					"name": {Type: "string"},
				},
			},
		},
	}

	// Override schema (simulating file load)
	override := &jsonql.JSONQLSchema{
		Tables: map[string]*jsonql.JSONQLTable{
			"users": {
				Fields: map[string]*jsonql.JSONQLField{
					"email": {Type: "string"},
				},
				Relations: map[string]*jsonql.JSONQLRelation{
					"posts": {Type: "hasMany"},
				},
			},
			"posts": {
				Fields: map[string]*jsonql.JSONQLField{
					"id": {Type: "integer"},
				},
			},
		},
	}

	manager := &Manager{}
	manager.mergeSchemas(base, override)

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
