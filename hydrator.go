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
func (h *Hydrator) Hydrate(rows *sql.Rows) ([]map[string]interface{}, error) {
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
				for j := 0; j < len(parts)-1; j++ {
					part := parts[j]
					if _, ok := currentMap[part]; !ok {
						currentMap[part] = make(map[string]interface{})
					}
					if nextMap, ok := currentMap[part].(map[string]interface{}); ok {
						currentMap = nextMap
					} else {
						// Should not happen if query is well-formed
						break
					}
				}
				currentMap[parts[len(parts)-1]] = finalVal
			} else {
				rowMap[colName] = finalVal
			}
		}
		results = append(results, rowMap)
	}

	return results, nil
}
