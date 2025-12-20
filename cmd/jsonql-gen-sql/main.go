package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// SchemaDefinition represents the root of the JSONQL schema
type SchemaDefinition map[string]TableDefinition

// TableDefinition represents a table in the schema
type TableDefinition struct {
	Fields    map[string]FieldDefinition    `json:"fields"`
	Relations map[string]RelationDefinition `json:"relations,omitempty"`
}

// FieldDefinition represents a field in a table
type FieldDefinition struct {
	Type        string                `json:"type"`
	Required    bool                  `json:"required"`
	Nullable    bool                  `json:"nullable,omitempty"`
	AllowSelect bool                  `json:"allowSelect"`
	AllowFilter bool                  `json:"allowFilter"`
	AllowSort   bool                  `json:"allowSort"`
	Validation  *ValidationDefinition `json:"validation,omitempty"`
}

// ValidationDefinition represents validation rules
type ValidationDefinition struct {
	Min     *float64  `json:"min,omitempty"`
	Max     *float64  `json:"max,omitempty"`
	Pattern string    `json:"pattern,omitempty"`
	Enum    []interface{} `json:"enum,omitempty"`
}

// RelationDefinition represents a relationship
type RelationDefinition struct {
	Type   string `json:"type"` // "one", "many"
	Target string `json:"target"`
	From   string `json:"from"`
	To     string `json:"to"`
}

func main() {
	inputFile := flag.String("input", "", "Path to input SQL DDL file")
	outputFile := flag.String("output", "jsonql.schema.json", "Path to output JSON file")
	flag.Parse()

	if *inputFile == "" {
		fmt.Println("Please provide an input file using -input")
		os.Exit(1)
	}

	content, err := os.ReadFile(*inputFile)
	if err != nil {
		fmt.Printf("Error reading file: %v\n", err)
		os.Exit(1)
	}

	schema, err := parseSQL(string(content))
	if err != nil {
		fmt.Printf("Error parsing SQL: %v\n", err)
		os.Exit(1)
	}

	output, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		fmt.Printf("Error marshaling JSON: %v\n", err)
		os.Exit(1)
	}

	err = os.WriteFile(*outputFile, output, 0644)
	if err != nil {
		fmt.Printf("Error writing output: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Successfully generated schema to %s\n", *outputFile)
}

func parseSQL(sql string) (SchemaDefinition, error) {
	schema := make(SchemaDefinition)

	// Very basic regex for CREATE TABLE
	// This is a simplified parser and won't handle all SQL cases
	// CREATE TABLE table_name ( ... );
	reTable := regexp.MustCompile(`(?i)CREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?["` + "`" + `]?(\w+)["` + "`" + `]?\s*\(([\s\S]*?)\);`)
	
	matches := reTable.FindAllStringSubmatch(sql, -1)
	
	for _, match := range matches {
		tableName := match[1]
		body := match[2]
		
		tableDef := TableDefinition{
			Fields:    make(map[string]FieldDefinition),
			Relations: make(map[string]RelationDefinition),
		}
		
		// Split by comma, but be careful about commas in parentheses (like DECIMAL(10,2))
		// For simplicity, we'll split by newline first or just regex scan lines
		lines := strings.Split(body, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "--") {
				continue
			}
			
			// Remove trailing comma
			line = strings.TrimSuffix(line, ",")
			
			// Parse column definition
			// name type [constraints]
			parts := strings.Fields(line)
			if len(parts) < 2 {
				continue
			}
			
			// Skip keys/indexes for now
			upperPart := strings.ToUpper(parts[0])
			if upperPart == "PRIMARY" || upperPart == "KEY" || upperPart == "CONSTRAINT" || upperPart == "FOREIGN" || upperPart == "INDEX" || upperPart == "UNIQUE" {
				continue
			}
			
			colName := strings.Trim(parts[0], "\"`")
			sqlType := strings.ToUpper(parts[1])
			
			jsonType := mapSQLTypeToJSONQL(sqlType)
			
			fieldDef := FieldDefinition{
				Type:        jsonType,
				Required:    false, // Default to nullable (not required) in SQL
				Nullable:    true,
				AllowSelect: true,
				AllowFilter: true,
				AllowSort:   true,
			}
			
			// Check for NULL/NOT NULL
			fullLineUpper := strings.ToUpper(line)
			if strings.Contains(fullLineUpper, "NOT NULL") {
				fieldDef.Required = true
				fieldDef.Nullable = false
			} else if strings.Contains(fullLineUpper, "PRIMARY KEY") {
				// Primary keys are implicitly not null
				fieldDef.Required = true
				fieldDef.Nullable = false
			}
			
			tableDef.Fields[colName] = fieldDef
		}
		
		schema[tableName] = tableDef
	}
	
	return schema, nil
}

func mapSQLTypeToJSONQL(sqlType string) string {
	// Remove size like VARCHAR(255)
	if idx := strings.Index(sqlType, "("); idx != -1 {
		sqlType = sqlType[:idx]
	}
	
	switch sqlType {
	case "INT", "INTEGER", "SMALLINT", "BIGINT", "DECIMAL", "NUMERIC", "FLOAT", "DOUBLE", "REAL":
		return "number"
	case "VARCHAR", "CHAR", "TEXT", "LONGTEXT", "MEDIUMTEXT", "TINYTEXT":
		return "string"
	case "BOOLEAN", "BOOL", "TINYINT": // TINYINT(1) is often bool
		return "boolean"
	case "DATE", "DATETIME", "TIMESTAMP":
		return "date" // or string depending on spec, spec says "date"
	case "JSON", "JSONB":
		return "json"
	default:
		return "string"
	}
}
