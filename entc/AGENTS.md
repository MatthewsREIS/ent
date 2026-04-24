<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-02-08 -->

# entc/ - Code Generation Engine

The code generation engine for Ent. Transforms user-defined schemas into a complete Go ORM package.

## Purpose

`entc` is the core code generation subsystem that:
- Loads user-defined schema packages via `go/packages`
- Builds a dependency graph of schema entities and their relationships
- Generates type-safe, idiomatic Go code for CRUD operations, queries, and mutations
- Supports multiple storage backends (SQL, Gremlin) and dialects (SQLite, MySQL, Postgres)
- Provides extension hooks for features like privacy, interceptors, and custom templates

## Key Types (entc.go)

**Public API:**
- `LoadGraph(schemaPath, cfg)` - Loads schema package and constructs `*gen.Graph`
- `Generate(schemaPath, cfg, options...)` - Full codegen pipeline
- `Option` - Functional option type for configuration

**Entry Points:**
- Schema path resolution and validation
- Config normalization (package names, build flags)
- Storage driver initialization (defaults to SQL if not specified)
- Environment preparation and cleanup

## Directory Structure

### gen/
Core code generation logic. Contains the Graph type (main codegen structure), Type (entity representation), Field, Edge, and template management.
**Key files:** `graph.go`, `type.go`, `template.go`, `func.go`, `feature.go`, `predicate.go`, `storage.go`, `globalid.go`

### gen/template/
Go `text/template` files that produce generated code. Organized by concern: builders, dialects (SQL/Gremlin), migrations, privacy.

### integration/
Test suites validating codegen against real schema scenarios. Each subdir tests a feature: cascading deletes, custom IDs, edge fields, edge schemas, hooks, privacy, migrations, multi-schema, templates.

### load/
Schema package loading via the Go build toolchain. Parses user-defined schema types and metadata.

### internal/
Snapshot and VCS integration for schema versioning and merge-conflict resolution.

## Workflow

1. User calls `entc.Generate("./ent/schema", config)`
2. `load.Config` discovers and parses user schema package
3. `gen.NewGraph` constructs a graph of types and edges from loaded schemas
4. Features and hooks modify the graph
5. Templates are executed for each type and graph-level asset
6. Generated Go code is written to target directory and formatted

## Extension Hooks

- **Features** - Gate optional codegen behavior (privacy, intercept, snapshot, namedges, bidiedges)
- **Hooks** - Pre/post-generation callbacks to modify the graph
- **Templates** - Override or extend built-in `.tmpl` files
- **Storage drivers** - Plug in alternate database backends

## Configuration (gen.Config)

- `Schema` - Go package path to user schemas (e.g., "github.com/org/project/ent/schema")
- `Package` - Target Go package path for generated code (e.g., "github.com/org/project/ent")
- `Target` - Filesystem path to write generated code
- `Storage` - Storage driver (SQL, Gremlin). Defaults to SQL.
- `IDType` - Field type for entity IDs (int or string). Defaults to int.
- `Features` - Enable optional codegen features
- `Hooks` - Pre/post-generation callbacks
- `Annotations` - Global config annotations accessible in templates
- `BuildFlags` - Custom flags for schema package compilation

## Testing

Run integration tests to validate the codegen pipeline:
```bash
cd entc/integration
go test -v ./...
```

Tests exercise real schema scenarios: custom ID types, cascade deletes, privacy policies, hooks, etc.

## Related Packages

- `entgo.io/ent/schema` - User-facing schema definition API
- `entgo.io/ent/dialect` - SQL/Gremlin dialect support
- `golang.org/x/tools/go/packages` - Schema package discovery
