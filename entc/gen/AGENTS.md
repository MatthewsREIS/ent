<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-02-08 -->

# entc/gen/ - Code Generation Core

The core code generation logic. Transforms a schema graph into Go assets.

## Purpose

`gen` implements the graph-based code generation engine:
- Models a schema as a directed graph of Types (entities) and Edges (relationships)
- Generates idiomatic Go code for CRUD, queries, mutations, and edge traversal
- Supports multiple storage backends and dialects with pluggable drivers
- Provides template execution framework for code generation
- Implements feature flags for optional codegen behavior

## Key Types (Public API)

### Config (graph.go)
Global codegen configuration shared across all generated nodes:
- `Schema` - Go package path to user schemas
- `Target` - Filesystem path for generated code
- `Package` - Go package path of generated code
- `Storage` - Storage driver (SQL, Gremlin)
- `IDType` - ID field type (int or string, default int)
- `Features` - Optional codegen features
- `Hooks` - Pre/post-generation callbacks
- `Templates` - Custom template overrides
- `Annotations` - Global config annotations for templates
- `BuildFlags` - Custom build flags for schema compilation

### Graph (graph.go)
The main codegen structure representing the entire schema:
- `Nodes` - Array of Type nodes (entities)
- `Config` - Global codegen config
- `Storage` - Storage driver interface
- Methods for code generation and template execution
- Hooks for modifying the graph before/after generation

### Type (type.go)
Represents a single entity (ent.Schema):
- `Name` - Entity name
- `ID` - Primary key Field
- `Fields` - Primitive fields []*Field
- `Edges` - Entity relationships []*Edge
- `Indexes` - Configured indexes
- `ForeignKeys` - Foreign key constraints
- `Annotations` - Type-specific annotations
- `EdgeSchema` - For edge schema types (join tables)

### Field (type.go)
Represents a single entity field:
- `Name` - Database column name
- `Type` - Field type (TypeInfo from schema/field)
- `Unique` - Uniqueness constraint
- `Optional` - Optional on create
- `Nillable` - Can be NULL
- `Default` - Default value
- `Immutable` - Cannot be modified
- `StructTag` - Go struct tag override
- `Validators` - Custom validation functions
- `Sensitive` - PII field (for privacy features)

### Edge (type.go, edge.go)
Represents a relationship between types:
- `Name` - Edge name
- `Type` - Target Type
- `Rel` - Relationship type (O2O, O2M, M2M)
- `Unique` - Uniqueness constraint
- `Optional` - Optional edge
- `Inverse` - Inverse edge reference
- `StorageKey` - Foreign key column/table
- Constraint options (cascade, no-delete, etc.)

### Storage (storage.go)
Interface for pluggable storage drivers:
- `Name` - Driver name ("sql", "gremlin")
- `Builder` - Query builder type
- `Dialects` - Supported database dialects
- `IdentName` - Identifier name for driver (e.g., "SQL")
- `SchemaMode` - Supported schema features (Unique, Indexes, Cascade, Migrate)
- `Ops` - Storage-specific predicates (e.g., EqualFold for SQL strings)
- `OpCode` - Maps operation enums to code strings
- `Init` - Optional initialization hook

## Core Files

**graph.go** (38KB)
- Graph type definition and methods
- Graph construction from Config and Schemas
- Code generation execution
- Template loading and rendering

**type.go** (69KB)
- Type, Field, Edge, Index, ForeignKey types
- Field/Edge methods for code generation
- Type validation and constraint checking

**template.go** (11KB)
- TypeTemplate and GraphTemplate structures
- Template registration and execution
- Built-in template definitions
- Extension patterns for dialect-specific templates

**feature.go** (7KB)
- Feature type and feature flags
- Built-in features: Privacy, Intercept, EntQL, NamedEdges, BidiEdgeRefs, Snapshot

**storage.go** (5KB)
- Storage driver interface
- SQL and Gremlin driver implementations
- SchemaMode and operation definitions

**func.go** (13KB)
- Template helper functions
- Type/field/edge formatting functions
- Code generation utility functions

**globalid.go** (5KB)
- Relay Global ID support
- Marshaling/unmarshaling helpers
- Global ID type and methods

**predicate.go** (2KB)
- Predicate generation helpers
- Operation type definitions (Equal, NotEqual, EqualFold, etc.)

## Template Execution

Templates are Go `text/template` files executed with:
- **Type Templates**: Executed for each Type node → one file per entity
  - Example: `create.tmpl` → `user_create.go`
  - Data: Type object with all fields, edges, methods
  - Extension patterns: `dialect/*/create/fields/additional/*`

- **Graph Templates**: Executed once for the entire graph → one shared file
  - Example: `client.tmpl` → `client.go`
  - Data: Graph object with all types and storage info
  - Used for client initialization, mutations, schema hooks

## Storage Drivers

### SQL (Default)
- Dialects: SQLite, MySQL, Postgres
- Features: Unique, Indexes, Cascade, Migrate
- Builder: `*sql.Selector`
- Uses sqlc and sqlgraph packages for query building

### Gremlin
- Graph database support
- Dialects: CosmosDB, JanusGraph
- Features: Cascade
- Builder: Gremlin DSL

## Feature Flags

- **PrivacyFeature** - Privacy layer for schema-level access control
- **InterceptFeature** - Interceptor support for hooks and middleware
- **EntQLFeature** - Generic runtime filtering capability
- **NamedEdgesFeature** - Dynamic edge loading with custom names
- **BidiEdgeRefsFeature** - Two-way reference loading for O2M/O2O edges
- **SnapshotFeature** - Schema versioning and merge-conflict resolution

## Extension Points

1. **Custom Templates** - Override built-in templates via `Config.Templates`
2. **Storage Drivers** - Implement Storage interface for new backends
3. **Features** - Define custom Feature with graph/type templates
4. **Hooks** - Modify graph pre/post-generation via Config.Hooks
5. **Annotations** - Pass global config data to templates
6. **Template Functions** - Add custom functions via template.FuncMap

## Testing

Run tests to validate code generation:
```bash
go test -v ./...
```

Tests cover:
- Graph construction and validation
- Type/field/edge creation and constraints
- Template execution and formatting
- Feature flag behavior
- Storage driver implementations

## Related Packages

- `entgo.io/ent/entc/load` - Schema package loading
- `entgo.io/ent/dialect/sql` - SQL builder and driver
- `entgo.io/ent/dialect/gremlin` - Gremlin driver
- `entgo.io/ent/schema` - User schema API
- `golang.org/x/tools/imports` - Code formatting
