<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-02-08 -->

# entc/load/ - Schema Package Loading

Loads user-defined ent.Schema packages via the Go build toolchain and extracts their metadata.

## Purpose

Schema loading bridges the gap between user schema definitions and code generation:
- Discovers and parses schema files in a user package
- Extracts metadata (fields, edges, indexes, hooks, policies)
- Compiles and executes schema reflection to get runtime information
- Produces a SchemaSpec containing serializable schema descriptors

This allows code generation to work programmatically without requiring the schema package to be importable directly.

## Key Types

### SchemaSpec
Serializable representation of all loaded schemas:
```go
type SchemaSpec struct {
    Schemas []*Schema      // Loaded schema descriptors
    PkgPath string         // Go package path (e.g., "github.com/org/proj/ent/schema")
    Module  *packages.Module // Go module info
}
```

### Config
Configuration for the schema loader:
```go
type Config struct {
    Path       string   // Path to schema package directory
    Names      []string // Specific schema names to load (empty = all)
    BuildFlags []string // Custom build flags for schema compilation
}
```

### Schema
Serializable ent.Schema descriptor:
```go
type Schema struct {
    Name          string           // Entity name
    Pos           string           // Source position (for error reporting)
    View          bool             // Is this a View (read-only)?
    Config        ent.Config       // Entity configuration
    Fields        []*Field         // Primitive fields
    Edges         []*Edge          // Entity relationships
    Indexes       []*Index         // Database indexes
    Hooks         []*Position      // Hook definitions
    Interceptors  []*Position      // Interceptor definitions
    Policy        []*Position      // Privacy policy methods
    Annotations   map[string]any   // Custom annotations
}
```

### Field
Serializable field descriptor:
```go
type Field struct {
    Name             string              // Field name
    Info             *field.TypeInfo     // Type information
    Tag              string              // Go struct tag
    Size             *int64              // Size constraint (strings, bytes)
    Enums            []struct{N, V string} // Enum values
    Unique           bool                // Uniqueness constraint
    Nillable         bool                // Can be NULL
    Optional         bool                // Optional on create
    Default          bool                // Has default value
    DefaultValue     any                 // Default value
    UpdateDefault    bool                // Default on update
    Immutable        bool                // Cannot be modified
    Validators       int                 // Number of validators
    StorageKey       string              // Database column name
    Sensitive        bool                // PII field (for privacy)
    SchemaType       map[string]string   // Dialect-specific types
    Annotations      map[string]any      // Custom annotations
    Comment          string              // Documentation comment
    Deprecated       bool                // Is deprecated
    DeprecatedReason string              // Deprecation reason
}
```

### Edge
Serializable edge descriptor:
```go
type Edge struct {
    Name        string    // Edge name
    Type        string    // Target type name
    Field       string    // Foreign key field (for O2M)
    RefName     string    // Inverse edge name
    Ref         *Edge     // Inverse edge reference
    Through     *struct{} // Through edge (for M2M)
    Unique      bool      // Uniqueness constraint
    Inverse     bool      // Is inverse edge
    Required    bool      // Required edge
    Immutable   bool      // Cannot be modified
    Annotations map[string]any // Custom annotations
}
```

### Index
Serializable index descriptor:
```go
type Index struct {
    Fields      []*Field          // Indexed fields
    Unique      bool              // Unique index
    Annotations map[string]any    // Custom annotations
}
```

### Position
Position tracking for hooks, interceptors, and policies:
```go
type Position struct {
    Index      int  // Index in field/hook list
    MixedIn    bool // Was mixed-in from mixin
    MixinIndex int  // Mixin index
}
```

## Core Files

**load.go** (9.9KB)
- Config type and Load() method
- Schema discovery in filesystem
- AST parsing of schema definitions
- Compilation and execution of schema package
- Build template for temporary Go program

**schema.go** (15.2KB)
- Schema, Field, Edge, Index types
- Serialization/deserialization (JSON)
- Reflection on ent.Schema interface
- Field and edge metadata extraction
- Hook/interceptor/policy discovery

## Loading Process

1. **Path Resolution**
   - Resolve schema directory path
   - Find all `.go` files in the directory

2. **AST Parsing**
   - Parse Go source files into AST
   - Find all types implementing `ent.Schema`
   - Extract schema names and method definitions

3. **Schema Discovery**
   - Identify Fields() method
   - Identify Edges() method
   - Identify Hooks() method
   - Identify Interceptors() method
   - Identify Policy() method
   - Identify Mixin() method

4. **Code Generation**
   - Generate temporary Go program that:
     - Imports the user schema package
     - Instantiates each schema type
     - Calls Fields(), Edges(), etc.
     - Serializes metadata to JSON
   - Compile and execute temporary program

5. **Metadata Extraction**
   - Parse JSON output from temporary program
   - Build SchemaSpec with all schemas
   - Return SchemaSpec to caller

## Example Usage

```go
// In entc.go or user code
cfg := &load.Config{
    Path: "./ent/schema",
    Names: []string{},  // Load all schemas
    BuildFlags: []string{},
}

spec, err := cfg.Load()
if err != nil {
    return err
}

// spec.Schemas contains all loaded schemas
// spec.PkgPath is the Go package path
for _, schema := range spec.Schemas {
    fmt.Printf("%s: %d fields, %d edges\n",
        schema.Name, len(schema.Fields), len(schema.Edges))
}
```

## Field Type Support

Supported Go types via `field.TypeInfo`:
- Primitives: int, int64, uint, uint64, string, bool, float32, float64
- Time: time.Time, time.Duration
- UUID: uuid.UUID
- JSON: json.RawMessage
- Enums (custom type wrappers)
- Custom types (with ValueScanner)

## Edge Relationship Types

- **O2O** (One-to-One) - Single target entity
- **O2M** (One-to-Many) - Multiple target entities
- **M2M** (Many-to-Many) - Via join table/through type

## Hook & Policy Discovery

Loader identifies:
- **Hooks** - Methods on Schema that define lifecycle hooks
- **Interceptors** - Methods that intercept operations
- **Policy** - Privacy policy method (if defined)

## Mixin Support

Schemas can use mixins for shared fields/edges:
- Fields/edges mixed-in from mixin are tracked
- Position.MixedIn indicates mixed-in origin
- MixinIndex points to mixin in hierarchy

## Build Template

Schema loader uses an embedded build template to create a temporary Go program:
- Imports user schema package
- Instantiates each schema
- Calls reflection on Fields(), Edges(), Hooks(), etc.
- Outputs JSON-serialized metadata

This approach avoids directly importing the schema package and allows the loader to run on different platforms/architectures.

## Error Handling

Common errors:
- **Missing schema directory** - Path not found or not a directory
- **No schemas found** - No types implementing ent.Schema
- **Build failure** - Compilation error in schema package
- **Reflection failure** - Schema doesn't properly implement ent.Schema interface

## Testing

Run tests:
```bash
go test -v ./...
```

Tests exercise:
- Schema discovery in various directory structures
- Field/edge/index extraction
- Hook and interceptor identification
- Mixin handling
- Serialization/deserialization

## Testdata

The `testdata/` directory contains fixture schemas for testing various scenarios:
- Different field types
- Complex relationship patterns
- Mixins and inheritance
- Custom annotations

## Related

- `entgo.io/ent/schema` - User schema API
- `entgo.io/ent/schema/field` - Field type definitions
- `entgo.io/ent/schema/edge` - Edge type definitions
- `golang.org/x/tools/go/packages` - Go package loading
