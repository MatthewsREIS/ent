<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-02-08 -->

# cmd/ - CLI Tools

Container directory for Ent framework command-line tools.

## Overview

The `cmd/` directory contains three executable entry points and shared internal utilities for the Ent code generation and schema management CLI.

### Structure

```
cmd/
├── ent/              # Main Ent CLI entry point
├── entc/             # Code generation CLI (legacy)
├── entfix/           # Database migration utility
└── internal/         # Shared utilities
    ├── base/         # CLI command implementations and package utilities
    └── printer/      # Graph schema pretty-printing
```

## Executables

### `cmd/ent/` - Main CLI

**Purpose:** Primary command-line interface for Ent framework operations.

**Entry Point:** `cmd/ent/ent.go`

**Commands:**
- `new` - Initialize new schema environments (with optional custom templates)
- `init` - Deprecated: older variant of the `new` command
- `describe` - Print schema graph description as formatted tables
- `generate` - Run code generation on schema directory
- `schema` - Dump DDL output for Atlas schema loader compatibility

**Key Dependencies:**
- `cmd/internal/base` - Command implementations
- `github.com/spf13/cobra` - CLI framework

**Usage Patterns:**
```bash
# Initialize new schema
ent new User Post

# Generate code from schemas
ent generate ./ent/schema

# Display schema structure
ent describe ./ent/schema

# Output DDL for Atlas
ent schema ./ent/schema --dialect postgres --version 15
```

### `cmd/entc/` - Legacy Code Generation CLI

**Purpose:** Legacy entry point for Ent code generation (ent → entc migration support).

**Entry Point:** `cmd/entc/entc.go`

**Commands:** Same as `cmd/ent/` (delegates to `cmd/internal/base`)

**Special Behavior:**
- Includes `migrate()` function that automatically rewrites code generation imports from `entgo.io/ent/cmd/entc` to `entgo.io/ent/cmd/ent`
- Runs during `generate` command post-processing
- Enables backward compatibility for projects using the old entc CLI

**Migration Support:**
The `migrate()` function:
1. Checks if generated code references old `entgo.io/ent/cmd/entc` import path
2. Rewrites to new `entgo.io/ent/cmd/ent` path
3. Allows projects to upgrade without manual code changes

### `cmd/entfix/` - Database Migration Utility

**Purpose:** Utility for migrating ent_types table data to schema-based global ID configuration.

**Entry Point:** `cmd/entfix/entfix.go`

**Commands:**
- `globalid` - Migrate from database-stored ID ranges (ent_types) to static schema configuration

**Arguments:**
- `--dialect` (required) - Database dialect: `mysql`, `postgres`, or `sqlite3`
- `--dsn` (required) - Database connection string
- `--path` (required) - Path to generated ent code

**How It Works:**
1. Reads from `ent_types` table (id, type columns)
2. Converts database ID ranges to `gen.IncrementStarts` configuration
3. Writes configuration to disk via `WriteToDisk()`
4. User must run code generation to apply changes

**Safety Features:**
- Interactive confirmation required (must enter "yes")
- Validates ent_types consistency across deployments
- Graceful signal handling (SIGINT, SIGTERM, SIGHUP)

**Example Usage:**
```bash
entfix globalid --dialect postgres --dsn "postgres://user:pass@localhost/db" --path ./ent
```

## Internal Utilities

### `cmd/internal/base/` - Command Implementations

**Purpose:** Shared CLI command definitions and package utilities for all Ent CLI tools.

**Files:**
- `base.go` - Command factory functions and execution logic
- `packages.go` - Go package path resolution utilities

**Commands Implemented:**

1. **NewCmd()** - Schema initialization
   - Creates new schema files from template
   - Validates schema names (must start with uppercase)
   - Supports custom templates via `--template` flag
   - Default location: `ent/schema/`
   - Automatically creates `ent/generate.go` for `go generate` integration

2. **DescribeCmd()** - Schema introspection
   - Loads schema graph from path
   - Renders formatted table output (fields, edges, constraints)
   - Supports remote package paths

3. **GenerateCmd()** - Code generation
   - Loads schema and generates Go code
   - Supports features via `--feature` flag
   - Custom templates via `--template` flag (dir, file, or glob)
   - Custom ID types via `--idtype` flag (int, int64, uint, uint64, string)
   - Storage drivers: `--storage` (default: sql)
   - Target directory resolution via PkgPath utility

4. **InitCmd()** - Deprecated legacy command
   - Alias for NewCmd
   - Marked as deprecated, users redirected to `new`

5. **SchemaCmd()** - DDL output
   - Generates SQL DDL from schema for Atlas integration
   - Requires `--dialect` and optional `--version`
   - Supports feature selection
   - Option to hash long symbols via `--hash-symbols`

**Package Utilities:**

- **PkgPath()** - Resolve Go package path from filesystem path
  - Works even if target path doesn't exist yet
  - Searches up to 2 parent directories for module/package root
  - Uses `golang.org/x/tools/go/packages` for loading package info
  - Returns fully-qualified package path (e.g., `entgo.io/ent/schema`)

**Configuration:**

- `IDType` - Custom flag type for ID field type validation
  - Supports: int, int64, uint, uint64, string
  - Flag name: `--idtype`
  - Default: int
  - Marked as hidden (legacy, superseded by field.Int("id") option)

### `cmd/internal/printer/` - Schema Visualization

**Purpose:** Pretty-print Ent graph schema as ASCII tables.

**Files:**
- `printer.go` - Table formatting and output

**Functionality:**

- **Fprint()** - Entry point, prints entire graph to io.Writer
- **Config.Print()** - Iterates through all nodes in graph
- **Config.node()** - Renders per-type tables

**Output Format:**

Two tables per entity type:

1. **Fields Table** with columns:
   - Field, Type, Unique, Optional, Nillable, Default, UpdateDefault, Immutable, StructTag, Validators, Comment

2. **Edges Table** with columns:
   - Edge, Type, Inverse, BackRef, Relation, Unique, Optional, Comment

**Features:**
- ASCII table formatting via `github.com/olekukonko/tablewriter`
- Smart column alignment (numeric right-aligned, text left-aligned)
- Reflects field and edge properties from gen.Type
- Indented output for readability (all lines prefixed with tabs)

## Dependencies

### External Libraries
- `github.com/spf13/cobra` - CLI command framework
- `github.com/alecthomas/kong` - CLI parsing (entfix only)
- `github.com/olekukonko/tablewriter` - Table rendering
- `golang.org/x/tools/go/packages` - Go package metadata
- Database drivers: `github.com/lib/pq`, `github.com/go-sql-driver/mysql`, `github.com/mattn/go-sqlite3`

### Internal Dependencies
- `entgo.io/ent/entc` - Code generation core
- `entgo.io/ent/entc/gen` - Schema graph types and utilities
- `entgo.io/ent/dialect/sql` - SQL dialect handling
- `entgo.io/ent/dialect/sql/schema` - DDL generation

## Testing

Test files exist for:
- `cmd/ent/ent_test.go` - Main CLI tests
- `cmd/entc/entc_test.go` - Code generation CLI tests
- `cmd/entfix/entfix_test.go` - Migration utility tests
- `cmd/internal/base/packages_test.go` - Package path resolution tests
- `cmd/internal/printer/printer_test.go` - Table rendering tests

## Usage Examples

### Initialize and Generate

```bash
# Create new schema environment with User and Post schemas
ent new User Post

# Generate code from schemas
ent generate ./ent/schema
```

### Inspect Schema

```bash
# Display all schema definitions
ent describe ./ent/schema

# Output SQL for a specific database
ent schema ./ent/schema --dialect mysql --version 5.7
```

### Migrate Global IDs

```bash
# Convert database-stored ID ranges to schema configuration
entfix globalid \
  --dialect postgres \
  --dsn "postgres://user:pass@localhost/mydb" \
  --path ./ent
```

### Custom Schema Generation

```bash
# With custom ID type
ent generate ./ent/schema --idtype string

# With additional features and templates
ent generate ./ent/schema \
  --feature sql/upsert \
  --feature entql \
  --template dir=/path/to/custom/templates
```
