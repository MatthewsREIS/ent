<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-02-08 -->

# dialect/sql/schema

Schema migration and table management. Provides declarative schema definitions, diff-based migration planning, and database-specific SQL generation for MySQL, PostgreSQL, and SQLite.

## Key Responsibilities

- **Schema Model**: Table, Column, Index, ForeignKey, Check constraint definitions
  - Immutable domain objects for schema representation
  - Support for unique constraints, collations, charset

- **Migration Engine (Atlas)**: Hash-based migrations to prevent re-running
  - Parses existing schema via `StateReader`
  - Compares with desired schema (from Ent definitions)
  - Generates execution plan (`Plan`) with `Change` entries
  - Applies changes via `ApplyChange` hooks

- **Database-Specific Logic**: MySQL, PostgreSQL, SQLite handlers
  - Type mappings (Ent field types → database types)
  - Charset and collation support (MySQL)
  - Serial/BigSerial sequences (PostgreSQL)
  - AUTOINCREMENT handling (SQLite)

- **SQL Generation**: CREATE TABLE, ALTER TABLE, DROP statements
  - Respects database syntax and constraints
  - Proper ordering for foreign keys and dependencies

## Architecture

```
schema/
├── schema.go           # Table, Column, Index, FK, Check domain model
├── atlas.go            # Atlas integration (36K+ lines)
│                       # StateReader, Plan, Change, Apply logic
├── migrate.go          # Migration options API (MigrateOption, WithGlobalUniqueID, etc.)
├── writer.go           # SQL generation (CREATE/ALTER/DROP)
├── mysql.go            # MySQL type mappings, charset/collation
├── postgres.go         # PostgreSQL type mappings, sequences
└── sqlite.go           # SQLite type mappings, AUTOINCREMENT
```

## Key Files

- `schema.go` (400+ lines): `Table`, `Column`, `Index`, `ForeignKey`, `Check` types; builder pattern
- `atlas.go` (36K+ lines): `Atlas` struct, `StateReader`, `Plan`, `Change`, `Apply` logic
- `migrate.go` (50+ lines): `MigrateOption` functional API, `WithGlobalUniqueID()`, `WithIndent()`
- `writer.go` (300+ lines): `SchemaWriter`, SQL DDL generation
- `mysql.go` (300+ lines): MySQL-specific type mapping, charset/collation logic
- `postgres.go` (300+ lines): PostgreSQL serial types, RETURNING clause support
- `sqlite.go` (200+ lines): SQLite type mapping, AUTOINCREMENT support

## Key Types

- `Table`: Schema table with columns, indexes, foreign keys
- `Column`: Field definition with type, constraints, defaults
- `Index`: Single or composite index definition
- `ForeignKey`: Referential integrity constraint
- `Atlas`: Migration engine (driver, state reader, hooks)
- `Plan`: Migration execution plan (list of `Change`)
- `Change`: A single schema change (ALTER/CREATE/DROP)

## Dependencies

- `context` (stdlib)
- `database/sql` (stdlib)
- `crypto/md5` - hash migration files
- `ariga.io/atlas/sql/migrate` - migration framework
- `ariga.io/atlas/sql/schema` - schema diffing
- `entgo.io/ent/dialect/sql` - SQL driver
- `entgo.io/ent/schema/field` - Ent field types

## Related Directories

- `../`: SQL dialect driver
- `../sqlgraph/`: Graph queries
- `../sqljson/`: JSON columns
- `../../entsql/`: SQL annotations

## Development Notes

- Migrations are hash-based to prevent duplicate execution
- Each database (MySQL/PostgreSQL/SQLite) has custom type mapping
- Charset and collation are MySQL-specific
- Foreign keys are applied after all tables exist
- Global unique IDs reserve the left 16 bits for type information (ent_types table)
- Atlas supports schema-scoped operations (PostgreSQL schemas)
- Migration files are formatted with optional indentation for readability
