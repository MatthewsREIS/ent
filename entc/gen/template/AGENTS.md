<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-02-08 -->

# entc/gen/template/ - Code Generation Templates

Go `text/template` files that produce generated Go code. These are executed by the code generation engine for each entity type and the overall graph.

## Purpose

Templates transform the Type and Graph objects into idiomatic Go code:
- **Type Templates** - Executed once per entity, produce entity-specific code (create, update, delete, query, model)
- **Graph Templates** - Executed once for the entire graph, produce shared assets (client, hooks, transactions)
- **Base Templates** - Reusable base code and utilities (header, import, runtime helpers)

## Root-Level Templates (*.tmpl)

Core code generation templates executed on every build:

**Client & Initialization**
- `client.tmpl` - Client type, constructor, transaction support
- `ent.tmpl` - Main ent package entry point
- `enttest.tmpl` - Test helpers and utilities
- `header.tmpl` - File header with copyright and generation comment

**Type Model & Metadata**
- `base.tmpl` - Base type interface and common methods
- `meta.tmpl` - Type metadata and configuration
- `runtime.tmpl` - Runtime type introspection and reflection helpers

**Persistence Layer**
- `where.tmpl` - WHERE clause predicates and filters
- `import.tmpl` - Package imports organization
- `internal.tmpl` - Internal helper package structure

**Advanced Features**
- `hook.tmpl` - Hook registration and execution for lifecycle events
- `intercept.tmpl` - Interceptor middleware for operations
- `tx.tmpl` - Transaction and batch execution support
- `predicate.tmpl` - Predicate types for type-safe filtering

## Type-Specific Templates (via builder/)

Executed for each Type that's not a View:

**CRUD Operations**
- `create.tmpl` - Single entity creation
- `update.tmpl` - Entity updates
- `delete.tmpl` - Entity deletion
- `mutation.tmpl` - Shared mutation infrastructure (Create/Update/Delete state)

**Query & Retrieval**
- `query.tmpl` - Type-specific query builders
- `setter.tmpl` - Field setters for mutations
- `entql.tmpl` - EntQL runtime filtering (if feature enabled)

## Dialect-Specific Templates (dialect/)

Override and extend base templates for specific storage backends.

### dialect/sql/ (16 files)
SQL dialect templates for SQLite, MySQL, Postgres:

**Core SQL Operations**
- `create.tmpl` - SQL INSERT statements and builders
- `update.tmpl` - SQL UPDATE with field-level updates
- `delete.tmpl` - SQL DELETE with filtering
- `query.tmpl` - SQL SELECT builders with joins and eager loading

**SQL Query Building**
- `by.tmpl` - BY clause (grouping, ordering)
- `decode.tmpl` - Result scanning and decoding
- `select.tmpl` - SELECT clause construction
- `group.tmpl` - GROUP BY aggregation
- `predicate.tmpl` - WHERE clause predicates (=, <>, LIKE, IN, etc.)
- `entql.tmpl` - EntQL dynamic filtering
- `tx.tmpl` - Transaction support

**SQL Utilities**
- `meta.tmpl` - Table/column metadata
- `ent.tmpl` - SQL dialect entry point
- `errors.tmpl` - SQL error handling
- `globals.tmpl` - Global SQL constants
- `open.tmpl` - Database connection opening

### dialect/sql/feature/ (7 files)
Feature-gated SQL extensions:

- `execquery.tmpl` - Raw query execution (ExecContext)
- `lock.tmpl` - Row locking (FOR UPDATE)
- `modifier.tmpl` - Query modifiers (hints, lock strength)
- `upsert.tmpl` - UPSERT/ON CONFLICT operations
- `namedges.tmpl` - Dynamic edge loading with custom names
- `schemaconfig.tmpl` - Schema configuration hooks
- `migratediff.tmpl` - Migration diffing (Atlas integration)

### dialect/gremlin/ (13 files)
Graph database (Gremlin) templates for CosmosDB, JanusGraph:

**Graph Traversal**
- `create.tmpl` - Graph vertex/edge creation
- `update.tmpl` - Vertex property updates
- `delete.tmpl` - Vertex/edge deletion
- `query.tmpl` - Graph traversal queries

**Graph Filtering & Navigation**
- `predicate.tmpl` - Gremlin predicates (has, eq, gt, etc.)
- `by.tmpl` - Sort/group by properties
- `decode.tmpl` - Result deserialization
- `select.tmpl` - Property projection
- `group.tmpl` - Aggregation
- `entql.tmpl` - EntQL for graphs

**Utilities**
- `meta.tmpl` - Vertex/edge metadata
- `open.tmpl` - Connection setup
- `globals.tmpl` - Constants

## Migration & Schema Templates (migrate/)

Schema migration and versioning:

- `schema.tmpl` - Schema definition codegen (CREATE TABLE statements)
- `migrate.tmpl` - Migration runner and version tracking

## Privacy Templates (privacy/)

Privacy and access control:

- `privacy.tmpl` - Privacy rule evaluation and filtering
- `filter.tmpl` - Privacy filter application in queries

## Extension Patterns

Templates support extending built-in templates via the ExtendPatterns mechanism. For example:

```
"dialect/*/create/fields/additional/*"
```

Allows adding custom field logic to all dialect CREATE templates.

## Template Data Context

### Type Template Data
```go
type Type struct {
    Name       string      // Entity name
    Fields     []*Field    // All fields
    Edges      []*Edge     // All relationships
    ID         *Field      // Primary key
    Config     *Config     // Global config
    Storage    *Storage    // Storage driver
    Indexes    []*Index    // Database indexes
    // ... methods for code generation
}
```

### Graph Template Data
```go
type Graph struct {
    Nodes      []*Type     // All entity types
    Config     *Config     // Global config
    Storage    *Storage    // Storage driver
    // ... methods for reflection, naming, imports
}
```

## Common Template Functions

Available in all templates:

- `pascal(name)` - Convert to PascalCase
- `camel(name)` - Convert to camelCase
- `snake(name)` - Convert to snake_case
- `quote(s)` - Add quotes
- `receiver(t)` - Short receiver name for type
- `receiver(t.Name)` - Receiver name for string
- `join`, `split`, `contains` - String operations
- `plural(name)` - Pluralize entity name
- `lower`, `upper` - Case conversion

## Execution Flow

1. Load base templates from embedded filesystem
2. Load storage dialect templates
3. Apply feature-gated templates (privacy, intercept, etc.)
4. Register template functions (pascal, camel, etc.)
5. For each Type: execute type templates → write to `<type>_<template>.go`
6. For graph: execute graph templates → write to shared files (client.go, etc.)
7. Run `goimports` to format and organize imports

## Adding Custom Templates

```go
config := &gen.Config{
    Templates: []*gen.Template{
        {
            Name:   "mytemplate",
            Type:   gen.TypeTemplate,
            Text:   templateContent,
            Format: "%s_custom.go",
        },
    },
}
```

## Testing

Integration tests exercise templates with real schema scenarios:
```bash
cd ../integration
go test -v ./...
```

Tests verify:
- Generated code compiles
- CRUD operations work
- Queries return correct results
- Predicates filter correctly
- Edge loading works
- Privacy rules enforce
- Transactions execute

## Related

- `entgo.io/ent/entc/gen` - Graph and code generation engine
- `entgo.io/ent/dialect/sql` - SQL query builder
- `entgo.io/ent/dialect/gremlin` - Gremlin DSL builder
