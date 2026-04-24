# cmd/internal/ - Shared CLI Utilities

<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-02-08 -->

Shared utilities for Ent CLI tools (ent, entc, entfix).

## Overview

The `cmd/internal/` directory contains reusable components used across multiple CLI entry points:
- **base/** - Command implementations and package path resolution
- **printer/** - Schema visualization and formatting

## Directory Structure

```
cmd/internal/
├── base/       # Cobra command factories and Go package utilities
│   ├── base.go
│   └── packages.go
└── printer/    # Graph schema table rendering
    └── printer.go
```

## Modules

### `base/` - CLI Command Factory

**Purpose:** Centralized command definitions and utilities shared by `cmd/ent` and `cmd/entc`.

**Files:**
- `base.go` - Main command implementations
- `packages.go` - Go package path resolution

#### Commands

**NewCmd()** - Initialize schema environment
- Creates new schema files in target directory (default: `ent/schema/`)
- Accepts schema names as arguments (must start with uppercase)
- Supports custom templates via `--template` flag
- Validates schema names at runtime
- Creates `ent/generate.go` in schema directory for `go generate` integration
- Returns `*cobra.Command` with Use: "new [flags] [schemas]"

**InitCmd()** - Legacy command (deprecated)
- Wrapper around NewCmd()
- Marked deprecated with message: `use "ent new" instead`
- Kept for backward compatibility

**DescribeCmd()** - Introspect schema graph
- Takes one required argument: schema path (local or remote package)
- Loads schema graph using `entc.LoadGraph()`
- Outputs formatted table via `printer.Fprint()`
- Returns `*cobra.Command` with Use: "describe [flags] path"
- Example: `ent describe ./ent/schema` or `ent describe github.com/a8m/x`

**GenerateCmd(postRun ...func(*gen.Config))** - Generate code from schema
- Required argument: schema path
- Options:
  - `--idtype` - ID field type (int, int64, uint, uint64, string; default: int; marked hidden)
  - `--storage` - Storage driver (default: "sql")
  - `--header` - Custom code generation header
  - `--target` - Target directory for generated code
  - `--feature` - Additional features (repeatable; e.g., sql/upsert, entql)
  - `--template` - External templates (repeatable; can specify type=path: dir, file, glob)
- Accepts variadic postRun callbacks (e.g., for migration in entc)
- Resolves package path if target directory specified (via PkgPath utility)
- Returns `*cobra.Command` with Use: "generate [flags] path"

**SchemaCmd()** - Output DDL for Atlas integration
- Required argument: schema path
- Required flag: `--dialect` (mysql, postgres, sqlite3)
- Optional flags:
  - `--version` - Database version
  - `--feature` - Additional features (repeatable)
  - `--build-tags` - Go build tags for schema loading (repeatable)
  - `--hash-symbols` - Hash long symbols in DDL (boolean)
- Loads schema graph, generates tables and views
- Calls `schema.DDL()` to generate SQL
- Outputs DDL to stdout
- Returns `*cobra.Command` with Use: "schema [flags] path"

#### Package Utilities

**PkgPath()** - Resolve Go package path from filesystem path
```go
func PkgPath(config *packages.Config, target string) (string, error)
```

**Parameters:**
- `config` - Package loading configuration (DefaultConfig if nil)
- `target` - Target path (absolute or relative)

**Behavior:**
1. Converts to absolute path
2. If path doesn't exist, extracts basename and searches parent directories
3. Searches up to 2 levels up for Go package root
4. Uses `golang.org/x/tools/go/packages` to load package metadata
5. Reconstructs full package path by appending extracted path components

**Returns:**
- Full package path (e.g., `entgo.io/ent/schema`)
- Error if module/package root not found within 2 levels

**Example:**
```go
pkgPath, err := PkgPath(nil, "./ent/schema")
// Returns: "entgo.io/ent/schema"
```

**DefaultConfig**
- Global variable: `&packages.Config{Mode: packages.NeedName}`
- Used as fallback if no custom config provided

#### Configuration Types

**IDType** - Custom pflag.Value for ID field type validation
- Implements `Set()`, `Type()`, `String()` methods
- Supports: int, int64, uint, uint64, string
- Default string representation: "int"
- Type description lists all supported types

#### Templates

**Default Schema Template**
- Used when no `--template` flag provided
- Go text template with `gen.Funcs` available
- Creates basic schema struct with Fields() and Edges() methods
- Variable: `defaultTemplate`

**Default Generation File** (`ent/generate.go`)
- Auto-created in schema directory by NewCmd()
- Content: `package ent\n\n//go:generate go run -mod=mod entgo.io/ent/cmd/ent generate ./schema\n`
- Enables `go generate ./ent` workflow
- Variable: `genFile`

### `printer/` - Schema Visualization

**Purpose:** Pretty-print Ent entity graph as ASCII tables.

**File:** `printer.go`

#### Public Interface

**Fprint(w io.Writer, g *gen.Graph)**
- Entry function for printing schema
- Iterates all nodes in graph and renders tables
- Writes to provided io.Writer
- No return value, error handling via table rendering

**Config.Print(g *gen.Graph)**
- Method on Config struct
- Iterates through `g.Nodes` and calls `node()` for each
- Config carries the io.Writer

#### Output Format

Per entity type, generates two ASCII tables:

**Table 1: Fields**
- Columns: Field, Type, Unique, Optional, Nillable, Default, UpdateDefault, Immutable, StructTag, Validators, Comment
- ID field (if exists) shown first
- All schema fields follow
- Comments extracted via `f.Comment()`

**Table 2: Edges**
- Columns: Edge, Type, Inverse, BackRef, Relation, Unique, Optional, Comment
- Omitted if entity has no edges
- Relation type rendered via `e.Rel.Type.String()`

#### Features

**Formatting:**
- ASCII table style (`tw.StyleASCII`)
- Smart column alignment: numeric fields right-aligned, text left-aligned
- Custom padding and cell configuration via tablewriter options
- Indented output: all lines prefixed with tab character

**Data Extraction:**
- Uses reflection to read field attributes from `*gen.Field`
- Maps struct field names to table columns (e.g., "Name" → "Field")
- Handles conversion to string representation

**Libraries:**
- `github.com/olekukonko/tablewriter` - Main table rendering
- `github.com/olekukonko/tablewriter/tw` - Advanced table options (alignment, padding, rendition)

#### Configuration

**Config Struct**
- Embeds `io.Writer`
- Single field (writer destination)
- No other configuration options

## Usage Patterns

### From CLI Entry Points

```go
// cmd/ent/ent.go
func main() {
  cmd := &cobra.Command{Use: "ent"}
  cmd.AddCommand(
    base.NewCmd(),
    base.DescribeCmd(),
    base.GenerateCmd(),    // No postRun callbacks
    base.InitCmd(),
    base.SchemaCmd(),
  )
  cobra.CheckErr(cmd.Execute())
}
```

```go
// cmd/entc/entc.go
func main() {
  cmd := &cobra.Command{Use: "entc"}
  cmd.AddCommand(
    base.NewCmd(),
    base.DescribeCmd(),
    base.GenerateCmd(migrate),  // With migration callback
    base.InitCmd(),
  )
  _ = cmd.Execute()
}
```

### From Command Handlers

```go
// Display schema structure
graph, err := entc.LoadGraph(path, &gen.Config{})
printer.Fprint(os.Stdout, graph)

// Resolve package path
pkgPath, err := base.PkgPath(nil, "./ent/schema")
cfg.Package = pkgPath
```

## Testing

### Test Coverage

- `cmd/internal/base/packages_test.go` - PkgPath() resolution tests
- `cmd/internal/printer/printer_test.go` - Table rendering tests

### Test Scenarios

**packages_test.go:**
- Package path resolution for existing directories
- Package path resolution for non-existent paths
- Multi-level directory traversal
- Module root detection

**printer_test.go:**
- Table output generation
- Field and edge rendering
- Comment extraction and display
- Column alignment and formatting

## Dependencies

### External Libraries
- `github.com/spf13/cobra` - CLI command framework (base)
- `github.com/olekukonko/tablewriter` - ASCII table rendering (printer)
- `golang.org/x/tools/go/packages` - Go package metadata (base)

### Internal Dependencies
- `entgo.io/ent/entc` - Code generation core (base)
- `entgo.io/ent/entc/gen` - Graph types and utilities (base, printer)
- `entgo.io/ent/dialect/sql/schema` - DDL generation (base)
- `entgo.io/ent/schema/field` - Field type definitions (base)

## Design Notes

**Command Sharing:** Both `ent` and `entc` CLIs share the same command implementations via base package. The only difference is that entc provides an additional `migrate` callback to GenerateCmd() for backward compatibility with old import paths.

**Package Path Resolution:** PkgPath() handles the common case where generated code target directory doesn't exist yet (e.g., `--target ./ent/gen` before mkdir). It walks up the tree to find the Go module root and reconstructs the package path.

**Printer Design:** The printer uses reflection to generically handle gen.Field properties, making it resilient to schema field additions without requiring code changes. Column alignment is computed per-row based on whether fields contain numeric values.
