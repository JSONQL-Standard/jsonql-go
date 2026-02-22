package jsonqlhttp

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
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
	parser     *jsonql.Parser
	transpiler *jsonql.Transpiler
	driver     jsonql.Driver
	hydrator   *jsonql.Hydrator
	options    AdapterOptions
	logger     jsonql.Logger
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

	return &Adapter{
		parser:     parser,
		transpiler: jsonql.NewTranspilerWithLogger(dialect, logger),
		driver:     opts.Driver,
		hydrator:   jsonql.NewHydratorWithLogger(logger),
		options:    opts,
		logger:     logger,
	}, nil
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

	schema := a.resolveSchema(r, query, tableName)
	validationSchema := schema
	if schema != nil && !hasTableFields(schema, tableName) {
		validationSchema = nil
	}

	// Per-request parser options
	parser := a.parser
	if a.options.ParserOptionsResolve != nil {
		if opts := a.options.ParserOptionsResolve(r); opts != nil {
			parser = jsonql.NewParserWithOptions(opts)
		}
	}

	parsedQuery, err := parser.Parse(query, validationSchema, tableName)
	if err != nil {
		return Response{}, &HandlerError{Status: http.StatusBadRequest, Message: "Invalid JSONQL Query"}
	}

	result, err := a.transpiler.Transpile(parsedQuery, tableName, schema)
	if err != nil {
		return Response{}, &HandlerError{Status: http.StatusBadRequest, Message: fmt.Sprintf("Transpile error: %v", err)}
	}

	rows, err := a.driver.Query(r.Context(), result.SQL, result.Args)
	if err != nil {
		return Response{}, &HandlerError{Status: http.StatusInternalServerError, Message: fmt.Sprintf("Database error: %v", err)}
	}
	defer rows.Close()

	data, err := a.hydrator.Hydrate(rows, schema, tableName)
	if err != nil {
		return Response{}, &HandlerError{Status: http.StatusInternalServerError, Message: fmt.Sprintf("Hydration error: %v", err)}
	}

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
	return &HandlerError{Status: http.StatusBadRequest, Message: err.Error()}
}

func EnsureContext(r *http.Request) context.Context {
	if r == nil {
		return context.Background()
	}
	return r.Context()
}
