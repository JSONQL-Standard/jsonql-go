package jsonql

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
)

// ParserOptions configures security and validation limits for the parser
type ParserOptions struct {
	// MaxNestingDepth limits the depth of nested includes (0 = unlimited)
	MaxNestingDepth int
	// MaxLimit caps the maximum value of the limit field (0 = unlimited)
	MaxLimit int
	// AllowedFields restricts which field names can appear in queries (nil = all allowed)
	AllowedFields []string
	// AllowedIncludes restricts which relation names can be included (nil = all allowed)
	AllowedIncludes []string
}

// Parser parses JSONQL queries
type Parser struct {
	Options *ParserOptions
}

// NewParser creates a new JSONQL parser with default options
func NewParser() *Parser {
	return &Parser{}
}

// NewParserWithOptions creates a new JSONQL parser with the given options
func NewParserWithOptions(opts *ParserOptions) *Parser {
	return &Parser{Options: opts}
}

// Parse parses and validates a JSONQL query from a raw map
func (p *Parser) Parse(input map[string]interface{}, schema *JSONQLSchema, table string) (*JSONQLQuery, error) {
	// Pre-validation...
	if fields, ok := input["fields"]; ok {
		if fieldsArr, ok := fields.([]interface{}); ok && len(fieldsArr) == 0 {
			return nil, errors.New("Fields array cannot be empty")
		}
	}

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

	if schema != nil {
		v := NewValidator(schema, table)
		if err := v.Validate(&query); err != nil {
			return nil, err
		}
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

// Validate checks if the query is valid, applying parser options
func (p *Parser) Validate(query *JSONQLQuery) error {
	// Check version
	if query.Version != "" && query.Version != "1.0" && query.Version != "1.1" {
		return errors.New("Query version must be \"1.0\" or \"1.1\"")
	}

	// Apply parser options
	if p.Options != nil {
		// MaxLimit enforcement
		if p.Options.MaxLimit > 0 && query.Limit != nil && *query.Limit > p.Options.MaxLimit {
			return fmt.Errorf("limit %d exceeds maximum allowed limit of %d", *query.Limit, p.Options.MaxLimit)
		}

		// MaxNestingDepth enforcement
		if p.Options.MaxNestingDepth > 0 && len(query.Include) > 0 {
			depth := calculateIncludeDepth(query.Include)
			if depth > p.Options.MaxNestingDepth {
				return fmt.Errorf("include nesting depth %d exceeds maximum allowed depth of %d", depth, p.Options.MaxNestingDepth)
			}
		}

		// AllowedFields enforcement
		if len(p.Options.AllowedFields) > 0 {
			allowed := make(map[string]bool, len(p.Options.AllowedFields))
			for _, f := range p.Options.AllowedFields {
				allowed[f] = true
			}
			for _, f := range query.Fields {
				if !allowed[f] {
					return fmt.Errorf("field '%s' is not in the allowed fields list", f)
				}
			}
		}

		// AllowedIncludes enforcement
		if len(p.Options.AllowedIncludes) > 0 {
			allowed := make(map[string]bool, len(p.Options.AllowedIncludes))
			for _, r := range p.Options.AllowedIncludes {
				allowed[r] = true
			}
			for rel := range query.Include {
				if !allowed[rel] {
					return fmt.Errorf("include '%s' is not in the allowed includes list", rel)
				}
			}
		}
	}

	// Validate Fields
	if len(query.Fields) > 0 {
		for _, f := range query.Fields {
			if f == "*" {
				continue
			}
			if !isValidIdentifier(f) {
				return fmt.Errorf("Invalid field name: %s", f)
			}
		}
	}

	if query.Limit != nil && *query.Limit < 0 {
		return errors.New("limit must be a non-negative number")
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

// calculateIncludeDepth computes the maximum depth of nested includes
func calculateIncludeDepth(include map[string]interface{}) int {
	maxDepth := 1
	for _, val := range include {
		if subMap, ok := val.(map[string]interface{}); ok {
			if nestedInclude, ok := subMap["include"].(map[string]interface{}); ok {
				depth := 1 + calculateIncludeDepth(nestedInclude)
				if depth > maxDepth {
					maxDepth = depth
				}
			}
		}
	}
	return maxDepth
}

var identifierRegex = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

func isValidIdentifier(s string) bool {
	return identifierRegex.MatchString(s)
}
