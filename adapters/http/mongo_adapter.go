package jsonqlhttp

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/jsonql-standard/jsonql-go"
)

// MongoAdapterOptions configures a MongoDB JSONQL adapter.
type MongoAdapterOptions struct {
	Driver jsonql.MongoDriver
	Schema *jsonql.JSONQLSchema
	Logger jsonql.Logger
	Debug  bool

	BeforeQuery QueryHook
	AfterQuery  QueryResultHook

	BeforeCreate MutationHook
	AfterCreate  MutationResultHook
	BeforeUpdate MutationHook
	AfterUpdate  MutationResultHook
	BeforeDelete MutationHook
	AfterDelete  MutationResultHook

	MutationStatus MutationStatusResolver
	ParserOptions  *jsonql.ParserOptions
}

// MongoAdapter handles JSONQL requests against MongoDB.
// It uses MongoTranspiler to convert queries to MongoDB operations
// and a MongoDriver to execute them.
type MongoAdapter struct {
	parser     *jsonql.Parser
	transpiler *jsonql.MongoTranspiler
	driver     jsonql.MongoDriver
	options    MongoAdapterOptions
	logger     jsonql.Logger
}

// NewMongoAdapter creates a new MongoDB adapter.
func NewMongoAdapter(opts MongoAdapterOptions) (*MongoAdapter, error) {
	if opts.Driver == nil {
		return nil, fmt.Errorf("mongodb driver is required")
	}

	if opts.MutationStatus == nil {
		opts.MutationStatus = func(op string, r *http.Request, mutation map[string]interface{}) int {
			return http.StatusOK
		}
	}

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

	return &MongoAdapter{
		parser:     parser,
		transpiler: jsonql.NewMongoTranspiler(),
		driver:     opts.Driver,
		options:    opts,
		logger:     logger,
	}, nil
}

// Handle processes a JSONQL request against MongoDB.
func (a *MongoAdapter) Handle(raw map[string]interface{}, collectionName string, r *http.Request) (Response, error) {
	if raw == nil {
		return Response{}, &HandlerError{Status: http.StatusBadRequest, Message: "Invalid JSONQL Query"}
	}
	if op, ok := raw["op"].(string); ok {
		a.logger.Debug("[JSONQL] MongoDB %s mutation on %s", op, collectionName)
		return a.handleMutation(op, raw, collectionName, r)
	}
	a.logger.Debug("[JSONQL] MongoDB query on %s", collectionName)
	return a.handleQuery(raw, collectionName, r)
}

func (a *MongoAdapter) handleQuery(raw map[string]interface{}, collectionName string, r *http.Request) (Response, error) {
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

	schema := a.options.Schema

	// Parse the raw query into JSONQLQuery
	parsedQuery, err := a.parser.Parse(query, schema, collectionName)
	if err != nil {
		return Response{}, &HandlerError{Status: http.StatusBadRequest, Message: "Invalid JSONQL Query"}
	}

	// Transpile to MongoResult
	result, err := a.transpiler.Transpile(parsedQuery, collectionName)
	if err != nil {
		return Response{}, &HandlerError{Status: http.StatusBadRequest, Message: fmt.Sprintf("Transpile error: %v", err)}
	}

	a.logger.Debug("[JSONQL] MongoDB op=%s collection=%s", result.Operation, result.Collection)

	// Execute against MongoDB
	rawResult, err := a.driver.Execute(r.Context(), result)
	if err != nil {
		return Response{}, &HandlerError{Status: http.StatusInternalServerError, Message: fmt.Sprintf("Database error: %v", err)}
	}

	// MongoDB returns maps directly — no SQL hydration needed
	data, ok := rawResult.([]map[string]interface{})
	if !ok {
		data = []map[string]interface{}{}
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

func (a *MongoAdapter) handleMutation(op string, raw map[string]interface{}, collectionName string, r *http.Request) (Response, error) {
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
		result, err := a.transpiler.TranspileInsert(collectionName, data)
		if err != nil {
			return Response{}, &HandlerError{Status: http.StatusBadRequest, Message: fmt.Sprintf("Transpile error: %v", err)}
		}
		rawResult, err := a.driver.Execute(r.Context(), result)
		if err != nil {
			return Response{}, &HandlerError{Status: http.StatusInternalServerError, Message: fmt.Sprintf("Database error: %v", err)}
		}
		resp := Response{Status: status, Data: rawResult}
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
		result, err := a.transpiler.TranspileUpdate(collectionName, patch, where)
		if err != nil {
			return Response{}, &HandlerError{Status: http.StatusBadRequest, Message: fmt.Sprintf("Transpile error: %v", err)}
		}
		rawResult, err := a.driver.Execute(r.Context(), result)
		if err != nil {
			return Response{}, &HandlerError{Status: http.StatusInternalServerError, Message: fmt.Sprintf("Database error: %v", err)}
		}
		resp := Response{Status: status, Data: rawResult}
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
		result, err := a.transpiler.TranspileDelete(collectionName, where)
		if err != nil {
			return Response{}, &HandlerError{Status: http.StatusBadRequest, Message: fmt.Sprintf("Transpile error: %v", err)}
		}
		rawResult, err := a.driver.Execute(r.Context(), result)
		if err != nil {
			return Response{}, &HandlerError{Status: http.StatusInternalServerError, Message: fmt.Sprintf("Database error: %v", err)}
		}
		resp := Response{Status: status, Data: rawResult}
		if a.options.AfterDelete != nil {
			return a.options.AfterDelete(resp, mutation, r)
		}
		return resp, nil

	default:
		return Response{}, &HandlerError{Status: http.StatusBadRequest, Message: "Invalid JSONQL Query"}
	}
}
