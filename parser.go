package jsonql

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
)

// Parser parses JSONQL queries
type Parser struct {
	// Configuration options
}

// NewParser creates a new JSONQL parser
func NewParser() *Parser {
	return &Parser{}
}

// Parse parses and validates a JSONQL query from a raw map
func (p *Parser) Parse(input map[string]interface{}) (*JSONQLQuery, error) {
	// Convert map to JSON bytes then to struct to leverage struct tags
	// This is a bit inefficient but safe. Optimization can come later.
	bytes, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal input: %w", err)
	}

	var query JSONQLQuery
	if err := json.Unmarshal(bytes, &query); err != nil {
		return nil, fmt.Errorf("failed to unmarshal into JSONQLQuery: %w", err)
	}

	if err := p.Validate(&query); err != nil {
		return nil, err
	}

	return &query, nil
}

// ParseJSON parses and validates a JSONQL query from a JSON string/bytes
func (p *Parser) ParseJSON(input []byte) (*JSONQLQuery, error) {
	var query JSONQLQuery
	if err := json.Unmarshal(input, &query); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	if err := p.Validate(&query); err != nil {
		return nil, err
	}

	return &query, nil
}

// Validate checks if the query is valid
func (p *Parser) Validate(query *JSONQLQuery) error {
	// Check version
	if query.Version != "" && query.Version != "1.0" && query.Version != "1.1" {
		return errors.New("Query version must be \"1.0\" or \"1.1\"")
	}

	// Validate Fields
	if len(query.Fields) > 0 {
		for _, f := range query.Fields {
			if !isValidIdentifier(f) {
				return fmt.Errorf("Invalid field name: %s", f)
			}
		}
	}

	// Validate Sort
	if len(query.Sort) > 0 {
		for _, s := range query.Sort {
			field := s
			if len(s) > 0 && s[0] == '-' {
				field = s[1:]
			}
			if !isValidIdentifier(field) {
				return fmt.Errorf("Invalid sort field: %s", s)
			}
		}
	}

	// Validate Aggregate
	if query.Aggregate != nil {
		for _, val := range query.Aggregate {
			if funcMap, ok := val.(map[string]interface{}); ok {
				// Check for valid functions: count, avg, sum, min, max
				validFuncs := map[string]bool{"count": true, "avg": true, "sum": true, "min": true, "max": true}
				for k := range funcMap {
					if !validFuncs[k] {
						return errors.New("Unknown aggregation function: " + k)
					}
				}
			}
		}
	}

	return nil
}

var identifierRegex = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

func isValidIdentifier(s string) bool {
	return identifierRegex.MatchString(s)
}
