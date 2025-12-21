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
}
