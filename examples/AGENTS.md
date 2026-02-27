<!-- Generated: 2026-02-08 -->
<!-- Parent: ../AGENTS.md -->

# AGENTS.md — examples

This document provides guidance for AI agents working in the **examples** directory.

**Directory**: `entgo.io/ent/examples`
**Purpose**: Self-contained, executable example projects demonstrating ent patterns
**Go Version**: 1.24+

---

## Overview

The `examples/` directory contains 30+ self-contained Go modules, each demonstrating specific ent patterns, features, and best practices. Each example is:

- A complete, runnable Go project
- Focused on a single pattern or feature
- Independently generated (contains its own `go.mod` and generated ent code)
- Tested by CI to ensure generated code compiles and runs

**Key Characteristics**:
- Each example has its own schema definitions in `ent/schema/`
- Generated ent code is NOT edited manually; regenerated via `go generate ./...`
- Examples serve dual purposes: documentation and regression testing

---

## Example Categories

### Relationship Patterns

These examples demonstrate various relationship types and cardinalities:

| Example | Pattern | Description |
|---------|---------|-------------|
| `o2o2types` | One-to-One with Types | O2O relationships with custom types |
| `o2obidi` | One-to-One Bidirectional | Symmetric O2O edges (e.g., mutual friends) |
| `o2orecur` | One-to-One Recursive | Self-referencing O2O (e.g., user profile) |
| `o2m2types` | One-to-Many with Types | O2M with custom field types |
| `o2mrecur` | One-to-Many Recursive | Self-referencing O2M (e.g., file tree) |
| `m2m2types` | Many-to-Many with Types | M2M relationships with custom types |
| `m2mbidi` | Many-to-Many Bidirectional | Symmetric M2M (e.g., mutual followers) |
| `m2mrecur` | Many-to-Many Recursive | Self-referencing M2M (e.g., job dependencies) |

### Feature Examples

| Example | Feature | Description |
|---------|---------|-------------|
| `compositetypes` | Composite Types | Using custom Go types as fields |
| `domaintypes` | Domain Types | Custom domain-specific types with JSON encoding |
| `edgeindex` | Edge Indexes | Adding indexes on relationship fields |
| `encryptfield` | Field Encryption | Encrypting sensitive fields at storage layer |
| `enumtypes` | Enum Types | Using Go enums and validation |
| `extensions` | Hooks/Extensions | Adding custom behavior via hooks |
| `fs` | File System | File storage integration |
| `functionalidx` | Functional Indexes | Indexes on computed/derived values |
| `jsonencode` | JSON Fields | Storing and querying JSON data |
| `migration` | Migrations | Running and managing schema migrations |
| `privacyadmin` | Privacy (Admin) | Role-based access control with admin override |
| `privacytenant` | Privacy (Tenant) | Multi-tenant authorization patterns |
| `rls` | Row-Level Security | Row-level access control policies |
| `start` | Getting Started | Minimal working example (entry point) |
| `traversal` | Graph Traversal | Walking relationships and paths |
| `triggers` | Hooks/Triggers | Mutation lifecycle hooks |
| `version` | Versioning | Implementing optimistic locking/versioning |
| `viewcomposite` | SQL Views (Composite) | Using database views with composite types |
| `viewschema` | SQL Views (Schema) | Using database views in schema |
| `entcpkg` | Entc Package | Using ent as a library (not CLI) |

---

## Common Tasks

### Running an Example

```bash
cd examples/start
go generate ./...     # Regenerate ent code
go build ./cmd/...    # Build the example
go run ./cmd/main.go  # Run the example
```

### Understanding Example Structure

Each example follows this pattern:

```
example-name/
├── go.mod              # Module definition (depends on parent ent)
├── ent/
│   ├── schema/         # User-defined schemas
│   │   ├── user.go
│   │   ├── post.go
│   │   └── ...
│   ├── entc.go         # Code generation directive (go:generate)
│   └── (generated code)
├── cmd/
│   └── main.go         # Entry point
├── Dockerfile          # (optional) for integration tests
└── README.md           # (optional) pattern explanation
```

### Modifying an Example

1. **Edit schemas** in `ent/schema/*.go`
2. **Run codegen**: `go generate ./...`
3. **Update code** using newly generated types
4. **Test**: Ensure the example still compiles and runs

Example schema modification:

```bash
cd examples/o2m2types
# Edit ent/schema/user.go to add a field
$EDITOR ent/schema/user.go
# Regenerate
go generate ./...
# Verify compilation
go build ./cmd/...
```

### Adding a New Example

1. **Create directory**: `mkdir -p examples/mynewexample`
2. **Create go.mod**: Reference parent ent module
3. **Create schema directory**: `mkdir -p ent/schema`
4. **Define schemas**: Add `*.go` files in `ent/schema/`
5. **Create entc.go**: Add `//go:generate entc -o ./ent` directive
6. **Generate code**: `go generate ./...`
7. **Write main.go**: Create example usage in `cmd/main.go`
8. **Test**: Ensure `go build ./cmd/...` works

### Updating Examples After Framework Changes

When ent changes (new features, API updates), all examples must be regenerated:

```bash
cd examples
for dir in */; do
  (cd "$dir" && go generate ./...)
done
# Then test compilation in each
for dir in */; do
  (cd "$dir" && go build ./cmd/...)
done
```

---

## Code Organization

### Schema Files (ent/schema/)

Define the data model. Example:

```go
// ent/schema/user.go
package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
)

type User struct {
	ent.Schema
}

func (User) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id").Immutable(),
		field.String("name"),
		field.String("email").Unique(),
	}
}

func (User) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("posts", Post.Type),
	}
}
```

### Generated Code (ent/)

**DO NOT EDIT** these files. They're regenerated via `go generate ./...`:

- `client.go`: Main database client
- `entc.go`: Code generation directive
- `mutation.go`, `query.go`: Operation builders
- `schema/`, `predicates.go`: Schema and filtering

### Main Usage (cmd/main.go)

Demonstrates the schema in action:

```go
package main

import (
	"context"
	"log"

	"mygithub/examples/myexample/ent"
	_ "github.com/mattn/go-sqlite3"
)

func main() {
	client, err := ent.Open("sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")
	if err != nil {
		log.Fatalf("failed opening connection to sqlite: %v", err)
	}
	defer client.Close()

	ctx := context.Background()
	if err := client.Schema.Create(ctx); err != nil {
		log.Fatalf("failed creating schema resources: %v", err)
	}

	// Use the client...
}
```

---

## Testing Examples

### Local Verification

Test each example after changes:

```bash
cd examples/start
go generate ./...     # Regenerate
go build ./...        # Should compile with no errors
go test ./...         # Run tests if present
```

### CI/CD Integration

Examples are tested in CI to ensure:
- Generated code compiles
- Examples run without panics
- Integration with database works (via Docker Compose)

---

## Common Pitfalls

### Pitfall 1: Manually Editing Generated Code

**Wrong**: Editing files in `ent/` directly
**Right**: Edit schemas in `ent/schema/`, run `go generate ./...`

### Pitfall 2: Forgetting to Regenerate After Schema Changes

**Wrong**: Changing a schema but not running `go generate`
**Right**: After any schema edit, run `go generate ./...`

### Pitfall 3: Not Testing All Examples

**Wrong**: Changing ent templates but only testing one example
**Right**: Regenerate and build all examples to catch regressions

### Pitfall 4: Breaking Dependency Chain

**Wrong**: Modifying `examples/*/go.mod` directly
**Right**: Examples inherit from parent ent; changes to parent ent propagate automatically

### Pitfall 5: Leaving Generated Files Dirty

**Wrong**: Uncommitted generated code changes in examples
**Right**: Examples always have clean generated code; regenerate before committing

---

## Verification Checklist

Before marking example work complete:

- [ ] Schema changes made only to `ent/schema/` files
- [ ] Code generated: `go generate ./...` completed without errors
- [ ] Compilation succeeds: `go build ./...` produces no errors
- [ ] Main function demonstrates the feature
- [ ] All imports are correct
- [ ] Generated files (in `ent/`) are NOT manually edited
- [ ] Example follows directory structure conventions
- [ ] README or comments explain the pattern
- [ ] Tests pass (if present): `go test ./...`

---

## Useful Commands

```bash
# Regenerate code in an example
cd examples/start && go generate ./...

# Build an example
cd examples/start && go build ./cmd/...

# Run an example
cd examples/start && go run ./cmd/main.go

# Regenerate all examples
cd examples && for d in */; do (cd "$d" && go generate ./...); done

# Build all examples
cd examples && for d in */; do (cd "$d" && go build ./...); done

# Lint an example
cd examples/start && golangci-lint run ./...
```

---

## Key Files Reference

| File | Purpose | When to Touch |
|------|---------|---------------|
| `ent/schema/*.go` | Schema definitions | When demonstrating new patterns or fields |
| `ent/entc.go` | Code generation directive | Rarely; only if codegen configuration changes |
| `cmd/main.go` | Example usage | When updating to demonstrate feature usage |
| `go.mod` | Module dependencies | Rarely; inherited from parent |

---

## Example Discovery

To find examples demonstrating a specific feature:

1. **Check directory names**: Most are self-descriptive (e.g., `m2mbidi` for bidirectional many-to-many)
2. **Search schema files**: `grep -r "field.String" examples/*/ent/schema/` for string fields
3. **Check README**: Some examples have `README.md` with explanation
4. **Inspect cmd/main.go**: Shows the feature in action

---

## Questions? Check These Resources

1. **Getting started**: `examples/start/cmd/main.go`
2. **Relationship patterns**: `examples/o2m2types/`, `examples/m2mbidi/`, etc.
3. **Feature examples**: Feature-named directories (e.g., `examples/triggers/`)
4. **Privacy patterns**: `examples/privacyadmin/`, `examples/privacytenant/`
5. **Advanced patterns**: `examples/traversal/`, `examples/viewcomposite/`

---

**Last Updated**: 2026-02-08
