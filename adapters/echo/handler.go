package jsonqlecho

import (
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"
	"github.com/jsonql-standard/jsonql-go"
)

// HandlerOptions configuration for the JSONQL Echo handler
type HandlerOptions struct {
	Driver  jsonql.Driver
	Dialect string
}

// NewHandler creates a new JSONQL Echo handler
func NewHandler(opts HandlerOptions) (echo.HandlerFunc, error) {
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

	return func(c echo.Context) error {
		// 1. Read Body
		var rawQuery map[string]interface{}
		if err := c.Bind(&rawQuery); err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Invalid JSON"})
		}

		// 2. Parse
		// Support { "tableName": { ... } } structure
		if len(rawQuery) != 1 {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "Request body must contain exactly one root key (the table name)"})
		}

		var tableName string
		var queryBody map[string]interface{}

		for k, v := range rawQuery {
			tableName = k
			if q, ok := v.(map[string]interface{}); ok {
				queryBody = q
			} else {
				return c.JSON(http.StatusBadRequest, map[string]string{"error": "Query body must be a JSON object"})
			}
		}

		parsedQuery, err := parser.Parse(queryBody, nil, tableName)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("Parse error: %v", err)})
		}

		// 3. Transpile
		result, err := transpiler.Transpile(parsedQuery, tableName)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("Transpile error: %v", err)})
		}

		// 4. Execute
		rows, err := opts.Driver.Query(c.Request().Context(), result.SQL, result.Args)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("Database error: %v", err)})
		}
		defer rows.Close()

		// 5. Hydrate
		data, err := hydrator.Hydrate(rows)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("Hydration error: %v", err)})
		}

		// 6. Response
		return c.JSON(http.StatusOK, data)
	}, nil
}
