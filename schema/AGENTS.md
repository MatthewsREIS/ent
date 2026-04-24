<!-- Parent: ../AGENTS.md -->
<!-- Generated: 2026-02-08 -->

# Schema Definition API for Ent

This is the public API for defining entity schemas in `entgo.io/ent/schema`. All exports from this package are part of the user-facing interface. Exercise extreme care with backward compatibility.

## Package Structure

- **`schema.go`** - Core annotation interfaces and Comment annotation
- **`field/`** - Field type builders and descriptors (string, int, float, bool, time, json, uuid, enum, bytes, other)
- **`field/type.go`** - Type information and runtime type reflection utilities
- **`field/numeric.go`** - Numeric field builders (int8-int64, uint8-uint64, float32-float64)
- **`edge/`** - Relationship builders (To, From) and storage key configuration
- **`index/`** - Index builders for fields and edges
- **`mixin/`** - Built-in mixins (Schema, CreateTime, UpdateTime, Time) and annotation helpers

## Key Design Principles

### Field API (schema/field/)

All field constructors return builder types that fluently chain configuration:

```go
// Signature pattern
func FieldType(name string) *fieldTypeBuilder
```

**Builders implement:**
- A `Descriptor() *Descriptor` method returning the final configuration
- Common methods across most types: `Optional()`, `Nillable()`, `Immutable()`, `Comment()`, `StructTag()`, `StorageKey()`, `SchemaType()`, `Annotations()`, `Deprecated()`
- Type-specific validators: `Unique()`, `Match()` (string), `Min()/Max()/Range()` (numeric), etc.
- Default value configuration: `Default()`, `DefaultFunc()`, `UpdateDefault()`

**Important Constraints:**
- Fields are **required by default**. Edges are optional by default.
- `Nillable` = pointer in struct. `Optional` = nullable in database.
- `Immutable` fields cannot be updated after creation.
- Numeric fields with custom Go types must implement an `Add(T) T` method to support mutations.

### Field Types

| Constructor | Type | Database | Notes |
|---|---|---|---|
| `String(name)` | string | VARCHAR | supports Match(), MinLen(), MaxLen() |
| `Text(name)` | string (unlimited) | LONGTEXT/TEXT | no size limit |
| `Bytes(name)` | []byte | BLOB | supports MinLen(), MaxLen() |
| `Bool(name)` | bool | BOOLEAN | - |
| `Time(name)` | time.Time | TIMESTAMP | use with Default(time.Now) |
| `Int(name)` | int | INTEGER | supports Min(), Max(), Range(), Positive(), Negative() |
| `Int8-Int64(name)` | int8-int64 | TINYINT-BIGINT | per-type numeric validators |
| `Uint, Uint8-Uint64(name)` | uint variants | UNSIGNED types | unsigned numeric validators |
| `Float(name)` | float64 | DOUBLE | supports Min(), Max(), Range(), Positive() |
| `Float32(name)` | float32 | FLOAT | as above |
| `Enum(name)` | string | VARCHAR/ENUM | use `.Values()` or `.NamedValues()` |
| `UUID(name, typ)` | custom UUID type | CHAR(36)/UUID | use with Default(uuid.New) |
| `JSON(name, typ)` | custom type | JSON/JSONB | supports any marshable type |
| `Strings(name)` | []string | JSON | slice convenience builder |
| `Ints(name)` | []int | JSON | slice convenience builder |
| `Floats(name)` | []float64 | JSON | slice convenience builder |
| `Any(name)` | interface{} | JSON | strongly discourage; use JSON with RawMessage |
| `Other(name, typ)` | custom (ValueScanner) | database-dependent | **must set SchemaType()** |

### Edge API (schema/edge/)

Two primary builders:

**To (forward edge):**
```go
edge.To("owner", User.Type).
    Unique().
    Required().
    Field("owner_id").
    StorageKey(edge.Column("owner_id")).
    Annotations(...)
```

**From (inverse edge):**
```go
edge.From("owned_items", Item.Type).
    Ref("owner").
    Unique()
```

**Storage Configuration:**
- `Field(name)` - binds edge to foreign key field
- `StorageKey(edge.Table(name), edge.Columns(...))` - for M2M edges
- `StorageKey(edge.Symbol(name))` - for O2O/O2M/M2O edges
- `Through(name, JoinType)` - explicit join table for M2M

**Edge Relationship Types:**
- `unique()` on both sides = one-to-one (O2O)
- `unique()` on "To" only = many-to-one (M2O)
- `unique()` on "From" only = one-to-many (O2M)
- Neither unique = many-to-many (M2M)

### Index API (schema/index/)

```go
index.Fields("name", "age").Unique().
    StorageKey("idx_name_age").
    Annotations(...)

index.Fields("name").
    Edges("parent").
    Unique()  // unique within each parent
```

- Indexes only work with SQL dialects (not Gremlin)
- Null values in unique indexes allow duplicates
- `Edges()` creates a sub-graph index (unique per edge scope)

### Mixin API (schema/mixin/)

**Built-in Mixins:**
- `mixin.Schema` - empty base mixin
- `mixin.CreateTime` - adds immutable `create_time` field (time.Now)
- `mixin.UpdateTime` - adds `update_time` field (Default + UpdateDefault: time.Now)
- `mixin.Time` - composes CreateTime + UpdateTime

**Annotation Helpers:**
```go
mixin.AnnotateFields(myMixin, entgql.OrderField("FIELD")).
    AnnotateEdges(myMixin, entgql.Bind())
```

## Common Modification Patterns

### Adding a New Field Type

If adding a new field type (e.g., `field.UUID`):

1. Create a builder struct in `field/field.go` or `field/numeric.go`
2. Implement all standard methods: `Optional()`, `Nillable()`, `Immutable()`, `Comment()`, `StructTag()`, `StorageKey()`, `Annotations()`, `Deprecated()`
3. Add type-specific validators as needed
4. Implement `Descriptor() *Descriptor` with appropriate type checks
5. Create a constructor function: `func FieldType(name string) *fieldTypeBuilder`
6. Update `field/type.go` if adding a new `Type` constant
7. Update numeric.go.tmpl if a numeric type

**Backward Compatibility:** Field constructors are the API surface. Never change method signatures or remove public methods.

### Adding Field Options

New builder methods are backward compatible. Add them to the appropriate builder type. Examples:
- `Sensitive()` marks PII fields
- `Match(regexp)` adds regex validation
- `GoType()` overrides generated type
- `ValueScanner()` provides custom serialization

### Changing Field Descriptors

The `Descriptor` struct is read-only by users. Additions are safe. Removals or renames break backward compatibility.

### Edge Modifications

Edge builders (`assocBuilder`, `inverseBuilder`) are fluent. New methods are safe. Do not remove or change existing method signatures.

### Annotation System

Annotations are extensible. New annotation types can be added without breaking existing code. The `Merger` interface allows custom merging logic.

## Testing Expectations

### Unit Tests
- Each field type builder should have tests for all public methods
- Test the `Descriptor()` output for valid states
- Test validator chain building
- Test default value handling

### Integration Expectations
- All field types must work with code generation
- All field types must work with at least one SQL dialect
- Numeric fields must support `Add()` operations
- Custom GoTypes must be properly serialized/deserialized

## Common Pitfalls for AI Agents

1. **Field Defaults:**
   - String/Bytes: are NOT optional by default. Must call `.Optional()` to make nullable.
   - Edges: ARE optional by default. Must call `.Required()` to make required.

2. **Nillable vs Optional:**
   - `Nillable()`: pointer type in struct. Use for types where nil makes semantic sense.
   - `Optional()`: NULL in database. Makes field nullable on creation.
   - Both can be true: creates `*T` pointer type with NULL storage.

3. **Numeric Validators:**
   - `Min()/Max()/Range()` are inclusive
   - `Positive()/Negative()` use special thresholds for floats (1e-06)
   - Custom numeric GoTypes must support `Add()` for mutations

4. **Custom Types (GoType, Other):**
   - Must implement `ValueScanner` interface or provide external scanner
   - `Other` types MUST call `SchemaType()` with per-dialect types
   - JSON types are automatically nillable for slices/pointers

5. **Edge Storage:**
   - Foreign key fields should be typed to match the referenced ID
   - M2M edges need explicit `Through()` or automatic join table
   - `StorageKey()` is per-dialect; validate against MySQL, Postgres, SQLite

6. **Immutability:**
   - `Immutable()` fields cannot be set in Update operations
   - All Create* mutations still set immutable fields
   - Use for audit fields: created_by, created_at

## Stability Status

This API is stable and widely used by thousands of projects. **All changes must be backward compatible.**

- Public constructors must never change signatures
- Public builder methods must never be removed
- Descriptor fields may be added (not removed)
- New field types can be added freely
- New validator methods can be added to builders
- New annotation types can be added freely

If major design changes are needed, implement new types alongside deprecated ones and deprecate gradually.
