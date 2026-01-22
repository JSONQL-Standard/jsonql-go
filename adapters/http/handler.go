package jsonqlhttp

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/jsonql-standard/jsonql-go"
)

// HandlerOptions configuration for the JSONQL HTTP handler
type HandlerOptions struct {
	Driver        jsonql.Driver
	Dialect       string // "sqlite", "postgres", etc. Defaults to "sqlite" or Driver.Dialect()
	Schema        *jsonql.JSONQLSchema
	AllowedTables []string          // Whitelist of allowed tables
	TableMap      map[string]string // Alias -> RealTableName
}

// Handler is an HTTP handler for JSONQL requests
type Handler struct {
	adapter       *Adapter
	allowedTables map[string]bool
	tableMap      map[string]string
}

// NewHandler creates a new JSONQL HTTP handler
func NewHandler(opts HandlerOptions) (*Handler, error) {
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

	adapter, err := NewAdapter(AdapterOptions{
		Driver:  opts.Driver,
		Dialect: dialect,
		Schema:  opts.Schema,
	})
	if err != nil {
		return nil, err
	}

	return &Handler{
		adapter:       adapter,
		allowedTables: allowed,
		tableMap:      opts.TableMap,
	}, nil
}

// ServeHTTP handles the HTTP request
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 1. Read Body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var rawQuery map[string]interface{}
	if err := json.Unmarshal(body, &rawQuery); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// 3. Transpile
	// Note: In a real scenario, we need to know the table name.
	// The rawQuery keys are usually table names if it's a multi-table query,
	// or the URL path might specify the table.
	// For this simple adapter, let's assume the root key is the table name
	// OR we support the structure { "tableName": { ... } }

	// However, parser.Parse expects the query object itself (fields, filter, etc).
	// If the input is { "users": { "fields": [...] } }, parser.Parse might fail if it expects just the inner part.
	// Let's look at how parser.Parse is implemented.
	// It takes map[string]interface{}.

	// If the input is { "users": { ... } }, we need to extract "users" as table name and { ... } as query.
	// If the input is just { "fields": ... }, we need the table name from somewhere else (e.g. URL).

	// Let's support:
	// 1. POST /api/jsonql -> Body: { "users": { ... } }
	// 2. POST /api/users  -> Body: { ... }

	// For now, let's implement case 1 (Body contains table name key).

	if len(rawQuery) != 1 {
		http.Error(w, "Request body must contain exactly one root key (the table name)", http.StatusBadRequest)
		return
	}

	var tableName string
	var queryBody map[string]interface{}

	for k, v := range rawQuery {
		tableName = k
		if q, ok := v.(map[string]interface{}); ok {
			queryBody = q
		} else {
			http.Error(w, "Query body must be a JSON object", http.StatusBadRequest)
			return
		}
	}

	// Security: Whitelisting
	if len(h.allowedTables) > 0 {
		if !h.allowedTables[tableName] {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
	}

	// Security: Table Mapping
	if realName, ok := h.tableMap[tableName]; ok {
		tableName = realName
	}

	resp, err := h.adapter.Handle(queryBody, tableName, r)
	if err != nil {
		herr := WrapError(err)
		http.Error(w, herr.Message, herr.Status)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.Status)
	json.NewEncoder(w).Encode(resp.Data)
}
