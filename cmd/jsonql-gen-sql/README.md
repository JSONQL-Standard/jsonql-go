# jsonql-gen-sql

A CLI tool to generate `jsonql.schema.json` from SQL DDL files.

## Usage

```bash
go run main.go -input <path-to-sql-file> -output <path-to-output-json>
```

## Example

Given a `schema.sql`:

```sql
CREATE TABLE users (
    id INT PRIMARY KEY,
    username VARCHAR(255) NOT NULL,
    email VARCHAR(255)
);
```

Run:

```bash
go run main.go -input schema.sql -output jsonql.schema.json
```

Output `jsonql.schema.json`:

```json
{
  "users": {
    "fields": {
      "id": { "type": "number", "required": true, ... },
      "username": { "type": "string", "required": true, ... },
      "email": { "type": "string", "required": false, "nullable": true, ... }
    }
  }
}
```

## Limitations

- Currently supports basic `CREATE TABLE` statements.
- Does not support complex constraints or `ALTER TABLE`.
- Does not infer relationships (foreign keys) yet.
