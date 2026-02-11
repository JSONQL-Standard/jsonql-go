package jsonql

import (
	"encoding/json"
	"errors"
)

// StringOrArray handles fields that can be a single string or an array of strings
type StringOrArray []string

func (s *StringOrArray) UnmarshalJSON(data []byte) error {
	var single string
	if err := json.Unmarshal(data, &single); err == nil {
		*s = []string{single}
		return nil
	}
	var multi []string
	if err := json.Unmarshal(data, &multi); err == nil {
		*s = multi
		return nil
	}
	return errors.New("field must be a string or array of strings")
}

// JSONQLQuery represents the structure of a JSONQL query
type JSONQLQuery struct {
	Version   string                 `json:"version,omitempty"`
	From      string                 `json:"from,omitempty"`
	Fields    []string               `json:"fields,omitempty"`
	Where     map[string]interface{} `json:"where,omitempty"`
	Sort      StringOrArray          `json:"sort,omitempty"`
	Limit     *int                   `json:"limit,omitempty"`
	Offset    *int                   `json:"offset,omitempty"`
	Aggregate map[string]interface{} `json:"aggregate,omitempty"`
	GroupBy   []string               `json:"groupBy,omitempty"`
	Include   map[string]interface{} `json:"include,omitempty"`
	Distinct  *DistinctOption        `json:"distinct,omitempty"`
}

// DistinctOption represents a "distinct" clause: true (all fields) or a list of specific fields.
type DistinctOption struct {
	All    bool     // true when distinct: true
	Fields []string // non-nil when distinct: ["field1", "field2"]
}

func (d *DistinctOption) UnmarshalJSON(data []byte) error {
	var b bool
	if err := json.Unmarshal(data, &b); err == nil {
		d.All = b
		return nil
	}
	var fields []string
	if err := json.Unmarshal(data, &fields); err == nil {
		d.Fields = fields
		return nil
	}
	return errors.New("distinct must be a boolean or array of strings")
}

// JSONQLMutation represents a mutation operation (create, update, delete)
type JSONQLMutation struct {
	Op    string                 `json:"op"`
	Data  map[string]interface{} `json:"data,omitempty"`
	Patch map[string]interface{} `json:"patch,omitempty"`
	Where map[string]interface{} `json:"where,omitempty"`
}

// ValidationError represents a single validation error with a code and message
type ValidationError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Path    string `json:"path,omitempty"`
}

// ValidationResult contains the outcome of validation
type ValidationResult struct {
	Valid  bool              `json:"valid"`
	Errors []ValidationError `json:"errors,omitempty"`
}
