package jsonqlgin

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	jsonqlhttp "github.com/jsonql-standard/jsonql-go/adapters/http"
)

// HandlerOptions configuration for the JSONQL Gin handler
type HandlerOptions struct {
	jsonqlhttp.AdapterOptions
	AllowedTables []string          // Whitelist of allowed tables
	TableMap      map[string]string // Alias -> RealTableName
}

// NewHandler creates a new JSONQL Gin handler
func NewHandler(opts HandlerOptions) (gin.HandlerFunc, error) {
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

		resp, err := adapter.Handle(queryBody, tableName, c.Request)
		if err != nil {
			herr := jsonqlhttp.WrapError(err)
			c.JSON(herr.Status, gin.H{"error": herr.Message})
			return
		}

		c.JSON(resp.Status, resp.Data)
	}
	return handler, nil
}

// Handler creates a simple gin.HandlerFunc that extracts the table name from
// the URL path and handles the full JSONQL request lifecycle.
//
// Usage:
//
//	handler, _ := jsonqlgin.Handler(opts.AdapterOptions)
//	r := gin.Default()
//	r.NoRoute(handler)
func Handler(opts jsonqlhttp.AdapterOptions) (gin.HandlerFunc, error) {
	adapter, err := jsonqlhttp.NewAdapter(opts)
	if err != nil {
		return nil, err
	}

	return func(c *gin.Context) {
		method := strings.ToUpper(c.Request.Method)
		if method != http.MethodPost && method != http.MethodGet &&
			method != http.MethodPatch && method != http.MethodDelete {
			c.JSON(http.StatusMethodNotAllowed, gin.H{"error": "Method not allowed"})
			return
		}

		tableName := strings.Trim(c.Request.URL.Path, "/")
		if tableName == "" || tableName == "favicon.ico" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Table name required in URL"})
			return
		}

		var queryBody map[string]interface{}

		if method == http.MethodPost || method == http.MethodPatch || method == http.MethodDelete {
			body, err := io.ReadAll(c.Request.Body)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read body"})
				return
			}
			defer c.Request.Body.Close()
			if len(body) > 0 {
				if err := json.Unmarshal(body, &queryBody); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON"})
					return
				}
			} else {
				queryBody = make(map[string]interface{})
			}
		} else {
			if q := c.Query("q"); q != "" {
				if err := json.Unmarshal([]byte(q), &queryBody); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON in q parameter"})
					return
				}
			} else {
				queryBody = make(map[string]interface{})
			}
		}

		queryBody = jsonqlhttp.InferMutationFromHTTP(method, queryBody)
		if _, ok := queryBody["op"]; ok {
			queryBody["from"] = tableName
		}

		resp, err := adapter.Handle(queryBody, tableName, c.Request)
		if err != nil {
			herr := jsonqlhttp.WrapError(err)
			c.JSON(herr.Status, gin.H{"error": herr.Message})
			return
		}

		c.JSON(resp.Status, gin.H{"data": resp.Data})
	}, nil
}
