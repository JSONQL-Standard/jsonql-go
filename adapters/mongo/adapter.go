// Package jsonqlmongo provides an HTTP adapter for JSONQL with MongoDB.
//
// It mirrors the SQL adapter (adapters/http) but uses MongoTranspiler and
// a *mongo.Database, giving MongoDB users the same zero-config ServeHTTP
// experience that SQL users enjoy.
//
// Usage:
//
//	db := jsonqlmongo.MustConnect("mongodb://localhost:27017", "mydb")
//	adapter, _ := jsonqlmongo.NewAdapter(jsonqlmongo.AdapterOptions{
//	    Database: db,
//	    Schema:   schema,
//	})
//	http.Handle("/", adapter)
package jsonqlmongo

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/jsonql-standard/jsonql-go"
	jsonqlhttp "github.com/jsonql-standard/jsonql-go/adapters/http"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ---------------------------------------------------------------------------
// Option types (compatible signatures with adapters/http)
// ---------------------------------------------------------------------------

// SchemaResolver dynamically resolves the schema for a request.
type SchemaResolver func(r *http.Request, raw map[string]interface{}, tableName string) *jsonql.JSONQLSchema

// QueryHook runs before a query is parsed.
type QueryHook func(query map[string]interface{}, r *http.Request) (map[string]interface{}, error)

// QueryResultHook runs after query results are fetched.
type QueryResultHook func(result []map[string]interface{}, r *http.Request) ([]map[string]interface{}, error)

// MutationHook runs before a mutation is executed.
type MutationHook func(mutation map[string]interface{}, r *http.Request) (map[string]interface{}, error)

// MutationResultHook runs after a mutation is executed.
type MutationResultHook func(result jsonqlhttp.Response, mutation map[string]interface{}, r *http.Request) (jsonqlhttp.Response, error)

// MutationStatusResolver determines the HTTP status for a mutation response.
type MutationStatusResolver func(op string, r *http.Request, mutation map[string]interface{}) int

// ParserOptionsResolver dynamically resolves parser options per request.
type ParserOptionsResolver func(r *http.Request) *jsonql.ParserOptions

// AdapterOptions configures the MongoDB adapter.
type AdapterOptions struct {
	Database *mongo.Database
	Schema   *jsonql.JSONQLSchema

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

// Adapter handles JSONQL HTTP requests against MongoDB.
type Adapter struct {
	db         *mongo.Database
	transpiler *jsonql.MongoTranspiler
	parser     *jsonql.Parser
	options    AdapterOptions
	logger     jsonql.Logger
}

// ---------------------------------------------------------------------------
// Connection helpers
// ---------------------------------------------------------------------------

// Connect creates a *mongo.Database from a URI and database name.
func Connect(uri, dbName string) (*mongo.Database, error) {
	client, err := mongo.Connect(context.Background(), options.Client().ApplyURI(uri))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to mongodb: %w", err)
	}
	if err := client.Ping(context.Background(), nil); err != nil {
		return nil, fmt.Errorf("failed to ping mongodb: %w", err)
	}
	return client.Database(dbName), nil
}

// MustConnect is like Connect but panics on error.
func MustConnect(uri, dbName string) *mongo.Database {
	db, err := Connect(uri, dbName)
	if err != nil {
		panic(err)
	}
	return db
}

// ---------------------------------------------------------------------------
// Constructor
// ---------------------------------------------------------------------------

// NewAdapter creates a new MongoDB adapter.
func NewAdapter(opts AdapterOptions) (*Adapter, error) {
	if opts.Database == nil {
		return nil, fmt.Errorf("Database is required")
	}

	// Default mutation status
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

	return &Adapter{
		db:         opts.Database,
		transpiler: jsonql.NewMongoTranspiler(),
		parser:     parser,
		options:    opts,
		logger:     logger,
	}, nil
}

// ---------------------------------------------------------------------------
// http.Handler — zero-config request handler
// ---------------------------------------------------------------------------

// ServeHTTP implements http.Handler, providing a zero-config request handler
// for MongoDB. It mirrors the behaviour of the SQL adapter's ServeHTTP.
func (a *Adapter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	method := strings.ToUpper(r.Method)
	if method != http.MethodPost && method != http.MethodGet &&
		method != http.MethodPatch && method != http.MethodDelete {
		jsonqlhttp.WriteJSON(w, http.StatusMethodNotAllowed, map[string]interface{}{"error": "Method not allowed"})
		return
	}

	tableName := strings.Trim(r.URL.Path, "/")
	if tableName == "" || tableName == "favicon.ico" {
		jsonqlhttp.WriteJSON(w, http.StatusBadRequest, map[string]interface{}{"error": "Table name required in URL"})
		return
	}

	var queryBody map[string]interface{}

	if method == http.MethodPost || method == http.MethodPatch || method == http.MethodDelete {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			jsonqlhttp.WriteError(w, "Failed to read body", http.StatusBadRequest)
			return
		}
		defer r.Body.Close()
		if len(body) > 0 {
			if err := json.Unmarshal(body, &queryBody); err != nil {
				jsonqlhttp.WriteError(w, "Invalid JSON", http.StatusBadRequest)
				return
			}
		} else {
			queryBody = make(map[string]interface{})
		}
	} else {
		if q := r.URL.Query().Get("q"); q != "" {
			if err := json.Unmarshal([]byte(q), &queryBody); err != nil {
				jsonqlhttp.WriteError(w, "Invalid JSON in q parameter", http.StatusBadRequest)
				return
			}
		} else {
			queryBody = make(map[string]interface{})
		}
	}

	queryBody = jsonqlhttp.InferMutationFromHTTP(method, queryBody)

	// Validate version
	if v, ok := queryBody["version"]; ok {
		vs := fmt.Sprintf("%v", v)
		if vs != "1" && vs != "1.0" && vs != "1.1" {
			jsonqlhttp.WriteError(w, "Invalid JSONQL Query", http.StatusBadRequest)
			return
		}
	}
	// Validate fields
	if fields, ok := queryBody["fields"]; ok {
		if arr, ok := fields.([]interface{}); ok && len(arr) == 0 {
			jsonqlhttp.WriteError(w, "Invalid JSONQL Query", http.StatusBadRequest)
			return
		}
	}

	if _, ok := queryBody["op"]; ok {
		queryBody["from"] = tableName
	}

	resp, err := a.Handle(queryBody, tableName, r)
	if err != nil {
		herr := jsonqlhttp.WrapError(err)
		jsonqlhttp.WriteJSON(w, herr.Status, map[string]interface{}{"error": herr.Message})
		return
	}

	jsonqlhttp.WriteJSON(w, resp.Status, map[string]interface{}{"data": resp.Data})
}

// Handler returns the adapter as an http.Handler (convenience alias).
func (a *Adapter) Handler() http.Handler {
	return a
}

// ---------------------------------------------------------------------------
// Handle — lower-level entry point usable from any framework
// ---------------------------------------------------------------------------

// Handle processes a parsed JSONQL query body against MongoDB.
func (a *Adapter) Handle(raw map[string]interface{}, tableName string, r *http.Request) (jsonqlhttp.Response, error) {
	if raw == nil {
		return jsonqlhttp.Response{}, &jsonqlhttp.HandlerError{Status: http.StatusBadRequest, Message: "Invalid JSONQL Query"}
	}
	if op, ok := raw["op"].(string); ok {
		a.logger.Debug("[JSONQL-Mongo] %s mutation on %s", op, tableName)
		return a.handleMutation(op, raw, tableName, r)
	}
	a.logger.Debug("[JSONQL-Mongo] query on %s", tableName)
	return a.handleQuery(raw, tableName, r)
}

// ---------------------------------------------------------------------------
// Query
// ---------------------------------------------------------------------------

func (a *Adapter) handleQuery(raw map[string]interface{}, tableName string, r *http.Request) (jsonqlhttp.Response, error) {
	ctx := r.Context()
	query := raw

	// BeforeQuery hook
	if a.options.BeforeQuery != nil {
		updated, err := a.options.BeforeQuery(query, r)
		if err != nil {
			return jsonqlhttp.Response{}, err
		}
		if updated != nil {
			query = updated
		}
	}

	// Resolve schema
	schema := a.resolveSchema(r, query, tableName)
	validationSchema := schema
	if schema != nil && !hasCollectionFields(schema, tableName) {
		validationSchema = nil
	}

	// Per-request parser options
	parser := a.parser
	if a.options.ParserOptionsResolve != nil {
		if opts := a.options.ParserOptionsResolve(r); opts != nil {
			parser = jsonql.NewParserWithOptions(opts)
		}
	}

	parsed, err := parser.Parse(query, validationSchema, tableName)
	if err != nil {
		return jsonqlhttp.Response{}, &jsonqlhttp.HandlerError{Status: http.StatusBadRequest, Message: "Invalid JSONQL Query"}
	}

	mongoResult, err := a.transpiler.Transpile(parsed, tableName)
	if err != nil {
		return jsonqlhttp.Response{}, &jsonqlhttp.HandlerError{Status: http.StatusBadRequest, Message: fmt.Sprintf("Transpile error: %v", err)}
	}

	var results []map[string]interface{}
	if mongoResult.Operation == "aggregate" {
		results, err = a.executeAggregate(ctx, tableName, mongoResult.Pipeline)
	} else {
		results, err = a.executeFind(ctx, tableName, mongoResult, parsed)
	}
	if err != nil {
		return jsonqlhttp.Response{}, &jsonqlhttp.HandlerError{Status: http.StatusInternalServerError, Message: fmt.Sprintf("Database error: %v", err)}
	}

	results = stripMongoID(results)

	// AfterQuery hook
	if a.options.AfterQuery != nil {
		updated, err := a.options.AfterQuery(results, r)
		if err != nil {
			return jsonqlhttp.Response{}, err
		}
		if updated != nil {
			results = updated
		}
	}

	return jsonqlhttp.Response{Status: http.StatusOK, Data: results}, nil
}

// ---------------------------------------------------------------------------
// Mutations
// ---------------------------------------------------------------------------

func (a *Adapter) handleMutation(op string, raw map[string]interface{}, tableName string, r *http.Request) (jsonqlhttp.Response, error) {
	status := a.options.MutationStatus(op, r, raw)
	mutation := raw
	ctx := r.Context()

	switch strings.ToLower(op) {
	case "create":
		return a.handleCreate(ctx, r, tableName, mutation, status)
	case "update":
		return a.handleUpdate(ctx, r, tableName, mutation, status)
	case "delete":
		return a.handleDelete(ctx, r, tableName, mutation, status)
	default:
		return jsonqlhttp.Response{}, &jsonqlhttp.HandlerError{Status: http.StatusBadRequest, Message: "Invalid JSONQL Query"}
	}
}

func (a *Adapter) handleCreate(ctx context.Context, r *http.Request, tableName string, mutation map[string]interface{}, status int) (jsonqlhttp.Response, error) {
	// BeforeCreate hook
	if a.options.BeforeCreate != nil {
		updated, err := a.options.BeforeCreate(mutation, r)
		if err != nil {
			return jsonqlhttp.Response{}, err
		}
		if updated != nil {
			mutation = updated
		}
	}

	data, ok := mutation["data"].(map[string]interface{})
	if !ok {
		return jsonqlhttp.Response{}, &jsonqlhttp.HandlerError{Status: http.StatusBadRequest, Message: "Missing data for create"}
	}

	// Auto-generate ID if missing
	if _, ok := data["id"]; !ok {
		if nextID, err := a.getNextID(ctx, tableName); err == nil {
			data["id"] = nextID
		}
	}

	if _, err := a.db.Collection(tableName).InsertOne(ctx, data); err != nil {
		return jsonqlhttp.Response{}, &jsonqlhttp.HandlerError{Status: http.StatusInternalServerError, Message: fmt.Sprintf("Database error: %v", err)}
	}

	delete(data, "_id")
	resp := jsonqlhttp.Response{Status: status, Data: data}

	// AfterCreate hook
	if a.options.AfterCreate != nil {
		return a.options.AfterCreate(resp, mutation, r)
	}
	return resp, nil
}

func (a *Adapter) handleUpdate(ctx context.Context, r *http.Request, tableName string, mutation map[string]interface{}, status int) (jsonqlhttp.Response, error) {
	// BeforeUpdate hook
	if a.options.BeforeUpdate != nil {
		updated, err := a.options.BeforeUpdate(mutation, r)
		if err != nil {
			return jsonqlhttp.Response{}, err
		}
		if updated != nil {
			mutation = updated
		}
	}

	patch, ok := mutation["patch"].(map[string]interface{})
	if !ok {
		return jsonqlhttp.Response{}, &jsonqlhttp.HandlerError{Status: http.StatusBadRequest, Message: "Invalid JSONQL Query"}
	}
	where, _ := mutation["where"].(map[string]interface{})

	mongoResult, err := a.transpiler.TranspileUpdate(tableName, patch, where)
	if err != nil {
		return jsonqlhttp.Response{}, &jsonqlhttp.HandlerError{Status: http.StatusBadRequest, Message: fmt.Sprintf("Transpile error: %v", err)}
	}

	filter := toBSON(mongoResult.Filter)
	if _, err := a.db.Collection(tableName).UpdateMany(ctx, filter, toBSON(mongoResult.Update)); err != nil {
		return jsonqlhttp.Response{}, &jsonqlhttp.HandlerError{Status: http.StatusInternalServerError, Message: fmt.Sprintf("Database error: %v", err)}
	}

	// Re-query to return updated documents
	cursor, err := a.db.Collection(tableName).Find(ctx, filter)
	if err != nil {
		return jsonqlhttp.Response{}, &jsonqlhttp.HandlerError{Status: http.StatusInternalServerError, Message: fmt.Sprintf("Database error: %v", err)}
	}
	var results []map[string]interface{}
	if err := cursor.All(ctx, &results); err != nil {
		return jsonqlhttp.Response{}, &jsonqlhttp.HandlerError{Status: http.StatusInternalServerError, Message: fmt.Sprintf("Database error: %v", err)}
	}
	results = stripMongoID(results)

	resp := jsonqlhttp.Response{Status: status, Data: results}

	// AfterUpdate hook
	if a.options.AfterUpdate != nil {
		return a.options.AfterUpdate(resp, mutation, r)
	}
	return resp, nil
}

func (a *Adapter) handleDelete(ctx context.Context, r *http.Request, tableName string, mutation map[string]interface{}, status int) (jsonqlhttp.Response, error) {
	// BeforeDelete hook
	if a.options.BeforeDelete != nil {
		updated, err := a.options.BeforeDelete(mutation, r)
		if err != nil {
			return jsonqlhttp.Response{}, err
		}
		if updated != nil {
			mutation = updated
		}
	}

	where, _ := mutation["where"].(map[string]interface{})

	mongoResult, err := a.transpiler.TranspileDelete(tableName, where)
	if err != nil {
		return jsonqlhttp.Response{}, &jsonqlhttp.HandlerError{Status: http.StatusBadRequest, Message: fmt.Sprintf("Transpile error: %v", err)}
	}

	filter := toBSON(mongoResult.Filter)

	// Fetch documents before deleting (to return them)
	cursor, err := a.db.Collection(tableName).Find(ctx, filter)
	if err != nil {
		return jsonqlhttp.Response{}, &jsonqlhttp.HandlerError{Status: http.StatusInternalServerError, Message: fmt.Sprintf("Database error: %v", err)}
	}
	var deleted []map[string]interface{}
	if err := cursor.All(ctx, &deleted); err != nil {
		return jsonqlhttp.Response{}, &jsonqlhttp.HandlerError{Status: http.StatusInternalServerError, Message: fmt.Sprintf("Database error: %v", err)}
	}
	deleted = stripMongoID(deleted)

	if _, err := a.db.Collection(tableName).DeleteMany(ctx, filter); err != nil {
		return jsonqlhttp.Response{}, &jsonqlhttp.HandlerError{Status: http.StatusInternalServerError, Message: fmt.Sprintf("Database error: %v", err)}
	}

	resp := jsonqlhttp.Response{Status: status, Data: deleted}

	// AfterDelete hook
	if a.options.AfterDelete != nil {
		return a.options.AfterDelete(resp, mutation, r)
	}
	return resp, nil
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

func (a *Adapter) executeFind(ctx context.Context, collection string, result *jsonql.MongoResult, parsed *jsonql.JSONQLQuery) ([]map[string]interface{}, error) {
	findOpts := options.Find()
	if result.Projection != nil {
		findOpts.SetProjection(toBSON(result.Projection))
	}
	if result.Sort != nil {
		sortD := bson.D{}
		for _, s := range parsed.Sort {
			field := s
			order := 1
			if strings.HasPrefix(s, "-") {
				field = s[1:]
				order = -1
			}
			sortD = append(sortD, bson.E{Key: field, Value: order})
		}
		findOpts.SetSort(sortD)
	}
	if result.Skip > 0 {
		findOpts.SetSkip(result.Skip)
	}
	if result.Limit > 0 {
		findOpts.SetLimit(result.Limit)
	}

	cursor, err := a.db.Collection(collection).Find(ctx, toBSON(result.Filter), findOpts)
	if err != nil {
		return nil, err
	}
	var results []map[string]interface{}
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	return results, nil
}

func (a *Adapter) executeAggregate(ctx context.Context, collection string, pipeline []map[string]interface{}) ([]map[string]interface{}, error) {
	bsonPipeline := make([]bson.M, len(pipeline))
	for i, stage := range pipeline {
		bsonPipeline[i] = toBSON(stage)
	}
	cursor, err := a.db.Collection(collection).Aggregate(ctx, bsonPipeline)
	if err != nil {
		return nil, err
	}
	var results []map[string]interface{}
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	return results, nil
}

func (a *Adapter) resolveSchema(r *http.Request, raw map[string]interface{}, tableName string) *jsonql.JSONQLSchema {
	if a.options.SchemaResolve != nil {
		return a.options.SchemaResolve(r, raw, tableName)
	}
	return a.options.Schema
}

func (a *Adapter) getNextID(ctx context.Context, collection string) (int64, error) {
	findOpts := options.Find().
		SetSort(bson.M{"id": -1}).
		SetLimit(1).
		SetProjection(bson.M{"id": 1})
	cursor, err := a.db.Collection(collection).Find(ctx, bson.M{}, findOpts)
	if err != nil {
		return 1, nil
	}
	var results []map[string]interface{}
	if err := cursor.All(ctx, &results); err != nil {
		return 1, nil
	}
	if len(results) == 0 {
		return 1, nil
	}
	if id, ok := results[0]["id"]; ok {
		switch v := id.(type) {
		case float64:
			return int64(v) + 1, nil
		case int64:
			return v + 1, nil
		case int32:
			return int64(v) + 1, nil
		case int:
			return int64(v) + 1, nil
		}
	}
	return 1, nil
}

func hasCollectionFields(schema *jsonql.JSONQLSchema, name string) bool {
	if schema == nil || schema.Tables == nil {
		return false
	}
	table, ok := schema.Tables[name]
	if !ok || table == nil {
		return false
	}
	return len(table.Fields) > 0
}

func stripMongoID(docs []map[string]interface{}) []map[string]interface{} {
	if docs == nil {
		return []map[string]interface{}{}
	}
	for _, doc := range docs {
		delete(doc, "_id")
	}
	return docs
}

// GetIDFromPath is a convenience alias for jsonqlhttp.GetIDFromQuery.
var GetIDFromPath = jsonqlhttp.GetIDFromQuery

// idFromQuery extracts "id" from URL query params as int64.
func idFromQuery(r *http.Request) (int64, bool) {
	val := r.URL.Query().Get("id")
	if val == "" {
		return 0, false
	}
	if n, err := strconv.ParseInt(val, 10, 64); err == nil {
		return n, true
	}
	return 0, false
}

// toBSON converts a map[string]interface{} to bson.M recursively.
func toBSON(m map[string]interface{}) bson.M {
	if m == nil {
		return bson.M{}
	}
	result := bson.M{}
	for k, v := range m {
		switch val := v.(type) {
		case map[string]interface{}:
			result[k] = toBSON(val)
		case []interface{}:
			arr := make([]interface{}, len(val))
			for i, item := range val {
				if itemMap, ok := item.(map[string]interface{}); ok {
					arr[i] = toBSON(itemMap)
				} else {
					arr[i] = item
				}
			}
			result[k] = arr
		default:
			result[k] = v
		}
	}
	return result
}
