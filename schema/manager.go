package schema

import (
	"encoding/json"
	"os"

	jsonql "github.com/jsonql-standard/jsonql-go"
)

// ManagerOptions configuration for SchemaManager
type ManagerOptions struct {
	Introspector   Introspector
	SchemaFilePath string
}

// Manager handles schema loading and merging
type Manager struct {
	options ManagerOptions
}

// NewManager creates a new SchemaManager
func NewManager(options ManagerOptions) *Manager {
	return &Manager{options: options}
}

// Load loads the schema from introspection and/or file
func (m *Manager) Load() (*jsonql.JSONQLSchema, error) {
	finalSchema := &jsonql.JSONQLSchema{
		Tables: make(map[string]*jsonql.JSONQLTable),
	}

	// 1. Introspection
	if m.options.Introspector != nil {
		introspected, err := m.options.Introspector.Introspect()
		if err != nil {
			return nil, err
		}
		m.mergeSchemas(finalSchema, introspected)
	}

	// 2. File
	if m.options.SchemaFilePath != "" {
		fileSchema, err := m.loadFromFile(m.options.SchemaFilePath)
		if err == nil {
			m.mergeSchemas(finalSchema, fileSchema)
		}
		// Note: We ignore file load errors if file doesn't exist or is invalid,
		// similar to the TS implementation which warns but proceeds.
		// For Go, maybe we should log or return error?
		// The TS implementation catches error and warns.
		// Here we'll just proceed if error (e.g. file not found).
	}

	return finalSchema, nil
}

func (m *Manager) loadFromFile(path string) (*jsonql.JSONQLSchema, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var schema jsonql.JSONQLSchema
	if err := json.Unmarshal(data, &schema); err != nil {
		return nil, err
	}
	return &schema, nil
}

func (m *Manager) mergeSchemas(base, override *jsonql.JSONQLSchema) {
	if override == nil || override.Tables == nil {
		return
	}
	if base.Tables == nil {
		base.Tables = make(map[string]*jsonql.JSONQLTable)
	}

	for tableName, table := range override.Tables {
		if baseTable, exists := base.Tables[tableName]; exists {
			// Merge fields
			if table.Fields != nil {
				if baseTable.Fields == nil {
					baseTable.Fields = make(map[string]*jsonql.JSONQLField)
				}
				for fieldName, field := range table.Fields {
					baseTable.Fields[fieldName] = field
				}
			}
			// Merge relations
			if table.Relations != nil {
				if baseTable.Relations == nil {
					baseTable.Relations = make(map[string]*jsonql.JSONQLRelation)
				}
				for relName, rel := range table.Relations {
					baseTable.Relations[relName] = rel
				}
			}
		} else {
			base.Tables[tableName] = table
		}
	}
}
