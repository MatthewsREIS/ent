<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-02-08 -->

# entc/integration/ - Integration Test Suites

Comprehensive test suites validating the code generator against real schema scenarios. Each subdirectory tests a specific feature or use case.

## Purpose

Integration tests verify that:
- Generated code compiles and runs without errors
- CRUD operations (Create, Read, Update, Delete) work correctly
- Queries produce correct results with proper filtering
- Edge relationships load and traverse correctly
- Schema features (indexes, constraints, cascade) behave as expected
- Edge cases and advanced features work together

## Main Test Files

- `integration_test.go` (106KB) - Comprehensive feature test suite
- `relation_test.go` (43KB) - Edge relationship tests
- `type_test.go` (8KB) - Type system validation
- `index_test.go` (2KB) - Index creation and usage
- `entql_test.go` (3KB) - EntQL filtering tests

## Test Feature Subdirectories

Each directory has its own schema (in `ent/schema/`) and test file:

### cascadelete/
**Tests:** Cascading delete behavior on edges.
- When a parent entity is deleted, dependent entities are automatically deleted.
- Tested for both O2M (one-to-many) and M2M (many-to-many) relationships.
- Schema: Post with Comments; User with Posts.

### config/
**Tests:** Schema-level configuration and metadata.
- Config struct generation and methods.
- Custom annotations and metadata handling.
- Policy and constraint configuration.

### customid/
**Tests:** Custom ID types and generation strategies.
- Non-integer ID types (UUID, string, etc.).
- Custom ID generation functions.
- ID uniqueness and constraints.

### edgefield/
**Tests:** Fields on edges (join table attributes).
- M2M edges with additional data (weights, timestamps, etc.).
- Field access through edge queries.
- Schema: User → Group with join table containing metadata.

### edgeschema/
**Tests:** Edge schemas (join tables as first-class entities).
- Treating an M2M edge as an explicit entity type.
- Direct access to edge records.
- Schema: User ↔ Group via GroupUser entity.

### gremlin/
**Tests:** Gremlin graph database backend.
- CosmosDB, JanusGraph compatibility.
- Graph traversal queries and predicates.
- Gremlin-specific features and operations.

### hooks/
**Tests:** Lifecycle hooks on create, update, delete.
- Hook registration and execution.
- Hook error handling and rollback.
- Multiple hooks on same operation.

### idtype/
**Tests:** Different ID field types.
- int, int64, uint, uint64, string, UUID.
- ID constraints and validation.
- Mixed ID types across entities.

### json/
**Tests:** JSON field type and operations.
- JSON encoding/decoding.
- JSON querying and filtering (if supported by dialect).
- NULL JSON values.

### migrate/
**Tests:** Schema migration and versioning.
- Auto-migration on startup.
- Schema diff detection.
- Adding/removing fields, edges, indexes.

### multischema/
**Tests:** Multiple schema packages in one graph.
- Loading schemas from different packages.
- Cross-package edge references.
- Package organization and imports.

### privacy/
**Tests:** Privacy layer for access control.
- Privacy rules evaluated at query time.
- Filtering results based on context.
- Privacy rule composition and priority.

### template/
**Tests:** Custom template functionality.
- Overriding built-in templates.
- Template execution and output.
- Custom template hooks.

### ent/ (main)
**Core schema:** Base entities used across multiple tests.
- User, Group, Role, Post, Comment, File, Pet, Card, Comment, Generated
- Used as foundation for relationship tests.

## Test Data Setup

### Docker Compose
- `docker-compose.yaml` - Services for testing
  - PostgreSQL
  - MySQL
  - SQLite (in-memory, no container)
  - Gremlin (optional for graph tests)

```bash
# Run database services
docker-compose up -d

# Run tests with Docker containers
go test -v ./...

# Cleanup
docker-compose down
```

### Database Drivers

Tests support multiple database dialects:
- SQLite (default, in-memory)
- MySQL
- PostgreSQL
- Gremlin (graph database)

## Test Organization

Each test directory structure:
```
<feature>/
├── ent/
│   ├── schema/           # User-defined schemas
│   │   ├── user.go
│   │   └── post.go
│   ├── enttest/          # Test utilities (generated)
│   ├── client.go         # Client (generated)
│   ├── ent.go            # Package init (generated)
│   └── ... (other generated files)
├── <feature>_test.go     # Feature-specific tests
└── schema.go             # Schema definition (generated)
```

## Running Tests

```bash
# Run all integration tests
go test -v ./...

# Run specific test feature
cd cascadelete
go test -v ./...

# Run with specific database dialect
go test -v -run TestMyFeature -dialect=postgres ./...

# Run with race detector
go test -race ./...

# Run with coverage
go test -cover ./...
```

## Test Patterns

### Schema Definition
```go
// ent/schema/user.go
type User struct {
    ent.Schema
}

func (User) Fields() []ent.Field {
    return []ent.Field{
        field.String("name").Unique(),
    }
}

func (User) Edges() []ent.Edge {
    return []ent.Edge{
        edge.To("posts", Post.Type),
    }
}
```

### Code Generation
Run code generation before tests:
```bash
cd <feature>
go generate ./ent
```

### CRUD Tests
```go
func TestCreate(t *testing.T) {
    client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared")
    defer client.Close()

    user, err := client.User.Create().SetName("Alice").Save(context.Background())
    require.NoError(t, err)
    require.Equal(t, "Alice", user.Name)
}
```

### Query Tests
```go
func TestQuery(t *testing.T) {
    users, err := client.User.Query().Where(user.Name("Alice")).All(context.Background())
    require.NoError(t, err)
    require.Len(t, users, 1)
}
```

### Edge Tests
```go
func TestEdges(t *testing.T) {
    posts, err := user.QueryPosts().All(context.Background())
    require.NoError(t, err)
    require.Len(t, posts, 3)
}
```

## Supported Dialects

All tests run against:
- **sqlite3** - In-memory, lightweight, always available
- **mysql** - Via Docker, full SQL feature set
- **postgres** - Via Docker, advanced SQL features
- **gremlin** - Graph database, selected tests only

## Common Test Utilities (enttest/)

Generated test helper package:

- `Open(t, driver, dsn)` - Open test database connection
- `WithOptions(opts ...)` - Connection options (fixtures, migrations)
- Schema auto-migration and cleanup

## CI/CD Integration

Tests run in CI pipelines with:
- Multiple Go versions
- Multiple database versions
- Race detection enabled
- Coverage reporting

## Debugging Tests

```bash
# Verbose output with test logs
go test -v -run TestMyTest ./...

# Print SQL queries (if supported)
go test -v -sql ./...

# Enable logging
ENTGO_LOG=debug go test -v ./...

# Keep database after test (for inspection)
go test -v -keep-db ./...
```

## Adding New Integration Tests

1. Create new subdirectory under `integration/`
2. Define schema in `ent/schema/`
3. Generate code: `go generate ./ent`
4. Write tests in `<feature>_test.go`
5. Run: `go test -v ./...`

Example:
```bash
mkdir integration/myfeature
cd integration/myfeature
mkdir -p ent/schema
# Create schema files
go generate ./ent
# Create myfeature_test.go
go test -v ./...
```

## Related

- `entgo.io/ent/entc/gen` - Code generation engine
- `entgo.io/ent/dialect/sql` - SQL dialect
- `entgo.io/ent/dialect/gremlin` - Gremlin dialect
- Database drivers: `github.com/lib/pq`, `github.com/go-sql-driver/mysql`
