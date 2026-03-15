package jsonql

import (
	"database/sql"
	"strconv"
	"strings"
)

// Hydrator converts SQL rows into JSON-friendly structures
type Hydrator struct {
	Logger Logger
}

// NewHydrator creates a new Hydrator
func NewHydrator() *Hydrator {
	return &Hydrator{Logger: NoOpLogger{}}
}

// NewHydratorWithLogger creates a new Hydrator with a logger
func NewHydratorWithLogger(logger Logger) *Hydrator {
	if logger == nil {
		logger = NoOpLogger{}
	}
	return &Hydrator{Logger: logger}
}

// Hydrate converts sql.Rows into a slice of maps
func (h *Hydrator) Hydrate(rows *sql.Rows, schema *JSONQLSchema, rootTable string) ([]map[string]interface{}, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	rawRows := []map[string]interface{}{}

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
				s := string(b)
				if f, err := strconv.ParseFloat(s, 64); err == nil {
					finalVal = f
				} else {
					finalVal = s
				}
			} else {
				finalVal = val
			}

			if strings.Contains(colName, "__") {
				parts := strings.Split(colName, "__")
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

					// If we wrap it immediately, subsequent fields (items__name) need to find the map INSIDE the slice.

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

	h.Logger.Debug("[JSONQL] Hydrator: scanned %d raw rows, %d columns", len(rawRows), len(cols))

	if schema != nil && rootTable != "" {
		merged := h.MergeRows(rawRows, schema, rootTable)
		h.Logger.Debug("[JSONQL] Hydrator: merged to %d results", len(merged))
		return merged, nil
	}

	return rawRows, nil
}

// MergeRows groups rows by primary key and merges relationships
func (h *Hydrator) MergeRows(rows []map[string]interface{}, schema *JSONQLSchema, tableName string) []map[string]interface{} {
	if len(rows) == 0 {
		return []map[string]interface{}{}
	}

	// Determine primary key from schema (default to "id")
	pk := "id"
	if tableDef, ok := schema.Tables[tableName]; ok && tableDef.PrimaryKey != "" {
		pk = tableDef.PrimaryKey
	}

	// Group by primary key
	groups := make(map[interface{}][]map[string]interface{})
	var order []interface{} // To preserve order

	for _, row := range rows {
		id, ok := row[pk]
		if !ok || id == nil {
			// Fallback: use a unique counter for rows without a PK
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
				// Determine PK for the target relation table
				targetTable := relDef.Table
				if targetTable == "" {
					targetTable = relName
				}
				childPK := "id"
				if targetTableDef, ok := schema.Tables[targetTable]; ok && targetTableDef.PrimaryKey != "" {
					childPK = targetTableDef.PrimaryKey
				}
				for _, r := range groupRows {
					if val, ok := r[relName]; ok {
						if m, ok := val.(map[string]interface{}); ok {
							if h.isValidRow(m, childPK) {
								subRows = append(subRows, m)
							}
						} else if s, ok := val.([]map[string]interface{}); ok {
							for _, item := range s {
								if h.isValidRow(item, childPK) {
									subRows = append(subRows, item)
								}
							}
						}
					}
				}

				mergedSub := h.MergeRows(subRows, schema, targetTable)

				if relDef.Type == "hasMany" {
					if mergedSub == nil {
						merged[relName] = []map[string]interface{}{}
					} else {
						// Check if this is an aggregate result (single row, no ID, fields not in schema)
						isAggregate := false
						if len(mergedSub) == 1 {
							row := mergedSub[0]
							if _, hasPK := row[childPK]; !hasPK {
								if targetTableDef, ok := schema.Tables[targetTable]; ok {
									allFieldsUnknown := true
									for k := range row {
										if _, isField := targetTableDef.Fields[k]; isField {
											allFieldsUnknown = false
											break
										}
									}
									if allFieldsUnknown {
										isAggregate = true
									}
								}
							}
						}

						if isAggregate {
							merged[relName] = mergedSub[0]
						} else {
							merged[relName] = mergedSub
						}
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
		results = append(results, merged)
	}

	return results
}

// isValidRow checks if a row has valid data (non-nil PK or at least one non-nil field)
func (h *Hydrator) isValidRow(row map[string]interface{}, pk string) bool {
	if id, ok := row[pk]; ok {
		return id != nil
	}
	for _, v := range row {
		if v != nil {
			return true
		}
	}
	return false
}
