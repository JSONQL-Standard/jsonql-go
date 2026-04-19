package jsonqlhttp

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/jsonql-standard/jsonql-go"
)

type SchemaResolver func(r *http.Request, raw map[string]interface{}, tableName string) *jsonql.JSONQLSchema
type QueryHook func(query map[string]interface{}, r *http.Request) (map[string]interface{}, error)
type QueryResultHook func(result []map[string]interface{}, r *http.Request) ([]map[string]interface{}, error)
type MutationHook func(mutation map[string]interface{}, r *http.Request) (map[string]interface{}, error)
type MutationResultHook func(result Response, mutation map[string]interface{}, r *http.Request) (Response, error)
type MutationStatusResolver func(op string, r *http.Request, mutation map[string]interface{}) int
type ParserOptionsResolver func(r *http.Request) *jsonql.ParserOptions

type HandlerError struct {
	Status  int
	Message string
	Details string
	Code    string
}

func (e *HandlerError) Error() string {
	return e.Message
}

type AdapterOptions struct {
	Driver        jsonql.Driver
	Dialect       string
	Schema        *jsonql.JSONQLSchema
	SchemaResolve SchemaResolver
	Logger        jsonql.Logger
	Debug         bool

	BeforeQuery QueryHook
	AfterQuery  QueryResultHook

	BeforeCreate MutationHook
	AfterCreate  MutationResultHook
	BeforeUpdate MutationHook
	AfterUpdate  MutationResultHook
	BeforeDelete MutationHook
	AfterDelete  MutationResultHook

	MutationStatus       MutationStatusResolver
	ParserOptions        *jsonql.ParserOptions
	ParserOptionsResolve ParserOptionsResolver
}

type Response struct {
	Status int
	Data   interface{}
}

type Adapter struct {
	parser      *jsonql.Parser
	transpiler  *jsonql.Transpiler
	driver      jsonql.Driver
	hydrator    *jsonql.Hydrator
	engine      *jsonql.Engine
	dialectName string
	options     AdapterOptions
	logger      jsonql.Logger
}

func NewAdapter(opts AdapterOptions) (*Adapter, error) {
	if opts.Driver == nil {
		return nil, fmt.Errorf("driver is required")
	}

	dialect := opts.Dialect
	if dialect == "" {
		dialect = opts.Driver.Dialect()
	}
	if opts.MutationStatus == nil {
		opts.MutationStatus = func(op string, r *http.Request, mutation map[string]interface{}) int {
			return http.StatusOK
		}
	}

	// Resolve logger
	var logger jsonql.Logger
	if opts.Logger != nil {
		logger = opts.Logger
	} else if opts.Debug {
		logger = jsonql.NewConsoleLogger(jsonql.LogLevelDebug)
	} else {
		logger = jsonql.NoOpLogger{}
	}

	parser := jsonql.NewParser()
	if opts.ParserOptions != nil {
		parser = jsonql.NewParserWithOptions(opts.ParserOptions)
	}

	// Build Engine for core pipeline delegation
	engineBuilder := jsonql.NewEngineBuilder().
		Dialect(dialect).
		WithDriver(opts.Driver).
		WithLogger(logger)
	if opts.Schema != nil {
		engineBuilder.Schema(opts.Schema)
	}
	if opts.ParserOptions != nil {
		engineBuilder.ParserOpts(opts.ParserOptions)
	}
	engine := engineBuilder.Build()

	return &Adapter{
		parser:      parser,
		transpiler:  jsonql.NewTranspilerWithLogger(dialect, logger),
		driver:      opts.Driver,
		hydrator:    jsonql.NewHydratorWithLogger(logger),
		engine:      engine,
		dialectName: dialect,
		options:     opts,
		logger:      logger,
	}, nil
}

// ServeHTTP implements http.Handler, providing a zero-config request handler.
//
// It handles the full request lifecycle:
//   - Extracts table name from URL path
//   - Parses request body (POST/PATCH/DELETE) or query string (GET)
//   - Infers mutation op from HTTP method (POST→create, PATCH→update, DELETE→delete)
//   - Calls Handle() and serializes the response as JSON
//
// Usage:
//
//	adapter, _ := jsonqlhttp.NewAdapter(opts)
//	http.Handle("/", adapter)
//	// or
//	mux.Handle("/", adapter)
func (a *Adapter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	method := strings.ToUpper(r.Method)
	if method != http.MethodPost && method != http.MethodGet &&
		method != http.MethodPatch && method != http.MethodDelete {
		WriteJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"error": "Method not allowed"})
		return
	}

	tableName := strings.Trim(r.URL.Path, "/")
	if tableName == "" || tableName == "favicon.ico" {
		WriteJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "Table name required in URL"})
		return
	}

	var queryBody map[string]interface{}

	if method == http.MethodPost || method == http.MethodPatch || method == http.MethodDelete {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			WriteJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "Failed to read body"})
			return
		}
		defer r.Body.Close()
		if len(body) > 0 {
			if err := json.Unmarshal(body, &queryBody); err != nil {
				WriteJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "Invalid JSON"})
				return
			}
		} else {
			queryBody = make(map[string]interface{})
		}
	} else {
		// GET: parse from ?q= or build from query params
		if q := r.URL.Query().Get("q"); q != "" {
			if err := json.Unmarshal([]byte(q), &queryBody); err != nil {
				WriteJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "Invalid JSON in q parameter"})
				return
			}
		} else {
			queryBody = make(map[string]interface{})
		}
	}

	// Infer mutation op from HTTP method when body has no explicit "op".
	// POST with "data" key → create; POST without "data" → query.
	// PATCH/PUT → update; DELETE → delete.
	if _, ok := queryBody["op"]; !ok {
		switch method {
		case http.MethodPost:
			if _, hasData := queryBody["data"]; hasData {
				queryBody["op"] = "create"
			}
		case http.MethodPatch, http.MethodPut:
			queryBody["op"] = "update"
		case http.MethodDelete:
			queryBody["op"] = "delete"
		}
	}
	if _, ok := queryBody["op"]; ok {
		queryBody["from"] = tableName
	}

	resp, err := a.Handle(queryBody, tableName, r)
	if err != nil {
		herr := WrapError(err)
		errResp := map[string]interface{}{"error": herr.Message}
		if herr.Code != "" {
			errResp["error_code"] = herr.Code
		}
		if herr.Details != "" {
			errResp["details"] = herr.Details
		}
		WriteJSON(w, herr.Status, errResp)
		return
	}

	WriteJSON(w, resp.Status, map[string]interface{}{"data": resp.Data})
}

// Handler returns the adapter as an http.Handler (convenience alias).
func (a *Adapter) Handler() http.Handler {
	return a
}

// WriteJSON writes a JSON response with the given status code.
func WriteJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// WriteError writes a JSON error response: {"error": message}.
func WriteError(w http.ResponseWriter, message string, status int) {
	WriteJSON(w, status, map[string]interface{}{"error": message})
}

// GetIDFromQuery extracts the "id" query parameter from a request.
// Returns the parsed value (int if numeric, string otherwise) and whether it was present.
func GetIDFromQuery(r *http.Request) (interface{}, bool) {
	val := r.URL.Query().Get("id")
	if val == "" {
		return nil, false
	}
	if parsed, err := strconv.Atoi(val); err == nil {
		return parsed, true
	}
	return val, true
}

// BuildRESTMutation converts a REST-style HTTP request into a JSONQL mutation.
//
// Mapping:
//   - POST  with {"data": {...}}        → {"op": "create", "data": {...}}
//   - PATCH with {"data": {...}} + ?id=X → {"op": "update", "where": {"id": X}, "patch": {...}}
//   - DELETE with ?id=X                  → {"op": "delete", "where": {"id": X}}
//
// If the body already has an "op" field, or the method is GET, the body is returned unchanged.
func BuildRESTMutation(r *http.Request, method string, body map[string]interface{}) map[string]interface{} {
	if body == nil {
		body = map[string]interface{}{}
	}
	method = strings.ToUpper(method)
	if method == http.MethodPost {
		if data, ok := body["data"]; ok {
			return map[string]interface{}{"op": "create", "data": data}
		}
	}
	if method == http.MethodPatch || method == http.MethodPut {
		id, ok := GetIDFromQuery(r)
		if !ok {
			return body
		}
		data, ok := body["data"].(map[string]interface{})
		if !ok {
			return body
		}
		return map[string]interface{}{
			"op":    "update",
			"where": map[string]interface{}{"id": id},
			"patch": data,
		}
	}
	if method == http.MethodDelete {
		id, ok := GetIDFromQuery(r)
		if !ok {
			return body
		}
		return map[string]interface{}{
			"op":    "delete",
			"where": map[string]interface{}{"id": id},
		}
	}
	return body
}

func (a *Adapter) Handle(raw map[string]interface{}, tableName string, r *http.Request) (Response, error) {
	if raw == nil {
		return Response{}, &HandlerError{Status: http.StatusBadRequest, Message: "Invalid JSONQL Query"}
	}
	if op, ok := raw["op"].(string); ok {
		a.logger.Debug("[JSONQL] %s mutation on %s", op, tableName)
		return a.handleMutation(op, raw, tableName, r)
	}
	a.logger.Debug("[JSONQL] query on %s", tableName)
	return a.handleQuery(raw, tableName, r)
}

func (a *Adapter) handleQuery(raw map[string]interface{}, tableName string, r *http.Request) (Response, error) {
	query := raw
	if a.options.BeforeQuery != nil {
		updated, err := a.options.BeforeQuery(query, r)
		if err != nil {
			return Response{}, err
		}
		if updated != nil {
			query = updated
		}
	}

	// Resolve schema and engine for this request
	engine := a.engineForRequest(r, query, tableName)

	// Delegate core pipeline to Engine: parse → validate → transpile → execute → hydrate
	result, err := engine.Execute(r.Context(), query, tableName)
	if err != nil {
		return Response{}, err
	}

	data := result.Data
	if a.options.AfterQuery != nil {
		updated, err := a.options.AfterQuery(data, r)
		if err != nil {
			return Response{}, err
		}
		if updated != nil {
			data = updated
		}
	}

	return Response{Status: http.StatusOK, Data: data}, nil
}

// engineForRequest returns the pre-built engine or builds a per-request engine
// when per-request schema resolution or parser options are needed.
func (a *Adapter) engineForRequest(r *http.Request, raw map[string]interface{}, tableName string) *jsonql.Engine {
	schema := a.resolveSchema(r, raw, tableName)
	needsCustomEngine := a.options.SchemaResolve != nil || a.options.ParserOptionsResolve != nil

	if !needsCustomEngine && schema == a.options.Schema {
		return a.engine
	}

	// Build a per-request engine (cheap — just assembles pointers, reuses driver)
	builder := jsonql.NewEngineBuilder().
		Dialect(a.dialectName).
		WithDriver(a.driver).
		WithLogger(a.logger)

	// Only pass schema if it has fields for this table (skip empty validation)
	if schema != nil && hasTableFields(schema, tableName) {
		builder.Schema(schema)
	}

	if a.options.ParserOptionsResolve != nil {
		if opts := a.options.ParserOptionsResolve(r); opts != nil {
			builder.ParserOpts(opts)
		}
	} else if a.options.ParserOptions != nil {
		builder.ParserOpts(a.options.ParserOptions)
	}

	return builder.Build()
}

func (a *Adapter) handleMutation(op string, raw map[string]interface{}, tableName string, r *http.Request) (Response, error) {
	status := a.options.MutationStatus(op, r, raw)
	mutation := raw

	switch strings.ToLower(op) {
	case "create":
		if a.options.BeforeCreate != nil {
			updated, err := a.options.BeforeCreate(mutation, r)
			if err != nil {
				return Response{}, err
			}
			if updated != nil {
				mutation = updated
			}
		}
		data, ok := mutation["data"].(map[string]interface{})
		if !ok {
			return Response{}, &HandlerError{Status: http.StatusBadRequest, Message: "Invalid JSONQL Query"}
		}
		if _, ok := data["id"]; !ok {
			nextID, err := getNextID(r.Context(), a.driver, tableName, a.transpiler)
			if err == nil {
				data["id"] = nextID
			}
		}

		result, err := a.transpiler.TranspileInsert(tableName, data)
		if err != nil {
			return Response{}, &HandlerError{Status: http.StatusBadRequest, Message: fmt.Sprintf("Transpile error: %v", err)}
		}
		if _, err := a.driver.Execute(r.Context(), result.SQL, result.Args); err != nil {
			return Response{}, &HandlerError{Status: http.StatusInternalServerError, Message: fmt.Sprintf("Database error: %v", err)}
		}

		// For non-RETURNING dialects, return the data map directly
		var respData interface{} = data
		if a.transpiler.Dialect.SupportsReturning() {
			// Could parse RETURNING rows here for richer response
			respData = data
		}
		resp := Response{Status: status, Data: respData}
		if a.options.AfterCreate != nil {
			return a.options.AfterCreate(resp, mutation, r)
		}
		return resp, nil
	case "update":
		if a.options.BeforeUpdate != nil {
			updated, err := a.options.BeforeUpdate(mutation, r)
			if err != nil {
				return Response{}, err
			}
			if updated != nil {
				mutation = updated
			}
		}
		patch, ok := mutation["patch"].(map[string]interface{})
		if !ok {
			return Response{}, &HandlerError{Status: http.StatusBadRequest, Message: "Invalid JSONQL Query"}
		}
		where, _ := mutation["where"].(map[string]interface{})

		result, err := a.transpiler.TranspileUpdate(tableName, patch, where)
		if err != nil {
			return Response{}, &HandlerError{Status: http.StatusBadRequest, Message: fmt.Sprintf("Transpile error: %v", err)}
		}
		if _, err := a.driver.Execute(r.Context(), result.SQL, result.Args); err != nil {
			return Response{}, &HandlerError{Status: http.StatusInternalServerError, Message: fmt.Sprintf("Database error: %v", err)}
		}
		resp := Response{Status: status, Data: []map[string]interface{}{}}
		if a.options.AfterUpdate != nil {
			return a.options.AfterUpdate(resp, mutation, r)
		}
		return resp, nil
	case "delete":
		if a.options.BeforeDelete != nil {
			updated, err := a.options.BeforeDelete(mutation, r)
			if err != nil {
				return Response{}, err
			}
			if updated != nil {
				mutation = updated
			}
		}
		where, _ := mutation["where"].(map[string]interface{})

		result, err := a.transpiler.TranspileDelete(tableName, where)
		if err != nil {
			return Response{}, &HandlerError{Status: http.StatusBadRequest, Message: fmt.Sprintf("Transpile error: %v", err)}
		}
		if _, err := a.driver.Execute(r.Context(), result.SQL, result.Args); err != nil {
			return Response{}, &HandlerError{Status: http.StatusInternalServerError, Message: fmt.Sprintf("Database error: %v", err)}
		}
		resp := Response{Status: status, Data: []map[string]interface{}{}}
		if a.options.AfterDelete != nil {
			return a.options.AfterDelete(resp, mutation, r)
		}
		return resp, nil
	default:
		return Response{}, &HandlerError{Status: http.StatusBadRequest, Message: "Invalid JSONQL Query"}
	}
}

func (a *Adapter) resolveSchema(r *http.Request, raw map[string]interface{}, tableName string) *jsonql.JSONQLSchema {
	if a.options.SchemaResolve != nil {
		return a.options.SchemaResolve(r, raw, tableName)
	}
	return a.options.Schema
}

func hasTableFields(schema *jsonql.JSONQLSchema, tableName string) bool {
	if schema == nil || schema.Tables == nil {
		return false
	}
	table, ok := schema.Tables[tableName]
	if !ok || table == nil {
		return false
	}
	return len(table.Fields) > 0
}

func extractIDFromWhere(where map[string]interface{}) (interface{}, bool) {
	if where == nil {
		return nil, false
	}
	raw, ok := where["id"]
	if !ok {
		return nil, false
	}
	if rawMap, ok := raw.(map[string]interface{}); ok {
		if eq, ok := rawMap["eq"]; ok {
			return eq, true
		}
	}
	return raw, true
}

// InferMutationFromHTTP infers the JSONQL mutation operation from the HTTP method.
// POST → create, PUT/PATCH → update, DELETE → delete.
func InferMutationFromHTTP(method string, body map[string]interface{}) map[string]interface{} {
	if body == nil {
		body = map[string]interface{}{}
	}
	// If op is already set, return as-is
	if _, ok := body["op"]; ok {
		return body
	}
	switch strings.ToUpper(method) {
	case "POST":
		body["op"] = "create"
	case "PUT", "PATCH":
		body["op"] = "update"
	case "DELETE":
		body["op"] = "delete"
	}
	return body
}

func getNextID(ctx context.Context, driver jsonql.Driver, tableName string, transpiler *jsonql.Transpiler) (int64, error) {
	query := fmt.Sprintf("SELECT COALESCE(MAX(%s), 0) + 1 AS next_id FROM %s",
		transpiler.Dialect.QuoteIdentifier("id"),
		transpiler.Dialect.QuoteIdentifier(tableName))
	rows, err := driver.Query(ctx, query, nil)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var nextID sql.NullInt64
	if rows.Next() {
		if err := rows.Scan(&nextID); err != nil {
			return 0, err
		}
	}
	if !nextID.Valid {
		return 0, fmt.Errorf("invalid next id")
	}
	return nextID.Int64, nil
}

func WrapError(err error) *HandlerError {
	if err == nil {
		return nil
	}
	if herr, ok := err.(*HandlerError); ok {
		return herr
	}
	// Extract error code and determine status from JSONQL typed errors
	type coder interface {
		Code() string
	}
	code := ""
	status := http.StatusBadRequest
	if c, ok := err.(coder); ok {
		code = c.Code()
		// Execution errors are server-side (5xx)
		if code == "EXECUTION_ERROR" {
			status = http.StatusInternalServerError
		}
	}
	msg := err.Error()
	details := ""
	// Wrap parse/validation errors with a generic message, put specifics in details
	if code == "PARSE_ERROR" || code == "VALIDATION_ERROR" {
		details = msg
		msg = "Invalid JSONQL Query"
	}
	return &HandlerError{Status: status, Message: msg, Details: details, Code: code}
}

func EnsureContext(r *http.Request) context.Context {
	if r == nil {
		return context.Background()
	}
	return r.Context()
}
