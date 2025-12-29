package schema

import (
	"context"
	"strings"

	jsonql "github.com/jsonql-standard/jsonql-go"
)

// SQLiteIntrospector implements Introspector for SQLite
type SQLiteIntrospector struct {
	driver jsonql.Driver
}

// NewSQLiteIntrospector creates a new SQLiteIntrospector
func NewSQLiteIntrospector(driver jsonql.Driver) *SQLiteIntrospector {
	return &SQLiteIntrospector{driver: driver}
}

// Introspect returns the schema derived from the database
func (i *SQLiteIntrospector) Introspect() (*jsonql.JSONQLSchema, error) {
	schema := &jsonql.JSONQLSchema{
		Tables: make(map[string]*jsonql.JSONQLTable),
	}

	ctx := context.Background()

	// 1. Get all tables
	rows, err := i.driver.Query(ctx, "SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'", nil)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		tables = append(tables, name)
	}

	// 2. Get columns for each table
	for _, tableName := range tables {
		tableSchema := &jsonql.JSONQLTable{
			Fields:    make(map[string]*jsonql.JSONQLField),
			Relations: make(map[string]*jsonql.JSONQLRelation),
		}

		// PRAGMA table_info returns: cid, name, type, notnull, dflt_value, pk
		colRows, err := i.driver.Query(ctx, "PRAGMA table_info(\""+tableName+"\")", nil)
		if err != nil {
			return nil, err
		}
		defer colRows.Close()

		for colRows.Next() {
			var cid int
			var name, dtype string
			var notnull, pk int
			var dfltValue interface{} // dflt_value can be null

			if err := colRows.Scan(&cid, &name, &dtype, &notnull, &dfltValue, &pk); err != nil {
				// Try scanning without dfltValue if it fails? No, standard scan should handle interface{} for nulls
				// But let's be safe, sometimes drivers behave differently.
				// For now assume standard sqlite driver behavior.
				return nil, err
			}

			fieldSchema := &jsonql.JSONQLField{
				Type:        i.mapSQLiteType(dtype),
				AllowSelect: true,
				AllowFilter: true,
				AllowSort:   true,
			}

			tableSchema.Fields[name] = fieldSchema
		}
		colRows.Close() // Close explicitly inside loop

		// 3. Get Foreign Keys
		fkRows, err := i.driver.Query(ctx, "PRAGMA foreign_key_list(\""+tableName+"\")", nil)
		if err != nil {
			return nil, err
		}
		defer fkRows.Close()

		for fkRows.Next() {
			var id, seq int
			var table, from, to, onUpdate, onDelete, match string
			if err := fkRows.Scan(&id, &seq, &table, &from, &to, &onUpdate, &onDelete, &match); err != nil {
				return nil, err
			}

			// Infer relation name from column name (e.g., user_id -> user)
			relName := table
			if strings.HasSuffix(from, "_id") {
				relName = strings.TrimSuffix(from, "_id")
			}

			tableSchema.Relations[relName] = &jsonql.JSONQLRelation{
				Type:  "hasOne",
				Field: from,
				Table: table,
			}
		}
		fkRows.Close()

		schema.Tables[tableName] = tableSchema
	}

	// 4. Infer hasMany relations (Inverse of hasOne)
	for tableName, table := range schema.Tables {
		for _, rel := range table.Relations {
			if rel.Type == "hasOne" {
				targetTable, ok := schema.Tables[rel.Table]
				if !ok {
					continue
				}
				// Add hasMany to target table
				// Relation name is the source table name (e.g., orders)
				targetTable.Relations[tableName] = &jsonql.JSONQLRelation{
					Type:  "hasMany",
					Field: rel.Field, // The field in the source table
					Table: tableName, // The source table
				}
			}
		}
	}

	return schema, nil
}

func (i *SQLiteIntrospector) mapSQLiteType(dtype string) string {
	t := strings.ToUpper(dtype)
	if strings.Contains(t, "INT") {
		return "integer"
	}
	if strings.Contains(t, "CHAR") || strings.Contains(t, "TEXT") || strings.Contains(t, "CLOB") {
		return "string"
	}
	if strings.Contains(t, "BLOB") {
		return "string"
	}
	if strings.Contains(t, "REAL") || strings.Contains(t, "FLOA") || strings.Contains(t, "DOUB") {
		return "float"
	}
	if strings.Contains(t, "BOOL") {
		return "boolean"
	}
	return "string"
}
