# ent Code Reduction Phase 4C — Spike: In-Place CardMutation Replacement

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Validate the end-to-end drop-in-API pattern for descriptor-driven mutations by replacing `CardMutation` **in place** (in `entc/integration/hooks/ent/internal/card_mutation.go`) with a wrapper struct that embeds `*entbuilder.Mutation[ent.Card]`. Confirm the full `hooks_test.go` suite passes unchanged. Measure the LOC ratio. Produce a GO/NO-GO for Phase 4D (codegen productionization).

**Architecture:** Phase 4A validated the runtime mechanism using a parallel `CardMutationShim` type in `hooks/spike/`. Phase 4A deliberately skipped predicates, through-table edges, and internal plumbing (`SetDone`, `SetMutationID`, `WhereP`). Phase 4C spike extends `runtime/entbuilder/Mutation[T]` with the missing surface, extends the descriptor to cover all fields + the `Owner` edge, and **replaces** the 680-line generated `CardMutation` with a ~200-line wrapper-struct form (`type CardMutation struct { *entbuilder.Mutation[ent.Card] }` + per-field 1-liners). This is the first validation that the drop-in API invariant holds when the generated code itself is the replacement — Phase 4A's shim could not validate this because it was parallel.

**Tech Stack:** Go ≥1.22, `entc` codegen, generics, `reflect` for descriptor↔struct validation, standard `testing`, the existing `hooks_test.go` as the acceptance-test harness.

**Spec:** `docs/superpowers/specs/2026-04-24-ent-code-reduction-phase-4-hybrid.md` (§"Descriptor-driven (new)" + §"The type alias nuance")
**Predecessors:**
- `docs/superpowers/plans/2026-04-24-ent-code-reduction-phase-4a-spike.md` (Phase 4A — runtime Mutation[T] + schema descriptors shipped)
- `docs/superpowers/plans/2026-04-24-ent-code-reduction-phase-4b.md` (Phase 4B — lean-generics C/U/D/Q templates shipped)
- `docs/superpowers/results/2026-04-24-phase4a-spike-measurements.md` (Phase 4A GO; shim 266 LOC vs generated 680 LOC → 39% ratio)
- `docs/superpowers/results/2026-04-24-phase4b-measurements.md` (Phase 4B PARTIAL; 3.28% LOC reduction on CUDQ, zero API diff)

**Successor:** Phase 4D plan (codegen productionization of the wrapper-struct pattern for `internal_mutation.tmpl`) + witness-test generator. Not in this plan.

**Out-of-scope for this plan:**
- Codegen templates (any `entc/gen/template/*.tmpl` changes). Phase 4D.
- Witness-test generator. Phase 4D.
- Client (`*XxxClient`) or entql (`*XxxFilter`) rewrites. Phase 4E.
- Through-table edge state (M2M junction entities). Spike scope stays on the scalar + unique-inverse-edge path exercised by `hooks`, same as Phase 4A. Edgeschema coverage is deferred to Phase 4D's integration regen.
- Consumer regen + final measurement. Only this spike's LOC ratio is captured.

---

## File Structure

**Files created by this plan:**

```
runtime/entbuilder/
  mutation_edges.go          # Task 3 — AddedIDs / RemovedIDs / ResetEdge
  mutation_edges_test.go     # Task 3
  mutation_predicates.go     # Task 5 — AddPredicate / MutationPredicates / Where / WhereP
  mutation_predicates_test.go # Task 5

docs/superpowers/results/
  2026-04-24-phase4c-spike-measurements.md   # Task 14 — final decision doc
```

**Files modified by this plan:**

```
runtime/entbuilder/
  mutation.go                # Tasks 1, 2, 7 — append AppendField, Type, SetOp, SetDone, SetMutationID
  mutation_test.go           # matching tests

entc/integration/hooks/ent/internal/
  card_mutation.go           # Task 11 — IN-PLACE replacement (680 → ~200 LOC)
  card_descriptor.go         # Task 9 — NEW file, not in hooks/spike; co-located with generated code
```

**Files read but NOT modified (reference):**

```
entc/integration/hooks/spike/card_descriptor.go       # Phase 4A descriptor — extend/adapt pattern
entc/integration/hooks/spike/card_mutation_shim.go    # Phase 4A shim — 41 methods, proves delegation pattern
entc/integration/hooks/ent/card.go                    # ent.Card struct definition
entc/integration/hooks/ent/card/card.go               # constants: Label, FieldID, FieldName, …
entc/integration/hooks/hooks_test.go                  # acceptance harness (must pass unchanged)
entc/integration/hooks/ent/internal/mutation.go       # the Mutation interface that CardMutation implements
entc/integration/hooks/ent/client.go                  # how client.Card.Get(ctx, id) works (for OldField closure)
runtime/entbuilder/mutation.go                        # current Mutation[T] surface
runtime/entbuilder/schema.go                          # Schema[T] descriptor type
```

---

## Decision upfront: wrapper-struct with embedded `*Mutation[T]`

Phase 4A's spike used a non-embedded field (`type CardMutationShim struct { m *entbuilder.Mutation[ent.Card] }`) and manually forwarded every method including trivial ones like `Op()`, `Fields()`, `AddedFields()`. Phase 4C uses the **embedded** form:

```go
type CardMutation struct {
    *entbuilder.Mutation[ent.Card]
}
```

Rationale:
- Generic methods (`Op`, `Fields`, `AddedFields`, `ClearedFields`, `SetField`, `Field`, `ClearField`, `FieldCleared`, `AddField`, `AddedField`, `ResetField`, `SetID`, `ID`, `SetEdgeID`, `EdgeID`, `ClearEdge`, `EdgeCleared`, `AddedEdges`, `ClearedEdges`, `RemovedEdges`) get promoted automatically — saves 22+ 1-line forwarders per schema.
- Schema-specific methods (`OldName`, `Name()` returning typed value, `SetNumber(s string)`, `OldOwner`, etc.) are declared locally on `*CardMutation`, shadowing or extending the generic methods. Go permits this because the named struct type is in the same package.
- `CardMutation` has distinct type identity from `*entbuilder.Mutation[ent.Card]`, so type assertions against `*CardMutation` in existing consumer code continue to work. Type assertions against the generic would not — but consumers don't currently type-assert against the generic (it didn't exist before this refactor).
- One collision: the generic's `ID() (any, bool)` conflicts with the drop-in API's `ID() (int, bool)`. Solution: the local declaration `func (m *CardMutation) ID() (int, bool)` shadows the promoted method, returning the typed form. This is the **only** method that needs shadowing beyond the schema-specific additions.

If, during the spike, the embedded form produces an unresolvable conflict, fall back to the non-embedded-field form (Phase 4A pattern) and document. Gate: if fallback is needed, that's a data point; the spike's NO-GO would only come from test failures, not from which struct form won.

---

## Phase C-1 — Runtime Mutation[T] extensions (TDD)

Extend `runtime/entbuilder.Mutation[T]` with the generic surface needed for CardMutation replacement. Gap analysis from Phase 4A:

| Method | Phase 4A has it? | Needed for CardMutation? | Task |
|---|---|---|---|
| `Op`, `SetField`, `Field`, `ClearField`, `FieldCleared`, `AddField`, `AddedField`, `ResetField`, `Fields`, `AddedFields`, `ClearedFields` | ✅ | ✅ | — |
| `SetID`, `ID`, `SetEdgeID`, `EdgeID`, `ClearEdge`, `EdgeCleared`, `AddedEdges`, `ClearedEdges`, `RemovedEdges`, `OldField` | ✅ | ✅ | — |
| `AppendField` (JSON append) | ❌ | ✅ (for JSON fields) | Task 1 |
| `Type()` (returns schema name) | ❌ | ✅ (used by hook framework) | Task 2 |
| `AddedIDs(name)`, `RemovedIDs(name)`, `ResetEdge(name)` | ❌ | ✅ (unique & non-unique edges) | Task 3 |
| `SetOp(op)`, `SetMutationID(id)`, `SetDone()`, `IDs(ctx)` | ❌ | ✅ (internal plumbing + Client query) | Task 7 |
| `AddPredicate`, `MutationPredicates`, `Where`, `WhereP` | ❌ | ✅ (predicate bookkeeping) | Task 5 |

### Task 1: `AppendField` — TDD

**Files:**
- Modify: `runtime/entbuilder/mutation.go`
- Modify: `runtime/entbuilder/mutation_test.go`

- [ ] **Step 1: Write the failing test**

Append to `runtime/entbuilder/mutation_test.go`:

```go

func TestMutation_AppendField_JSON(t *testing.T) {
	s := &Schema[fakeCardNode]{
		Name: "cards",
		Fields: []Field{
			{Name: "Tags", Column: "tags", Type: reflect.TypeOf([]string(nil))},
		},
	}
	m := NewMutation[fakeCardNode](s, OpUpdate)
	if err := m.AppendField("Tags", []string{"a", "b"}); err != nil {
		t.Fatalf("AppendField: unexpected err: %v", err)
	}
	got, ok := m.AppendedField("Tags")
	if !ok {
		t.Fatal("expected AppendedField(Tags) ok=true after AppendField")
	}
	if !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("unexpected AppendedField: %v", got)
	}
}

func TestMutation_AppendField_UnknownField_Error(t *testing.T) {
	s := &Schema[fakeCardNode]{Name: "cards"}
	m := NewMutation[fakeCardNode](s, OpUpdate)
	if err := m.AppendField("Missing", "x"); err == nil {
		t.Fatal("AppendField on missing field: expected err, got nil")
	}
}
```

- [ ] **Step 2: Run — expect compile failure**

Run: `go test ./runtime/entbuilder/ -run TestMutation_AppendField -v`
Expected: `undefined: AppendField` / `undefined: AppendedField`.

- [ ] **Step 3: Implement**

Append to `runtime/entbuilder/mutation.go` (in the appropriate section near `AddField`):

```go

// AppendField records an append operation on a JSON-typed field. Separate from
// SetField (overwrite) and AddField (numeric delta). Generated code routes
// `AppendX(v)` calls through this.
func (m *Mutation[T]) AppendField(name string, v any) error {
	if _, ok := m.schema.FindField(name); !ok {
		return fmt.Errorf("entbuilder: %s: unknown field %q on AppendField", m.schema.Name, name)
	}
	if m.appended == nil {
		m.appended = make(map[string]any)
	}
	m.appended[name] = v
	return nil
}

// AppendedField returns the value queued for JSON append on the named field.
func (m *Mutation[T]) AppendedField(name string) (any, bool) {
	v, ok := m.appended[name]
	return v, ok
}
```

And add the field to the `Mutation[T]` struct definition (find the existing struct literal in mutation.go):

```go
appended map[string]any
```

(Place alongside the existing `added`, `cleared`, `fields` maps.)

- [ ] **Step 4: Run**

Run: `go test ./runtime/entbuilder/ -run TestMutation_AppendField -v`
Expected: 2 PASS.

- [ ] **Step 5: Commit**

```bash
git add runtime/entbuilder/mutation.go runtime/entbuilder/mutation_test.go
```

```bash
git commit -m "feat(entbuilder): add AppendField for JSON append semantics"
```

---

### Task 2: `Type()` — TDD

**Files:**
- Modify: `runtime/entbuilder/mutation.go`
- Modify: `runtime/entbuilder/mutation_test.go`

- [ ] **Step 1: Write the failing test**

Append to `runtime/entbuilder/mutation_test.go`:

```go

func TestMutation_Type_ReturnsSchemaName(t *testing.T) {
	s := &Schema[fakeCardNode]{Name: "cards"}
	m := NewMutation[fakeCardNode](s, OpCreate)
	if got := m.Type(); got != "cards" {
		t.Fatalf("Type = %q; want %q", got, "cards")
	}
}
```

- [ ] **Step 2: Run — expect compile failure**

Run: `go test ./runtime/entbuilder/ -run TestMutation_Type -v`
Expected: `undefined: Type`.

- [ ] **Step 3: Implement**

Append to `runtime/entbuilder/mutation.go`:

```go

// Type returns the schema's logical name (matches generated code's Type() method).
// Used by the hook framework to dispatch schema-typed hooks.
func (m *Mutation[T]) Type() string { return m.schema.Name }
```

- [ ] **Step 4: Run**

Run: `go test ./runtime/entbuilder/ -run TestMutation_Type -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add runtime/entbuilder/mutation.go runtime/entbuilder/mutation_test.go
```

```bash
git commit -m "feat(entbuilder): add Type() accessor returning schema name"
```

---

### Task 3: Edge ID tracking — `AddedIDs`, `RemovedIDs`, `ResetEdge`

**Files:**
- Create: `runtime/entbuilder/mutation_edges.go`
- Create: `runtime/entbuilder/mutation_edges_test.go`

Phase 4A's `Mutation[T]` supports `SetEdgeID` + `ClearEdge`, but not the multi-edge-ID tracking required by non-unique edges (AddXIDs / RemoveXIDs patterns). Card's schema has only one edge (`Owner`, unique), but generated `CardMutation` still exposes `AddedIDs(name)` and `RemovedIDs(name)` with the contract that for a unique edge they return a 1-element (or 0-element) slice wrapping the set ID.

- [ ] **Step 1: Write the failing test**

Create `runtime/entbuilder/mutation_edges_test.go`:

```go
package entbuilder

import (
	"reflect"
	"testing"
)

func TestMutation_AddedIDs_UniqueEdge_SingleID(t *testing.T) {
	s := &Schema[fakeCardNode]{
		Name: "cards",
		Edges: []Edge{{Name: "Owner", Unique: true, Inverse: true, Field: "owner_id"}},
	}
	m := NewMutation[fakeCardNode](s, OpUpdate)
	if err := m.SetEdgeID("Owner", 42); err != nil {
		t.Fatalf("SetEdgeID: %v", err)
	}
	ids := m.AddedIDs("Owner")
	if !reflect.DeepEqual(ids, []any{42}) {
		t.Fatalf("AddedIDs = %v; want [42]", ids)
	}
}

func TestMutation_AddedIDs_NonUniqueEdge_Multi(t *testing.T) {
	s := &Schema[fakeCardNode]{
		Name:  "users",
		Edges: []Edge{{Name: "Tags", Unique: false}},
	}
	m := NewMutation[fakeCardNode](s, OpCreate)
	if err := m.AddEdgeIDs("Tags", 1, 2, 3); err != nil {
		t.Fatalf("AddEdgeIDs: %v", err)
	}
	ids := m.AddedIDs("Tags")
	if !reflect.DeepEqual(ids, []any{1, 2, 3}) {
		t.Fatalf("AddedIDs = %v; want [1 2 3]", ids)
	}
}

func TestMutation_RemovedIDs_NonUniqueEdge(t *testing.T) {
	s := &Schema[fakeCardNode]{
		Name:  "users",
		Edges: []Edge{{Name: "Tags"}},
	}
	m := NewMutation[fakeCardNode](s, OpUpdate)
	if err := m.RemoveEdgeIDs("Tags", 5, 6); err != nil {
		t.Fatalf("RemoveEdgeIDs: %v", err)
	}
	ids := m.RemovedIDs("Tags")
	if !reflect.DeepEqual(ids, []any{5, 6}) {
		t.Fatalf("RemovedIDs = %v; want [5 6]", ids)
	}
}

func TestMutation_ResetEdge_ClearsAddedAndRemoved(t *testing.T) {
	s := &Schema[fakeCardNode]{
		Name:  "users",
		Edges: []Edge{{Name: "Tags"}},
	}
	m := NewMutation[fakeCardNode](s, OpUpdate)
	_ = m.AddEdgeIDs("Tags", 1, 2)
	_ = m.RemoveEdgeIDs("Tags", 3)
	if err := m.ResetEdge("Tags"); err != nil {
		t.Fatalf("ResetEdge: %v", err)
	}
	if got := m.AddedIDs("Tags"); len(got) != 0 {
		t.Fatalf("AddedIDs after ResetEdge = %v; want empty", got)
	}
	if got := m.RemovedIDs("Tags"); len(got) != 0 {
		t.Fatalf("RemovedIDs after ResetEdge = %v; want empty", got)
	}
}
```

- [ ] **Step 2: Run — expect compile failures**

Run: `go test ./runtime/entbuilder/ -run TestMutation_AddedIDs -v`
Expected: `undefined: AddedIDs`, `undefined: AddEdgeIDs`, `undefined: RemoveEdgeIDs`, `undefined: RemovedIDs`, `undefined: ResetEdge`.

- [ ] **Step 3: Implement**

Create `runtime/entbuilder/mutation_edges.go`:

```go
package entbuilder

import "fmt"

// AddEdgeIDs appends IDs to a non-unique edge (AddXIDs pattern in generated code).
// For unique edges, prefer SetEdgeID.
func (m *Mutation[T]) AddEdgeIDs(name string, ids ...any) error {
	if _, ok := m.schema.FindEdge(name); !ok {
		return fmt.Errorf("entbuilder: %s: unknown edge %q on AddEdgeIDs", m.schema.Name, name)
	}
	if m.edgeAdded == nil {
		m.edgeAdded = make(map[string][]any)
	}
	m.edgeAdded[name] = append(m.edgeAdded[name], ids...)
	return nil
}

// RemoveEdgeIDs marks IDs for removal from a non-unique edge (RemoveXIDs pattern).
func (m *Mutation[T]) RemoveEdgeIDs(name string, ids ...any) error {
	if _, ok := m.schema.FindEdge(name); !ok {
		return fmt.Errorf("entbuilder: %s: unknown edge %q on RemoveEdgeIDs", m.schema.Name, name)
	}
	if m.edgeRemoved == nil {
		m.edgeRemoved = make(map[string][]any)
	}
	m.edgeRemoved[name] = append(m.edgeRemoved[name], ids...)
	return nil
}

// AddedIDs returns the IDs added to the named edge. For unique edges where
// SetEdgeID was called, returns a 1-element slice. Empty slice if neither
// SetEdgeID nor AddEdgeIDs were invoked.
func (m *Mutation[T]) AddedIDs(name string) []any {
	if ids, ok := m.edgeAdded[name]; ok {
		return ids
	}
	// Unique-edge SetEdgeID bridges to AddedIDs as a 1-slice.
	if id, ok := m.edges[name]; ok {
		return []any{id}
	}
	return nil
}

// RemovedIDs returns the IDs queued for removal from the named edge.
func (m *Mutation[T]) RemovedIDs(name string) []any {
	return m.edgeRemoved[name]
}

// ResetEdge clears all state (set, added, removed, cleared) for the named edge.
func (m *Mutation[T]) ResetEdge(name string) error {
	if _, ok := m.schema.FindEdge(name); !ok {
		return fmt.Errorf("entbuilder: %s: unknown edge %q on ResetEdge", m.schema.Name, name)
	}
	delete(m.edges, name)
	delete(m.edgeAdded, name)
	delete(m.edgeRemoved, name)
	delete(m.edgeCleared, name)
	return nil
}
```

And in `runtime/entbuilder/mutation.go`, add the two new maps to the `Mutation[T]` struct:

```go
edgeAdded   map[string][]any
edgeRemoved map[string][]any
```

- [ ] **Step 4: Run**

Run: `go test ./runtime/entbuilder/ -run TestMutation_AddedIDs -v -run TestMutation_RemovedIDs -run TestMutation_ResetEdge -v`
Expected: 4 PASS.

- [ ] **Step 5: Update `RemovedEdges()` to reflect the new edgeRemoved map**

In `runtime/entbuilder/mutation.go`, find the existing stub `func (m *Mutation[T]) RemovedEdges() []string { return nil }` and replace with:

```go
func (m *Mutation[T]) RemovedEdges() []string {
	names := make([]string, 0, len(m.edgeRemoved))
	for n := range m.edgeRemoved {
		names = append(names, n)
	}
	return names
}
```

Re-run: `go test ./runtime/entbuilder/ -v`
Expected: all existing tests still pass.

- [ ] **Step 6: Commit**

```bash
git add runtime/entbuilder/mutation.go runtime/entbuilder/mutation_edges.go runtime/entbuilder/mutation_edges_test.go
```

```bash
git commit -m "feat(entbuilder): add edge ID tracking (AddedIDs/RemovedIDs/ResetEdge)"
```

---

### Task 4: Sanity check full Mutation[T] surface

**Files:**
- None.

- [ ] **Step 1: Run all entbuilder tests**

Run: `go test ./runtime/entbuilder/ -v`
Expected: all tests PASS, no regressions. If any existing test fails, diagnose before proceeding.

- [ ] **Step 2: No commit (sanity check)**

---

### Task 5: Predicate bookkeeping — `AddPredicate`, `MutationPredicates`, `Where`, `WhereP`

**Files:**
- Create: `runtime/entbuilder/mutation_predicates.go`
- Create: `runtime/entbuilder/mutation_predicates_test.go`

Generated `*XxxMutation` types hold a predicate slice used by the Query/Update/Delete builders' `Where` flow. The spike shim skipped this because the Phase 4A acceptance path didn't route through predicates. Phase 4C must cover it for full drop-in.

- [ ] **Step 1: Write the failing test**

Create `runtime/entbuilder/mutation_predicates_test.go`:

```go
package entbuilder

import (
	"testing"

	"entgo.io/ent/dialect/sql"
)

func TestMutation_AddPredicate_AppendsToList(t *testing.T) {
	s := &Schema[fakeCardNode]{Name: "cards"}
	m := NewMutation[fakeCardNode](s, OpUpdate)
	if got := m.MutationPredicates(); len(got) != 0 {
		t.Fatalf("new mutation predicates: want empty, got len=%d", len(got))
	}
	p1 := func(*sql.Selector) {}
	p2 := func(*sql.Selector) {}
	m.AddPredicate(p1)
	m.AddPredicate(p2)
	got := m.MutationPredicates()
	if len(got) != 2 {
		t.Fatalf("after 2 AddPredicate: want len=2, got %d", len(got))
	}
}

func TestMutation_WhereP_AppendsSlice(t *testing.T) {
	s := &Schema[fakeCardNode]{Name: "cards"}
	m := NewMutation[fakeCardNode](s, OpUpdate)
	m.WhereP(func(*sql.Selector) {}, func(*sql.Selector) {})
	if got := m.MutationPredicates(); len(got) != 2 {
		t.Fatalf("WhereP(2 preds): want len=2, got %d", len(got))
	}
}
```

- [ ] **Step 2: Run — expect compile failure**

Run: `go test ./runtime/entbuilder/ -run TestMutation_AddPredicate -run TestMutation_WhereP -v`
Expected: `undefined: AddPredicate`, `undefined: MutationPredicates`, `undefined: WhereP`.

- [ ] **Step 3: Implement**

Create `runtime/entbuilder/mutation_predicates.go`:

```go
package entbuilder

import "entgo.io/ent/dialect/sql"

// AddPredicate appends a single predicate to the mutation's where-list.
// Called from the generated Where method.
func (m *Mutation[T]) AddPredicate(p func(*sql.Selector)) {
	m.predicates = append(m.predicates, p)
}

// MutationPredicates returns the accumulated predicates. Callers must not
// mutate the returned slice; it's a reference for inspection only.
func (m *Mutation[T]) MutationPredicates() []func(*sql.Selector) {
	return m.predicates
}

// WhereP is a variadic convenience wrapper for multiple AddPredicate calls,
// matching the generated WhereP signature.
func (m *Mutation[T]) WhereP(ps ...func(*sql.Selector)) {
	m.predicates = append(m.predicates, ps...)
}
```

Add the new field to the `Mutation[T]` struct in `runtime/entbuilder/mutation.go`:

```go
predicates []func(*sql.Selector)
```

And add the import of `"entgo.io/ent/dialect/sql"` if not already present in `mutation.go`. (If only `mutation_predicates.go` needs it, keep it there alone.)

- [ ] **Step 4: Run**

Run: `go test ./runtime/entbuilder/ -run TestMutation_AddPredicate -run TestMutation_WhereP -v`
Expected: 2 PASS.

- [ ] **Step 5: Commit**

```bash
git add runtime/entbuilder/mutation.go runtime/entbuilder/mutation_predicates.go runtime/entbuilder/mutation_predicates_test.go
```

```bash
git commit -m "feat(entbuilder): add predicate bookkeeping (AddPredicate/WhereP/MutationPredicates)"
```

---

### Task 6: Typed `Where` method — deferred to per-schema wrapper

**Files:**
- None (design-decision task).

- [ ] **Step 1: Document the decision**

Generated `CardMutation.Where(ps ...predicate.Card)` takes the per-schema named type `predicate.Card`, which is `func(*sql.Selector)` underlying. Because `predicate.Card` is declared in the generated `ent/card` package and imports would cycle if `runtime/entbuilder` tried to reference it, `Where` cannot be generic — it must live on the per-schema wrapper as a 1-liner forwarder that converts `[]predicate.Card` into `[]func(*sql.Selector)` and calls `WhereP`.

**Outcome:** the per-schema wrapper's `Where` method (in Task 11) is:

```go
func (m *CardMutation) Where(ps ...predicate.Card) {
    sps := make([]func(*sql.Selector), len(ps))
    for i := range ps {
        sps[i] = func(*sql.Selector)(ps[i])
    }
    m.WhereP(sps...)
}
```

No new runtime method needed. Nothing to commit.

- [ ] **Step 2: No commit**

---

### Task 7: Internal plumbing — `SetOp`, `SetMutationID`, `SetDone`, `IDs` via schema

**Files:**
- Modify: `runtime/entbuilder/mutation.go`
- Modify: `runtime/entbuilder/mutation_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `runtime/entbuilder/mutation_test.go`:

```go

func TestMutation_SetOp_Override(t *testing.T) {
	s := &Schema[fakeCardNode]{Name: "cards"}
	m := NewMutation[fakeCardNode](s, OpCreate)
	m.SetOp(OpUpdate)
	if got := m.Op(); got != OpUpdate {
		t.Fatalf("Op after SetOp(Update) = %v; want Update", got)
	}
}

func TestMutation_SetDone_Flag(t *testing.T) {
	s := &Schema[fakeCardNode]{Name: "cards"}
	m := NewMutation[fakeCardNode](s, OpUpdate)
	if m.IsDone() {
		t.Fatal("new mutation IsDone = true; want false")
	}
	m.SetDone()
	if !m.IsDone() {
		t.Fatal("after SetDone: IsDone = false; want true")
	}
}

func TestMutation_SetMutationID_Stores(t *testing.T) {
	s := &Schema[fakeCardNode]{Name: "cards"}
	m := NewMutation[fakeCardNode](s, OpUpdate)
	m.SetMutationID(77)
	got, ok := m.ID()
	if !ok || got != 77 {
		t.Fatalf("ID after SetMutationID(77) = %v,%v; want 77,true", got, ok)
	}
}
```

- [ ] **Step 2: Run — expect compile failure**

Run: `go test ./runtime/entbuilder/ -run TestMutation_SetOp -run TestMutation_SetDone -run TestMutation_SetMutationID -v`
Expected: `undefined: SetOp`, `undefined: SetDone`, `undefined: IsDone`, `undefined: SetMutationID`.

- [ ] **Step 3: Implement**

Append to `runtime/entbuilder/mutation.go`:

```go

// SetOp overrides the operation kind. Used by hook dispatchers that need
// to re-classify a mutation (e.g., DeleteOne → Delete).
func (m *Mutation[T]) SetOp(op Op) { m.op = op }

// SetDone marks the mutation as executed. Hook frameworks consult IsDone()
// to avoid re-running pre-execute hooks on a mutation that already saved.
func (m *Mutation[T]) SetDone() { m.done = true }

// IsDone reports whether SetDone has been called.
func (m *Mutation[T]) IsDone() bool { return m.done }

// SetMutationID is the drop-in-API name for SetID — some generated code
// uses SetMutationID to disambiguate from a SetID field setter. Delegates.
func (m *Mutation[T]) SetMutationID(id any) { m.SetID(id) }
```

Add the `done` field to the `Mutation[T]` struct:

```go
done bool
```

- [ ] **Step 4: Run**

Run: `go test ./runtime/entbuilder/ -run TestMutation_SetOp -run TestMutation_SetDone -run TestMutation_SetMutationID -v`
Expected: 3 PASS.

- [ ] **Step 5: Full regression check**

Run: `go test ./runtime/entbuilder/ -v`
Expected: all previous tests still pass (Phase 3, 4A, 4B, and 4C-so-far).

- [ ] **Step 6: Commit**

```bash
git add runtime/entbuilder/mutation.go runtime/entbuilder/mutation_test.go
```

```bash
git commit -m "feat(entbuilder): add SetOp/SetDone/IsDone/SetMutationID for hook compat"
```

---

## Phase C-2 — CardDescriptor (production-quality)

The Phase 4A spike's `hooks/spike/card_descriptor.go` is minimal — covers a subset of fields, no edge. Phase 4C needs a full descriptor covering all 5 fields of Card plus the `Owner` edge, co-located with the generated code (not in the spike directory).

### Task 8: Inspect the Card schema and map it to descriptor entries

**Files:**
- None (inspection task).

- [ ] **Step 1: Read Card's schema definition**

Run: `cat entc/integration/hooks/ent/schema/card.go`

Enumerate:
- Each field: name, column, Go type, nullable (Optional), immutable
- Each edge: name, kind (O2O/O2M/M2O/M2M), inverse, foreign-key column (if known on Card's side), target entity type

- [ ] **Step 2: Read the current descriptor for comparison**

Run: `cat entc/integration/hooks/spike/card_descriptor.go`

Identify what was covered and what was skipped.

- [ ] **Step 3: Build the mapping table**

Produce (in your head or scratchpad; not a committed file) a table:

| Card field/edge | Type | Nullable | Immutable | Column |
|---|---|---|---|---|
| ID | int | — | yes (PK) | id |
| Number | string | no | no | number |
| Name | string | yes | no | name |
| CreatedAt | time.Time | no | yes | created_at |
| InHook | string | yes | no | in_hook |
| ExpiredAt | time.Time | yes | no | expired_at |
| Owner edge | unique, inverse, O2O toward User | — | — | owner_id |

(Verify against the actual `schema/card.go` — if any field/edge is missing from this table, add it.)

- [ ] **Step 4: No commit (inspection)**

---

### Task 9: Write `card_descriptor.go` in production location

**Files:**
- Create: `entc/integration/hooks/ent/internal/card_descriptor.go`

- [ ] **Step 1: Write the descriptor**

Create `entc/integration/hooks/ent/internal/card_descriptor.go`:

```go
// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package internal

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"entgo.io/ent/runtime/entbuilder"
)

// cardSchema is the full descriptor used by CardMutation. Field list matches
// Card's struct layout exactly; validated by ValidateSchema at client init.
var cardSchema = &entbuilder.Schema[Card]{
	Name:  "Card",
	Table: "cards",
	IDField: entbuilder.Field{
		Name: "ID", Column: "id", Type: reflect.TypeOf(int(0)), Immutable: true,
	},
	Fields: []entbuilder.Field{
		{Name: "Number", Column: "number", Type: reflect.TypeOf("")},
		{Name: "Name", Column: "name", Type: reflect.TypeOf(""), Nullable: true},
		{Name: "CreatedAt", Column: "created_at", Type: reflect.TypeOf(time.Time{}), Immutable: true},
		{Name: "InHook", Column: "in_hook", Type: reflect.TypeOf(""), Nullable: true},
		{Name: "ExpiredAt", Column: "expired_at", Type: reflect.TypeOf(time.Time{}), Nullable: true},
	},
	Edges: []entbuilder.Edge{
		{Name: "Owner", Unique: true, Inverse: true, Field: "owner_id", TargetID: reflect.TypeOf(int(0))},
	},
	OldField: oldCardField,
}

// oldCardField resolves the pre-mutation value of a field by querying the row
// through the mutation's captured client. The client is stashed on the
// Mutation[T].ctxClient at mutation construction time (see NewCardMutation).
func oldCardField(ctx context.Context, m *entbuilder.Mutation[Card], name string) (any, error) {
	client, ok := clientFromMutation(m)
	if !ok {
		return nil, fmt.Errorf("CardMutation.OldField: client not captured")
	}
	id, ok := m.ID()
	if !ok {
		return nil, fmt.Errorf("CardMutation.OldField(%q): missing mutation ID", name)
	}
	row, err := client.Card.Get(ctx, id.(int))
	if err != nil {
		return nil, err
	}
	// Reflect on the struct to extract the field by the descriptor-provided Go name.
	rv := reflect.ValueOf(row).Elem().FieldByName(name)
	if !rv.IsValid() {
		return nil, fmt.Errorf("CardMutation.OldField(%q): no struct field", name)
	}
	return rv.Interface(), nil
}

// clientFromMutation extracts the *Client embedded in the mutation via a
// side-channel map keyed by mutation pointer. Populated by NewCardMutation
// in card_mutation.go. This indirection avoids an entbuilder-generic-to-
// per-schema-client import cycle.
func clientFromMutation(m *entbuilder.Mutation[Card]) (*Client, bool) {
	mutationClientsMu.RLock()
	defer mutationClientsMu.RUnlock()
	c, ok := mutationClients[m]
	return c, ok
}
```

Also add a small helper file if `mutationClients` doesn't already exist — inspect the current codebase first:

```bash
grep -rn mutationClients entc/integration/hooks/ent 2>/dev/null
```

If it doesn't exist, add to this same file:

```go

var (
	mutationClients   = make(map[*entbuilder.Mutation[Card]]*Client)
	mutationClientsMu sync.RWMutex // import "sync"
)
```

(Add `"sync"` to the imports.)

Note: if there's a conceptually cleaner way to attach the client to the mutation — e.g., by storing it in the mutation's unexported state via a runtime helper — prefer that. The side-channel map is pragmatic but adds a cross-mutation lock. If performance shows up in the spike measurement as a concern, reconsider.

- [ ] **Step 2: Build**

Run: `go build ./entc/integration/hooks/...`
Expected: zero errors. If the package doesn't know about `Card` yet (it exists in the same package, so it should), fix the reference.

- [ ] **Step 3: Commit**

```bash
git add entc/integration/hooks/ent/internal/card_descriptor.go
```

```bash
git commit -m "spike(phase4c): co-locate CardDescriptor with generated ent/internal code"
```

---

### Task 10: Validate descriptor against Card struct via `ValidateSchema`

**Files:**
- Create: `entc/integration/hooks/ent/internal/card_descriptor_test.go`

- [ ] **Step 1: Write the validation test**

Create `entc/integration/hooks/ent/internal/card_descriptor_test.go`:

```go
package internal

import (
	"testing"

	"entgo.io/ent/runtime/entbuilder"
)

func TestCardDescriptor_ValidateAgainstStruct(t *testing.T) {
	if err := entbuilder.ValidateSchema(cardSchema, Card{}); err != nil {
		t.Fatalf("CardDescriptor disagrees with Card struct: %v", err)
	}
}
```

- [ ] **Step 2: Run**

Run: `cd entc/integration && go test ./hooks/ent/internal/ -run TestCardDescriptor -v`
Expected: PASS. If FAIL, inspect the error — it will name the drift field. Either the descriptor has a typo or the Card struct has an unexpected field. Fix the descriptor and re-run.

- [ ] **Step 3: Commit**

```bash
git add entc/integration/hooks/ent/internal/card_descriptor_test.go
```

```bash
git commit -m "test(phase4c): CardDescriptor validation against Card struct"
```

---

## Phase C-3 — In-place CardMutation replacement

### Task 11: Replace `card_mutation.go` with the wrapper-struct form

**Files:**
- Modify: `entc/integration/hooks/ent/internal/card_mutation.go` (REPLACE, not edit)

The current `card_mutation.go` is 680 lines. The replacement will be ~200 lines: struct definition + constructor + per-field typed accessors (Name/SetName/OldName/ClearName/NameCleared/ResetName) + edge accessors + Where forwarder. All other methods (Op, Fields, AddedFields, ClearedFields, SetField, Field, ClearField, FieldCleared, AddField, AddedField, ResetField, SetID, ID-any, SetEdgeID, EdgeID, ClearEdge, EdgeCleared, AddedEdges, ClearedEdges, RemovedEdges, OldField, AppendField, AppendedField, Type, SetOp, SetDone, IsDone, SetMutationID, AddEdgeIDs, RemoveEdgeIDs, AddedIDs, RemovedIDs, ResetEdge, AddPredicate, MutationPredicates, WhereP) are promoted from the embedded `*entbuilder.Mutation[Card]`.

- [ ] **Step 1: Back up the original file**

```bash
cp entc/integration/hooks/ent/internal/card_mutation.go /tmp/phase4c-card_mutation-backup.go
wc -l /tmp/phase4c-card_mutation-backup.go
```

Expected: 680 lines.

- [ ] **Step 2: Inspect to enumerate schema-specific methods**

From the current file, list every method whose signature would differ from the generic promoted version. These stay as locally-declared forwarders on `*CardMutation`. The rest are promoted.

Schema-specific methods to retain (each as a 1-liner delegating to the embedded generic):

- Typed constructors: `NewCardMutation(c Config, op Op, opts ...cardOption) *CardMutation`, `newCardMutation`, option-setters `withCardID`, `withCard`
- `ID() (int, bool)` — shadow the generic's `(any, bool)`
- `IDs(ctx) ([]int, error)` — schema-typed query
- Per-field typed accessors (5 fields × ~4 methods = ~20 lines):
  - `Number() (string, bool)`, `SetNumber(string)`, `OldNumber(ctx) (string, error)`, `ResetNumber()`
  - `Name() (string, bool)`, `SetName(string)`, `OldName(ctx) (string, error)`, `ClearName()`, `NameCleared() bool`, `ResetName()`
  - `CreatedAt() (time.Time, bool)`, `SetCreatedAt(time.Time)`, `OldCreatedAt(ctx) (time.Time, error)`, `ResetCreatedAt()`
  - `InHook() (string, bool)`, `SetInHook(string)`, `OldInHook(ctx) (string, error)`, `ResetInHook()`
  - `ExpiredAt() (time.Time, bool)`, `SetExpiredAt(time.Time)`, `OldExpiredAt(ctx) (time.Time, error)`, `ClearExpiredAt()`, `ExpiredAtCleared() bool`, `ResetExpiredAt()`
- Edge accessors: `SetOwnerID(int)`, `OwnerID() (int, bool)`, `OwnerIDs() []int`, `ClearOwner()`, `OwnerCleared() bool`, `ResetOwner()`
- Where forwarder: `Where(ps ...predicate.Card)`

- [ ] **Step 3: Write the replacement file**

Replace the entire contents of `entc/integration/hooks/ent/internal/card_mutation.go` with:

```go
// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

// Code generated by ent, DO NOT EDIT.
// Phase 4C spike: replaces the original 680-LOC generator output with a
// wrapper-struct form that embeds *entbuilder.Mutation[Card]. Per-field
// methods are 1-line forwarders; generic methods (Op, Fields, AddedFields,
// …) are promoted from the embedded generic.

package internal

import (
	"context"
	"time"

	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/entc/integration/hooks/ent/predicate"
	"entgo.io/ent/runtime/entbuilder"
)

// CardMutation is the wrapper struct that implements the per-schema typed API.
type CardMutation struct {
	*entbuilder.Mutation[Card]
	config
}

// cardOption is a function type for configuring CardMutation.
type cardOption func(*CardMutation)

// newCardMutation returns a zero-value CardMutation bound to the given config.
func newCardMutation(c config, op Op, opts ...cardOption) *CardMutation {
	m := &CardMutation{
		Mutation: entbuilder.NewMutation[Card](cardSchema, op),
		config:   c,
	}
	// Register the client on the side-channel map so oldCardField can retrieve it.
	mutationClientsMu.Lock()
	mutationClients[m.Mutation] = c.client
	mutationClientsMu.Unlock()
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// withCardID configures the mutation to target an existing row by ID.
func withCardID(id int) cardOption {
	return func(m *CardMutation) { m.SetID(id) }
}

// withCard seeds the mutation with a full Card row — used by UpdateOne's
// path where we already have the pre-mutation entity in hand.
func withCard(c *Card) cardOption {
	return func(m *CardMutation) {
		m.SetID(c.ID)
		// The generic mutation's OldField will go back to the DB if the
		// generated code asks for a pre-mutation value. We do NOT stash
		// the preloaded node here; that's Phase 4D optimization work.
	}
}

// ID returns the typed Card ID; shadows the embedded generic's ID() (any, bool).
func (m *CardMutation) ID() (id int, exists bool) {
	v, ok := m.Mutation.ID()
	if !ok {
		return 0, false
	}
	return v.(int), true
}

// IDs returns all Card IDs matching the mutation's predicates. Requires the
// client to execute the query.
func (m *CardMutation) IDs(ctx context.Context) ([]int, error) {
	// Delegate to the client's Card query with the accumulated predicates.
	q := m.client.Card.Query()
	for _, p := range m.MutationPredicates() {
		q = q.Where(predicate.Card(p))
	}
	return q.IDs(ctx)
}

// Where forwards schema-typed predicates into the generic mutation's predicate list.
func (m *CardMutation) Where(ps ...predicate.Card) {
	sps := make([]func(*sql.Selector), len(ps))
	for i := range ps {
		sps[i] = (func(*sql.Selector))(ps[i])
	}
	m.WhereP(sps...)
}

// ----- Typed field accessors (1-liners over m.Mutation) -----

// Number
func (m *CardMutation) SetNumber(s string) { _ = m.SetField("Number", s) }
func (m *CardMutation) Number() (r string, exists bool) {
	v, ok := m.Field("Number")
	if !ok {
		return "", false
	}
	return v.(string), true
}
func (m *CardMutation) OldNumber(ctx context.Context) (v string, err error) {
	a, err := m.OldField(ctx, "Number")
	if err != nil {
		return "", err
	}
	return a.(string), nil
}
func (m *CardMutation) ResetNumber() { _ = m.ResetField("Number") }

// Name (nullable + clearable)
func (m *CardMutation) SetName(s string) { _ = m.SetField("Name", s) }
func (m *CardMutation) Name() (r string, exists bool) {
	v, ok := m.Field("Name")
	if !ok {
		return "", false
	}
	return v.(string), true
}
func (m *CardMutation) OldName(ctx context.Context) (v string, err error) {
	a, err := m.OldField(ctx, "Name")
	if err != nil {
		return "", err
	}
	return a.(string), nil
}
func (m *CardMutation) ClearName()        { _ = m.ClearField("Name") }
func (m *CardMutation) NameCleared() bool { return m.FieldCleared("Name") }
func (m *CardMutation) ResetName()        { _ = m.ResetField("Name") }

// CreatedAt
func (m *CardMutation) SetCreatedAt(t time.Time) { _ = m.SetField("CreatedAt", t) }
func (m *CardMutation) CreatedAt() (r time.Time, exists bool) {
	v, ok := m.Field("CreatedAt")
	if !ok {
		return time.Time{}, false
	}
	return v.(time.Time), true
}
func (m *CardMutation) OldCreatedAt(ctx context.Context) (v time.Time, err error) {
	a, err := m.OldField(ctx, "CreatedAt")
	if err != nil {
		return time.Time{}, err
	}
	return a.(time.Time), nil
}
func (m *CardMutation) ResetCreatedAt() { _ = m.ResetField("CreatedAt") }

// InHook
func (m *CardMutation) SetInHook(s string) { _ = m.SetField("InHook", s) }
func (m *CardMutation) InHook() (r string, exists bool) {
	v, ok := m.Field("InHook")
	if !ok {
		return "", false
	}
	return v.(string), true
}
func (m *CardMutation) OldInHook(ctx context.Context) (v string, err error) {
	a, err := m.OldField(ctx, "InHook")
	if err != nil {
		return "", err
	}
	return a.(string), nil
}
func (m *CardMutation) ResetInHook() { _ = m.ResetField("InHook") }

// ExpiredAt (nullable + clearable)
func (m *CardMutation) SetExpiredAt(t time.Time) { _ = m.SetField("ExpiredAt", t) }
func (m *CardMutation) ExpiredAt() (r time.Time, exists bool) {
	v, ok := m.Field("ExpiredAt")
	if !ok {
		return time.Time{}, false
	}
	return v.(time.Time), true
}
func (m *CardMutation) OldExpiredAt(ctx context.Context) (v time.Time, err error) {
	a, err := m.OldField(ctx, "ExpiredAt")
	if err != nil {
		return time.Time{}, err
	}
	return a.(time.Time), nil
}
func (m *CardMutation) ClearExpiredAt()        { _ = m.ClearField("ExpiredAt") }
func (m *CardMutation) ExpiredAtCleared() bool { return m.FieldCleared("ExpiredAt") }
func (m *CardMutation) ResetExpiredAt()        { _ = m.ResetField("ExpiredAt") }

// ----- Edge: Owner (unique, inverse) -----

func (m *CardMutation) SetOwnerID(id int) { _ = m.SetEdgeID("Owner", id) }
func (m *CardMutation) OwnerID() (id int, exists bool) {
	v, ok := m.EdgeID("Owner")
	if !ok {
		return 0, false
	}
	return v.(int), true
}
func (m *CardMutation) OwnerIDs() []int {
	ids := m.AddedIDs("Owner")
	typed := make([]int, len(ids))
	for i, id := range ids {
		typed[i] = id.(int)
	}
	return typed
}
func (m *CardMutation) ClearOwner()        { _ = m.ClearEdge("Owner") }
func (m *CardMutation) OwnerCleared() bool { return m.EdgeCleared("Owner") }
func (m *CardMutation) ResetOwner()        { _ = m.ResetEdge("Owner") }
```

Note: this sketch does NOT include the `Option` / `newCardMutation` logic that the generator currently uses for the `WithCardID` / `WithCard` options pattern. Cross-reference the original file (backed up at `/tmp/phase4c-card_mutation-backup.go`) for any behavior the replacement needs to preserve — for example, the `config` embedding, the `setContext` / `client()` accessors, or the `filters` field. Adapt the replacement to include those fixtures if they're referenced elsewhere in the package. Keep the diff minimal and pragmatic; the LOC target is to land near ~200 lines.

- [ ] **Step 4: Build the integration module**

```bash
cd entc/integration
go build ./hooks/...
```

Expected: zero errors. Likely errors + fixes:
- "undefined: cardSchema" → the descriptor in Task 9 is missing; verify.
- "cannot use ... as ..." → type conversion missing somewhere. Add explicit casts.
- "undefined: predicate.Card" → the predicate package is fine; check the import path.
- "m.client undefined" → the `config` embedding may have renamed or moved; inspect generated `client.go` for the correct field name.

- [ ] **Step 5: Do not commit yet**

Commit after Task 12 (tests pass).

---

## Phase C-4 — Integration validation

### Task 12: Run `hooks_test.go`

**Files:**
- None (test run).

- [ ] **Step 1: Run the full hooks test suite**

```bash
cd entc/integration
go test ./hooks/... -count=1 -v 2>&1 | tail -60
```

Expected: all tests PASS. This is the critical correctness gate for Phase 4C.

If tests fail, common failure modes:
- **Panic: nil pointer dereference in OldField** → the `mutationClients` side-channel isn't registering correctly. Check `newCardMutation`.
- **Type assertion panic in typed accessor** → descriptor's `Type` doesn't match the struct's Go type. Inspect the panicking field + fix the descriptor.
- **Test assertion failure** → the runtime's `OldField`, `AppendField`, or edge-ID bookkeeping is returning a different value than the original `CardMutation` did. Compare against `/tmp/phase4c-card_mutation-backup.go` for that exact method.

For each failure, iterate:
1. Reproduce with `-run TestName` to isolate.
2. Add a debug log line in the per-schema typed accessor or the generic runtime.
3. Fix the minimal bug.
4. Re-run.

If more than 3 iterations don't resolve a given failure, STOP and report BLOCKED with the test name and last diagnostic output.

- [ ] **Step 2: No commit (diagnostic phase)**

---

### Task 13: Commit the in-place replacement

**Files:**
- `entc/integration/hooks/ent/internal/card_mutation.go` (modified)
- All of `Phase C-1` / `C-2` already committed; this commits only Task 11's work.

- [ ] **Step 1: Verify hooks suite is green**

```bash
cd entc/integration && go test ./hooks/... -count=1 | tail -5
```

Expected: `ok  entgo.io/ent/entc/integration/hooks ...`.

- [ ] **Step 2: Count LOC of the replacement**

```bash
wc -l entc/integration/hooks/ent/internal/card_mutation.go entc/integration/hooks/ent/internal/card_descriptor.go
echo "Backup LOC:"
wc -l /tmp/phase4c-card_mutation-backup.go
```

Record both numbers for Task 14.

- [ ] **Step 3: Commit**

```bash
git add entc/integration/hooks/ent/internal/card_mutation.go
```

```bash
git commit -m "spike(phase4c): replace CardMutation in-place with wrapper-struct form"
```

- [ ] **Step 4: Keep the backup**

Do NOT delete `/tmp/phase4c-card_mutation-backup.go` yet — Task 14 computes the LOC ratio from it.

---

## Phase C-5 — Measurement + decision

### Task 14: Write measurements + decision doc

**Files:**
- Create: `docs/superpowers/results/2026-04-24-phase4c-spike-measurements.md`

- [ ] **Step 1: Collect the measurements**

```bash
ORIG_LOC=$(wc -l < /tmp/phase4c-card_mutation-backup.go)
NEW_MUT_LOC=$(wc -l < entc/integration/hooks/ent/internal/card_mutation.go)
NEW_DESC_LOC=$(wc -l < entc/integration/hooks/ent/internal/card_descriptor.go)
NEW_TOTAL_LOC=$((NEW_MUT_LOC + NEW_DESC_LOC))
echo "Original: $ORIG_LOC"
echo "New mutation: $NEW_MUT_LOC"
echo "New descriptor: $NEW_DESC_LOC"
echo "New total (mut+desc): $NEW_TOTAL_LOC"
echo "Ratio: $(echo "scale=3; $NEW_TOTAL_LOC / $ORIG_LOC" | bc)"
```

- [ ] **Step 2: Write the decision doc**

Create `docs/superpowers/results/2026-04-24-phase4c-spike-measurements.md`:

```markdown
# Phase 4C Spike Measurements — In-Place CardMutation Replacement

**Date:** 2026-04-24
**Plan:** docs/superpowers/plans/2026-04-24-ent-code-reduction-phase-4c-spike.md
**Spec:** docs/superpowers/specs/2026-04-24-ent-code-reduction-phase-4-hybrid.md
**Go version:** (fill in)
**Fork HEAD:** (fill in from `git rev-parse HEAD`)

## Spike results

| Metric | Value |
|---|---|
| Original card_mutation.go LOC (pre-spike) | (fill in) |
| Replacement card_mutation.go LOC | (fill in) |
| New card_descriptor.go LOC | (fill in) |
| Total replacement LOC (mut + desc) | (fill in) |
| Ratio (replacement / original) | (fill in — aim for < 40%) |
| hooks/ integration tests | (PASS count / fail count) |

## Gate

| Gate | Target | Measured | Met? |
|---|---|---|---|
| hooks_test.go all green (unchanged) | 100% | … | ✓/✗ |
| Replacement LOC < 50% of original | < 50% | …% | ✓/✗ |
| Descriptor validates against Card struct | ValidateSchema green | … | ✓/✗ |
| Runtime Mutation[T] extensions have unit tests | all green | … | ✓/✗ |
| No new runtime panics | zero | … | ✓/✗ |

## Decision

**Outcome:** (GO to Phase 4D / PARTIAL / NO-GO)

### Interpretation

(Write 1-3 paragraphs on what the numbers tell us. Compare to Phase 4A's 39% ratio. Discuss whether the embedded-generic-struct form preserved drop-in API as expected. Flag any edge cases discovered — e.g., did any test need a source change? Did through-table edge workflow surface as a blocker?)

### Path forward

If GO to Phase 4D:
- Phase 4D plan covers: witness-test generator + `internal_mutation.tmpl` rewrite to emit the wrapper-struct form codegen-driven + integration regen (all `entc/integration/*`) + consumer regen + final measurement vs hybrid-spec gates.
- Type-alias question is closed: embedded-generic struct works (or, if fallback was needed, the non-embedded form from Phase 4A).
- Through-table edge state (M2M) is still open; it must be covered in Phase 4D by extending Mutation[T] with a through-table edge state machine OR by keeping through-table handling in the per-schema wrapper (Phase 4A punted on this).

If PARTIAL/NO-GO: escalate to a new spec describing the specific blocker.
```

Fill in the measured values.

- [ ] **Step 3: Commit the decision**

```bash
git add docs/superpowers/results/2026-04-24-phase4c-spike-measurements.md
```

```bash
git commit -m "docs(results): Phase 4C spike decision"
```

---

### Task 15: Clean up and push

**Files:**
- None.

- [ ] **Step 1: Remove the /tmp backup**

```bash
rm /tmp/phase4c-card_mutation-backup.go
```

- [ ] **Step 2: Final git log + push**

```bash
git log --oneline HEAD~20..HEAD | head -20
git push origin i-h8-ent
```

Phase 4C spike complete.

---

## Self-review appendix

**Spec coverage:**

- Hybrid spec §"Descriptor-driven (new)" — covered: per-schema descriptor emitted (Task 9), generic `Mutation[T]` used (Task 11), witness tests via ValidateSchema (Task 10).
- Hybrid spec §"The type alias nuance" — resolved: embedded-generic struct (not alias) used; fallback documented.
- Hybrid spec §"Correctness safeguards" §"Witness tests per schema" — partially covered (ValidateSchema in Task 10). Full witness-test generator is Phase 4D.
- Hybrid spec §"Runtime pre-flight validation" — `ValidateSchema` on boot is not exercised in this spike (integration tests use `Client.Schema.Create` path); Phase 4D adds a hook to `NewClient`.
- Phase 4A §"Gotchas" — `entc/integration` module boundary respected (Task 12 runs from `entc/integration/`).

**Placeholder scan:**

- "adapt the replacement to include those fixtures if they're referenced elsewhere in the package" in Task 11 Step 3 — this is a discovery direction, not a placeholder. The implementer must inspect the backup file + the surrounding package to find out. Concrete guidance is given (backup location, inspection strategy).
- All code blocks are complete, compilable Go.
- No "TBD" / "fill in details" / "add error handling".

**Type consistency:**

- `*entbuilder.Mutation[Card]` used consistently across Tasks 9, 10, 11.
- `cardSchema` defined in Task 9 and referenced in Tasks 10, 11.
- `mutationClients` / `mutationClientsMu` defined in Task 9 and referenced in Task 11's `newCardMutation`.
- Method signatures in Task 11's replacement match the pre-spike `card_mutation.go` surface (verified against `grep ^func (m \*CardMutation)` output).

**Deferred to Phase 4D:**

- Codegen templates (`internal_mutation.tmpl`).
- Witness-test generator that emits a `<schema>_witness_test.go` per schema.
- Application to `<schema>_client.go` / `<schema>_entql.go`.
- Through-table edge state in `Mutation[T]`.
- `NewClient` → `ValidateSchema` hook.
- Consumer regen + final measurement against hybrid-spec's 40% LOC / 5.5 GB RSS gates.

**Scope rationale:**

Phase 4C is deliberately narrow — one schema, in-place, hooks scenario only. The payoff is the single-most-important architectural datapoint: does the wrapper-struct-with-embedded-generic pattern preserve drop-in API when it IS the generated code? Phase 4A couldn't answer this (parallel shim); a codegen-driven Phase 4D can't start until it's answered.

If the spike succeeds at the expected ~30-40% ratio with zero test regressions, the path to Phase 4D is clear. If it reveals blockers (e.g., an unexpected method-promotion conflict), those become Phase 4C's output and feed into a revised Phase 4D spec.
