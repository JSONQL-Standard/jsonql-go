package jsonql

import (
	"context"
	"database/sql"
	"fmt"
)

// ExecuteFunc is the type for user-supplied SQL executor callbacks.
type ExecuteFunc func(ctx context.Context, sql string, args []interface{}) (*sql.Rows, error)

// Engine is the high-level facade that wires the full JSONQL pipeline:
// parse → validate → transpile → execute → hydrate.
//
// Create one with the builder:
//
//	engine := jsonql.NewEngineBuilder().
//	    Postgres().
//	    Schema(schema).
//	    Driver(driver).
//	    Build()
//
//	result, err := engine.Execute(ctx, raw, "users")
type Engine struct {
	parser     *Parser
	transpiler *Transpiler
	hydrator   *Hydrator
	schema     *JSONQLSchema
	driver     Driver
	executor   ExecuteFunc
	logger     Logger
}

// EngineResult wraps the output of an engine execution.
type EngineResult struct {
	Data       []map[string]interface{}
	IsMutation bool
}

// Execute runs the full pipeline: parse → validate → transpile → execute → hydrate.
func (e *Engine) Execute(ctx context.Context, raw map[string]interface{}, table string) (*EngineResult, error) {
	// Dispatch mutations before parsing (parser rejects mutation keys)
	if e.isMutation(raw) {
		return e.executeMutation(ctx, raw, table)
	}

	// 1. Parse
	query, err := e.parser.Parse(raw, e.schema, table)
	if err != nil {
		return nil, err
	}

	// 2. Validate (if schema is set and table has fields defined).
	// Skip validation for tables without explicit field definitions —
	// the schema may still be needed for relationship/join resolution.
	if e.schema != nil && e.hasTableFields(table) {
		validator := NewValidator(e.schema, table)
		if vErr := validator.Validate(query); vErr != nil {
			return nil, vErr
		}
	}

	return e.executeQuery(ctx, query, table)
}

func (e *Engine) executeQuery(ctx context.Context, query *JSONQLQuery, table string) (*EngineResult, error) {
	// 3. Transpile
	result, err := e.transpiler.Transpile(query, table, e.schema)
	if err != nil {
		return nil, err
	}
	e.logger.Debug("SQL: %s", result.SQL)
	e.logger.Debug("Params: %v", result.Args)

	// 4. Execute
	rows, err := e.exec(ctx, result.SQL, result.Args)
	if err != nil {
		return nil, &JsonQLExecutionError{Msg: err.Error(), Cause: err}
	}
	defer rows.Close()

	// 5. Hydrate
	data, err := e.hydrator.Hydrate(rows, e.schema, table)
	if err != nil {
		return nil, &JsonQLExecutionError{Msg: err.Error(), Cause: err}
	}

	return &EngineResult{Data: data, IsMutation: false}, nil
}

func (e *Engine) executeMutation(ctx context.Context, raw map[string]interface{}, table string) (*EngineResult, error) {
	var result *TranspileResult
	var err error

	if data, ok := raw["data"]; ok {
		dataMap, _ := data.(map[string]interface{})
		result, err = e.transpiler.TranspileInsert(table, dataMap)
	} else if patch, ok := raw["patch"]; ok {
		patchMap, _ := patch.(map[string]interface{})
		whereMap, _ := raw["where"].(map[string]interface{})
		result, err = e.transpiler.TranspileUpdate(table, patchMap, whereMap)
	} else if _, ok := raw["delete"]; ok {
		whereMap, _ := raw["where"].(map[string]interface{})
		result, err = e.transpiler.TranspileDelete(table, whereMap)
	} else {
		return nil, &JsonQLTranspileError{Msg: "unknown mutation type"}
	}

	if err != nil {
		return nil, err
	}

	e.logger.Debug("SQL: %s", result.SQL)
	e.logger.Debug("Params: %v", result.Args)

	rows, err := e.exec(ctx, result.SQL, result.Args)
	if err != nil {
		return nil, &JsonQLExecutionError{Msg: err.Error(), Cause: err}
	}
	defer rows.Close()

	data, err := e.hydrator.Hydrate(rows, nil, table)
	if err != nil {
		return nil, &JsonQLExecutionError{Msg: err.Error(), Cause: err}
	}

	return &EngineResult{Data: data, IsMutation: true}, nil
}

func (e *Engine) exec(ctx context.Context, sqlStr string, args []interface{}) (*sql.Rows, error) {
	if e.driver != nil {
		return e.driver.Query(ctx, sqlStr, args)
	}
	if e.executor != nil {
		return e.executor(ctx, sqlStr, args)
	}
	return nil, fmt.Errorf("no driver or executor configured")
}

func (e *Engine) hasTableFields(table string) bool {
	if e.schema == nil || e.schema.Tables == nil {
		return false
	}
	t, ok := e.schema.Tables[table]
	if !ok || t == nil {
		return false
	}
	return len(t.Fields) > 0
}

func (e *Engine) isMutation(raw map[string]interface{}) bool {
	_, hasData := raw["data"]
	_, hasPatch := raw["patch"]
	_, hasDelete := raw["delete"]
	return hasData || hasPatch || hasDelete
}

// EngineBuilder constructs an Engine with a fluent API.
type EngineBuilder struct {
	dialectName   string
	schema        *JSONQLSchema
	driver        Driver
	executor      ExecuteFunc
	logger        Logger
	parserOptions *ParserOptions
	debug         bool
}

// NewEngineBuilder creates a new engine builder with default settings.
func NewEngineBuilder() *EngineBuilder {
	return &EngineBuilder{dialectName: "sqlite"}
}

// Dialect sets the SQL dialect by name.
func (b *EngineBuilder) Dialect(name string) *EngineBuilder {
	b.dialectName = name
	return b
}

// Postgres sets the dialect to PostgreSQL.
func (b *EngineBuilder) Postgres() *EngineBuilder { return b.Dialect("postgres") }

// MySQL sets the dialect to MySQL.
func (b *EngineBuilder) MySQL() *EngineBuilder { return b.Dialect("mysql") }

// SQLite sets the dialect to SQLite.
func (b *EngineBuilder) SQLite() *EngineBuilder { return b.Dialect("sqlite") }

// MSSQL sets the dialect to Microsoft SQL Server.
func (b *EngineBuilder) MSSQL() *EngineBuilder { return b.Dialect("mssql") }

// Schema sets the JSONQL schema for validation and relationship resolution.
func (b *EngineBuilder) Schema(schema *JSONQLSchema) *EngineBuilder {
	b.schema = schema
	return b
}

// WithDriver sets the database driver for query execution.
func (b *EngineBuilder) WithDriver(driver Driver) *EngineBuilder {
	b.driver = driver
	if b.dialectName == "sqlite" || b.dialectName == "" {
		b.dialectName = driver.Dialect()
	}
	return b
}

// Executor sets a raw SQL executor function (alternative to Driver).
func (b *EngineBuilder) Executor(fn ExecuteFunc) *EngineBuilder {
	b.executor = fn
	return b
}

// WithLogger sets the logger.
func (b *EngineBuilder) WithLogger(logger Logger) *EngineBuilder {
	b.logger = logger
	return b
}

// ParserOpts sets parser security options.
func (b *EngineBuilder) ParserOpts(opts *ParserOptions) *EngineBuilder {
	b.parserOptions = opts
	return b
}

// Debug enables debug logging (uses ConsoleLogger if no logger is set).
func (b *EngineBuilder) Debug(enabled bool) *EngineBuilder {
	b.debug = enabled
	return b
}

// Build creates the Engine.
func (b *EngineBuilder) Build() *Engine {
	var logger Logger
	switch {
	case b.logger != nil:
		logger = b.logger
	case b.debug:
		logger = NewConsoleLogger(LogLevelDebug)
	default:
		logger = NoOpLogger{}
	}

	parser := NewParser()
	if b.parserOptions != nil {
		parser = NewParserWithOptions(b.parserOptions)
	}

	transpiler := NewTranspilerWithLogger(b.dialectName, logger)
	hydrator := NewHydratorWithLogger(logger)

	return &Engine{
		parser:     parser,
		transpiler: transpiler,
		hydrator:   hydrator,
		schema:     b.schema,
		driver:     b.driver,
		executor:   b.executor,
		logger:     logger,
	}
}
