package jsonqlecho

import (
	"fmt"
	"net/http"

	jsonqlhttp "github.com/jsonql-standard/jsonql-go/adapters/http"
	"github.com/labstack/echo/v4"
)

// HandlerOptions configuration for the JSONQL Echo handler
type HandlerOptions struct {
	jsonqlhttp.AdapterOptions
	AllowedTables []string          // Whitelist of allowed tables
	TableMap      map[string]string // Alias -> RealTableName
}

// NewHandler creates a new JSONQL Echo handler
func NewHandler(opts HandlerOptions) (echo.HandlerFunc, error) {
	if opts.Driver == nil {
		return nil, fmt.Errorf("driver is required")
	}

	allowed := make(map[string]bool)
	for _, t := range opts.AllowedTables {
		allowed[t] = true
	}

	adapter, err := jsonqlhttp.NewAdapter(opts.AdapterOptions)
	if err != nil {
		return nil, err
	}

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

		// Security: Whitelisting
		if len(allowed) > 0 {
			if !allowed[tableName] {
				return c.JSON(http.StatusForbidden, map[string]string{"error": "Forbidden"})
			}
		}

		// Security: Table Mapping
		if realName, ok := opts.TableMap[tableName]; ok {
			tableName = realName
		}

		resp, err := adapter.Handle(queryBody, tableName, c.Request())
		if err != nil {
			herr := jsonqlhttp.WrapError(err)
			return c.JSON(herr.Status, map[string]string{"error": herr.Message})
		}

		return c.JSON(resp.Status, resp.Data)
	}, nil
}
