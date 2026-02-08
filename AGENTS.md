<!-- Generated: 2026-02-08 -->

# AGENTS.md — ent Framework

This document provides guidance for AI agents and developers working in the **ent** framework repository.

**Project**: ent (Entity Framework for Go)
**Module**: `entgo.io/ent`
**Go Version**: 1.24+
**License**: Apache 2.0
**Origin**: Meta's internal entity framework, now open-source

---

## Quick Start for AI Agents

### Directory Map

```
ent/
├── ent.go                    # Core interfaces (Schema, Field, Edge, Index, etc.)
├── op_string.go              # Generated stringer for Op type
├── go.mod / go.sum           # Module definition
├── .golangci.yml             # Linter configuration
├── LICENSE, CONTRIBUTING.md  # Project governance
│
├── cmd/                      # CLI tools
│   ├── ent/                  # Schema explorer
│   ├── entc/                 # Code generator (main entry point)
│   ├── entfix/               # Migration fixer
│   └── internal/
│
├── dialect/                  # Database backends
│   ├── dialect.go            # Interface and registry
│   ├── sql/                  # SQL implementation
│   │   ├── builder.go        # Query builder
│   │   ├── driver.go         # Driver integration
│   │   ├── schema/           # Migration and schema logic
│   │   └── sqljson/          # JSON extension
│   ├── entsql/               # SQL annotations
│   └── gremlin/              # Graph database support
│       ├── client.go
│       ├── encoding/graphson/
│       └── graph/
│
├── entc/                     # Code generation
│   ├── gen/                  # Generation logic + templates
│   ├── load/                 # Schema loader (runtime reflection)
│   ├── internal/             # Generation helpers
│   └── integration/          # Integration tests
│
├── schema/                   # User schema API
│   ├── field/                # Field builders (Int, String, Text, JSON, etc.)
│   ├── edge/                 # Relationship builders (To, From, Required, Unique, etc.)
│   ├── index/                # Index builders
│   └── mixin/                # Mixin system for code reuse
│
├── entql/                    # EntQL query language
│   └── internal/
│
├── privacy/                  # Access control and privacy
│
├── examples/                 # 30+ example projects
│   ├── start/                # Minimal getting started
│   ├── m2m*/                 # Many-to-many patterns
│   ├── o2m*, o2o*           # Other relationship patterns
│   ├── privacyadmin/         # Privacy/auth example
│   ├── triggers/             # Hook system example
│   └── ... (15+ more)
│
├── doc/                      # Documentation
│   ├── md/                   # Markdown source files
│   └── website/              # Docusaurus website code
│
└── .github/                  # CI/CD workflows and templates
```

### Common Tasks

#### Adding a New Feature
1. **Plan**: Understand impact on `schema/`, `entc/gen/`, and `dialect/sql/` (if database-related).
2. **Implement**: Follow the existing codegen pattern (templates in `entc/gen/template/`).
3. **Test**: Add tests in the same package + integration tests in `entc/integration/`.
4. **Codegen**: Run `go generate ./...` after template changes.
5. **Examples**: Add a minimal example in `examples/` if user-facing.

#### Fixing a Bug
1. **Isolate**: Reproduce in an example or write a targeted test.
2. **Locate**: Bug is likely in `entc/gen/`, `dialect/`, or `schema/`.
3. **Fix & Test**: Fix the issue, then run `go test ./...` locally.
4. **Integration**: Run integration tests in `entc/integration/` (requires Docker).
5. **Verify**: Ensure generated code still works (check examples compile).

#### Schema or Type System Changes
1. **Update Core**: Modify `ent.go` interfaces (Field, Edge, Index, etc.) if needed.
2. **Update Codegen**: Adjust templates in `entc/gen/template/` to emit new code patterns.
3. **Update Loaders**: Modify `entc/load/` to parse new schema directives.
4. **Test Broadly**: Changes here affect all generated code — test across multiple examples.

---

## Development Workflow

### Prerequisites
- **Go 1.24+** (see `go.mod`)
- **Docker** (for integration tests with PostgreSQL, MySQL, SQLite)
- **atlas** CLI (optional, for migration inspection)

### Running Tests

**Unit tests** (fast, no database):
```bash
go test ./...
```

**Codegen tests**:
```bash
go test ./entc/...
```

**Integration tests** (requires Docker):
```bash
cd entc/integration
docker-compose -f docker-compose.yaml up -d
go test ./...
docker-compose down
```

**Example verification** (check that generated code compiles):
```bash
cd examples/start
go generate ./...
go build ./cmd/...
```

### Codegen Workflow

When modifying templates or codegen logic:
```bash
# Edit templates in entc/gen/template/
vim entc/gen/template/client.tmpl

# Regenerate code in examples
cd examples/start
go generate ./...

# Verify compilation
go build ./...

# Run tests
go test ./...
```

**Key Pattern**: `entc/gen/` contains templates (`.tmpl` files) that define generated code. Use `go generate` to trigger codegen after edits.

### Multi-Dialect Support

Ent supports SQL (PostgreSQL, MySQL, SQLite, MariaDB, TiDB) and Gremlin (graph databases).

- **SQL**: Primary focus. Code path: `dialect/sql/` → `entc/gen/template/` (for SQL-specific templates).
- **Gremlin**: Secondary. Code path: `dialect/gremlin/` → separate templates in `entc/gen/template/gremlin/`.

When adding a feature:
1. Check if it applies to both dialects or only SQL.
2. Add dialect-specific logic in `dialect/sql/` or `dialect/gremlin/`.
3. Update codegen templates accordingly.
4. Test with integration tests covering all dialects.

---

## Code Organization & Conventions

### Go Conventions
- **Formatting**: `gofmt` (enforced by CI)
- **Linting**: `golangci-lint` (see `.golangci.yml`)
- **Testing**: Table-driven tests preferred
- **Errors**: Use `fmt.Errorf` with wrapped errors (`%w`)
- **Context**: Always accept `context.Context` for cancellation and timeouts

### Codegen Patterns

Generated code is marked with a header comment:
```go
// Code generated by entc, DO NOT EDIT.

package ent
```

**Never manually edit generated code.** Regenerate it via `go generate ./...`.

### Schema Definition Pattern

Users define schemas like this:
```go
// ent/schema/user.go
type User struct { ent.Schema }

func (User) Fields() []ent.Field {
	return []ent.Field{
		field.Int("id"),
		field.String("name"),
	}
}

func (User) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("posts", Post.Type),
	}
}
```

The codegen loads these at `entc/load/`, inspects them via reflection, and generates typed clients.

### Key Interfaces (in `ent.go`)

- **Interface**: Schema definition interface (methods: Type, Fields, Edges, etc.)
- **Field**: Field descriptor wrapper (calls `field.Descriptor()`)
- **Edge**: Edge descriptor wrapper (calls `edge.Descriptor()`)
- **Index**: Index descriptor wrapper (calls `index.Descriptor()`)
- **Mixin**: Composable schema extension (reusable Fields/Edges/Hooks)
- **Policy**: Privacy/access control (per-operation rules)
- **Hook**: Mutation lifecycle hooks (Before/After Create/Update/Delete)
- **Interceptor**: Query-level intercepts (for logging, tracing, etc.)

---

## Dependency Overview

### Key External Dependencies
- **atlas** (`ariga.io/atlas`): Migration framework, schema diffing
- **inflect** (`go-openapi/inflect`): String pluralization (for table/edge naming)
- **uuid** (`google/uuid`): UUID generation
- **cobra** (`spf13/cobra`): CLI framework (for `entc` command)
- **websocket** (`gorilla/websocket`): WebSocket support in Gremlin dialect

### Internal Imports
- **entc/gen**: The codegen engine — start here for code generation logic
- **entc/load**: Schema loader — uses reflection to parse user schemas
- **dialect**: Backend abstraction — SQL and Gremlin implementations
- **schema**: User-facing API for defining schemas (field builders, edge builders, etc.)
- **privacy**: Access control framework

---

## Testing Strategy

### Unit Tests
Located in the same package (`*_test.go`). Test public functions and interfaces.

**Example**:
```go
// entc/gen/gen_test.go
func TestGenerator_Generate(t *testing.T) {
	// ...
}
```

### Integration Tests
Located in `entc/integration/`. These test end-to-end codegen + execution against real databases.

**Running**:
```bash
cd entc/integration
docker-compose up -d
go test ./...
docker-compose down
```

### Example Projects
Located in `examples/`. These serve dual purposes:
1. **Documentation**: Show idiomatic usage patterns
2. **Regression tests**: Ensure generated code compiles and runs

When you change codegen, regenerate examples:
```bash
cd examples/start
go generate ./...
go build ./...  # Verify compilation
```

---

## Common Pitfalls

### Pitfall 1: Manually Editing Generated Code
**Wrong**: Edit `.go` files that have `// Code generated by entc, DO NOT EDIT.`
**Right**: Edit templates in `entc/gen/template/` and run `go generate ./...`

### Pitfall 2: Skipping Integration Tests
**Wrong**: Only running `go test ./...` locally without Docker.
**Right**: Run full integration test suite before submitting changes to `entc/` or `dialect/`.

### Pitfall 3: Forgetting to Update Examples
**Wrong**: Adding a feature but not adding a simple example.
**Right**: Add a minimal example in `examples/` that demonstrates the feature. Ensure it compiles.

### Pitfall 4: Not Testing All Dialects
**Wrong**: Testing only SQL features.
**Right**: Check if feature applies to Gremlin too. If yes, test both via integration tests.

### Pitfall 5: Breaking Schema Compatibility
**Wrong**: Changing the `ent.Schema` interface without versioning.
**Right**: New methods on `ent.Schema` must be optional or backward-compatible (use embedding).

---

## Agent Collaboration Patterns

### For Bug Fixes
1. **Debugger** → isolate the issue (write minimal test case)
2. **Executor** → implement fix in the identified package
3. **Test Engineer** → add regression test
4. **Verifier** → confirm fix works across examples + integration tests

### For Feature Development
1. **Analyst** → clarify requirements (schema changes? codegen changes? dialect-specific?)
2. **Architect** → design schema/interface changes if needed
3. **Executor** → implement codegen templates + core logic
4. **Verifier** → verify generated code is correct (compare old vs. new output)

### For Documentation Updates
1. **Writer** → update `.md` files in `doc/md/` or add example to `examples/`
2. **Executor** → if example code needed, ensure it compiles
3. **Verifier** → confirm example runs and output matches docs

---

## Verification Checklist

Before marking work complete:

- [ ] Code compiles: `go build ./...`
- [ ] Linter passes: `golangci-lint run`
- [ ] Unit tests pass: `go test ./...`
- [ ] Integration tests pass (if touching `entc/` or `dialect/`): see integration test section
- [ ] Examples regenerate cleanly: `cd examples/start && go generate ./... && go build ./...`
- [ ] Generated code has proper header comment
- [ ] No manual edits to generated files
- [ ] All new public functions/types are documented
- [ ] Backward compatibility maintained (no breaking schema changes)

---

## Useful Commands

```bash
# Run all tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Generate code in examples
cd examples/start && go generate ./...

# Lint the codebase
golangci-lint run

# Regenerate generated files (op_string.go, etc.)
go generate ./...

# Run integration tests (requires Docker)
cd entc/integration
docker-compose up -d
go test ./...
docker-compose down

# Check Go version
go version
```

---

## Key Files to Know

| File/Dir | Purpose | When to Touch |
|----------|---------|---------------|
| `ent.go` | Core interfaces | Schema/type system changes |
| `entc/gen/` | Code generation engine | Adding codegen features |
| `entc/gen/template/` | Go templates for generated code | Output format changes |
| `entc/load/` | Schema introspection | Parsing new schema annotations |
| `dialect/sql/` | SQL backend | SQL-specific features |
| `dialect/gremlin/` | Graph database backend | Graph-specific features |
| `schema/field/` | Field builder API | Adding new field types |
| `schema/edge/` | Edge/relationship builder API | Edge relationship features |
| `privacy/` | Access control | Authorization/policy changes |
| `examples/*/` | Reference projects | Feature demonstration |

---

## Questions? Check These Resources

1. **Codebase structure**: See CONTRIBUTING.md
2. **API usage**: See examples in `examples/`
3. **Codegen internals**: Read `entc/gen/gen.go` (entry point)
4. **Schema loading**: Read `entc/load/load.go`
5. **SQL dialect**: Read `dialect/sql/builder.go` and `dialect/sql/schema/migrate.go`

---

**Last Updated**: 2026-02-08
**Go Version**: 1.24+
**License**: Apache 2.0
