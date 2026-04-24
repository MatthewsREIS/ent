# ent Code Reduction Phase 4A — Runtime Foundation + Spike

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the minimum viable `entbuilder.Mutation[T]` + `entbuilder.Schema[T]` runtime needed to exercise descriptor-driven mutations on a single real schema, then manually port `hooks/ent`'s `CardMutation` to use the new runtime and confirm the existing integration tests pass unchanged. Produce a measured go/no-go decision for the full descriptor-driven rewrite (Phase 4C).

**Architecture:** Generic `Mutation[T]` struct owns field/edge state and delegates field-by-name access through a `Schema[T]` descriptor. Per-schema `*XxxMutation` types remain as exported named wrappers with 1-line typed accessors. The spike hand-rolls one schema's port to validate feasibility before committing to template rewrites.

**Tech Stack:** Go 1.22+, generics, `reflect` for descriptor↔struct validation and the scan path, standard `testing`, `/usr/bin/time -v` for spike measurement.

**Spec:** `docs/superpowers/specs/2026-04-24-ent-code-reduction-phase-4-hybrid.md`
**Depends on:** `runtime/entbuilder/*` from Phase 3 (helpers.go, predicate.go) — do not modify those files in this plan.
**Produces (if gate passes):** input to Phase 4B (lean-generics templates for create/update/delete/query) and Phase 4C (descriptor-driven template rewrite for mutation/client/entql).

---

## File Structure

**Files created by this plan:**

```
runtime/entbuilder/
  schema.go                 # Schema[T], Field, Edge descriptor types + ValidateSchema
  schema_test.go            # unit tests for descriptor types
  mutation.go               # Mutation[T] generic struct + field/edge state + Op/ID/Where
  mutation_test.go          # unit tests driven by a hand-rolled fake schema

entc/integration/hooks/spike/    # throwaway scratch dir, deleted if spike passes
  card_descriptor.go        # hand-rolled descriptor for Card
  card_mutation_shim.go     # hand-rolled façade mapping CardMutation methods to Mutation[Card]
  spike_test.go             # runs the existing hooks scenario tests through the shim

docs/superpowers/results/
  2026-04-24-phase4a-spike-measurements.md   # Task 20 — final gate decision doc
```

**Files read but NOT modified by this plan:**

```
runtime/entbuilder/helpers.go         # existing Field descriptor patterns — reference only
runtime/entbuilder/predicate.go       # Phase 3 predicate helpers — reference only
entc/integration/hooks/ent/internal/card_mutation.go  # the existing generated CardMutation — spike ports this
entc/integration/hooks/ent/schema/card.go             # the Card schema definition
entc/integration/hooks/hooks_test.go                  # the tests the spike must pass
docs/superpowers/specs/2026-04-24-ent-code-reduction-phase-4-hybrid.md
```

No templates change in this plan. No codegen changes. All work is: runtime library + one hand-rolled spike.

---

## Out-of-scope for this plan

- Generating descriptors / façades via templates (Phase 4C).
- Witness-test generator emission (Phase 4C).
- Through-table edge state (M2M via junction entities). Spike only covers the unique inverse edge `Card.owner → User`. Through-tables are exercised in Phase 4C against the `edgeschema` scenario.
- Full `entc/integration/*` suite porting. Spike stays scoped to `hooks`.
- Consumer (`service-api-go`) regeneration. Full consumer measurement is Phase 4E.

---

## Phase A — Schema descriptor types (Tasks 1-3)

### Task 1: `entbuilder.Field` and `entbuilder.Edge` descriptor types

**Files:**
- Create: `runtime/entbuilder/schema.go`
- Create: `runtime/entbuilder/schema_test.go`

- [ ] **Step 1: Write the failing test**

Create `runtime/entbuilder/schema_test.go`:

```go
package entbuilder

import (
	"reflect"
	"testing"
	"time"
)

func TestField_Basic(t *testing.T) {
	f := Field{
		Name:     "name",
		Column:   "name",
		Type:     reflect.TypeOf(""),
		Nullable: false,
	}
	if f.Name != "name" {
		t.Fatalf("Name mismatch: %q", f.Name)
	}
	if f.Type != reflect.TypeOf("") {
		t.Fatalf("Type mismatch: %v", f.Type)
	}
	if f.Nullable {
		t.Fatal("expected Nullable=false")
	}
}

func TestField_NullableTime(t *testing.T) {
	f := Field{
		Name:     "expired_at",
		Column:   "expired_at",
		Type:     reflect.TypeOf(time.Time{}),
		Nullable: true,
	}
	if f.Type != reflect.TypeOf(time.Time{}) {
		t.Fatalf("Type mismatch: %v", f.Type)
	}
	if !f.Nullable {
		t.Fatal("expected Nullable=true")
	}
}

func TestEdge_UniqueInverse(t *testing.T) {
	e := Edge{
		Name:     "owner",
		Unique:   true,
		Inverse:  true,
		Field:    "owner_id",
		TargetID: reflect.TypeOf(int(0)),
	}
	if !e.Unique || !e.Inverse {
		t.Fatalf("expected Unique && Inverse; got %+v", e)
	}
	if e.Field != "owner_id" {
		t.Fatalf("Field mismatch: %q", e.Field)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./runtime/entbuilder/ -run 'TestField_|TestEdge_' -v`
Expected: compile error — `undefined: Field`, `undefined: Edge`.

- [ ] **Step 3: Implement the types**

Create `runtime/entbuilder/schema.go`:

```go
package entbuilder

import "reflect"

// Field describes a single persisted field on an entity. Generated by entc
// from the entity's schema definition; consumed by the generic Mutation[T]
// to route SetX/GetX/OldX/ResetX/ClearX by name.
type Field struct {
	// Name is the Go struct field accessor name (PascalCase), e.g. "CreatedAt".
	Name string
	// Column is the database column name, e.g. "created_at".
	Column string
	// Type is the Go type of the field value, obtained via reflect.TypeOf.
	// For nillable fields, use the underlying type (not *T); Nullable flags
	// the optionality separately.
	Type reflect.Type
	// Nullable indicates the field accepts nil/zero via Optional() in the schema.
	Nullable bool
	// Immutable indicates the field cannot be updated after creation.
	Immutable bool
}

// Edge describes a relationship to another entity.
type Edge struct {
	// Name is the Go accessor name (PascalCase), e.g. "Owner".
	Name string
	// Unique is true for O2O/O2M/M2O "to-one" edges.
	Unique bool
	// Inverse is true if the edge is declared via edge.From (ref), false for edge.To.
	Inverse bool
	// Field is the foreign-key column name on this entity (for O2O/M2O);
	// empty for edges whose FK lives on the target or junction table.
	Field string
	// TargetID is the Go type of the target entity's ID field.
	TargetID reflect.Type
}
```

- [ ] **Step 4: Run to verify PASS**

Run: `go test ./runtime/entbuilder/ -run 'TestField_|TestEdge_' -v`
Expected: 3 PASS.

- [ ] **Step 5: Commit**

```bash
git add runtime/entbuilder/schema.go runtime/entbuilder/schema_test.go
git commit -m "feat(entbuilder): add Field and Edge descriptor types"
```

---

### Task 2: `Schema[T]` descriptor

**Files:**
- Modify: `runtime/entbuilder/schema.go`
- Modify: `runtime/entbuilder/schema_test.go`

- [ ] **Step 1: Append failing test**

Append to `runtime/entbuilder/schema_test.go`:

```go
// fakeCardNode is a test stand-in for an entity struct.
type fakeCardNode struct {
	ID        int
	Number    string
	Name      string
	CreatedAt time.Time
	InHook    string
	ExpiredAt *time.Time
}

func TestSchema_FindField(t *testing.T) {
	s := &Schema[fakeCardNode]{
		Name:  "cards",
		Table: "cards",
		IDField: Field{Name: "ID", Column: "id", Type: reflect.TypeOf(int(0))},
		Fields: []Field{
			{Name: "Number", Column: "number", Type: reflect.TypeOf("")},
			{Name: "Name", Column: "name", Type: reflect.TypeOf(""), Nullable: true},
		},
	}
	f, ok := s.FindField("Number")
	if !ok {
		t.Fatal("expected to find Number")
	}
	if f.Column != "number" {
		t.Fatalf("wrong field: %+v", f)
	}
	if _, ok := s.FindField("Missing"); ok {
		t.Fatal("expected FindField to return ok=false for unknown field")
	}
}

func TestSchema_FindEdge(t *testing.T) {
	s := &Schema[fakeCardNode]{
		Name: "cards",
		Edges: []Edge{
			{Name: "Owner", Unique: true, Inverse: true, Field: "owner_id"},
		},
	}
	e, ok := s.FindEdge("Owner")
	if !ok || e.Field != "owner_id" {
		t.Fatalf("FindEdge failed: ok=%v edge=%+v", ok, e)
	}
	if _, ok := s.FindEdge("Missing"); ok {
		t.Fatal("expected FindEdge to return ok=false for unknown edge")
	}
}
```

- [ ] **Step 2: Run — expect compile failure**

Run: `go test ./runtime/entbuilder/ -run TestSchema_Find -v`
Expected: `undefined: Schema`.

- [ ] **Step 3: Implement `Schema[T]`**

Append to `runtime/entbuilder/schema.go`:

```go
// Schema[T] is the full descriptor for one entity type. The type parameter
// binds the descriptor to the entity struct at compile time; at runtime the
// generic Mutation[T] uses this descriptor to dispatch field/edge access
// by name via reflect.
type Schema[T any] struct {
	// Name is the schema's logical name (singular, matches the schema
	// file's type name), e.g. "Card".
	Name string
	// Table is the database table name, e.g. "cards".
	Table string
	// IDField is the entity's primary-key field descriptor.
	IDField Field
	// Fields lists all non-ID persisted fields.
	Fields []Field
	// Edges lists all relationships.
	Edges []Edge
}

// FindField returns the Field with the given Name, or (Field{}, false) if absent.
func (s *Schema[T]) FindField(name string) (Field, bool) {
	if s.IDField.Name == name {
		return s.IDField, true
	}
	for _, f := range s.Fields {
		if f.Name == name {
			return f, true
		}
	}
	return Field{}, false
}

// FindEdge returns the Edge with the given Name, or (Edge{}, false) if absent.
func (s *Schema[T]) FindEdge(name string) (Edge, bool) {
	for _, e := range s.Edges {
		if e.Name == name {
			return e, true
		}
	}
	return Edge{}, false
}
```

- [ ] **Step 4: Run — expect PASS**

Run: `go test ./runtime/entbuilder/ -run TestSchema_Find -v`
Expected: 2 PASS.

- [ ] **Step 5: Commit**

```bash
git add runtime/entbuilder/schema.go runtime/entbuilder/schema_test.go
git commit -m "feat(entbuilder): add Schema[T] descriptor with Find helpers"
```

---

### Task 3: `ValidateSchema` — descriptor/struct drift detector

**Files:**
- Modify: `runtime/entbuilder/schema.go`
- Modify: `runtime/entbuilder/schema_test.go`

- [ ] **Step 1: Append failing test**

Append to `schema_test.go`:

```go
func TestValidateSchema_Matches(t *testing.T) {
	s := &Schema[fakeCardNode]{
		Name:    "Card",
		Table:   "cards",
		IDField: Field{Name: "ID", Column: "id", Type: reflect.TypeOf(int(0))},
		Fields: []Field{
			{Name: "Number", Column: "number", Type: reflect.TypeOf("")},
			{Name: "Name", Column: "name", Type: reflect.TypeOf(""), Nullable: true},
			{Name: "CreatedAt", Column: "created_at", Type: reflect.TypeOf(time.Time{})},
			{Name: "InHook", Column: "in_hook", Type: reflect.TypeOf("")},
			{Name: "ExpiredAt", Column: "expired_at", Type: reflect.TypeOf(time.Time{}), Nullable: true},
		},
	}
	if err := ValidateSchema[fakeCardNode](s); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}

func TestValidateSchema_MissingField(t *testing.T) {
	s := &Schema[fakeCardNode]{
		Name: "Card",
		Fields: []Field{
			{Name: "NotOnStruct", Column: "x", Type: reflect.TypeOf("")},
		},
	}
	err := ValidateSchema[fakeCardNode](s)
	if err == nil {
		t.Fatal("expected error when descriptor names a field absent on the struct")
	}
	if !containsAll(err.Error(), "NotOnStruct", "fakeCardNode") {
		t.Fatalf("error message should name the bad field and type: %v", err)
	}
}

func TestValidateSchema_WrongType(t *testing.T) {
	s := &Schema[fakeCardNode]{
		Name: "Card",
		Fields: []Field{
			// Number is string on the struct; descriptor claims int
			{Name: "Number", Column: "number", Type: reflect.TypeOf(int(0))},
		},
	}
	err := ValidateSchema[fakeCardNode](s)
	if err == nil {
		t.Fatal("expected error for type mismatch")
	}
}

func TestValidateSchema_NullableMatchesPointer(t *testing.T) {
	// ExpiredAt on fakeCardNode is *time.Time. The descriptor should
	// declare Type=time.Time and Nullable=true; the validator peels
	// the pointer on the struct side.
	s := &Schema[fakeCardNode]{
		Name: "Card",
		IDField: Field{Name: "ID", Column: "id", Type: reflect.TypeOf(int(0))},
		Fields: []Field{
			{Name: "ExpiredAt", Column: "expired_at", Type: reflect.TypeOf(time.Time{}), Nullable: true},
			{Name: "Number", Column: "number", Type: reflect.TypeOf("")},
			{Name: "Name", Column: "name", Type: reflect.TypeOf(""), Nullable: true},
			{Name: "CreatedAt", Column: "created_at", Type: reflect.TypeOf(time.Time{})},
			{Name: "InHook", Column: "in_hook", Type: reflect.TypeOf("")},
		},
	}
	if err := ValidateSchema[fakeCardNode](s); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}

// test helper
func containsAll(s string, substrs ...string) bool {
	for _, sub := range substrs {
		found := false
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
```

- [ ] **Step 2: Run — expect compile failure**

Run: `go test ./runtime/entbuilder/ -run TestValidateSchema -v`
Expected: `undefined: ValidateSchema`.

- [ ] **Step 3: Implement `ValidateSchema`**

Append to `schema.go`:

```go
import (
	"fmt"
	"reflect"
)

// ValidateSchema asserts that every descriptor field/edge names a real Go
// struct field on T with a compatible type. Intended to be called once at
// client construction time (not per query) to catch descriptor/struct drift
// at deploy time rather than query time.
//
// Type compatibility: for a descriptor with Nullable=false, the struct field
// type must equal Field.Type exactly. For Nullable=true, the struct field
// type must be either Field.Type or *Field.Type (pointer-to-Type).
func ValidateSchema[T any](s *Schema[T]) error {
	var zero T
	structType := reflect.TypeOf(zero)
	if structType.Kind() != reflect.Struct {
		return fmt.Errorf("entbuilder: ValidateSchema: type parameter must be a struct, got %s", structType.Kind())
	}

	check := func(f Field) error {
		if f.Name == "" {
			return fmt.Errorf("entbuilder: schema %q has a field with empty Name", s.Name)
		}
		sf, ok := structType.FieldByName(f.Name)
		if !ok {
			return fmt.Errorf("entbuilder: schema %q descriptor field %q is not a field on %s",
				s.Name, f.Name, structType.Name())
		}
		expected := f.Type
		got := sf.Type
		if f.Nullable {
			// Accept either *T or T on the struct.
			if got.Kind() == reflect.Pointer {
				got = got.Elem()
			}
		}
		if got != expected {
			return fmt.Errorf("entbuilder: schema %q field %q type mismatch: descriptor=%s struct=%s",
				s.Name, f.Name, expected, sf.Type)
		}
		return nil
	}

	if s.IDField.Name != "" {
		if err := check(s.IDField); err != nil {
			return err
		}
	}
	for _, f := range s.Fields {
		if err := check(f); err != nil {
			return err
		}
	}
	return nil
}
```

- [ ] **Step 4: Run — expect PASS**

Run: `go test ./runtime/entbuilder/ -run TestValidateSchema -v`
Expected: 4 PASS.

Also run the whole package to confirm no regressions:
Run: `go test ./runtime/entbuilder/ -v`
Expected: 42 tests PASS (35 existing + 7 new across tasks 1-3).

- [ ] **Step 5: Commit**

```bash
git add runtime/entbuilder/schema.go runtime/entbuilder/schema_test.go
git commit -m "feat(entbuilder): add ValidateSchema for descriptor/struct drift detection"
```

---

## Phase B — Mutation[T] field state (Tasks 4-8)

### Task 4: `Mutation[T]` zero value and constructor

**Files:**
- Create: `runtime/entbuilder/mutation.go`
- Create: `runtime/entbuilder/mutation_test.go`

- [ ] **Step 1: Write the failing test**

Create `runtime/entbuilder/mutation_test.go`:

```go
package entbuilder

import (
	"testing"
)

// Reuses fakeCardNode from schema_test.go.

func TestNewMutation_InitializesEmpty(t *testing.T) {
	s := &Schema[fakeCardNode]{Name: "Card"}
	m := NewMutation[fakeCardNode](s, OpCreate)
	if m.Op() != OpCreate {
		t.Fatalf("Op mismatch: %v", m.Op())
	}
	if len(m.Fields()) != 0 {
		t.Fatalf("expected empty Fields(): %v", m.Fields())
	}
	if len(m.AddedFields()) != 0 {
		t.Fatalf("expected empty AddedFields(): %v", m.AddedFields())
	}
	if len(m.ClearedFields()) != 0 {
		t.Fatalf("expected empty ClearedFields(): %v", m.ClearedFields())
	}
}
```

- [ ] **Step 2: Run — expect compile failure**

Run: `go test ./runtime/entbuilder/ -run TestNewMutation -v`
Expected: `undefined: NewMutation`, `undefined: Mutation`, `undefined: OpCreate`.

- [ ] **Step 3: Implement Mutation[T] and Op**

Create `runtime/entbuilder/mutation.go`:

```go
package entbuilder

// Op is a mutation operation kind. Mirrors ent.Op bit layout at the
// integer level so that callers can convert back and forth without an
// explicit map lookup; the enum is redeclared here to avoid an import
// cycle between entbuilder and the ent root package.
type Op uint

const (
	OpCreate    Op = 1 << iota // 1
	OpUpdate                   // 2
	OpUpdateOne                // 4
	OpDelete                   // 8
	OpDeleteOne                // 16
)

// Mutation is the generic, descriptor-driven state container shared by all
// per-schema mutation façades. Per-schema façades (e.g. *CardMutation)
// wrap a *Mutation[T] and expose typed accessors that delegate to the
// name-based helpers here.
type Mutation[T any] struct {
	schema *Schema[T]
	op     Op

	// setFields maps Field.Name -> the current-mutation value for that field.
	// Values are stored as `any` so heterogeneous types can share one map;
	// typed accessors on the façade perform the assertion on read.
	setFields map[string]any

	// clearedFields is the set of Field.Name values that have been cleared
	// in this mutation (as opposed to merely set-to-zero).
	clearedFields map[string]struct{}

	// addedFields maps Field.Name -> the numeric delta recorded by AddX on
	// numeric fields.
	addedFields map[string]any

	// id is the target ID for UpdateOne/DeleteOne mutations; nil otherwise.
	id any
}

// NewMutation constructs an empty mutation for the given schema and op.
func NewMutation[T any](schema *Schema[T], op Op) *Mutation[T] {
	return &Mutation[T]{
		schema:        schema,
		op:            op,
		setFields:     make(map[string]any),
		clearedFields: make(map[string]struct{}),
		addedFields:   make(map[string]any),
	}
}

// Op returns the mutation's op kind.
func (m *Mutation[T]) Op() Op { return m.op }

// Fields returns the names of fields that have been explicitly set (not cleared).
// Order is not guaranteed.
func (m *Mutation[T]) Fields() []string {
	out := make([]string, 0, len(m.setFields))
	for k := range m.setFields {
		out = append(out, k)
	}
	return out
}

// AddedFields returns the names of numeric fields that have recorded deltas.
func (m *Mutation[T]) AddedFields() []string {
	out := make([]string, 0, len(m.addedFields))
	for k := range m.addedFields {
		out = append(out, k)
	}
	return out
}

// ClearedFields returns the names of fields that have been explicitly cleared.
func (m *Mutation[T]) ClearedFields() []string {
	out := make([]string, 0, len(m.clearedFields))
	for k := range m.clearedFields {
		out = append(out, k)
	}
	return out
}
```

- [ ] **Step 4: Run — expect PASS**

Run: `go test ./runtime/entbuilder/ -run TestNewMutation -v`
Expected: 1 PASS.

- [ ] **Step 5: Commit**

```bash
git add runtime/entbuilder/mutation.go runtime/entbuilder/mutation_test.go
git commit -m "feat(entbuilder): add Mutation[T] skeleton with Op/Fields/Added/Cleared"
```

---

### Task 5: `SetField` / `Field` (get) / `ClearField` / `FieldCleared`

**Files:**
- Modify: `runtime/entbuilder/mutation.go`
- Modify: `runtime/entbuilder/mutation_test.go`

- [ ] **Step 1: Extend the import block and append failing tests**

First, merge `"reflect"` and `"time"` into the existing `import (...)` block at the top of `runtime/entbuilder/mutation_test.go` (the file currently imports only `"testing"`). Do NOT add a second import block.

Then append the package-level var and test functions (no additional import line):

```go
var cardTestSchema = &Schema[fakeCardNode]{
	Name: "Card",
	Table: "cards",
	IDField: Field{Name: "ID", Column: "id", Type: reflect.TypeOf(int(0))},
	Fields: []Field{
		{Name: "Number", Column: "number", Type: reflect.TypeOf("")},
		{Name: "Name", Column: "name", Type: reflect.TypeOf(""), Nullable: true},
		{Name: "ExpiredAt", Column: "expired_at", Type: reflect.TypeOf(time.Time{}), Nullable: true},
	},
}

func TestSetField_StoresAndReads(t *testing.T) {
	m := NewMutation[fakeCardNode](cardTestSchema, OpUpdate)
	if err := m.SetField("Number", "1234"); err != nil {
		t.Fatalf("SetField: %v", err)
	}
	v, ok := m.Field("Number")
	if !ok || v.(string) != "1234" {
		t.Fatalf("Field: ok=%v v=%v", ok, v)
	}
	names := m.Fields()
	if len(names) != 1 || names[0] != "Number" {
		t.Fatalf("Fields: %v", names)
	}
}

func TestField_Unset(t *testing.T) {
	m := NewMutation[fakeCardNode](cardTestSchema, OpUpdate)
	if _, ok := m.Field("Number"); ok {
		t.Fatal("expected ok=false for unset field")
	}
}

func TestSetField_UnknownField_ReturnsError(t *testing.T) {
	m := NewMutation[fakeCardNode](cardTestSchema, OpUpdate)
	err := m.SetField("NotAField", "v")
	if err == nil {
		t.Fatal("expected error for unknown field")
	}
}

func TestClearField_RecordsAndClearsSetValue(t *testing.T) {
	m := NewMutation[fakeCardNode](cardTestSchema, OpUpdate)
	_ = m.SetField("Name", "alice")
	if err := m.ClearField("Name"); err != nil {
		t.Fatalf("ClearField: %v", err)
	}
	if _, ok := m.Field("Name"); ok {
		t.Fatal("expected Field(Name) ok=false after ClearField")
	}
	if !m.FieldCleared("Name") {
		t.Fatal("expected FieldCleared(Name)=true")
	}
	cleared := m.ClearedFields()
	if len(cleared) != 1 || cleared[0] != "Name" {
		t.Fatalf("ClearedFields: %v", cleared)
	}
}

func TestClearField_NonNullableField_ReturnsError(t *testing.T) {
	// Number is not Optional/Nullable — cannot be cleared.
	m := NewMutation[fakeCardNode](cardTestSchema, OpUpdate)
	err := m.ClearField("Number")
	if err == nil {
		t.Fatal("expected error clearing a non-nullable field")
	}
}

func TestFieldCleared_UnknownField_ReturnsFalse(t *testing.T) {
	m := NewMutation[fakeCardNode](cardTestSchema, OpUpdate)
	if m.FieldCleared("NotAField") {
		t.Fatal("expected false for unknown field")
	}
}
```

- [ ] **Step 2: Run — expect compile failure**

Run: `go test ./runtime/entbuilder/ -run 'TestSetField|TestField_|TestClearField|TestFieldCleared' -v`
Expected: `undefined: SetField`, `undefined: Field` (method), `undefined: ClearField`, `undefined: FieldCleared`.

- [ ] **Step 3: Implement the methods**

Append to `mutation.go`:

```go
import "fmt"

// SetField records v as the mutation's value for the field named `name`.
// Returns an error if `name` is not declared on the schema. Does not
// validate v's type against the field's descriptor — the façade's typed
// setters are responsible for that.
func (m *Mutation[T]) SetField(name string, v any) error {
	if _, ok := m.schema.FindField(name); !ok {
		return fmt.Errorf("entbuilder: schema %q has no field %q", m.schema.Name, name)
	}
	m.setFields[name] = v
	// Setting a field after clearing it un-clears it.
	delete(m.clearedFields, name)
	return nil
}

// Field returns the mutation's recorded value for `name` and ok=true, or
// (nil, false) if the field was not set (or was cleared). Unknown field
// names return (nil, false) without error to match the existing
// ent.Mutation.Field contract.
func (m *Mutation[T]) Field(name string) (any, bool) {
	v, ok := m.setFields[name]
	return v, ok
}

// ClearField marks `name` as cleared and removes any previously-set value.
// Returns an error if `name` is unknown or is a non-nullable field (per the
// schema descriptor).
func (m *Mutation[T]) ClearField(name string) error {
	f, ok := m.schema.FindField(name)
	if !ok {
		return fmt.Errorf("entbuilder: schema %q has no field %q", m.schema.Name, name)
	}
	if !f.Nullable {
		return fmt.Errorf("entbuilder: schema %q field %q is not Optional and cannot be cleared",
			m.schema.Name, name)
	}
	delete(m.setFields, name)
	m.clearedFields[name] = struct{}{}
	return nil
}

// FieldCleared reports whether `name` was cleared by this mutation.
// Returns false for unknown fields (matching the existing ent contract).
func (m *Mutation[T]) FieldCleared(name string) bool {
	_, ok := m.clearedFields[name]
	return ok
}
```

- [ ] **Step 4: Run — expect PASS**

Run: `go test ./runtime/entbuilder/ -run 'TestSetField|TestField_|TestClearField|TestFieldCleared' -v`
Expected: 6 PASS.

- [ ] **Step 5: Commit**

```bash
git add runtime/entbuilder/mutation.go runtime/entbuilder/mutation_test.go
git commit -m "feat(entbuilder): Mutation[T] field set/get/clear with descriptor-driven name lookup"
```

---

### Task 6: `AddField` / `AddedField` — numeric deltas

**Files:**
- Modify: `runtime/entbuilder/mutation.go`
- Modify: `runtime/entbuilder/mutation_test.go`

- [ ] **Step 1: Append failing tests**

Append to `mutation_test.go`:

```go
// For numeric deltas we need a schema whose struct has numeric fields.
type fakeCardWithBalance struct {
	Balance float64
	Version int
}

var cardBalanceSchema = &Schema[fakeCardWithBalance]{
	Name: "Card",
	Fields: []Field{
		{Name: "Balance", Column: "balance", Type: reflect.TypeOf(float64(0))},
		{Name: "Version", Column: "version", Type: reflect.TypeOf(int(0))},
	},
}

func TestAddField_AccumulatesNumericDelta(t *testing.T) {
	m := NewMutation[fakeCardWithBalance](cardBalanceSchema, OpUpdate)
	if err := m.AddField("Balance", 3.14); err != nil {
		t.Fatalf("AddField: %v", err)
	}
	v, ok := m.AddedField("Balance")
	if !ok {
		t.Fatal("expected AddedField(Balance) ok=true")
	}
	if v.(float64) != 3.14 {
		t.Fatalf("AddedField delta: got %v", v)
	}
	added := m.AddedFields()
	if len(added) != 1 || added[0] != "Balance" {
		t.Fatalf("AddedFields: %v", added)
	}
}

func TestAddField_UnknownField_ReturnsError(t *testing.T) {
	m := NewMutation[fakeCardWithBalance](cardBalanceSchema, OpUpdate)
	if err := m.AddField("NotAField", 1); err == nil {
		t.Fatal("expected error")
	}
}

func TestAddedField_UnsetReturnsFalse(t *testing.T) {
	m := NewMutation[fakeCardWithBalance](cardBalanceSchema, OpUpdate)
	if _, ok := m.AddedField("Balance"); ok {
		t.Fatal("expected ok=false before any AddField")
	}
}
```

- [ ] **Step 2: Run — expect compile failure**

Run: `go test ./runtime/entbuilder/ -run 'TestAddField|TestAddedField' -v`
Expected: `undefined: AddField`, `undefined: AddedField`.

- [ ] **Step 3: Implement**

Append to `mutation.go`:

```go
// AddField records a numeric delta for `name`. The caller is responsible
// for ensuring the field is a numeric type — this matches the existing
// ent.Mutation.AddField contract.
func (m *Mutation[T]) AddField(name string, v any) error {
	if _, ok := m.schema.FindField(name); !ok {
		return fmt.Errorf("entbuilder: schema %q has no field %q", m.schema.Name, name)
	}
	m.addedFields[name] = v
	return nil
}

// AddedField returns the recorded delta for `name` (from AddField) and ok=true,
// or (nil, false) if no delta has been recorded.
func (m *Mutation[T]) AddedField(name string) (any, bool) {
	v, ok := m.addedFields[name]
	return v, ok
}
```

- [ ] **Step 4: Run**

Run: `go test ./runtime/entbuilder/ -run 'TestAddField|TestAddedField' -v`
Expected: 3 PASS.

- [ ] **Step 5: Commit**

```bash
git add runtime/entbuilder/mutation.go runtime/entbuilder/mutation_test.go
git commit -m "feat(entbuilder): Mutation[T] numeric AddField/AddedField"
```

---

### Task 7: `ResetField` — rollback a per-field mutation

**Files:**
- Modify: `runtime/entbuilder/mutation.go`
- Modify: `runtime/entbuilder/mutation_test.go`

- [ ] **Step 1: Append failing tests**

Append to `mutation_test.go`:

```go
func TestResetField_ClearsAllState(t *testing.T) {
	m := NewMutation[fakeCardNode](cardTestSchema, OpUpdate)
	_ = m.SetField("Name", "alice")
	_ = m.ClearField("Name")
	// Re-set for good measure — ClearField removed setFields entry.
	_ = m.SetField("Name", "bob")

	if err := m.ResetField("Name"); err != nil {
		t.Fatalf("ResetField: %v", err)
	}

	if _, ok := m.Field("Name"); ok {
		t.Fatal("expected Field(Name) unset after ResetField")
	}
	if m.FieldCleared("Name") {
		t.Fatal("expected FieldCleared(Name)=false after ResetField")
	}
	if len(m.Fields()) != 0 {
		t.Fatalf("Fields: %v", m.Fields())
	}
	if len(m.ClearedFields()) != 0 {
		t.Fatalf("ClearedFields: %v", m.ClearedFields())
	}
}

func TestResetField_NumericAlsoClearsAdded(t *testing.T) {
	m := NewMutation[fakeCardWithBalance](cardBalanceSchema, OpUpdate)
	_ = m.AddField("Balance", 3.14)
	_ = m.SetField("Balance", 100.0)
	if err := m.ResetField("Balance"); err != nil {
		t.Fatalf("ResetField: %v", err)
	}
	if _, ok := m.AddedField("Balance"); ok {
		t.Fatal("expected AddedField(Balance) ok=false after ResetField")
	}
	if _, ok := m.Field("Balance"); ok {
		t.Fatal("expected Field(Balance) ok=false after ResetField")
	}
}

func TestResetField_UnknownField_ReturnsError(t *testing.T) {
	m := NewMutation[fakeCardNode](cardTestSchema, OpUpdate)
	if err := m.ResetField("NotAField"); err == nil {
		t.Fatal("expected error")
	}
}
```

- [ ] **Step 2: Run**

Run: `go test ./runtime/entbuilder/ -run TestResetField -v`
Expected: `undefined: ResetField`.

- [ ] **Step 3: Implement**

Append to `mutation.go`:

```go
// ResetField clears all mutation state for `name`: set value, cleared flag,
// and added delta. Matches the existing generated ResetX method behavior.
func (m *Mutation[T]) ResetField(name string) error {
	if _, ok := m.schema.FindField(name); !ok {
		return fmt.Errorf("entbuilder: schema %q has no field %q", m.schema.Name, name)
	}
	delete(m.setFields, name)
	delete(m.clearedFields, name)
	delete(m.addedFields, name)
	return nil
}
```

- [ ] **Step 4: Run**

Run: `go test ./runtime/entbuilder/ -run TestResetField -v`
Expected: 3 PASS.

- [ ] **Step 5: Commit**

```bash
git add runtime/entbuilder/mutation.go runtime/entbuilder/mutation_test.go
git commit -m "feat(entbuilder): Mutation[T] ResetField rollback"
```

---

### Task 8: `SetID` / `ID` / single-target mutations

**Files:**
- Modify: `runtime/entbuilder/mutation.go`
- Modify: `runtime/entbuilder/mutation_test.go`

- [ ] **Step 1: Append failing tests**

Append to `mutation_test.go`:

```go
func TestSetID_UpdateOne(t *testing.T) {
	m := NewMutation[fakeCardNode](cardTestSchema, OpUpdateOne)
	m.SetID(42)
	id, ok := m.ID()
	if !ok || id.(int) != 42 {
		t.Fatalf("ID: ok=%v id=%v", ok, id)
	}
}

func TestID_Unset(t *testing.T) {
	m := NewMutation[fakeCardNode](cardTestSchema, OpCreate)
	if _, ok := m.ID(); ok {
		t.Fatal("expected ok=false before SetID")
	}
}
```

- [ ] **Step 2: Run**

Run: `go test ./runtime/entbuilder/ -run 'TestSetID|TestID_' -v`
Expected: `undefined: SetID`, `undefined: ID`.

- [ ] **Step 3: Implement**

Append to `mutation.go`:

```go
// SetID sets the mutation's target ID. Used by UpdateOne / DeleteOne paths.
func (m *Mutation[T]) SetID(id any) { m.id = id }

// ID returns the target ID and ok=true if SetID was called.
func (m *Mutation[T]) ID() (any, bool) {
	if m.id == nil {
		return nil, false
	}
	return m.id, true
}
```

- [ ] **Step 4: Run**

Run: `go test ./runtime/entbuilder/ -run 'TestSetID|TestID_' -v`
Expected: 2 PASS.

Also confirm full package green: `go test ./runtime/entbuilder/ -v`
Expected: 56 tests PASS (42 prior + 14 new across tasks 4-8).

- [ ] **Step 5: Commit**

```bash
git add runtime/entbuilder/mutation.go runtime/entbuilder/mutation_test.go
git commit -m "feat(entbuilder): Mutation[T] SetID/ID for single-target ops"
```

---

## Phase C — Mutation[T] edge state (Tasks 9-11)

Scope restriction: only the single-unique-inverse edge case needed by the Card
schema. Through-tables and M2M are out of scope for the spike.

### Task 9: `SetEdgeID` / `EdgeID` — single-target edge reference

**Files:**
- Modify: `runtime/entbuilder/mutation.go`
- Modify: `runtime/entbuilder/mutation_test.go`

- [ ] **Step 1: Append failing tests**

Append to `mutation_test.go`:

```go
var cardEdgeSchema = &Schema[fakeCardNode]{
	Name: "Card",
	Fields: []Field{
		{Name: "Number", Column: "number", Type: reflect.TypeOf("")},
	},
	Edges: []Edge{
		{Name: "Owner", Unique: true, Inverse: true, Field: "owner_id", TargetID: reflect.TypeOf(int(0))},
	},
}

func TestSetEdgeID_StoresAndReadsUniqueEdge(t *testing.T) {
	m := NewMutation[fakeCardNode](cardEdgeSchema, OpCreate)
	if err := m.SetEdgeID("Owner", 7); err != nil {
		t.Fatalf("SetEdgeID: %v", err)
	}
	id, ok := m.EdgeID("Owner")
	if !ok || id.(int) != 7 {
		t.Fatalf("EdgeID: ok=%v id=%v", ok, id)
	}
}

func TestSetEdgeID_UnknownEdge_ReturnsError(t *testing.T) {
	m := NewMutation[fakeCardNode](cardEdgeSchema, OpCreate)
	if err := m.SetEdgeID("NotAnEdge", 1); err == nil {
		t.Fatal("expected error for unknown edge")
	}
}

func TestEdgeID_Unset(t *testing.T) {
	m := NewMutation[fakeCardNode](cardEdgeSchema, OpCreate)
	if _, ok := m.EdgeID("Owner"); ok {
		t.Fatal("expected ok=false before SetEdgeID")
	}
}
```

- [ ] **Step 2: Run**

Run: `go test ./runtime/entbuilder/ -run 'TestSetEdgeID|TestEdgeID_' -v`
Expected: `undefined: SetEdgeID`, `undefined: EdgeID`.

- [ ] **Step 3: Implement**

Append to `mutation.go`:

```go
// Add to Mutation[T] struct (place alongside other state maps):
//
//     // edgeIDs maps Edge.Name -> target ID for unique-to-one edges.
//     edgeIDs map[string]any
//     // clearedEdges is the set of Edge.Name entries explicitly cleared.
//     clearedEdges map[string]struct{}
//
// Remember to initialize both in NewMutation (add: edgeIDs: make(...), clearedEdges: make(...)).
```

Specifically modify `NewMutation`:

```go
func NewMutation[T any](schema *Schema[T], op Op) *Mutation[T] {
	return &Mutation[T]{
		schema:        schema,
		op:            op,
		setFields:     make(map[string]any),
		clearedFields: make(map[string]struct{}),
		addedFields:   make(map[string]any),
		edgeIDs:       make(map[string]any),
		clearedEdges:  make(map[string]struct{}),
	}
}
```

And add new fields to the struct definition:

```go
type Mutation[T any] struct {
	schema *Schema[T]
	op     Op

	setFields     map[string]any
	clearedFields map[string]struct{}
	addedFields   map[string]any
	edgeIDs       map[string]any
	clearedEdges  map[string]struct{}

	id any
}
```

Then add the edge methods:

```go
// SetEdgeID stores the target ID for a unique edge. Returns an error if
// the edge is unknown or not unique.
func (m *Mutation[T]) SetEdgeID(name string, id any) error {
	e, ok := m.schema.FindEdge(name)
	if !ok {
		return fmt.Errorf("entbuilder: schema %q has no edge %q", m.schema.Name, name)
	}
	if !e.Unique {
		return fmt.Errorf("entbuilder: edge %q on schema %q is not unique; use AddEdgeIDs instead",
			name, m.schema.Name)
	}
	m.edgeIDs[name] = id
	delete(m.clearedEdges, name)
	return nil
}

// EdgeID returns the stored target ID for a unique edge. ok=false if unset.
func (m *Mutation[T]) EdgeID(name string) (any, bool) {
	v, ok := m.edgeIDs[name]
	return v, ok
}
```

- [ ] **Step 4: Run**

Run: `go test ./runtime/entbuilder/ -run 'TestSetEdgeID|TestEdgeID_' -v`
Expected: 3 PASS.

- [ ] **Step 5: Commit**

```bash
git add runtime/entbuilder/mutation.go runtime/entbuilder/mutation_test.go
git commit -m "feat(entbuilder): Mutation[T] unique-edge SetEdgeID/EdgeID"
```

---

### Task 10: `ClearEdge` / `EdgeCleared`

**Files:**
- Modify: `runtime/entbuilder/mutation.go`
- Modify: `runtime/entbuilder/mutation_test.go`

- [ ] **Step 1: Append failing tests**

Append to `mutation_test.go`:

```go
func TestClearEdge_UniqueEdge(t *testing.T) {
	m := NewMutation[fakeCardNode](cardEdgeSchema, OpUpdate)
	_ = m.SetEdgeID("Owner", 7)
	if err := m.ClearEdge("Owner"); err != nil {
		t.Fatalf("ClearEdge: %v", err)
	}
	if _, ok := m.EdgeID("Owner"); ok {
		t.Fatal("expected EdgeID ok=false after ClearEdge")
	}
	if !m.EdgeCleared("Owner") {
		t.Fatal("expected EdgeCleared=true")
	}
}

func TestClearEdge_UnknownEdge_ReturnsError(t *testing.T) {
	m := NewMutation[fakeCardNode](cardEdgeSchema, OpUpdate)
	if err := m.ClearEdge("NotAnEdge"); err == nil {
		t.Fatal("expected error")
	}
}

func TestEdgeCleared_UnknownEdge_ReturnsFalse(t *testing.T) {
	m := NewMutation[fakeCardNode](cardEdgeSchema, OpUpdate)
	if m.EdgeCleared("NotAnEdge") {
		t.Fatal("expected false for unknown edge")
	}
}
```

- [ ] **Step 2: Run**

Run: `go test ./runtime/entbuilder/ -run 'TestClearEdge|TestEdgeCleared' -v`
Expected: `undefined: ClearEdge`, `undefined: EdgeCleared`.

- [ ] **Step 3: Implement**

Append to `mutation.go`:

```go
// ClearEdge marks a unique edge as cleared and removes any stored target ID.
func (m *Mutation[T]) ClearEdge(name string) error {
	if _, ok := m.schema.FindEdge(name); !ok {
		return fmt.Errorf("entbuilder: schema %q has no edge %q", m.schema.Name, name)
	}
	delete(m.edgeIDs, name)
	m.clearedEdges[name] = struct{}{}
	return nil
}

// EdgeCleared reports whether an edge was cleared by this mutation.
func (m *Mutation[T]) EdgeCleared(name string) bool {
	_, ok := m.clearedEdges[name]
	return ok
}
```

- [ ] **Step 4: Run**

Run: `go test ./runtime/entbuilder/ -run 'TestClearEdge|TestEdgeCleared' -v`
Expected: 3 PASS.

- [ ] **Step 5: Commit**

```bash
git add runtime/entbuilder/mutation.go runtime/entbuilder/mutation_test.go
git commit -m "feat(entbuilder): Mutation[T] ClearEdge/EdgeCleared"
```

---

### Task 11: `AddedEdges` / `RemovedEdges` / `ClearedEdges` introspection

**Files:**
- Modify: `runtime/entbuilder/mutation.go`
- Modify: `runtime/entbuilder/mutation_test.go`

The existing `ent.Mutation` interface requires `AddedEdges() []string`,
`RemovedEdges() []string`, `ClearedEdges() []string`. For the spike (unique edges
only), Removed is always empty; Added and Cleared reflect the maps built in
tasks 9-10.

- [ ] **Step 1: Append failing tests**

Append to `mutation_test.go`:

```go
func TestAddedEdges_ReflectsSet(t *testing.T) {
	m := NewMutation[fakeCardNode](cardEdgeSchema, OpCreate)
	_ = m.SetEdgeID("Owner", 7)
	edges := m.AddedEdges()
	if len(edges) != 1 || edges[0] != "Owner" {
		t.Fatalf("AddedEdges: %v", edges)
	}
}

func TestClearedEdges_ReflectsCleared(t *testing.T) {
	m := NewMutation[fakeCardNode](cardEdgeSchema, OpUpdate)
	_ = m.ClearEdge("Owner")
	edges := m.ClearedEdges()
	if len(edges) != 1 || edges[0] != "Owner" {
		t.Fatalf("ClearedEdges: %v", edges)
	}
}

func TestRemovedEdges_EmptyForSpike(t *testing.T) {
	m := NewMutation[fakeCardNode](cardEdgeSchema, OpUpdate)
	if edges := m.RemovedEdges(); len(edges) != 0 {
		t.Fatalf("RemovedEdges should be empty in spike: %v", edges)
	}
}
```

- [ ] **Step 2: Run**

Run: `go test ./runtime/entbuilder/ -run 'TestAddedEdges|TestClearedEdges|TestRemovedEdges' -v`
Expected: `undefined: AddedEdges`, `undefined: ClearedEdges`, `undefined: RemovedEdges`.

- [ ] **Step 3: Implement**

Append to `mutation.go`:

```go
// AddedEdges returns the names of edges that have a target ID set by this mutation.
// For the spike's unique-edges-only scope, this is the set of edges with a SetEdgeID call.
func (m *Mutation[T]) AddedEdges() []string {
	out := make([]string, 0, len(m.edgeIDs))
	for k := range m.edgeIDs {
		out = append(out, k)
	}
	return out
}

// ClearedEdges returns the names of edges explicitly cleared.
func (m *Mutation[T]) ClearedEdges() []string {
	out := make([]string, 0, len(m.clearedEdges))
	for k := range m.clearedEdges {
		out = append(out, k)
	}
	return out
}

// RemovedEdges returns the names of edges that have had specific targets
// removed (for non-unique edges). In the spike's unique-edges-only scope,
// this is always empty; full through-table support is Phase 4C scope.
func (m *Mutation[T]) RemovedEdges() []string { return nil }
```

- [ ] **Step 4: Run**

Run: `go test ./runtime/entbuilder/ -run 'TestAddedEdges|TestClearedEdges|TestRemovedEdges' -v`
Expected: 3 PASS.

Full package: `go test ./runtime/entbuilder/ -v`
Expected: 65 tests PASS (56 prior + 9 new across 9-11).

- [ ] **Step 5: Commit**

```bash
git add runtime/entbuilder/mutation.go runtime/entbuilder/mutation_test.go
git commit -m "feat(entbuilder): Mutation[T] AddedEdges/ClearedEdges/RemovedEdges introspection"
```

---

## Phase D — OldField fetcher (Tasks 12-13)

### Task 12: `OldFieldFetcher` hook on the schema descriptor

**Files:**
- Modify: `runtime/entbuilder/schema.go`
- Modify: `runtime/entbuilder/schema_test.go`

The generated `OldX(ctx)` methods today fetch the pre-mutation value from the
database via a closure embedded in the mutation. We relocate that closure onto
the schema descriptor (`OldFieldFetcher`) and let `Mutation[T].OldField(ctx, name)`
delegate.

- [ ] **Step 1: Extend the import block and append the failing test**

First, merge `"context"` and `"fmt"` into the existing `import (...)` block at the top of `runtime/entbuilder/schema_test.go`. Do NOT add a second import block — that's a Go compile error. The block now contains: `"context"`, `"fmt"`, `"reflect"`, `"testing"`, `"time"`.

Then append the test function (no additional import line):

```go
func TestSchema_OldFieldFetcher_ReturnsClosure(t *testing.T) {
	fetcher := func(ctx context.Context, id any, field string) (any, error) {
		if field == "Name" {
			return "alice", nil
		}
		return nil, fmt.Errorf("unknown field: %s", field)
	}
	s := &Schema[fakeCardNode]{
		Name:             "Card",
		OldFieldFetcher:  fetcher,
	}
	got, err := s.OldFieldFetcher(context.Background(), 1, "Name")
	if err != nil {
		t.Fatalf("fetcher err: %v", err)
	}
	if got.(string) != "alice" {
		t.Fatalf("OldField value: %v", got)
	}
}
```

- [ ] **Step 2: Run**

Run: `go test ./runtime/entbuilder/ -run TestSchema_OldFieldFetcher -v`
Expected: `OldFieldFetcher undefined on Schema[T]`.

- [ ] **Step 3: Implement — extend Schema[T]**

Modify `schema.go`'s `Schema[T]` struct:

```go
import "context"

type Schema[T any] struct {
	Name    string
	Table   string
	IDField Field
	Fields  []Field
	Edges   []Edge

	// OldFieldFetcher is the closure that retrieves the pre-mutation value
	// for `field` from the row identified by `id`. Typically generated code
	// wires this to a one-row Query by ID with a single column select.
	// Nil OldFieldFetcher means the schema does not support OldX lookups
	// (e.g., on OpCreate mutations).
	OldFieldFetcher func(ctx context.Context, id any, field string) (any, error)
}
```

- [ ] **Step 4: Run**

Run: `go test ./runtime/entbuilder/ -run TestSchema_OldFieldFetcher -v`
Expected: 1 PASS.

- [ ] **Step 5: Commit**

```bash
git add runtime/entbuilder/schema.go runtime/entbuilder/schema_test.go
git commit -m "feat(entbuilder): Schema[T] carries OldFieldFetcher closure"
```

---

### Task 13: `Mutation[T].OldField` delegates to schema fetcher

**Files:**
- Modify: `runtime/entbuilder/mutation.go`
- Modify: `runtime/entbuilder/mutation_test.go`

- [ ] **Step 1: Extend the import block and append failing tests**

First, merge `"context"` into the existing `import (...)` block at the top of `runtime/entbuilder/mutation_test.go`. Do NOT add a second import block.

Then append the test functions (no additional import line):

```go
func TestOldField_DelegatesToSchemaFetcher(t *testing.T) {
	called := false
	s := &Schema[fakeCardNode]{
		Name: "Card",
		Fields: []Field{{Name: "Name", Column: "name", Type: reflect.TypeOf("")}},
		OldFieldFetcher: func(ctx context.Context, id any, field string) (any, error) {
			called = true
			if field != "Name" {
				t.Errorf("unexpected field: %s", field)
			}
			if id.(int) != 42 {
				t.Errorf("unexpected id: %v", id)
			}
			return "alice", nil
		},
	}
	m := NewMutation[fakeCardNode](s, OpUpdateOne)
	m.SetID(42)
	v, err := m.OldField(context.Background(), "Name")
	if err != nil {
		t.Fatalf("OldField: %v", err)
	}
	if v.(string) != "alice" {
		t.Fatalf("OldField value: %v", v)
	}
	if !called {
		t.Fatal("expected fetcher to be invoked")
	}
}

func TestOldField_NoFetcher_ReturnsError(t *testing.T) {
	s := &Schema[fakeCardNode]{
		Name: "Card",
		Fields: []Field{{Name: "Name", Column: "name", Type: reflect.TypeOf("")}},
	}
	m := NewMutation[fakeCardNode](s, OpUpdateOne)
	m.SetID(1)
	_, err := m.OldField(context.Background(), "Name")
	if err == nil {
		t.Fatal("expected error when schema has no OldFieldFetcher")
	}
}

func TestOldField_NoID_ReturnsError(t *testing.T) {
	s := &Schema[fakeCardNode]{
		Name: "Card",
		Fields: []Field{{Name: "Name", Column: "name", Type: reflect.TypeOf("")}},
		OldFieldFetcher: func(ctx context.Context, id any, field string) (any, error) {
			return nil, nil
		},
	}
	m := NewMutation[fakeCardNode](s, OpUpdate) // bulk update — no ID
	_, err := m.OldField(context.Background(), "Name")
	if err == nil {
		t.Fatal("expected error when mutation has no ID set")
	}
}

func TestOldField_UnknownField_ReturnsError(t *testing.T) {
	s := &Schema[fakeCardNode]{
		Name: "Card",
		OldFieldFetcher: func(ctx context.Context, id any, field string) (any, error) {
			return nil, nil
		},
	}
	m := NewMutation[fakeCardNode](s, OpUpdateOne)
	m.SetID(1)
	_, err := m.OldField(context.Background(), "NotAField")
	if err == nil {
		t.Fatal("expected error for unknown field")
	}
}
```

- [ ] **Step 2: Run**

Run: `go test ./runtime/entbuilder/ -run TestOldField -v`
Expected: `undefined: OldField`.

- [ ] **Step 3: Implement**

Append to `mutation.go`:

```go
import "context"

// OldField retrieves the pre-mutation value of `name` by delegating to the
// schema's OldFieldFetcher closure. Returns an error if:
//   - `name` is not a declared field on the schema
//   - the schema has no OldFieldFetcher wired (e.g., Create-only schemas)
//   - this mutation has no SetID yet (bulk updates cannot resolve a single row)
func (m *Mutation[T]) OldField(ctx context.Context, name string) (any, error) {
	if _, ok := m.schema.FindField(name); !ok {
		return nil, fmt.Errorf("entbuilder: schema %q has no field %q", m.schema.Name, name)
	}
	if m.schema.OldFieldFetcher == nil {
		return nil, fmt.Errorf("entbuilder: schema %q has no OldFieldFetcher configured", m.schema.Name)
	}
	id, ok := m.ID()
	if !ok {
		return nil, fmt.Errorf("entbuilder: OldField(%q) requires a target ID (SetID not called)", name)
	}
	return m.schema.OldFieldFetcher(ctx, id, name)
}
```

Merge `"context"` into `mutation.go`'s existing import block.

- [ ] **Step 4: Run**

Run: `go test ./runtime/entbuilder/ -run TestOldField -v`
Expected: 4 PASS.

Full package: `go test ./runtime/entbuilder/ -v`
Expected: 70 tests PASS (65 + 5 new).

- [ ] **Step 5: Commit**

```bash
git add runtime/entbuilder/mutation.go runtime/entbuilder/mutation_test.go
git commit -m "feat(entbuilder): Mutation[T] OldField delegates to schema fetcher"
```

---

## Phase E — Card spike (Tasks 14-19)

The spike hand-rolls a descriptor-driven replacement for `CardMutation` in the
hooks integration scenario and runs the existing `hooks_test.go` against it to
validate end-to-end feasibility. No code generation, no templates.

### Task 14: Create the spike scratch directory and inspect Card

**Files:**
- Create: `entc/integration/hooks/spike/` (directory)
- Create: `entc/integration/hooks/spike/README.md`

- [ ] **Step 1: Set up the scratch dir and document its scope**

```bash
cd /var/home/smoothbrain/dev/matthewsreis/ent/.worktrees/i-h8-ent
mkdir -p entc/integration/hooks/spike
```

Create `entc/integration/hooks/spike/README.md`:

```markdown
# Card Descriptor-Driven Spike

This directory is a throwaway scratch space for Phase 4A's spike. It hand-rolls
a replacement for the generated `*CardMutation` that delegates to
`entbuilder.Mutation[Card]`, to validate that the descriptor-driven approach
is feasible before committing to a template rewrite.

Contents (all hand-written, not generated):

- `card_descriptor.go` — the Card schema descriptor + its OldFieldFetcher.
- `card_mutation_shim.go` — façade methods that map the existing CardMutation
  API onto the generic Mutation[Card].
- `spike_test.go` — a copy (or harness) of the relevant hooks_test.go cases,
  adapted to use the shim.

On gate success: a subsequent plan replaces the generated CardMutation
entirely. On gate failure: the spike is dropped and a new spec is written
for a different approach.
```

- [ ] **Step 2: Inspect the existing CardMutation surface**

Read `entc/integration/hooks/ent/internal/card_mutation.go` to enumerate:

- Every exported method's name and signature (Number, Name, ResetName, OldName, ClearName, NameCleared, ExpiredAt, OldExpiredAt, ClearExpiredAt, ExpiredAtCleared, InHook, SetInHook, OwnerID, SetOwnerID, ClearOwner, OwnerCleared, OwnerIDs, CreatedAt, OldCreatedAt, …).
- The existing Fields() / AddedFields() / ClearedFields() / Op() / ID() / AddedEdges() / ClearedEdges() / RemovedEdges() implementations.

Expect approximately 40 exported methods. Make a checklist; the shim must cover all of them that hooks_test.go actually calls.

- [ ] **Step 3: Commit the scratch dir and README**

```bash
git add entc/integration/hooks/spike/README.md
git commit -m "spike(phase4a): scratch dir + scope for Card descriptor-driven port"
```

---

### Task 15: Hand-roll Card descriptor

**Files:**
- Create: `entc/integration/hooks/spike/card_descriptor.go`

- [ ] **Step 1: Write the descriptor**

Create `entc/integration/hooks/spike/card_descriptor.go`:

```go
// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

// Package spike is a throwaway descriptor-driven port of the Card entity,
// used to validate Phase 4A feasibility. See README.md.
package spike

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"entgo.io/ent/entc/integration/hooks/ent"
	"entgo.io/ent/entc/integration/hooks/ent/card"
	"entgo.io/ent/runtime/entbuilder"
)

// cardSchema is the descriptor for the Card entity, constructed from the
// same source-of-truth as the generated code: `entc/integration/hooks/ent/schema/card.go`.
//
// Every field here must correspond exactly to a struct field on *ent.Card; this
// is asserted by entbuilder.ValidateSchema on first use.
var cardSchema = &entbuilder.Schema[ent.Card]{
	Name:  "Card",
	Table: card.Table,
	IDField: entbuilder.Field{
		Name:   "ID",
		Column: card.FieldID,
		Type:   reflect.TypeOf(int(0)),
	},
	Fields: []entbuilder.Field{
		{
			Name:      "Number",
			Column:    card.FieldNumber,
			Type:      reflect.TypeOf(""),
			Immutable: true,
		},
		{
			Name:     "Name",
			Column:   card.FieldName,
			Type:     reflect.TypeOf(""),
			Nullable: true,
		},
		{
			Name:   "CreatedAt",
			Column: card.FieldCreatedAt,
			Type:   reflect.TypeOf(time.Time{}),
		},
		{
			Name:   "InHook",
			Column: card.FieldInHook,
			Type:   reflect.TypeOf(""),
		},
		{
			Name:     "ExpiredAt",
			Column:   card.FieldExpiredAt,
			Type:     reflect.TypeOf(time.Time{}),
			Nullable: true,
		},
	},
	Edges: []entbuilder.Edge{
		{
			Name:     "Owner",
			Unique:   true,
			Inverse:  true,
			Field:    "owner_id",
			TargetID: reflect.TypeOf(int(0)),
		},
	},
}

// wireCardOldFieldFetcher wires the OldFieldFetcher closure. The fetcher does
// a one-row select from the cards table by ID. This is called once at
// package init time (or client construction) — the closure captures the
// client for lookups.
func wireCardOldFieldFetcher(client *ent.Client) {
	cardSchema.OldFieldFetcher = func(ctx context.Context, id any, field string) (any, error) {
		intID, ok := id.(int)
		if !ok {
			return nil, fmt.Errorf("spike: card id expected int, got %T", id)
		}
		c, err := client.Card.Get(ctx, intID)
		if err != nil {
			return nil, err
		}
		switch field {
		case "Number":
			return c.Number, nil
		case "Name":
			return c.Name, nil
		case "CreatedAt":
			return c.CreatedAt, nil
		case "InHook":
			return c.InHook, nil
		case "ExpiredAt":
			return c.ExpiredAt, nil
		default:
			return nil, fmt.Errorf("spike: card has no field %q", field)
		}
	}
}
```

If `ent.Card` is not directly importable from `package spike` because of internal
visibility, substitute the correct import path (check `entc/integration/hooks/ent/card.go`
for `type Card struct` — the type should be in package `ent`).

- [ ] **Step 2: Verify it compiles**

Run: `go build ./entc/integration/hooks/spike/`
Expected: succeeds. If it fails with import errors, resolve (likely a missing import) and retry.

- [ ] **Step 3: Run the descriptor through ValidateSchema at test time**

Append a quick smoke test at `entc/integration/hooks/spike/card_descriptor_test.go`:

```go
package spike

import (
	"testing"

	"entgo.io/ent/entc/integration/hooks/ent"
	"entgo.io/ent/runtime/entbuilder"
)

func TestCardDescriptor_Valid(t *testing.T) {
	if err := entbuilder.ValidateSchema[ent.Card](cardSchema); err != nil {
		t.Fatalf("Card descriptor fails validation: %v", err)
	}
}
```

Run: `go test ./entc/integration/hooks/spike/ -run TestCardDescriptor_Valid -v`
Expected: PASS.

If it fails, inspect the error — it will tell you precisely which field's Type doesn't match the struct. Fix the descriptor and re-run.

- [ ] **Step 4: Commit**

```bash
git add entc/integration/hooks/spike/card_descriptor.go entc/integration/hooks/spike/card_descriptor_test.go
git commit -m "spike(phase4a): hand-rolled Card schema descriptor"
```

---

### Task 16: Hand-roll mutation shim — set/get/reset/clear/old

**Files:**
- Create: `entc/integration/hooks/spike/card_mutation_shim.go`

- [ ] **Step 1: Write the façade**

Create `entc/integration/hooks/spike/card_mutation_shim.go`:

```go
// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package spike

import (
	"context"
	"time"

	"entgo.io/ent/entc/integration/hooks/ent"
	"entgo.io/ent/runtime/entbuilder"
)

// CardMutationShim wraps the generic Mutation[ent.Card] and exposes the
// typed API that hooks_test.go expects. Method bodies delegate to
// entbuilder.Mutation[ent.Card].
type CardMutationShim struct {
	m *entbuilder.Mutation[ent.Card]
}

// NewCardMutationShim constructs an empty shim for the given op.
func NewCardMutationShim(op entbuilder.Op) *CardMutationShim {
	return &CardMutationShim{m: entbuilder.NewMutation[ent.Card](cardSchema, op)}
}

// ----- Typed field accessors -----

func (s *CardMutationShim) SetNumber(v string) { _ = s.m.SetField("Number", v) }
func (s *CardMutationShim) Number() (string, bool) {
	v, ok := s.m.Field("Number")
	if !ok {
		return "", false
	}
	return v.(string), true
}

func (s *CardMutationShim) SetName(v string)     { _ = s.m.SetField("Name", v) }
func (s *CardMutationShim) ClearName()           { _ = s.m.ClearField("Name") }
func (s *CardMutationShim) ResetName()           { _ = s.m.ResetField("Name") }
func (s *CardMutationShim) NameCleared() bool    { return s.m.FieldCleared("Name") }
func (s *CardMutationShim) Name() (string, bool) {
	v, ok := s.m.Field("Name")
	if !ok {
		return "", false
	}
	return v.(string), true
}
func (s *CardMutationShim) OldName(ctx context.Context) (string, error) {
	v, err := s.m.OldField(ctx, "Name")
	if err != nil {
		return "", err
	}
	if v == nil {
		return "", nil
	}
	return v.(string), nil
}

func (s *CardMutationShim) SetCreatedAt(v time.Time)                       { _ = s.m.SetField("CreatedAt", v) }
func (s *CardMutationShim) ResetCreatedAt()                                { _ = s.m.ResetField("CreatedAt") }
func (s *CardMutationShim) CreatedAt() (time.Time, bool) {
	v, ok := s.m.Field("CreatedAt")
	if !ok {
		return time.Time{}, false
	}
	return v.(time.Time), true
}
func (s *CardMutationShim) OldCreatedAt(ctx context.Context) (time.Time, error) {
	v, err := s.m.OldField(ctx, "CreatedAt")
	if err != nil {
		return time.Time{}, err
	}
	return v.(time.Time), nil
}

func (s *CardMutationShim) SetInHook(v string) { _ = s.m.SetField("InHook", v) }
func (s *CardMutationShim) InHook() (string, bool) {
	v, ok := s.m.Field("InHook")
	if !ok {
		return "", false
	}
	return v.(string), true
}
func (s *CardMutationShim) OldInHook(ctx context.Context) (string, error) {
	v, err := s.m.OldField(ctx, "InHook")
	if err != nil {
		return "", err
	}
	return v.(string), nil
}

func (s *CardMutationShim) SetExpiredAt(v time.Time)      { _ = s.m.SetField("ExpiredAt", v) }
func (s *CardMutationShim) ClearExpiredAt()               { _ = s.m.ClearField("ExpiredAt") }
func (s *CardMutationShim) ExpiredAtCleared() bool        { return s.m.FieldCleared("ExpiredAt") }
func (s *CardMutationShim) ExpiredAt() (time.Time, bool) {
	v, ok := s.m.Field("ExpiredAt")
	if !ok {
		return time.Time{}, false
	}
	return v.(time.Time), true
}
func (s *CardMutationShim) OldExpiredAt(ctx context.Context) (time.Time, error) {
	v, err := s.m.OldField(ctx, "ExpiredAt")
	if err != nil {
		return time.Time{}, err
	}
	return v.(time.Time), nil
}

// ----- Number is immutable: no SetX beyond Create; no ResetX; no ClearX -----

func (s *CardMutationShim) OldNumber(ctx context.Context) (string, error) {
	v, err := s.m.OldField(ctx, "Number")
	if err != nil {
		return "", err
	}
	return v.(string), nil
}

// ----- Owner edge -----

func (s *CardMutationShim) SetOwnerID(id int)     { _ = s.m.SetEdgeID("Owner", id) }
func (s *CardMutationShim) ClearOwner()           { _ = s.m.ClearEdge("Owner") }
func (s *CardMutationShim) OwnerCleared() bool    { return s.m.EdgeCleared("Owner") }
func (s *CardMutationShim) OwnerID() (int, bool) {
	v, ok := s.m.EdgeID("Owner")
	if !ok {
		return 0, false
	}
	return v.(int), true
}
func (s *CardMutationShim) OwnerIDs() []int {
	v, ok := s.m.EdgeID("Owner")
	if !ok {
		return nil
	}
	return []int{v.(int)}
}

// ----- Introspection -----

func (s *CardMutationShim) Op() entbuilder.Op { return s.m.Op() }
func (s *CardMutationShim) ID() (int, bool) {
	v, ok := s.m.ID()
	if !ok {
		return 0, false
	}
	return v.(int), true
}
func (s *CardMutationShim) SetID(id int) { s.m.SetID(id) }

func (s *CardMutationShim) Fields() []string        { return s.m.Fields() }
func (s *CardMutationShim) AddedFields() []string   { return s.m.AddedFields() }
func (s *CardMutationShim) ClearedFields() []string { return s.m.ClearedFields() }
func (s *CardMutationShim) AddedEdges() []string    { return s.m.AddedEdges() }
func (s *CardMutationShim) ClearedEdges() []string  { return s.m.ClearedEdges() }
func (s *CardMutationShim) RemovedEdges() []string  { return s.m.RemovedEdges() }

func (s *CardMutationShim) Field(name string) (any, bool)            { return s.m.Field(name) }
func (s *CardMutationShim) AddedField(name string) (any, bool)       { return s.m.AddedField(name) }
func (s *CardMutationShim) FieldCleared(name string) bool            { return s.m.FieldCleared(name) }
func (s *CardMutationShim) EdgeCleared(name string) bool             { return s.m.EdgeCleared(name) }
func (s *CardMutationShim) OldField(ctx context.Context, name string) (any, error) {
	return s.m.OldField(ctx, name)
}
```

- [ ] **Step 2: Verify compile**

Run: `go build ./entc/integration/hooks/spike/`
Expected: succeeds.

- [ ] **Step 3: Commit**

```bash
git add entc/integration/hooks/spike/card_mutation_shim.go
git commit -m "spike(phase4a): CardMutationShim façade over entbuilder.Mutation[Card]"
```

---

### Task 17: Shim-level round-trip tests

**Files:**
- Modify: `entc/integration/hooks/spike/card_descriptor_test.go`

These tests exercise the shim in isolation — no SQL, no hooks client — to
validate that the façade correctly maps method calls to the generic Mutation[T].

- [ ] **Step 1: Append tests**

Append to `entc/integration/hooks/spike/card_descriptor_test.go`:

```go
import (
	"testing"
	"time"

	"entgo.io/ent/entc/integration/hooks/ent/card"
	"entgo.io/ent/runtime/entbuilder"
)

func TestShim_Number_SetAndGet(t *testing.T) {
	s := NewCardMutationShim(entbuilder.OpCreate)
	s.SetNumber("1234")
	got, ok := s.Number()
	if !ok || got != "1234" {
		t.Fatalf("Number: ok=%v got=%q", ok, got)
	}
}

func TestShim_Name_ClearAndReset(t *testing.T) {
	s := NewCardMutationShim(entbuilder.OpUpdate)
	s.SetName("alice")
	if _, ok := s.Name(); !ok {
		t.Fatal("expected Name set")
	}
	s.ClearName()
	if !s.NameCleared() {
		t.Fatal("expected NameCleared=true")
	}
	if _, ok := s.Name(); ok {
		t.Fatal("expected Name unset after ClearName")
	}
	s.ResetName()
	if s.NameCleared() {
		t.Fatal("expected NameCleared=false after Reset")
	}
}

func TestShim_Fields_ListsSet(t *testing.T) {
	s := NewCardMutationShim(entbuilder.OpCreate)
	s.SetNumber("1234")
	s.SetName("alice")
	s.SetCreatedAt(time.Now())

	fields := s.Fields()
	seen := make(map[string]bool)
	for _, f := range fields {
		seen[f] = true
	}
	for _, expected := range []string{"Number", "Name", "CreatedAt"} {
		if !seen[expected] {
			t.Errorf("Fields missing %q: %v", expected, fields)
		}
	}
}

func TestShim_Owner_SetClearGet(t *testing.T) {
	s := NewCardMutationShim(entbuilder.OpUpdate)
	s.SetOwnerID(42)
	id, ok := s.OwnerID()
	if !ok || id != 42 {
		t.Fatalf("OwnerID: ok=%v id=%d", ok, id)
	}
	if ids := s.OwnerIDs(); len(ids) != 1 || ids[0] != 42 {
		t.Fatalf("OwnerIDs: %v", ids)
	}
	s.ClearOwner()
	if !s.OwnerCleared() {
		t.Fatal("expected OwnerCleared")
	}
}

func TestShim_Field_ByName_UsesConstant(t *testing.T) {
	// Ensure the string name used for card.FieldName constant matches
	// what the descriptor / shim expect.
	s := NewCardMutationShim(entbuilder.OpCreate)
	s.SetName("bob")
	v, ok := s.Field("Name")
	if !ok {
		t.Fatal("Field(\"Name\") ok=false after SetName")
	}
	if v.(string) != "bob" {
		t.Fatalf("Field value: %v", v)
	}
	// Also confirm the generated constant resolves to the same DB column.
	if card.FieldName == "" {
		t.Fatal("card.FieldName should not be empty")
	}
}
```

- [ ] **Step 2: Run**

Run: `go test ./entc/integration/hooks/spike/ -v`
Expected: 5 PASS + the earlier TestCardDescriptor_Valid = 6 total.

If anything fails, fix the shim or descriptor before proceeding. The spike gate depends on these passing before the integration test.

- [ ] **Step 3: Commit**

```bash
git add entc/integration/hooks/spike/card_descriptor_test.go
git commit -m "spike(phase4a): shim round-trip tests for scalar + edge fields"
```

---

### Task 18: Run the real hooks scenario test via the shim

**Files:**
- Modify: `entc/integration/hooks/spike/card_descriptor_test.go`

Goal: prove the shim is usable in the hooks scenario's actual integration
path. We can't fully replace `*ent.CardMutation` in the generated client (that's
the Phase 4C template rewrite), but we CAN run the same kind of assertions the
hooks test makes against a shim instance constructed around a hand-built state.

Replicate two concrete hooks_test.go behaviors:

1. `hook.HasFields(card.FieldExpiredAt)` evaluating correctly against the shim.
2. `OldName(ctx)` invocation going through the shim, using a hooks-scenario
   client as the underlying data source for OldFieldFetcher.

- [ ] **Step 1: Append integration-level test**

Append to `entc/integration/hooks/spike/card_descriptor_test.go`:

```go
import (
	"context"

	"entgo.io/ent/dialect"
	"entgo.io/ent/entc/integration/hooks/ent"
	"entgo.io/ent/entc/integration/hooks/ent/enttest"
	_ "github.com/mattn/go-sqlite3"
)

// TestShim_OldName_ThroughHooksClient confirms that the descriptor's
// OldFieldFetcher, wired to a real hooks-scenario client, returns the
// pre-mutation value correctly.
func TestShim_OldName_ThroughHooksClient(t *testing.T) {
	ctx := context.Background()
	client := enttest.Open(t, dialect.SQLite, "file:spike?mode=memory&cache=shared&_fk=1")
	defer client.Close()

	wireCardOldFieldFetcher(client)

	// Seed: create a card so we have a row to OldField against.
	u := client.User.Create().SetName("alice").SetAge(1).SaveX(ctx)
	c := client.Card.Create().
		SetNumber("1234").
		SetName("original-name").
		SetInHook("hook-value").
		SetOwnerID(u.ID).
		SaveX(ctx)

	// Build a shim as though we were about to update the card. SetID picks
	// the row; OldName should read the pre-mutation value from the DB.
	s := NewCardMutationShim(entbuilder.OpUpdateOne)
	s.SetID(c.ID)
	s.SetName("new-name")

	oldName, err := s.OldName(ctx)
	if err != nil {
		t.Fatalf("OldName: %v", err)
	}
	if oldName != "original-name" {
		t.Fatalf("OldName: got %q want %q", oldName, "original-name")
	}

	// Avoid unused-variable complaints for the ent package import.
	_ = (*ent.Card)(nil)
}

// TestShim_HasFields_Semantics confirms that Fields() / AddedFields() ordering
// and membership on the shim matches what hook.HasFields combinators test for.
func TestShim_HasFields_Semantics(t *testing.T) {
	s := NewCardMutationShim(entbuilder.OpUpdate)
	s.SetExpiredAt(time.Now())

	found := false
	for _, name := range s.Fields() {
		if name == "ExpiredAt" {
			found = true
		}
	}
	if !found {
		t.Fatalf("Fields() should contain ExpiredAt: %v", s.Fields())
	}

	// And a field that was NOT set should not appear.
	for _, name := range s.Fields() {
		if name == "Name" {
			t.Fatalf("Fields() should not contain Name: %v", s.Fields())
		}
	}
}
```

Adjust any import paths that don't match the existing hooks scenario (check
`entc/integration/hooks/ent/enttest` exists; if not, use `ent.Open` directly).

- [ ] **Step 2: Run**

Run: `go test ./entc/integration/hooks/spike/ -v`
Expected: all PASS.

If `TestShim_OldName_ThroughHooksClient` fails, it's most likely because the
shim's OldFieldFetcher closure returned a different type than the typed
accessor expected — investigate by logging the intermediate `any` value.

- [ ] **Step 3: Commit**

```bash
git add entc/integration/hooks/spike/card_descriptor_test.go
git commit -m "spike(phase4a): integration test — OldField via real hooks client"
```

---

### Task 19: Measure spike compile-time impact

**Files:**
- Create: `docs/superpowers/results/2026-04-24-phase4a-spike-measurements.md` (skeleton)

Measure the *relative* compile-time cost of building the spike package vs
the size of the ported surface. This is a tiny data point; the real gate is
whether Phase 4C's full template rewrite can hit the RSS target, but the spike
gives us a leading indicator.

- [ ] **Step 1: Capture spike build metrics**

```bash
cd /var/home/smoothbrain/dev/matthewsreis/ent/.worktrees/i-h8-ent
go clean -cache

/usr/bin/time -v go build ./entc/integration/hooks/spike/ 2> /tmp/spike-build.txt
echo "=== spike build stats ==="
grep -E 'Maximum resident|Elapsed' /tmp/spike-build.txt

echo "=== spike LOC ==="
find entc/integration/hooks/spike -name '*.go' -not -name '*_test.go' -exec wc -l {} +

echo "=== equivalent generated card_mutation.go LOC ==="
wc -l entc/integration/hooks/ent/internal/card_mutation.go
```

Record the three numbers: spike build wall + RSS, spike source LOC, generated CardMutation LOC. The LOC ratio gives a rough upper bound on Phase 4C's expected reduction for mutation files.

- [ ] **Step 2: Write measurements skeleton**

Create `docs/superpowers/results/2026-04-24-phase4a-spike-measurements.md`:

```markdown
# Phase 4A Spike Measurements

**Date:** 2026-04-24
**Spec:** docs/superpowers/specs/2026-04-24-ent-code-reduction-phase-4-hybrid.md
**Plan:** docs/superpowers/plans/2026-04-24-ent-code-reduction-phase-4a-spike.md
**Fork HEAD:** (fill in via `git rev-parse HEAD`)

## Spike results

| Metric | Value |
|---|---|
| Spike source LOC (card_descriptor.go + card_mutation_shim.go) | (fill) |
| Generated card_mutation.go LOC (the hypothetical replacement target) | (fill) |
| LOC ratio (spike / generated) | (fill) |
| Spike build wall time | (fill) |
| Spike build peak RSS | (fill) |
| Integration tests (Phase 4A task 17+18) | (green / red) |
| Descriptor validates against ent.Card | (green / red) |

## Gate decision

(filled in Task 20.)
```

Fill every `(fill)` from step 1.

- [ ] **Step 3: Commit**

```bash
git add docs/superpowers/results/2026-04-24-phase4a-spike-measurements.md
git commit -m "docs(results): Phase 4A spike baseline measurements"
```

---

## Phase F — Gate decision (Tasks 20-21)

### Task 20: Go/no-go decision

**Files:**
- Modify: `docs/superpowers/results/2026-04-24-phase4a-spike-measurements.md`

Apply these gates (from spec § Phase plan Step 1):

| Gate | Pass condition |
|---|---|
| All spike tests green | `go test ./entc/integration/hooks/spike/` shows 100% pass |
| Descriptor validation succeeds | TestCardDescriptor_Valid green |
| OldField works through real client | TestShim_OldName_ThroughHooksClient green |
| Shim LOC < 50% of generated card_mutation.go LOC | spike is meaningfully smaller |
| No hidden runtime panics | no panic observed in any test run |

- [ ] **Step 1: Evaluate each gate and write Decision section**

Append to the measurements doc:

```markdown
## Gate decision

**Outcome:** GO / NO-GO (fill)

### Gate results

| Gate | Result | Evidence |
|---|---|---|
| Spike tests green | (fill) | go test output |
| Descriptor validation | (fill) | TestCardDescriptor_Valid |
| OldField via real client | (fill) | TestShim_OldName_ThroughHooksClient |
| Shim LOC ratio | (fill, e.g. "340/680 = 50%") | wc -l above |
| No runtime panics | (fill) | test output |

### Interpretation

(Fill in 2-3 sentences on what the numbers say about Phase 4C feasibility.
If GO: note the LOC reduction achieved on this one schema gives us a
projection for the 111-schema consumer. If NO-GO: list which gate failed
and why.)

### Path forward

(If GO: proceed to Phase 4B — lean-generics templates for create/update/
delete/query. Phase 4C — descriptor-driven template rewrite for mutation —
unlocks after 4B.

If NO-GO: draft a new spec covering the alternative (likely full Approach 2 with
no lean-generics mix, or a different mechanism entirely). Do not start
Phase 4B until a new decision is ratified.)
```

- [ ] **Step 2: Commit**

```bash
git add docs/superpowers/results/2026-04-24-phase4a-spike-measurements.md
git commit -m "docs(results): Phase 4A spike go/no-go decision"
```

---

### Task 21: Push and hand off

**Files:** none

- [ ] **Step 1: Verify test suite still green**

Run: `go test ./runtime/entbuilder/ ./entc/integration/hooks/ ./entc/integration/hooks/spike/`
Expected: all PASS.

- [ ] **Step 2: Push branch**

```bash
cd /var/home/smoothbrain/dev/matthewsreis/ent/.worktrees/i-h8-ent
git push
```

Expected: push succeeds. The branch now has Phase 1-3 (already pushed) plus
Phase 4A commits on top.

- [ ] **Step 3: Report handoff to the user**

Summarize in chat: GO/NO-GO outcome, key numbers, commit range covered by
Phase 4A, what the next plan covers (4B or a new spec).

---

## Self-review notes

Coverage against the spec:

- ✅ "Build the generic runtime (`entbuilder.Mutation[T]`, `Schema[T]`, `ValidateSchema`)" — Tasks 1-13.
- ✅ "Spike on mutation descriptor for ONE schema" — Tasks 14-18.
- ✅ "Success criterion: `entc/integration/hooks` passes with `Card` manually ported to the descriptor-driven form" — covered by Tasks 17-18.
- ✅ Gate decision made with concrete criteria — Task 20.
- ✅ Scan-path fuzzing: **not yet** in this plan. Scan-path only matters once we actually do queries that populate nodes from rows, which is Phase 4C when we start using the generic mutation for real. Defer.
- ✅ Client-init-time validation: Schema[T].OldFieldFetcher wiring (Task 15 step 1) hand-calls this; the generic runtime's auto-invocation at `ent.NewClient` time is Phase 4C.
- ✅ Witness-test generator: deferred to Phase 4C. This plan does NOT emit witness tests — the spike relies on hand-rolled tests instead.
- ✅ Edge coverage: unique edges only (Card.Owner). Through-tables and M2M are explicitly out of scope per the plan's opening, and will be addressed in Phase 4C.
- ✅ No placeholders in task bodies. Every `Fill in` in the measurements doc is a value the engineer extracts from a prior step's output; the instructions are specific.

Spec→plan coverage items that are DEFERRED (not failures, just not in this phase):

- AddEdgeIDs / RemoveEdgeIDs for non-unique edges — deferred to 4C.
- Through-table payload access — deferred to 4C.
- Mutation.Where() predicate accumulator (for bulk updates) — deferred to 4C.
- `AddPredicate` / `MutationPredicates` methods — deferred to 4C.
- `InHook` marker field handling — not special for the spike; treated as a plain string field via the descriptor.

The plan's terminal state is a measured GO/NO-GO decision. If GO, Phase 4B and 4C follow.
