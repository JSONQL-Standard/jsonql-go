package jsonqlgin

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jsonql-standard/jsonql-go"
)

// HandlerOptions configuration for the JSONQL Gin handler
type HandlerOptions struct {
	Driver        jsonql.Driver
	Dialect       string
	Schema        *jsonql.JSONQLSchema
	AllowedTables []string          // Whitelist of allowed tables
	TableMap      map[string]string // Alias -> RealTableName
}

// NewHandler creates a new JSONQL Gin handler
func NewHandler(opts HandlerOptions) (gin.HandlerFunc, error) {
	if opts.Driver == nil {
		return nil, fmt.Errorf("driver is required")
	}

	dialect := opts.Dialect
	if dialect == "" {
		dialect = opts.Driver.Dialect()
	}

	allowed := make(map[string]bool)
	for _, t := range opts.AllowedTables {
		allowed[t] = true
	}

	parser := jsonql.NewParser()
	transpiler := jsonql.NewTranspiler(dialect)
	hydrator := jsonql.NewHydrator()

	handler := func(c *gin.Context) {
		// 0. Check for resource param (RESTful style)
		resourceParam := c.Param("resource")

		// 1. Read Body
		var rawQuery map[string]interface{}
		// If GET, try to parse 'q' query param
		if c.Request.Method == "GET" {
			q := c.Query("q")
			if q == "" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Missing 'q' query parameter"})
				return
			}
			if err := json.Unmarshal([]byte(q), &rawQuery); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON in 'q' parameter"})
				return
			}
		} else {
			if err := c.ShouldBindJSON(&rawQuery); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON"})
				return
			}
		}

		// 2. Parse
		var tableName string
		var queryBody map[string]interface{}

		if resourceParam != "" {
			tableName = resourceParam
			queryBody = rawQuery
		} else {
			// Check for standard spec format: { "from": "table", ... }
			if fromVal, ok := rawQuery["from"]; ok {
				if fromStr, ok := fromVal.(string); ok {
					tableName = fromStr
					queryBody = rawQuery
				} else {
					c.JSON(http.StatusBadRequest, gin.H{"error": "'from' field must be a string"})
					return
				}
			} else {
				// Fallback to adapter style: { "tableName": { ... } }
				if len(rawQuery) != 1 {
					c.JSON(http.StatusBadRequest, gin.H{"error": "Request body must contain 'from' field or exactly one root key (the table name)"})
					return
				}

				for k, v := range rawQuery {
					tableName = k
					if q, ok := v.(map[string]interface{}); ok {
						queryBody = q
					} else {
						c.JSON(http.StatusBadRequest, gin.H{"error": "Query body must be a JSON object"})
						return
					}
				}
			}
		}

		// Security: Whitelisting
		if len(allowed) > 0 {
			if !allowed[tableName] {
				c.JSON(http.StatusForbidden, gin.H{"error": "Forbidden"})
				return
			}
		}

		// Security: Table Mapping
		if realName, ok := opts.TableMap[tableName]; ok {
			tableName = realName
		}

		parsedQuery, err := parser.Parse(queryBody, opts.Schema, tableName)
		if err != nil {
			// Return standard error message for compliance tests
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSONQL Query"})
			return
		}

		// 3. Transpile
		result, err := transpiler.Transpile(parsedQuery, tableName, opts.Schema)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Transpile error: %v", err)})
			return
		}

		// 4. Execute
		rows, err := opts.Driver.Query(c.Request.Context(), result.SQL, result.Args)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Database error: %v", err)})
			return
		}
		defer rows.Close()

		// 5. Hydrate
		data, err := hydrator.Hydrate(rows, opts.Schema, tableName)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Hydration error: %v", err)})
			return
		}

		// 6. Response
		c.JSON(http.StatusOK, data)
	}
	return handler, nil
}
