package jsonql

// JSONQLQuery represents the structure of a JSONQL query
type JSONQLQuery struct {
	Version   string                 `json:"version,omitempty"`
	From      string                 `json:"from,omitempty"`
	Fields    []string               `json:"fields,omitempty"`
	Where     map[string]interface{} `json:"where,omitempty"`
	Sort      []string               `json:"sort,omitempty"`
	Limit     *int                   `json:"limit,omitempty"`
	Offset    *int                   `json:"offset,omitempty"`
	Aggregate map[string]interface{} `json:"aggregate,omitempty"`
}

// ValidationResult represents the result of a validation operation
type ValidationResult struct {
	Valid  bool     `json:"valid"`
	Errors []string `json:"errors,omitempty"`
}
