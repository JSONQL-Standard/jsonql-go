# JSONQL Go SDK

The official Go implementation of the [JSONQL Standard](https://github.com/jsonql-standard/jsonql-spec).

JSONQL allows clients to fetch exactly the data they need, with support for nested relationships, filtering, sorting, and pagination, all through a simple JSON-based query language.

## Features

- **Full JSONQL v1.1 Support**: Selects, Includes (Relationships), Filtering, Sorting, Pagination, and Aggregation.
- **Database Agnostic**: Built-in support for **SQLite** and **PostgreSQL**. Easily extensible for others.
- **Framework Integrations**: Adapters for **Gin**, **Echo**, and standard `net/http`.
- **Security**: Schema-based validation and SQL injection prevention via parameterized queries.
- **High Performance**: Efficient transpilation to SQL and hydration of results.

## Installation

```bash
go get github.com/jsonql-standard/jsonql-go
```

## Quick Start

Here's a simple example using **SQLite** and **Gin**:

```go
package main

import (
	"database/sql"
	"log"

	"github.com/gin-gonic/gin"
	"github.com/jsonql-standard/jsonql-go"
	jsonqlgin "github.com/jsonql-standard/jsonql-go/adapters/gin"
	"github.com/jsonql-standard/jsonql-go/drivers/sqlite"
	_ "modernc.org/sqlite"
)

func main() {
	// 1. Define your Schema
	schema := &jsonql.JSONQLSchema{
		Tables: map[string]jsonql.TableSchema{
			"users": {
				Fields: []string{"id", "name", "email", "created_at"},
				Relationships: map[string]jsonql.Relationship{
					"posts": {Type: "hasMany", Table: "posts", Field: "user_id"},
				},
			},
			"posts": {
				Fields: []string{"id", "user_id", "title", "content"},
				Relationships: map[string]jsonql.Relationship{
					"author": {Type: "belongsTo", Table: "users", Field: "user_id"},
				},
			},
		},
	}

	// 2. Initialize Driver
	driver, err := sqlite.NewDriver("./my.db")
	if err != nil {
		log.Fatal(err)
	}

	// 3. Create Handler
	handler, err := jsonqlgin.NewHandler(jsonqlgin.HandlerOptions{
		Driver: driver,
		Schema: schema,
	})
	if err != nil {
		log.Fatal(err)
	}

	// 4. Setup Server
	r := gin.Default()
	r.POST("/api/jsonql", handler)
	
	log.Println("Server running on :8080")
	r.Run(":8080")
}
```

## Architecture

The SDK follows a modular pipeline:

1.  **Parser**: Validates the incoming JSON query against the defined Schema.
2.  **Transpiler**: Converts the JSONQL query into a dialect-specific SQL query (e.g., handling `LIMIT`, `OFFSET`, `JOIN`s).
3.  **Driver**: Executes the SQL query against the database.
4.  **Hydrator**: Takes the flat rows returned by SQL and reconstructs the nested JSON response expected by the client.

## Supported Drivers

| Database | Import Path |
|----------|-------------|
| SQLite   | `github.com/jsonql-standard/jsonql-go/drivers/sqlite` |
| Postgres | `github.com/jsonql-standard/jsonql-go/drivers/postgres` |

## Supported Frameworks

| Framework | Import Path |
|-----------|-------------|
| Gin       | `github.com/jsonql-standard/jsonql-go/adapters/gin` |
| Echo      | `github.com/jsonql-standard/jsonql-go/adapters/echo` |
| net/http  | `github.com/jsonql-standard/jsonql-go/adapters/http` |

## Contributing

Contributions are welcome! Please ensure you run the tests before submitting a PR.

```bash
make test
```

For Jest-like test output (summaries, colors), install `gotestsum`:

```bash
go install gotest.tools/gotestsum@latest
make test
```

You can also run tests in watch mode:

```bash
make test-watch
```
