package jsonql

import (
	"database/sql"
	"strings"
)

// Hydrator converts SQL rows into JSON-friendly structures
type Hydrator struct{}

// NewHydrator creates a new Hydrator
func NewHydrator() *Hydrator {
	return &Hydrator{}
}

// Hydrate converts sql.Rows into a slice of maps
func (h *Hydrator) Hydrate(rows *sql.Rows, schema *JSONQLSchema, rootTable string) ([]map[string]interface{}, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	var results []map[string]interface{}

	for rows.Next() {
		// Create a slice of interface{} to hold pointers to values
		columns := make([]interface{}, len(cols))
		columnPointers := make([]interface{}, len(cols))
		for i := range columns {
			columnPointers[i] = &columns[i]
		}

		if err := rows.Scan(columnPointers...); err != nil {
			return nil, err
		}

		rowMap := make(map[string]interface{})
		for i, colName := range cols {
			val := columns[i]

			var finalVal interface{}
			if val == nil {
				finalVal = nil
			} else if b, ok := val.([]byte); ok {
				finalVal = string(b)
			} else {
				finalVal = val
			}

			if strings.Contains(colName, "___") {
				parts := strings.Split(colName, "___")
				currentMap := rowMap

				// Traverse the path
				for j := 0; j < len(parts)-1; j++ {
					part := parts[j]

					// Check if this part exists
					if _, ok := currentMap[part]; !ok {
						// It's a new nested object
						currentMap[part] = make(map[string]interface{})
					}

					// Move deeper
					// Note: This simple logic assumes 1:1 nesting for now during construction
					// We will fix the structure (Map vs Slice) in a post-processing step or be smarter here.
					// Being smarter here is hard because we are building the map field by field.
					// Let's keep building it as a map, and then wrap it if needed?
					// Or wrap it immediately?

					// If we wrap it immediately, subsequent fields (items___name) need to find the map INSIDE the slice.

					val := currentMap[part]
					if m, ok := val.(map[string]interface{}); ok {
						currentMap = m
					} else if s, ok := val.([]map[string]interface{}); ok {
						// Already a slice, use the last element (current row context)
						if len(s) > 0 {
							currentMap = s[len(s)-1]
						} else {
							// Should not happen
							newMap := make(map[string]interface{})
							currentMap[part] = append(s, newMap)
							currentMap = newMap
						}
					} else {
						// Should not happen
						break
					}
				}
				currentMap[parts[len(parts)-1]] = finalVal
			} else {
				rowMap[colName] = finalVal
			}
		}

		// Post-process the row to enforce Schema types (hasMany -> Slice)
		if schema != nil && rootTable != "" {
			h.enforceSliceTypes(rowMap, schema, rootTable)
		}

		results = append(results, rowMap)
	}

	return results, nil
}

func (h *Hydrator) enforceSliceTypes(data map[string]interface{}, schema *JSONQLSchema, tableName string) {
	tableDef, ok := schema.Tables[tableName]
	if !ok {
		return
	}

	for key, val := range data {
		// Check if this key is a relation
		if rel, ok := tableDef.Relations[key]; ok {
			// If it's hasMany, ensure it's a slice
			if rel.Type == "hasMany" {
				if m, ok := val.(map[string]interface{}); ok {
					// Convert map to slice of map
					data[key] = []map[string]interface{}{m}
					// Recurse
					h.enforceSliceTypes(m, schema, rel.Table)
				} else if s, ok := val.([]map[string]interface{}); ok {
					// Already a slice, recurse on elements
					for _, item := range s {
						h.enforceSliceTypes(item, schema, rel.Table)
					}
				}
			} else if rel.Type == "hasOne" || rel.Type == "belongsTo" {
				// Recurse
				if m, ok := val.(map[string]interface{}); ok {
					h.enforceSliceTypes(m, schema, rel.Table)
				}
			}
		}
	}
}
