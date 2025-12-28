package jsonql

import (
	"database/sql"
	"log"
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

	var rawRows []map[string]interface{}

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
		// We don't do it per row anymore, we do it after collecting all rows
		rawRows = append(rawRows, rowMap)
	}

	if schema != nil && rootTable != "" {
		return h.mergeRows(rawRows, schema, rootTable), nil
	}

	return rawRows, nil
}

// mergeRows groups rows by ID and merges relationships
func (h *Hydrator) mergeRows(rows []map[string]interface{}, schema *JSONQLSchema, tableName string) []map[string]interface{} {
	if len(rows) == 0 {
		return nil
	}

	// Group by ID
	groups := make(map[interface{}][]map[string]interface{})
	var order []interface{} // To preserve order

	for _, row := range rows {
		// We assume 'id' is the PK. If missing, we can't merge, so treat as unique.
		id, ok := row["id"]
		if !ok || id == nil {
			// If no ID, we can't group. Just append to results?
			// Or maybe this row IS the result (e.g. aggregate result without ID).
			// If we are in a recursion, this might be a problem.
			// Let's treat it as a unique group key.
			// Use the row itself as key? No, map key must be comparable.
			// Use index?
			// For now, let's just skip grouping for rows without ID and return them as is?
			// But we need to return a consistent slice.
			// Let's use a pointer to the row as a unique ID substitute?
			// Or just append to a "no-id" list.
			// For simplicity in this app: if no ID, don't merge.
			// But we must return []map.
			// Let's just return the rows as is if we can't find ID in the first row?
			// No, mixed content is possible.

			// Fallback: use a unique counter
			id = &row // Pointer as unique key
		}

		if _, exists := groups[id]; !exists {
			order = append(order, id)
		}
		groups[id] = append(groups[id], row)
	}

	var results []map[string]interface{}

	tableDef, hasSchema := schema.Tables[tableName]

	for _, id := range order {
		groupRows := groups[id]
		baseRow := groupRows[0]

		merged := make(map[string]interface{})

		// Copy non-relation fields from baseRow
		for k, v := range baseRow {
			isRelation := false
			if hasSchema {
				if _, ok := tableDef.Relations[k]; ok {
					isRelation = true
				}
			}
			if !isRelation {
				merged[k] = v
			}
		}

		// Handle Relations
		if hasSchema {
			for relName, relDef := range tableDef.Relations {
				// Check if this relation is present in any of the rows
				// If not present in the raw rows, it means it wasn't requested in the query
				relationPresent := false
				for _, r := range groupRows {
					if _, ok := r[relName]; ok {
						relationPresent = true
						break
					}
				}

				// Heuristic to match TS behavior:
				// If specific fields were selected (subset), TS excludes relations.
				// If all fields were selected (default), TS includes relations as null.
				// We check if all defined fields are present in the baseRow.
				allColumnsSelected := true
				for fieldName := range tableDef.Fields {
					if _, ok := baseRow[fieldName]; !ok {
						allColumnsSelected = false
						break
					}
				}

				if !relationPresent && !allColumnsSelected {
					continue
				}
				var subRows []map[string]interface{}
				for _, r := range groupRows {
					if val, ok := r[relName]; ok {
						if m, ok := val.(map[string]interface{}); ok {
							if h.isValidRow(m) {
								subRows = append(subRows, m)
							}
						} else if s, ok := val.([]map[string]interface{}); ok {
							for _, item := range s {
								if h.isValidRow(item) {
									subRows = append(subRows, item)
								}
							}
						}
					}
				}

				targetTable := relDef.Table
				if targetTable == "" {
					targetTable = relName
				}

				mergedSub := h.mergeRows(subRows, schema, targetTable)

				if relDef.Type == "hasMany" {
					if mergedSub == nil {
						merged[relName] = []map[string]interface{}{}
					} else {
						merged[relName] = mergedSub
					}
				} else {
					// hasOne
					if len(mergedSub) > 0 {
						merged[relName] = mergedSub[0]
					} else {
						merged[relName] = nil
					}
				}
			}
		}
		log.Printf("Merged row: %+v\n", merged)
		results = append(results, merged)
	}

	return results
}

// isValidRow checks if a row has valid data (non-nil ID or at least one non-nil field)
func (h *Hydrator) isValidRow(row map[string]interface{}) bool {
	if id, ok := row["id"]; ok {
		return id != nil
	}
	for _, v := range row {
		if v != nil {
			return true
		}
	}
	return false
}
