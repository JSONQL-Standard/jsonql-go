# jsonql-go

The official Go SDK for **JSONQL**.

[![CI](https://github.com/JSONQL-Standard/jsonql-go/actions/workflows/ci.yml/badge.svg)](https://github.com/JSONQL-Standard/jsonql-go/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/jsonql-standard/jsonql-go.svg)](https://pkg.go.dev/github.com/jsonql-standard/jsonql-go)
[![Go Version](https://img.shields.io/badge/go-%3E%3D1.23-blue)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

| | |
|---|---|
| **Module** | `github.com/jsonql-standard/jsonql-go` |
| **Go** | ≥ 1.23 |
| **Docs** | [pkg.go.dev](https://pkg.go.dev/github.com/jsonql-standard/jsonql-go) |

**JSONQL** is a secure, lightweight, and polyglot JSON-based query language for filtering, sorting, pagination, field selection, and mutations in RESTful APIs.

## Features

- **JSONQL v1.1 Parser** — parse and validate incoming JSON queries and mutations
- **Query Builder** — fluent API with condition helpers (`Eq`, `Gt`, `And`, `Or`, etc.)
- **Mutation Builder** — fluent API for create / update / delete
- **SQL Transpiler** — convert parsed queries → parameterized SQL (PostgreSQL, MySQL, SQLite, MSSQL)
- **MongoDB Transpiler** — convert parsed queries → MongoDB aggregation pipelines
- **Schema Validation** — permission checking and field-level validation
- **Result Hydrator** — flatten SQL JOIN rows into nested JSON trees
- **Driver Factory** — `CreateDriver()` with auto-config for Postgres, MySQL, SQLite, MSSQL
- **Lifecycle Hooks** — `BeforeQuery`, `AfterCreate`, `BeforeUpdate`, etc.
- **Condition Helpers** — `Eq`, `Gt`, `Contains`, `And`, `Or`, `Not`, etc.
- **net/http Adapter** — implements `http.Handler` with parse-only or full-lifecycle execution
- **Gin Adapter** — handler with parse-only or full-lifecycle execution
- **Echo Adapter** — handler with parse-only or full-lifecycle execution
- **MongoDB Adapter** — full MongoDB CRUD via `http.Handler`

## Installation

```bash
go get github.com/jsonql-standard/jsonql-go
```

Database drivers are imported for side effects — import only what you need:

```go
import (
    _ "github.com/jsonql-standard/jsonql-go/drivers/postgres" // PostgreSQL
    _ "github.com/jsonql-standard/jsonql-go/drivers/mysql"    // MySQL
    _ "github.com/jsonql-standard/jsonql-go/drivers/sqlite"   // SQLite
    _ "github.com/jsonql-standard/jsonql-go/drivers/mssql"    // MSSQL
)
```

## Quick Start

A working JSONQL API in under 20 lines:

```go
// main.go
package main

import (
    "log"

    "github.com/gin-gonic/gin"
    jsonql "github.com/jsonql-standard/jsonql-go"
    jsonqlgin "github.com/jsonql-standard/jsonql-go/adapters/gin"
    jsonqlhttp "github.com/jsonql-standard/jsonql-go/adapters/http"
    _ "github.com/jsonql-standard/jsonql-go/drivers/postgres"
)

func main() {
    driver, _ := jsonql.CreateDriver("postgres") // reads DB_DSN from env
    defer driver.Close()

    handler, _ := jsonqlgin.Handler(jsonqlhttp.AdapterOptions{
        Driver: driver,                          // dialect auto-detected
        Schema: jsonql.MustLoadSchema("schema.json"), // or define inline
    })

    r := gin.Default()
    r.NoRoute(handler) // handles GET/POST/PATCH/DELETE on /:table
    log.Println("JSONQL API → http://localhost:8080")
    r.Run(":8080")
}
```

<details>
<summary>schema.json</summary>

```json
{
  "tables": {
    "users": {
      "fields": {
        "id":   { "type": "number" },
        "name": { "type": "string" },
        "age":  { "type": "number" }
      }
    }
  }
}
```
</details>

> **Prefer inline?** Replace `MustLoadSchema(...)` with a `&jsonql.JSONQLSchema{...}` literal — see [Schema Validation](#schema-validation).

```bash
export DB_DSN="postgresql://user:pass@localhost:5432/mydb?sslmode=disable"
go run main.go
# JSONQL API → http://localhost:8080
```

```bash
curl -s -X POST http://localhost:8080/users \
  -H "Content-Type: application/json" \
  -d '{"fields":["id","name"],"where":{"age":{"gt":18}},"sort":["name"],"limit":10}'
```

```json
{
  "data": [
    { "id": 1, "name": "Alice" },
    { "id": 2, "name": "Bob" }
  ]
}
```

## Builders

### Query Builder

```go
import (
    jsonql "github.com/jsonql-standard/jsonql-go"
    "github.com/jsonql-standard/jsonql-go/builder"
)

query := builder.New().
    From("users").
    Select("id", "name", "email").
    Where(builder.And(
        builder.Field("status", builder.Eq("active")),
        builder.Field("age", builder.Gt(18)),
    )).
    OrderBy("name", "-age").
    Limit(10).
    Build()
```

### Mutation Builder

```go
import "github.com/jsonql-standard/jsonql-go/builder"

// Create
mutation, _ := builder.NewMutation().
    Create("users", map[string]interface{}{"name": "Alice", "age": 30}).
    Build()

// Update
mutation, _ = builder.NewMutation().
    Update("users", map[string]interface{}{"name": "Alice Smith"}).
    Where(builder.Eq(1)).
    Build()

// Delete
mutation, _ = builder.NewMutation().
    Delete("users").
    Where(map[string]interface{}{"id": builder.Eq(1)}).
    Build()
```

## Transpilers

### SQL Transpiler

```go
import jsonql "github.com/jsonql-standard/jsonql-go"

transpiler := jsonql.NewTranspiler("postgres")
result, err := transpiler.Transpile(query, "users", schema)
fmt.Println(result.SQL)
// SELECT "users"."id", "users"."name" FROM "users" WHERE "users"."status" = $1 AND "users"."age" > $2 ...
fmt.Println(result.Args) // ["active", 18]
```

### MongoDB Transpiler

```go
import jsonql "github.com/jsonql-standard/jsonql-go"

transpiler := jsonql.NewMongoTranspiler()
result, err := transpiler.Transpile(query, "users")
// result.Collection = "users"
// result.Operation = "find"
// result.Filter = {"status": "active", "age": {"$gt": 18}}
```

## Schema Validation

```go
import jsonql "github.com/jsonql-standard/jsonql-go"

schema := &jsonql.JSONQLSchema{
    Tables: map[string]*jsonql.JSONQLTable{
        "users": {
            Fields: map[string]*jsonql.JSONQLField{
                "id":       {Type: "number"},
                "name":     {Type: "string"},
                "password": {Type: "string", AllowFilter: boolPtr(false)}, // blocked
            },
        },
    },
}

validator := jsonql.NewValidator(schema, "users")
err := validator.Validate(query) // returns error if query violates schema

result := validator.ValidateAll(query) // returns ValidationResult with all errors
// result.Valid, result.Errors
```

## Result Hydrator

```go
import jsonql "github.com/jsonql-standard/jsonql-go"

hydrator := jsonql.NewHydrator()

// Flatten SQL JOIN rows into nested JSON
// rows with "posts__id", "posts__title" columns → nested "posts" array
results, err := hydrator.Hydrate(sqlRows, schema, "users")
// [{"id": 1, "name": "Alice", "posts": [{"id": 10, "title": "Hello"}, ...]}]
```

## Framework Adapters

### net/http

```go
import (
    jsonql "github.com/jsonql-standard/jsonql-go"
    jsonqlhttp "github.com/jsonql-standard/jsonql-go/adapters/http"
    _ "github.com/jsonql-standard/jsonql-go/drivers/postgres"
)

driver, _ := jsonql.CreateDriver("postgres")
adapter, _ := jsonqlhttp.NewAdapter(jsonqlhttp.AdapterOptions{
    Driver: driver,
    Schema: jsonql.MustLoadSchema("schema.json"),
})

http.Handle("/", adapter) // implements http.Handler
http.ListenAndServe(":8080", nil)
```

### Gin

See [Quick Start](#quick-start) for the full Gin example.

### Echo

```go
import (
    "github.com/labstack/echo/v4"
    jsonql "github.com/jsonql-standard/jsonql-go"
    jsonqlecho "github.com/jsonql-standard/jsonql-go/adapters/echo"
    jsonqlhttp "github.com/jsonql-standard/jsonql-go/adapters/http"
    _ "github.com/jsonql-standard/jsonql-go/drivers/postgres"
)

driver, _ := jsonql.CreateDriver("postgres")
handler, _ := jsonqlecho.NewHandler(jsonqlecho.HandlerOptions{
    AdapterOptions: jsonqlhttp.AdapterOptions{
        Driver: driver,
        Schema: jsonql.MustLoadSchema("schema.json"),
    },
})

e := echo.New()
e.Any("/:table", handler)
e.Start(":8080")
```

### MongoDB

```go
import (
    jsonql "github.com/jsonql-standard/jsonql-go"
    jsonqlmongo "github.com/jsonql-standard/jsonql-go/adapters/mongo"
)

db := jsonqlmongo.MustConnect("mongodb://localhost:27017", "mydb")
adapter, _ := jsonqlmongo.NewAdapter(jsonqlmongo.AdapterOptions{
    Database: db,
    Schema:   schema,
})

http.Handle("/", adapter) // implements http.Handler
```

## Core API

| Package | Export | Purpose |
|---------|--------|---------|
| `jsonql-go` | `Parser` | Parse & validate incoming JSON |
| `jsonql-go` | `Transpiler` | Convert parsed query → parameterized SQL |
| `jsonql-go` | `MongoTranspiler` | Convert parsed query → MongoDB pipeline |
| `jsonql-go` | `Validator` | Schema-based permission checking |
| `jsonql-go` | `Hydrator` | Flatten SQL joins → nested JSON |
| `jsonql-go` | `CreateDriver` | Factory for database drivers |
| `jsonql-go` | `LoadSchema` | Load schema from JSON file |
| `builder` | `QueryBuilder` | Fluent query construction |
| `builder` | `MutationBuilder` | Fluent mutation construction |
| `adapters/http` | `Adapter` | `http.Handler` implementation |
| `adapters/gin` | `Handler` | Gin handler function |
| `adapters/echo` | `NewHandler` | Echo handler function |
| `adapters/mongo` | `Adapter` | MongoDB `http.Handler` |

## Supported Dialects

| Dialect    | Placeholder | Quoting      | RETURNING |
|------------|-------------|--------------|-----------|
| `postgres` | `$1, $2`    | `"col"`      | ✅        |
| `mysql`    | `?, ?`      | `` `col` ``  | ❌        |
| `sqlite`   | `?, ?`      | `"col"`      | ❌        |
| `mssql`    | `@p1, @p2`  | `[col]`      | ❌        |

## Condition Helpers

```go
import "github.com/jsonql-standard/jsonql-go/builder"

builder.Eq(value)          // {"eq": value}
builder.Neq(value)         // {"neq": value}
builder.Gt(value)          // {"gt": value}
builder.Gte(value)         // {"gte": value}
builder.Lt(value)          // {"lt": value}
builder.Lte(value)         // {"lte": value}
builder.In(values...)      // {"in": [...]}
builder.Nin(values...)     // {"nin": [...]}
builder.Like(pattern)      // {"like": pattern}
builder.Contains(value)    // {"contains": value}
builder.StartsWith(value)  // {"starts": value}
builder.EndsWith(value)    // {"ends": value}
builder.Field(name, cond)  // {name: cond}
builder.And(conditions...) // {"and": [...]}
builder.Or(conditions...)  // {"or": [...]}
builder.Not(condition)     // {"not": condition}
```

## Error Hierarchy

```
JsonQLError (interface)
├── JsonQLValidationError   (Code: VALIDATION_ERROR)
├── JsonQLTranspileError    (Code: TRANSPILE_ERROR)
└── JsonQLExecutionError    (Code: EXECUTION_ERROR)
```

## Compliance

All 6 Go integration adapters pass the full compliance test suite:

| Adapter | Type | PostgreSQL |
|---------|------|:----------:|
| **net/http** | simple | ✅ |
| **net/http** | lifecycle | ✅ |
| **Gin** | simple | ✅ |
| **Gin** | lifecycle | ✅ |
| **Echo** | simple | ✅ |
| **Echo** | lifecycle | ✅ |

Tests run via [jsonql-tests](https://github.com/JSONQL-Standard/jsonql-tests).

## Development

```bash
make deps         # download dependencies
make test         # run tests with gotestsum
make build        # build CLI tool
make lint         # go vet
make format       # go fmt
make test-watch   # watch mode
```

## Links

- 📖 [Documentation](https://pkg.go.dev/github.com/jsonql-standard/jsonql-go)
- 📋 [JSONQL Spec](https://github.com/JSONQL-Standard/jsonql-spec)
- 🧪 [Compliance Tests](https://github.com/JSONQL-Standard/jsonql-tests)
- 🐛 [Issues](https://github.com/JSONQL-Standard/jsonql-go/issues)

## License

MIT
