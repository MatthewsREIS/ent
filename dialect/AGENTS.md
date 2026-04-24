<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-02-08 -->

# dialect

Database dialect abstraction layer. Defines the core interfaces that all database drivers (SQL, Gremlin) must implement, and provides debug/instrumentation wrappers.

## Key Responsibilities

- **Driver Interface**: `dialect.Driver` is the contract all database backends implement
  - `Exec`: Execute statements (INSERT, UPDATE, DELETE)
  - `Query`: Execute statements that return rows (SELECT)
  - `Tx`: Start transactions with context support
  - `Dialect()`: Return dialect identifier (mysql, postgres, sqlite3, gremlin)

- **Transaction Wrapper**: `Tx` interface wraps both ent queries and `database/sql/driver.Tx`

- **Debug Instrumentation**: `DebugDriver` and `DebugTx` provide transparent query/transaction logging

- **Dialect Constants**: MySQL, SQLite, Postgres, Gremlin dialect names

## Architecture

```
dialect/
├── dialect.go          # Core Driver, Tx, ExecQuerier interfaces
├── entsql/             # SQL annotations (schema, table, charset, collation)
├── sql/                # SQL driver implementation + builders
│   ├── schema/         # Schema migration (Atlas, db-specific logic)
│   ├── sqlgraph/       # Graph queries over SQL
│   └── sqljson/        # JSON column support
└── gremlin/            # Gremlin graph database support
    ├── client.go       # Gremlin client and transport
    ├── encoding/       # GraphSON codec
    ├── graph/          # Graph element types (vertex, edge)
    ├── internal/       # WebSocket transport
    └── ocgremlin/      # OpenCensus instrumentation
```

## Key Files

- `dialect.go` (209 lines): Defines `Driver`, `Tx`, `ExecQuerier` interfaces; `DebugDriver` and `DebugTx` decorators
- Subdirectories implement specific backends (see AGENTS.md in each)

## Dependencies

- `context` (stdlib)
- `database/sql` (stdlib)
- `google/uuid` (for transaction IDs in debug mode)

## Related Directories

- `entsql/`: SQL schema annotations
- `sql/`: SQL dialect driver
- `gremlin/`: Gremlin graph database driver

## Development Notes

- All drivers must satisfy the `dialect.Driver` interface
- Debug instrumentation is transparent and can wrap any driver
- Transaction context is tracked via UUID for debugging
- Dialect names are used as registry keys (see `dialect.MySQL`, `dialect.Postgres`, etc.)
