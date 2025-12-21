package jsonqlgin

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jsonql-standard/jsonql-go"
)

// HandlerOptions configuration for the JSONQL Gin handler
type HandlerOptions struct {
	Driver  jsonql.Driver
	Dialect string
	Schema  *jsonql.JSONQLSchema
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

	parser := jsonql.NewParser()
	transpiler := jsonql.NewTranspiler(dialect)
	hydrator := jsonql.NewHydrator()

	return func(c *gin.Context) {
		// 1. Read Body
		var rawQuery map[string]interface{}
		if err := c.ShouldBindJSON(&rawQuery); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON"})
			return
		}

		// 2. Parse
		// Support { "tableName": { ... } } structure
		if len(rawQuery) != 1 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Request body must contain exactly one root key (the table name)"})
			return
		}

		var tableName string
		var queryBody map[string]interface{}

		for k, v := range rawQuery {
			tableName = k
			if q, ok := v.(map[string]interface{}); ok {
				queryBody = q
			} else {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Query body must be a JSON object"})
				return
			}
		}

		parsedQuery, err := parser.Parse(queryBody, opts.Schema, tableName)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Parse error: %v", err)})
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
		data, err := hydrator.Hydrate(rows)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Hydration error: %v", err)})
			return
		}

		// 6. Response
		c.JSON(http.StatusOK, data)
	}, nil
}
