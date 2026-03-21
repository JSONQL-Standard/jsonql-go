package jsonqlecho

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

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
// Handler creates a simple echo.HandlerFunc that extracts the table name from
// the URL path and handles the full JSONQL request lifecycle.
//
// Usage:
//
//	handler, _ := jsonqlecho.Handler(opts)
//	e := echo.New()
//	e.Any("/*", handler)
func Handler(opts jsonqlhttp.AdapterOptions) (echo.HandlerFunc, error) {
	adapter, err := jsonqlhttp.NewAdapter(opts)
	if err != nil {
		return nil, err
	}

	return func(c echo.Context) error {
		method := strings.ToUpper(c.Request().Method)
		if method != http.MethodPost && method != http.MethodGet &&
			method != http.MethodPatch && method != http.MethodDelete {
			return c.JSON(http.StatusMethodNotAllowed, map[string]interface{}{"error": "Method not allowed"})
		}

		tableName := strings.Trim(c.Request().URL.Path, "/")
		if tableName == "" || tableName == "favicon.ico" {
			return c.JSON(http.StatusBadRequest, map[string]interface{}{"error": "Table name required in URL"})
		}

		var queryBody map[string]interface{}

		if method == http.MethodPost || method == http.MethodPatch || method == http.MethodDelete {
			body, readErr := io.ReadAll(c.Request().Body)
			if readErr != nil {
				return c.JSON(http.StatusBadRequest, map[string]interface{}{"error": "Failed to read body"})
			}
			defer c.Request().Body.Close()
			if len(body) > 0 {
				if jsonErr := json.Unmarshal(body, &queryBody); jsonErr != nil {
					return c.JSON(http.StatusBadRequest, map[string]interface{}{"error": "Invalid JSON"})
				}
			} else {
				queryBody = make(map[string]interface{})
			}
		} else {
			if q := c.QueryParam("q"); q != "" {
				if jsonErr := json.Unmarshal([]byte(q), &queryBody); jsonErr != nil {
					return c.JSON(http.StatusBadRequest, map[string]interface{}{"error": "Invalid JSON in q parameter"})
				}
			} else {
				queryBody = make(map[string]interface{})
			}
		}

		queryBody = jsonqlhttp.InferMutationFromHTTP(method, queryBody)
		if _, ok := queryBody["op"]; ok {
			queryBody["from"] = tableName
		}

		resp, handleErr := adapter.Handle(queryBody, tableName, c.Request())
		if handleErr != nil {
			herr := jsonqlhttp.WrapError(handleErr)
			return c.JSON(herr.Status, map[string]interface{}{"error": herr.Message})
		}

		return c.JSON(resp.Status, map[string]interface{}{"data": resp.Data})
	}, nil
}