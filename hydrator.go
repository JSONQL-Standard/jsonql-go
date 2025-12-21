package jsonql

import (
	"database/sql"
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
			
			// Handle nil
			if val == nil {
				rowMap[colName] = nil
				continue
			}

			// Handle []byte for strings/blobs
			if b, ok := val.([]byte); ok {
				// Try to unmarshal as JSON if it looks like JSON (object or array)
				// This is a simple heuristic. In a real app, we might use schema info.
				// For now, let's just treat as string unless we want to auto-expand JSON columns.
				// The TS SDK hydrator does nested object expansion if the query implies it.
				// For now, let's just return string.
				rowMap[colName] = string(b)
			} else {
				rowMap[colName] = val
			}
		}
		results = append(results, rowMap)
	}

	return results, nil
}
