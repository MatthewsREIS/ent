<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-02-08 -->

# dialect/sql

SQL dialect driver implementation. Wraps `database/sql` and provides query builders, row scanning, and transaction support for MySQL, PostgreSQL, and SQLite.

## Key Responsibilities

- **Driver Implementation**: `sql.Driver` implements `dialect.Driver` for all SQL databases
  - Wraps `*sql.DB` and `sql.Conn`
  - Supports `Open()` and `OpenDB()` factory methods
  - Delegates `Exec`, `Query`, `Tx` to underlying connection

- **Query Builders**: `Builder`, `ColumnBuilder`, `TableBuilder`, `SelectBuilder`, `InsertBuilder`, `UpdateBuilder`, `DeleteBuilder`
  - Type-safe SQL AST construction
  - Querier interface returns (string, []any) for parameterized queries
  - No SQL validation (left to generated code)

- **Row Scanning**: `Rows` and `scanner` types
  - Wraps `*sql.Rows`
  - Provides column scanning for various field types
  - Handles NULL values

- **Utilities**: Connection management, result handling, dialect-specific SQL generation

## Architecture

```
sql/
├── driver.go           # Driver implementation wrapping *sql.DB
├── builder.go          # Query builders (SELECT, INSERT, UPDATE, DELETE)
├── scan.go             # Row scanning and column type mapping
├── sql.go              # Conn, Result, Rows wrappers
├── schema/             # Schema migration and table management
│   ├── atlas.go        # Atlas integration (hash-based migration)
│   ├── migrate.go      # Migration options and API
│   ├── schema.go       # Table, Column, Index, ForeignKey definitions
│   ├── writer.go       # SQL generation for CREATE/ALTER TABLE
│   ├── mysql.go        # MySQL-specific schema logic
│   ├── postgres.go     # PostgreSQL-specific schema logic
│   └── sqlite.go       # SQLite-specific schema logic
├── sqlgraph/           # Graph queries over SQL
│   ├── graph.go        # Graph traversal builder
│   ├── entql.go        # EntQL query dialect
│   └── errors.go       # Graph-specific errors
└── sqljson/            # JSON column support
    ├── sqljson.go      # JSON value encoding/decoding
    └── dialect.go      # JSON function mapping per dialect
```

## Key Files

- `driver.go` (150+ lines): `Driver` struct, `Open()`, `OpenDB()`, `DB()` accessor
- `builder.go` (500+ lines): Query builders for all DML/DDL operations
- `scan.go` (300+ lines): Row scanning and column type handling
- `sql.go` (200+ lines): Connection wrappers (Conn, Result, Rows, scanner)
- `schema/atlas.go` (36K+ lines): Atlas integration for hash-based migrations
- `schema/migrate.go` (50+ lines): Migration options and type table management
- `schema/schema.go` (400+ lines): Table/Column/Index/FK domain model
- `schema/writer.go` (300+ lines): SQL DDL generation

## Dependencies

- `database/sql` (stdlib) - underlying database
- `entgo.io/ent/dialect` - dialect.Driver interface
- `entgo.io/ent/schema/field` - field type definitions
- `ariga.io/atlas` (schema/): Atlas migration engine

## Related Directories

- `schema/`: Schema migration and table management (see AGENTS.md)
- `sqlgraph/`: Graph queries via SQL
- `sqljson/`: JSON column support
- `../entsql/`: SQL schema annotations
- `../`: Parent dialect layer

## Development Notes

- Builders do NOT validate SQL syntax (validation in generated code)
- Parameterized queries use `?` placeholders (dialect-specific)
- Row scanning handles NULL and type conversions
- Schema migrations are hash-based (Atlas) to prevent re-running
- JSON support varies by database (PostgreSQL native, MySQL JSON functions, SQLite TEXT)
