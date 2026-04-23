# ent Code Reduction — Phases 1-3 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Audit test coverage, shore up gaps, and run a proof-of-concept that tests whether Approach 1 (lean generics + 1-line generated shims) actually delivers compile-time wins on a large-schema consumer. Produces a measured go/no-go decision.

**Architecture:** Three sequential phases. Phase 1 produces coverage documentation (no code). Phase 2 adds missing unit tests to `runtime/entbuilder/` and a SQL-snapshot regression test harness. Phase 3 rewrites the `where` template and one bit of `create` generation, regenerates a small integration scenario and a single consumer schema against a local replace, measures LOC + compile time + `go vet` RSS, and produces a decision doc. A separate Phase 4 plan follows iff the gate passes.

**Tech Stack:** Go ≥1.22, `entc` codegen, Go templates, standard-library `testing`, `go vet`, `go build -o /dev/null`, shell tools (`wc`, `/usr/bin/time -v`).

**Spec:** `docs/superpowers/specs/2026-04-23-ent-code-reduction-design.md`

**Downstream consumer:** `/home/smoothbrain/dev/matthewsreis/worktrees/main/service-api-go` — already has a `replace entgo.io/ent => github.com/MatthewsREIS/ent …` in `go.mod:272`; POC will temporarily swap this for a local path.

---

## File Structure

**New files created by this plan:**

```
docs/superpowers/coverage/
  ent-integration-matrix.md           # Task 1 output
  service-api-go-matrix.md             # Task 2 output
  gap-fill-list.md                     # Task 3 output

docs/superpowers/results/
  2026-04-23-poc-where-measurements.md # Task 18 output (final decision doc)

runtime/entbuilder/
  helpers_test.go                      # Tasks 4-8 unit tests
  predicate.go                         # Task 12 — generic predicate helpers
  predicate_test.go                    # Task 13 — unit tests for predicate.go

entc/gen/
  sqlsnapshot_test.go                  # Task 9 — SQL regression harness
  testdata/sql_snapshots/              # snapshot files
```

**Modified files:**

```
entc/gen/template/where.tmpl           # Task 14 — 1-line predicate output
entc/integration/edgeschema/ent/**     # Task 15 — regenerated, committed as diff
```

**Files read but NOT modified (reference):**

```
runtime/entbuilder/helpers.go          # existing helpers under test in Phase 2
entc/gen/template/dialect/sql/predicate.tmpl  # sub-template already emits single expressions
COMPACT_HELPERS_RESULTS.md             # prior experiment; format for Task 18 report
```

---

## Phase 1 — Coverage Audit (user step 3)

No code changes. Three deliverables, each a markdown doc committed to `docs/superpowers/coverage/`.

### Task 1: Catalog ent integration-test coverage

**Files:**
- Create: `docs/superpowers/coverage/ent-integration-matrix.md`
- Read: `entc/integration/*/` (one directory per scenario)

- [ ] **Step 1: Enumerate integration scenarios**

Run: `ls -d entc/integration/*/`
Expected: output lists `cascadelete/`, `compose/`, `config/`, `customid/`, `edgefield/`, `edgeschema/`, `gremlin/`, `hooks/`, `idtype/`, `initcheck/`, `json/`, `migrate/`, `multischema/`, `privacy/`, `template/`. Each has its own `ent/` generated dir and a `*_test.go`.

- [ ] **Step 2: For each scenario, classify test coverage by file-category touched**

For each scenario, open its `*_test.go` (e.g. `entc/integration/hooks/hooks_test.go`) and for each test function note which **generated file categories** it exercises:
- `where.go` (predicates)
- `create.go` / builder setters
- `update.go` / builder `SetX`/`AddX`/`ClearX`
- `delete.go`
- `internal/*_mutation.go` (mutation state, `OldX`, `ResetX`)
- `client.go` (top-level entrypoints, hooks, transactions)
- `entql.go` / entql predicates
- `query.go` (top-level query entrypoints, With* eager loading)

A single test can touch many. Record the best fit.

- [ ] **Step 3: Write the matrix**

Write `docs/superpowers/coverage/ent-integration-matrix.md` with one table:

```markdown
# Ent Integration Test Coverage Matrix

| Scenario     | where | create | update | delete | mutation | client | entql | query | Notes |
|--------------|-------|--------|--------|--------|----------|--------|-------|-------|-------|
| cascadelete  |   ✓   |   ✓    |        |   ✓    |          |        |       |   ✓   | focus on cascading deletes through edges |
| customid     |       |   ✓    |        |        |    ✓     |   ✓    |       |       | non-int ID types |
| edgeschema   |   ✓   |   ✓    |   ✓    |        |          |        |       |   ✓   | through-tables |
| hooks        |       |   ✓    |   ✓    |        |    ✓     |   ✓    |       |       | pre/post hooks on mutations |
| ...          |       |        |        |        |          |        |       |       |       |
```

Mark `✓` for scenarios that **meaningfully** cover a category (not just compile-exercise it). Blank = untested by that scenario. Add a short `Notes` column for peculiarities.

- [ ] **Step 4: Commit**

```bash
git add docs/superpowers/coverage/ent-integration-matrix.md
git commit -m "docs: catalog ent integration test coverage matrix"
```

---

### Task 2: Catalog consumer-repo coverage

**Files:**
- Create: `docs/superpowers/coverage/service-api-go-matrix.md`
- Read (external): `/home/smoothbrain/dev/matthewsreis/worktrees/main/service-api-go/**/*_test.go`

- [ ] **Step 1: Find tests that exercise generated ent code**

Run (from the worktree root — not `cd`):

```bash
CONSUMER=/home/smoothbrain/dev/matthewsreis/worktrees/main/service-api-go
rg -l 'ent/gen' $CONSUMER --glob '*_test.go' | sort > /tmp/consumer-ent-tests.txt
wc -l /tmp/consumer-ent-tests.txt
```

Expected: list of test files that import the generated `ent/gen` package.

- [ ] **Step 2: Classify each test file by file-category usage**

For each file in `/tmp/consumer-ent-tests.txt`, identify which generated surfaces it hits. Use:

```bash
for f in $(cat /tmp/consumer-ent-tests.txt); do
  echo "== $f =="
  rg -o 'client\.\w+\.(Query|Create|Update|Delete|UpdateOne|UpdateOneID|DeleteOne|DeleteOneID)' $f | head -5
done
```

Record: which entities are exercised in tests, and by what operation class.

- [ ] **Step 3: Write the matrix**

Write `docs/superpowers/coverage/service-api-go-matrix.md`. Table columns mirror Task 1 plus an `Entities covered` column. Goal: identify which of the 111 schemas have no downstream test coverage at all (these are the riskiest for a refactor).

- [ ] **Step 4: Commit**

```bash
git add docs/superpowers/coverage/service-api-go-matrix.md
git commit -m "docs: catalog service-api-go ent usage coverage matrix"
```

---

### Task 3: Identify critical gaps

**Files:**
- Create: `docs/superpowers/coverage/gap-fill-list.md`
- Read: the two matrix docs from Tasks 1 and 2

- [ ] **Step 1: Cross-reference the two matrices**

Inspect both markdown tables side by side. For each file category in the target list (spec § "Target files"), find:

- Does at least one ent integration scenario cover this category with non-trivial tests?
- Does at least one consumer test exercise this category in the wild?

Categories with "no" to both = critical risk.

- [ ] **Step 2: Enumerate gaps**

Write `docs/superpowers/coverage/gap-fill-list.md` with an ordered list:

```markdown
# Coverage Gaps Required Before POC

## Critical (must fill before Phase 3)

1. `<schema>/update.go` `AddX` semantics on numeric fields — only touched tangentially in `hooks/`. Add a focused test.
2. …

## Should-fill (fill in parallel with Phase 2)

1. …

## Acceptable gaps (document, don't fill)

1. Gremlin dialect — consumer doesn't use it; only ent integration covers.
```

- [ ] **Step 3: Commit**

```bash
git add docs/superpowers/coverage/gap-fill-list.md
git commit -m "docs: list coverage gaps to fill before POC"
```

---

## Phase 2 — Shore up tests (user step 4)

Add unit tests to `runtime/entbuilder/` (currently 0 tests), add a SQL snapshot regression harness, and fill critical gaps from Task 3.

### Task 4: Write unit tests for `SimpleField`

**Files:**
- Create: `runtime/entbuilder/helpers_test.go`

- [ ] **Step 1: Write the failing test**

Write `runtime/entbuilder/helpers_test.go` with this initial test:

```go
package entbuilder

import (
	"testing"

	"entgo.io/ent/schema/field"
)

// fakeMut is a stand-in for a generated *XxxMutation.
// Tests use it to exercise SimpleField without pulling in real generated code.
type fakeMut struct {
	intVal *int
	strVal *string
}

func (m fakeMut) Age() (int, bool) {
	if m.intVal == nil {
		return 0, false
	}
	return *m.intVal, true
}

func (m fakeMut) Name() (string, bool) {
	if m.strVal == nil {
		return "", false
	}
	return *m.strVal, true
}

// fakeNode is a stand-in for a generated *Xxx entity.
type fakeNode struct {
	Age  int
	Name string
}

type fakeCfg struct{}

func TestSimpleField_Int_Value_Set(t *testing.T) {
	n := 42
	desc := SimpleField[fakeCfg, fakeNode, fakeMut, int](
		"age",
		field.TypeInt,
		fakeMut.Age,
		func(n *fakeNode, v int) { n.Age = v },
	)

	fv, ok, err := desc.Value(fakeMut{intVal: &n})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true when value is set")
	}
	if fv.Spec != 42 || fv.Node != 42 {
		t.Fatalf("unexpected FieldValue: %+v", fv)
	}
}

func TestSimpleField_Int_Value_Unset(t *testing.T) {
	desc := SimpleField[fakeCfg, fakeNode, fakeMut, int](
		"age",
		field.TypeInt,
		fakeMut.Age,
		func(n *fakeNode, v int) { n.Age = v },
	)

	fv, ok, err := desc.Value(fakeMut{})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if ok {
		t.Fatal("expected ok=false when value is unset")
	}
	if fv != (FieldValue{}) {
		t.Fatalf("expected zero FieldValue, got: %+v", fv)
	}
}

func TestSimpleField_Assign_Int(t *testing.T) {
	desc := SimpleField[fakeCfg, fakeNode, fakeMut, int](
		"age",
		field.TypeInt,
		fakeMut.Age,
		func(n *fakeNode, v int) { n.Age = v },
	)

	var node fakeNode
	if err := desc.Assign(&node, FieldValue{Node: 99}); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if node.Age != 99 {
		t.Fatalf("expected node.Age=99, got %d", node.Age)
	}
}

func TestSimpleField_Assign_WrongType_Panics(t *testing.T) {
	desc := SimpleField[fakeCfg, fakeNode, fakeMut, int](
		"age",
		field.TypeInt,
		fakeMut.Age,
		func(n *fakeNode, v int) { n.Age = v },
	)
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on type assertion mismatch")
		}
	}()
	var node fakeNode
	_ = desc.Assign(&node, FieldValue{Node: "not-an-int"})
}
```

- [ ] **Step 2: Run the test to verify it fails or passes correctly**

Run: `go test ./runtime/entbuilder/ -run TestSimpleField -v`
Expected: all four tests PASS (they're exercising existing code, not driving new code). If any FAIL, investigate: the test may have uncovered a real bug in the existing helper, in which case file the bug and keep the test.

- [ ] **Step 3: Commit**

```bash
git add runtime/entbuilder/helpers_test.go
git commit -m "test(entbuilder): unit tests for SimpleField helper"
```

---

### Task 5: Unit tests for `NillableField`

**Files:**
- Modify: `runtime/entbuilder/helpers_test.go`

- [ ] **Step 1: Append NillableField tests to the existing file**

Append to `runtime/entbuilder/helpers_test.go`:

```go
type fakeMutNillable struct {
	strVal *string // set means value present; mutation reports (val, true)
}

func (m fakeMutNillable) Nickname() (string, bool) {
	if m.strVal == nil {
		return "", false
	}
	return *m.strVal, true
}

type fakeNodeNillable struct {
	Nickname *string
}

func TestNillableField_Value_Set(t *testing.T) {
	s := "alice"
	desc := NillableField[fakeCfg, fakeNodeNillable, fakeMutNillable, string](
		"nickname",
		field.TypeString,
		fakeMutNillable.Nickname,
		func(n *fakeNodeNillable, v *string) { n.Nickname = v },
	)
	fv, ok, err := desc.Value(fakeMutNillable{strVal: &s})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true")
	}
	if fv.Spec != "alice" {
		t.Fatalf("unexpected Spec: %v", fv.Spec)
	}
	// Node must be a *string pointing at a copy, not the original.
	ptr, isPtr := fv.Node.(*string)
	if !isPtr {
		t.Fatalf("expected Node to be *string, got %T", fv.Node)
	}
	if ptr == &s {
		t.Fatal("Node must be a COPY, not alias of the caller's value")
	}
	if *ptr != "alice" {
		t.Fatalf("unexpected *Node: %v", *ptr)
	}
}

func TestNillableField_Value_Unset(t *testing.T) {
	desc := NillableField[fakeCfg, fakeNodeNillable, fakeMutNillable, string](
		"nickname",
		field.TypeString,
		fakeMutNillable.Nickname,
		func(n *fakeNodeNillable, v *string) { n.Nickname = v },
	)
	fv, ok, err := desc.Value(fakeMutNillable{})
	if err != nil || ok || fv != (FieldValue{}) {
		t.Fatalf("expected zero result, got ok=%v err=%v fv=%+v", ok, err, fv)
	}
}

func TestNillableField_Assign_Pointer(t *testing.T) {
	desc := NillableField[fakeCfg, fakeNodeNillable, fakeMutNillable, string](
		"nickname",
		field.TypeString,
		fakeMutNillable.Nickname,
		func(n *fakeNodeNillable, v *string) { n.Nickname = v },
	)
	s := "bob"
	var node fakeNodeNillable
	if err := desc.Assign(&node, FieldValue{Node: &s}); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if node.Nickname == nil || *node.Nickname != "bob" {
		t.Fatalf("expected *node.Nickname=bob, got %v", node.Nickname)
	}
}

func TestNillableField_Assign_WrongType_NoOp(t *testing.T) {
	// Existing helper silently skips on wrong type (type assertion returns ok=false).
	// Test documents this behavior so a future refactor doesn't accidentally panic.
	desc := NillableField[fakeCfg, fakeNodeNillable, fakeMutNillable, string](
		"nickname",
		field.TypeString,
		fakeMutNillable.Nickname,
		func(n *fakeNodeNillable, v *string) { n.Nickname = v },
	)
	var node fakeNodeNillable
	if err := desc.Assign(&node, FieldValue{Node: "not-a-pointer"}); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if node.Nickname != nil {
		t.Fatal("expected node.Nickname unchanged when type assertion fails")
	}
}
```

- [ ] **Step 2: Run**

Run: `go test ./runtime/entbuilder/ -run TestNillableField -v`
Expected: all four PASS.

- [ ] **Step 3: Commit**

```bash
git add runtime/entbuilder/helpers_test.go
git commit -m "test(entbuilder): unit tests for NillableField helper"
```

---

### Task 6: Unit tests for `FieldWithScanner` and `NillableFieldWithScanner`

**Files:**
- Modify: `runtime/entbuilder/helpers_test.go`

- [ ] **Step 1: Extend the import block at the top of `helpers_test.go`**

Merge `"database/sql/driver"` and `"errors"` into the existing `import (…)` block at the top of the file. The file now has a single `import` block containing all of: `"database/sql/driver"`, `"errors"`, `"testing"`, `"entgo.io/ent/schema/field"`.

- [ ] **Step 2: Append tests (no further import changes)**

Append to `runtime/entbuilder/helpers_test.go`:

```go

type uuidLike [16]byte

func uuidScanner(v uuidLike) (driver.Value, error) {
	return v[:], nil
}

func failingScanner(v uuidLike) (driver.Value, error) {
	return nil, errors.New("boom")
}

type fakeMutUUID struct{ id *uuidLike }

func (m fakeMutUUID) ID() (uuidLike, bool) {
	if m.id == nil {
		return uuidLike{}, false
	}
	return *m.id, true
}

type fakeNodeUUID struct{ ID uuidLike }

func TestFieldWithScanner_Value_OK(t *testing.T) {
	u := uuidLike{1, 2, 3}
	desc := FieldWithScanner[fakeCfg, fakeNodeUUID, fakeMutUUID, uuidLike](
		"id", field.TypeUUID, fakeMutUUID.ID, uuidScanner,
		func(n *fakeNodeUUID, v uuidLike) { n.ID = v },
	)
	fv, ok, err := desc.Value(fakeMutUUID{id: &u})
	if err != nil || !ok {
		t.Fatalf("unexpected ok=%v err=%v", ok, err)
	}
	spec, isBytes := fv.Spec.([]byte)
	if !isBytes || len(spec) != 16 || spec[0] != 1 {
		t.Fatalf("unexpected Spec: %+v", fv.Spec)
	}
	if fv.Node.(uuidLike) != u {
		t.Fatalf("unexpected Node: %+v", fv.Node)
	}
}

func TestFieldWithScanner_ScannerError_PropagatesAsNotOK(t *testing.T) {
	u := uuidLike{1, 2, 3}
	desc := FieldWithScanner[fakeCfg, fakeNodeUUID, fakeMutUUID, uuidLike](
		"id", field.TypeUUID, fakeMutUUID.ID, failingScanner,
		func(n *fakeNodeUUID, v uuidLike) { n.ID = v },
	)
	fv, ok, err := desc.Value(fakeMutUUID{id: &u})
	if err == nil {
		t.Fatal("expected scanner error")
	}
	if ok {
		t.Fatal("expected ok=false on scanner error")
	}
	if fv != (FieldValue{}) {
		t.Fatalf("expected zero FieldValue on error, got %+v", fv)
	}
}

func TestNillableFieldWithScanner_Value_OK(t *testing.T) {
	u := uuidLike{9, 9}
	desc := NillableFieldWithScanner[fakeCfg, fakeNodeUUID, fakeMutUUID, uuidLike](
		"id", field.TypeUUID, fakeMutUUID.ID, uuidScanner,
		func(n *fakeNodeUUID, v *uuidLike) { if v != nil { n.ID = *v } },
	)
	fv, ok, err := desc.Value(fakeMutUUID{id: &u})
	if err != nil || !ok {
		t.Fatalf("unexpected ok=%v err=%v", ok, err)
	}
	ptr, isPtr := fv.Node.(*uuidLike)
	if !isPtr || *ptr != u {
		t.Fatalf("unexpected Node: %+v", fv.Node)
	}
	if ptr == &u {
		t.Fatal("Node must be a copy, not alias")
	}
}
```

- [ ] **Step 3: Run**

Run: `go test ./runtime/entbuilder/ -run 'TestFieldWithScanner|TestNillableFieldWithScanner' -v`
Expected: all PASS.

- [ ] **Step 4: Commit**

```bash
git add runtime/entbuilder/helpers_test.go
git commit -m "test(entbuilder): unit tests for scanner helpers"
```

---

### Task 7: Unit tests for `LazyScanner`

**Files:**
- Modify: `runtime/entbuilder/helpers_test.go`

- [ ] **Step 1: Append test**

Append:

```go
func TestLazyScanner_DefersLookup(t *testing.T) {
	var invoked int
	inner := func(v int) (driver.Value, error) {
		invoked++
		return int64(v * 2), nil
	}
	lazy := LazyScanner(func() ScannerFunc[int] {
		return inner
	})
	if invoked != 0 {
		t.Fatal("LazyScanner must not invoke inner until called")
	}
	got, err := lazy(5)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got.(int64) != 10 {
		t.Fatalf("unexpected result: %v", got)
	}
	if invoked != 1 {
		t.Fatalf("expected 1 invocation, got %d", invoked)
	}
}
```

- [ ] **Step 2: Run**

Run: `go test ./runtime/entbuilder/ -run TestLazyScanner -v`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add runtime/entbuilder/helpers_test.go
git commit -m "test(entbuilder): unit tests for LazyScanner"
```

---

### Task 8: Unit tests for `NewEdgeSpec`

**Files:**
- Modify: `runtime/entbuilder/helpers_test.go`

- [ ] **Step 1: Extend the import block**

Merge `"reflect"` and `"entgo.io/ent/dialect/sql/sqlgraph"` into the existing `import (…)` block at the top of `helpers_test.go`.

- [ ] **Step 2: Append tests (no further import changes)**

Append to `runtime/entbuilder/helpers_test.go`:

```go

func TestNewEdgeSpec_M2M_PKColumns(t *testing.T) {
	spec := NewEdgeSpec(EdgeSpecParams{
		Rel:     sqlgraph.M2M,
		Inverse: false,
		Table:   "user_groups",
		Columns: []string{"user_id", "group_id"},
		Bidi:    true,
		TargetColumn: "id",
		TargetType:   field.TypeInt,
	})
	if spec.Rel != sqlgraph.M2M {
		t.Fatalf("wrong rel: %v", spec.Rel)
	}
	if !reflect.DeepEqual(spec.Columns, []string{"user_id", "group_id"}) {
		t.Fatalf("wrong columns: %v", spec.Columns)
	}
	if !spec.Bidi {
		t.Fatal("Bidi not propagated")
	}
	if spec.Target == nil || spec.Target.IDSpec == nil || spec.Target.IDSpec.Column != "id" {
		t.Fatalf("Target missing or wrong: %+v", spec.Target)
	}
}

func TestNewEdgeSpec_O2M_SingleColumnString(t *testing.T) {
	spec := NewEdgeSpec(EdgeSpecParams{
		Rel:          sqlgraph.O2M,
		Table:        "posts",
		Columns:      "user_id", // single string, should be wrapped
		TargetColumn: "id",
		TargetType:   field.TypeInt,
	})
	if !reflect.DeepEqual(spec.Columns, []string{"user_id"}) {
		t.Fatalf("expected single-element slice, got: %v", spec.Columns)
	}
}

func TestNewEdgeSpec_InvalidColumnsType_EmptyColumns(t *testing.T) {
	// Current helper silently produces empty Columns on unknown types.
	// Document this via test so we notice if it changes.
	spec := NewEdgeSpec(EdgeSpecParams{
		Rel:          sqlgraph.M2O,
		Table:        "x",
		Columns:      42, // int; unsupported
		TargetColumn: "id",
		TargetType:   field.TypeInt,
	})
	if len(spec.Columns) != 0 {
		t.Fatalf("expected empty Columns on unknown type, got: %v", spec.Columns)
	}
}
```

- [ ] **Step 3: Run**

Run: `go test ./runtime/entbuilder/ -run TestNewEdgeSpec -v`
Expected: all three PASS.

- [ ] **Step 4: Commit**

```bash
git add runtime/entbuilder/helpers_test.go
git commit -m "test(entbuilder): unit tests for NewEdgeSpec"
```

---

### Task 9: SQL snapshot regression harness

**Files:**
- Create: `entc/gen/sqlsnapshot_test.go`
- Create: `entc/gen/testdata/sql_snapshots/` (directory)

- [ ] **Step 1: Pick one integration scenario as the snapshot source**

Use `entc/integration/edgeschema/ent/` as the source. Rationale: it exercises all edge kinds (M2M, O2M, O2O) and has complete CRUD generation.

- [ ] **Step 2: Write the harness**

Create `entc/gen/sqlsnapshot_test.go`:

```go
package gen_test

// SQL snapshot regression harness.
//
// Captures SQL emitted by representative queries against a sqlite in-memory DB,
// compared to a frozen snapshot on disk. A failing snapshot means the generator
// changed the SQL produced for a canonical query — either a bug or a deliberate
// change that needs a matching snapshot update.

import (
	"context"
	"database/sql/driver"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	"entgo.io/ent/dialect"
	"entgo.io/ent/entc/integration/edgeschema/ent"
	"entgo.io/ent/entc/integration/edgeschema/ent/migrate"
)

var updateSnapshots = flag.Bool("update-sql-snapshots", false,
	"rewrite SQL snapshot files to match current output")

// sqlCapture wraps a driver.Conn to log every Exec/Query.
// For a full implementation we would wire in via the ent Debug driver instead.
// This stub shows the intended shape; the concrete implementation is
// developed as part of this task.
type sqlCapture struct {
	stmts []string
}

func (c *sqlCapture) log(q string) { c.stmts = append(c.stmts, normalizeSQL(q)) }

func normalizeSQL(s string) string {
	// Collapse whitespace so gofmt/driver differences don't trip snapshots.
	fields := strings.Fields(s)
	return strings.Join(fields, " ")
}

// snapshot loads expected from disk; if -update-sql-snapshots is set, writes got.
func snapshot(t *testing.T, name string, got []string) {
	t.Helper()
	path := filepath.Join("testdata", "sql_snapshots", name+".sql")
	if *updateSnapshots {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte(strings.Join(got, "\n")+"\n"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read snapshot %s: %v (run with -update-sql-snapshots to create)", path, err)
	}
	if strings.TrimSpace(string(want)) != strings.TrimSpace(strings.Join(got, "\n")) {
		t.Fatalf("snapshot %s mismatch:\n--- want ---\n%s\n--- got ---\n%s",
			path, string(want), strings.Join(got, "\n"))
	}
}

// TestSnapshot_CreateTweet captures the SQL emitted by a representative
// create + edge-add operation.
func TestSnapshot_CreateTweet(t *testing.T) {
	cap := &sqlCapture{}
	_ = cap // wire into ent debug driver in implementation
	ctx := context.Background()
	client, err := ent.Open(dialect.SQLite, "file:memdb1?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx, migrate.WithGlobalUniqueID(true)); err != nil {
		t.Fatal(err)
	}
	_ = driver.Value(nil)
	// Representative operation — keep small, deterministic.
	u := client.User.Create().SetName("alice").SaveX(ctx)
	client.Tweet.Create().SetText("hi").SetOwner(u).SaveX(ctx)

	// TODO(implementer): plumb sqlCapture into client via Debug or intercepting driver.
	// For now, this test exists as scaffolding; if the plumbing isn't in place
	// by the end of this task, skip rather than leaving it red.
	if len(cap.stmts) == 0 {
		t.Skip("SQL capture wiring incomplete; see task instructions")
	}
	snapshot(t, "create_tweet_with_owner", cap.stmts)
}
```

**Plumbing note:** The simplest concrete way to capture SQL is to use ent's `Debug()` on the client and intercept with a `dialect.DebugDriver` that writes to a buffer. During implementation, replace the `sqlCapture` stub + TODO with:

```go
import (
	"bytes"

	"entgo.io/ent/dialect"
)

// Open a DebugDriver wrapping sqlite.
drv, err := sql.Open(dialect.SQLite, "...")
var buf bytes.Buffer
client := ent.NewClient(ent.Driver(dialect.DebugWithContext(drv,
	func(_ context.Context, args ...any) {
		for _, a := range args {
			if s, ok := a.(string); ok { buf.WriteString(normalizeSQL(s) + "\n") }
		}
	},
)))
// ... run operations ...
got := strings.Split(strings.TrimSpace(buf.String()), "\n")
snapshot(t, "create_tweet_with_owner", got)
```

- [ ] **Step 3: Flesh out the capture plumbing**

Replace the stub + TODO in step 2 with the concrete `DebugWithContext` wiring shown above. Confirm by running:

```bash
go test ./entc/gen/ -run TestSnapshot_CreateTweet -update-sql-snapshots
```

Expected: creates `entc/gen/testdata/sql_snapshots/create_tweet_with_owner.sql` containing a handful of normalized SQL lines. Eyeball the file — if it's empty, the plumbing isn't capturing; fix before proceeding.

- [ ] **Step 4: Re-run without the update flag**

Run: `go test ./entc/gen/ -run TestSnapshot_CreateTweet -v`
Expected: PASS.

- [ ] **Step 5: Add three more representative queries to the harness**

Extend with:
- `TestSnapshot_UserQuery_WithTweets` — tests `With*` eager loading.
- `TestSnapshot_TweetUpdate_SetEdge` — tests a simple update with edge change.
- `TestSnapshot_TweetDelete_WhereID` — tests a delete predicate.

Each follows the same pattern as `TestSnapshot_CreateTweet`. Generate snapshots via the update flag, verify by eyeball, check in.

- [ ] **Step 6: Commit**

```bash
git add entc/gen/sqlsnapshot_test.go entc/gen/testdata/sql_snapshots/
git commit -m "test(gen): SQL snapshot regression harness for POC gate"
```

---

### Task 10: Fill critical gaps from Task 3

**Files:**
- Modify: one or more scenarios under `entc/integration/*`

- [ ] **Step 1: Work through the "Critical" list from `gap-fill-list.md`**

For each critical gap, add a focused test. Keep tests within the existing scenario most closely matching the feature. Pattern:

```go
func TestCriticalGap_<Category>_<Feature>(t *testing.T) {
    // arrange: use existing scenario client
    // act: exercise the specific code path
    // assert: concrete SQL or behavioral expectation
}
```

For each gap, complete:

- [ ] Write the test using the existing scenario fixture
- [ ] Run: `go test ./entc/integration/<scenario>/ -run TestCriticalGap_<Category>_<Feature> -v`
- [ ] Expect: PASS on current unmodified code. If FAIL, the code has a real bug — file it; push the bug fix into a separate commit/PR.
- [ ] Commit: `git add <files>; git commit -m "test(integration/<scenario>): cover <gap description>"`

Do this for each Critical entry. Do not proceed to Phase 3 while critical gaps remain.

---

## Phase 3 — POC: `where.go` compaction (user step 5)

The core question this phase answers: **does Approach 1 meaningfully reduce compile-time memory on a large-schema consumer?** The spec defines the gate: ≥30% drop in `go vet` RSS on the consumer repo.

### Task 11: Confirm baseline measurements

**Files:**
- Create: `docs/superpowers/results/2026-04-23-poc-where-measurements.md` (skeleton)

- [ ] **Step 1: Regenerate the consumer against the current (pre-POC) fork**

In a scratch terminal, in the consumer repo:

```bash
CONSUMER=/home/smoothbrain/dev/matthewsreis/worktrees/main/service-api-go
FORK=/var/home/smoothbrain/dev/matthewsreis/ent/.worktrees/i-h8-ent

# Swap replace directive to local fork
cd $CONSUMER
cp go.mod go.mod.bak
go mod edit -replace entgo.io/ent=$FORK
go mod tidy

# Regenerate
go generate ./...
```

Expected: regeneration succeeds. `go.mod` shows `replace entgo.io/ent => /var/home/smoothbrain/dev/matthewsreis/ent/.worktrees/i-h8-ent` (no version).

- [ ] **Step 2: Capture baseline LOC, compile time, vet RSS**

```bash
# LOC
find $CONSUMER/api-graphql/src/ent/gen -name '*.go' -exec cat {} + | wc -l > /tmp/baseline-loc.txt
find $CONSUMER/api-graphql/src/ent/gen/*/where.go -exec cat {} + | wc -l > /tmp/baseline-where-loc.txt

# Cold build wall time + RSS
cd $CONSUMER/api-graphql/src
go clean -cache
/usr/bin/time -v go build -o /dev/null ./ent/gen/... 2> /tmp/baseline-build.txt

# Vet wall time + RSS
/usr/bin/time -v go vet ./ent/gen/... 2> /tmp/baseline-vet.txt

# Exported API snapshot (for Task 17 diff)
mkdir -p /tmp/api-baseline
for pkg in $(go list ./ent/gen/...); do
  go doc -all "$pkg" > /tmp/api-baseline/$(echo $pkg | tr / _).txt 2>/dev/null || true
done
```

Expected: all four output files/dirs populated. Note from `baseline-vet.txt` the `Maximum resident set size (kbytes)` value — this is the gate metric. `/tmp/api-baseline/` should contain one file per generated package.

- [ ] **Step 3: Create measurements doc skeleton**

Write `docs/superpowers/results/2026-04-23-poc-where-measurements.md`:

```markdown
# POC Measurements — `where.go` Compaction

**Date:** 2026-04-23
**Target spec:** docs/superpowers/specs/2026-04-23-ent-code-reduction-design.md
**Go version:** $(go version) — fill in
**Consumer SHA:** $(git -C $CONSUMER rev-parse HEAD) — fill in
**Fork SHA (baseline):** $(git rev-parse HEAD) — fill in

## Baseline (pre-POC)

| Metric | Value |
|---|---|
| Consumer ent/gen total LOC | (from /tmp/baseline-loc.txt) |
| Consumer where.go total LOC | (from /tmp/baseline-where-loc.txt) |
| Cold `go build ./ent/gen/...` wall time | (from /tmp/baseline-build.txt) |
| Cold `go build` peak RSS | (kbytes, from /tmp/baseline-build.txt) |
| `go vet ./ent/gen/...` wall time | (from /tmp/baseline-vet.txt) |
| `go vet` peak RSS | (kbytes, from /tmp/baseline-vet.txt) |

## Post-POC

(filled in at Task 17)

## Decision

(filled in at Task 18)
```

Fill in the baseline column from the `/tmp/baseline-*.txt` files.

- [ ] **Step 4: Commit measurements doc skeleton**

```bash
cd $FORK
git add docs/superpowers/results/2026-04-23-poc-where-measurements.md
git commit -m "docs(results): POC baseline measurements"
```

- [ ] **Step 5: Restore consumer go.mod**

```bash
cd $CONSUMER
mv go.mod.bak go.mod
go mod tidy
```

Expected: `go.mod:272` reverts to the original `replace entgo.io/ent => github.com/MatthewsREIS/ent @ <version>`. Do not commit in the consumer; the baseline-measurement regen was a temporary local operation.

---

### Task 12: Add `runtime/entbuilder/predicate.go`

**Files:**
- Create: `runtime/entbuilder/predicate.go`

- [ ] **Step 1: Write the failing test first**

Create `runtime/entbuilder/predicate_test.go`:

```go
package entbuilder

import (
	"testing"

	"entgo.io/ent/dialect/sql"
)

// fakePred is a stand-in for a generated predicate.Xxx type.
// predicate.Xxx is defined as: type Xxx func(*sql.Selector)
type fakePred func(*sql.Selector)

func TestFieldEQ_WrapsSQLFieldEQ(t *testing.T) {
	p := FieldEQ[fakePred]("name", "alice")
	if p == nil {
		t.Fatal("nil predicate")
	}
	// Apply to a fresh selector and confirm it produces the same SQL
	// fragment as sql.FieldEQ directly.
	s1 := sql.Select().From(sql.Table("users"))
	p(s1)
	got, _ := s1.Query()

	s2 := sql.Select().From(sql.Table("users"))
	sql.FieldEQ("name", "alice")(s2)
	want, _ := s2.Query()

	if got != want {
		t.Fatalf("FieldEQ output differs from sql.FieldEQ:\n got: %s\nwant: %s", got, want)
	}
}

func TestFieldEQ_InstantiatedForMultiplePredicateTypes(t *testing.T) {
	// Ensure the generic is actually instantiable for distinct named predicate types
	// — i.e. generic instantiation compiles for two separate types without combining.
	type predA func(*sql.Selector)
	type predB func(*sql.Selector)
	_ = FieldEQ[predA]("a", 1)
	_ = FieldEQ[predB]("b", 2)
}
```

- [ ] **Step 2: Run — test fails because `FieldEQ` doesn't exist yet**

Run: `go test ./runtime/entbuilder/ -run TestFieldEQ -v`
Expected: compile error — `undefined: FieldEQ`.

- [ ] **Step 3: Write minimal implementation**

Create `runtime/entbuilder/predicate.go`:

```go
package entbuilder

import "entgo.io/ent/dialect/sql"

// FieldEQ returns a predicate of type P that applies sql.FieldEQ under the hood.
// P must be a named function type whose underlying type is func(*sql.Selector)
// (i.e. the `predicate.Xxx` types produced by codegen).
//
// Purpose: codegen can replace the repeated body
//
//	return predicate.Xxx(sql.FieldEQ(FieldY, v))
//
// with a single generic call:
//
//	return FieldEQ[predicate.Xxx](FieldY, v)
//
// The type parameter is the only mechanism keeping predicate.Xxx from predicate.Yyy
// at the call site. Instantiation cost scales with the number of distinct named
// predicate types in the consumer's codebase (one per schema).
func FieldEQ[P ~func(*sql.Selector)](field string, v any) P {
	return P(sql.FieldEQ(field, v))
}
```

- [ ] **Step 4: Run test**

Run: `go test ./runtime/entbuilder/ -run TestFieldEQ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add runtime/entbuilder/predicate.go runtime/entbuilder/predicate_test.go
git commit -m "feat(entbuilder): add generic FieldEQ predicate helper"
```

---

### Task 13: Extend `predicate.go` with remaining operators

**Files:**
- Modify: `runtime/entbuilder/predicate.go`
- Modify: `runtime/entbuilder/predicate_test.go`

The full operator set (see `entc/gen/template/dialect/sql/predicate.tmpl` + `sql.Field*` functions in `dialect/sql`): `EQ, NEQ, GT, GTE, LT, LTE, In (variadic), NotIn (variadic), IsNull (niladic), NotNull (niladic), Contains, HasPrefix, HasSuffix, EqualFold, ContainsFold`.

Plus non-field group predicates: `And, Or, Not`.

- [ ] **Step 1: Write one failing test per operator category**

Extend `runtime/entbuilder/predicate_test.go` to cover each: write `TestField<Op>_WrapsSQL<Op>` for each operator. Use the same "apply-to-selector and compare" technique as `TestFieldEQ_WrapsSQLFieldEQ`. For variadic ones, pass a slice. For niladic ones, drop the `v` arg.

Example stub:

```go
func TestFieldIn_WrapsSQLFieldIn(t *testing.T) {
	p := FieldIn[fakePred]("status", []any{"active", "pending"})
	s1 := sql.Select().From(sql.Table("users"))
	p(s1)
	got, _ := s1.Query()
	s2 := sql.Select().From(sql.Table("users"))
	sql.FieldIn("status", "active", "pending")(s2)
	want, _ := s2.Query()
	if got != want {
		t.Fatalf("mismatch:\n got: %s\nwant: %s", got, want)
	}
}
```

- [ ] **Step 2: Run — expect compile failures**

Run: `go test ./runtime/entbuilder/ -run TestField -v`
Expected: compile errors for each unimplemented helper.

- [ ] **Step 3: Implement each helper**

Extend `runtime/entbuilder/predicate.go`:

```go
func FieldNEQ[P ~func(*sql.Selector)](field string, v any) P {
	return P(sql.FieldNEQ(field, v))
}
func FieldGT[P ~func(*sql.Selector)](field string, v any) P {
	return P(sql.FieldGT(field, v))
}
func FieldGTE[P ~func(*sql.Selector)](field string, v any) P {
	return P(sql.FieldGTE(field, v))
}
func FieldLT[P ~func(*sql.Selector)](field string, v any) P {
	return P(sql.FieldLT(field, v))
}
func FieldLTE[P ~func(*sql.Selector)](field string, v any) P {
	return P(sql.FieldLTE(field, v))
}
func FieldIn[P ~func(*sql.Selector)](field string, vs []any) P {
	return P(sql.FieldIn(field, vs...))
}
func FieldNotIn[P ~func(*sql.Selector)](field string, vs []any) P {
	return P(sql.FieldNotIn(field, vs...))
}
func FieldIsNull[P ~func(*sql.Selector)](field string) P {
	return P(sql.FieldIsNull(field))
}
func FieldNotNull[P ~func(*sql.Selector)](field string) P {
	return P(sql.FieldNotNull(field))
}
func FieldContains[P ~func(*sql.Selector)](field string, v string) P {
	return P(sql.FieldContains(field, v))
}
func FieldHasPrefix[P ~func(*sql.Selector)](field string, v string) P {
	return P(sql.FieldHasPrefix(field, v))
}
func FieldHasSuffix[P ~func(*sql.Selector)](field string, v string) P {
	return P(sql.FieldHasSuffix(field, v))
}
func FieldEqualFold[P ~func(*sql.Selector)](field string, v string) P {
	return P(sql.FieldEqualFold(field, v))
}
func FieldContainsFold[P ~func(*sql.Selector)](field string, v string) P {
	return P(sql.FieldContainsFold(field, v))
}

// And / Or / Not for variadic / unary groupings.
func AndPreds[P ~func(*sql.Selector)](ps ...P) P {
	asSel := make([]func(*sql.Selector), len(ps))
	for i := range ps {
		asSel[i] = (func(*sql.Selector))(ps[i])
	}
	return P(sql.AndPredicates(asSel...))
}
func OrPreds[P ~func(*sql.Selector)](ps ...P) P {
	asSel := make([]func(*sql.Selector), len(ps))
	for i := range ps {
		asSel[i] = (func(*sql.Selector))(ps[i])
	}
	return P(sql.OrPredicates(asSel...))
}
func NotPred[P ~func(*sql.Selector)](p P) P {
	return P(sql.NotPredicates((func(*sql.Selector))(p)))
}
```

- [ ] **Step 4: Run**

Run: `go test ./runtime/entbuilder/ -run TestField -v`
Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add runtime/entbuilder/predicate.go runtime/entbuilder/predicate_test.go
git commit -m "feat(entbuilder): complete generic field predicate helpers"
```

---

### Task 14: Rewrite `where.tmpl` for 1-line output

**Files:**
- Modify: `entc/gen/template/where.tmpl`

The existing `where.tmpl` emits each function as ~3 source lines (func sig on line 1, body on line 2, close brace on line 3) plus a doc comment. The target is: doc comment on line 1 (preserved), the entire function definition on line 2, no close brace line.

**Before (current output of `where.tmpl`):**

```go
// NameEQ applies the EQ predicate on the "name" field.
func NameEQ(v string) predicate.User {
	return predicate.User(sql.FieldEQ(FieldName, v))
}
```

**Target (after this task):**

```go
// NameEQ applies the EQ predicate on the "name" field.
func NameEQ(v string) predicate.User { return entbuilder.FieldEQ[predicate.User](FieldName, v) }
```

This change exercises two hypotheses simultaneously: (a) per-function source collapses from 3 body lines to 1, (b) every predicate function becomes a generic instantiation. (b) is the real compile-time test.

- [ ] **Step 1: Read the current template**

Open `entc/gen/template/where.tmpl` (193 lines, reviewed during brainstorming). Note the three main loops:

1. ID predicates (`ID`, `IDEQ`, `IDNEQ`, …) — lines 17–39.
2. Field equality predicates (`<Field>`, one per field) — lines 41–73.
3. Field operator predicates (`<Field><Op>`) — lines 75–133.
4. Edge predicates (`Has<Edge>`, `Has<Edge>With`) — lines 135–156.
5. `And`, `Or`, `Not` — lines 158–180.

The sub-template `dialect/sql/predicate/field/ops` (in `entc/gen/template/dialect/sql/predicate.tmpl:25`) produces expressions like `sql.FieldEQ(FieldName, v)`. We keep those sub-templates; we change only how the outer function wraps them.

- [ ] **Step 2: Modify the top-level wrapping to be single-line and route through `entbuilder`**

For simple (non-value-scanner, non-variadic-with-prepass) predicates, collapse to one line and route through `entbuilder.Field<Op>`. Keep the existing multi-line emission path for predicates that require pre-processing (value scanner, variadic with error accumulation) — those are a minority.

Concretely, change the Field-equality loop (currently template/where.tmpl:41–73). Current shape:

```gotemplate
{{- else }}
    {{- if and $f.HasGoType (not $f.Type.Valuer) }}
        vc := {{ $f.BasicType "v" }}
        {{- $arg = "vc" }}
    {{- end }}
    return predicate.{{ $.Name }}(
        {{- with extend $ "Arg" $arg "Field" $f -}}
            {{ $tmpl := printf "dialect/%s/predicate/field" $.Storage }}
            {{- xtemplate $tmpl . }}
        {{- end -}}
    )
{{- end }}
```

Replace with a version that, when there's no pre-processing required (`$f.HasGoType` false AND no value scanner), emits a single-line body like:

```gotemplate
{{- else }}
    {{- if and $f.HasGoType (not $f.Type.Valuer) }}
        // Pre-processing case: keep multi-line for readability
        vc := {{ $f.BasicType "v" }}
        {{- $arg = "vc" }}
        return predicate.{{ $.Name }}(
            {{- with extend $ "Arg" $arg "Field" $f -}}
                {{ $tmpl := printf "dialect/%s/predicate/field" $.Storage }}
                {{- xtemplate $tmpl . }}
            {{- end -}}
        )
    {{- else }}
        // Simple case: single-line via entbuilder generic.
        return entbuilder.FieldEQ[predicate.{{ $.Name }}]({{ $f.Constant }}, {{ $arg }})
    {{- end }}
{{- end }}
```

Apply the same pattern to the operator-predicates loop (template/where.tmpl:75–133): if no pre-processing, emit `return entbuilder.Field<Op>[predicate.<Name>]({{ $f.Constant }}, {{ $arg }})` on one line.

- [ ] **Step 3: Add `entbuilder` import to the generated `where.go`**

The where package currently imports `entgo.io/ent/dialect/sql` etc. Add `entbuilder` via the standard codegen `import.tmpl` mechanism — locate where `where.tmpl` declares its imports (via `{{ template "import" $ }}` at line 15) and confirm that when `entbuilder` symbols are referenced, the import gets added automatically. If the codegen's import resolver doesn't auto-detect runtime imports, add a static import in `where.tmpl` conditional on at least one simple-case predicate being emitted:

```gotemplate
{{ template "import" $ }}
import "entgo.io/ent/runtime/entbuilder"
```

(The proper approach is to wire `entbuilder` into the import resolver in `entc/gen/func.go` or wherever package imports are computed; use that if it's straightforward. Falling back to a bare `import` statement in the template is acceptable for this POC.)

- [ ] **Step 4: Regenerate ent's own integration code**

```bash
cd /var/home/smoothbrain/dev/matthewsreis/ent/.worktrees/i-h8-ent
go generate ./entc/integration/edgeschema/...
```

Expected: regeneration succeeds. Inspect `entc/integration/edgeschema/ent/user/where.go` (or similar file in that scenario) — most predicate function bodies should now be 1-line; some may retain multi-line shape where pre-processing is required.

- [ ] **Step 5: Run the full integration suite against the regenerated code**

```bash
go test ./entc/integration/edgeschema/... -v
```

Expected: all tests PASS. If any fail, investigate — the SQL snapshot test from Task 9 should catch behavioral regressions; see which assertion fires.

- [ ] **Step 6: Run the SQL snapshot tests**

```bash
go test ./entc/gen/ -run TestSnapshot -v
```

Expected: PASS. If they fail with a diff, the template change shifted emitted SQL. Read the diff — if it's pure whitespace or equivalent, update snapshots with `-update-sql-snapshots`; if semantically different, there's a real regression. Fix the template.

- [ ] **Step 7: Commit**

```bash
git add entc/gen/template/where.tmpl entc/integration/edgeschema/ent
git commit -m "gen: compact where.tmpl simple predicates through entbuilder"
```

---

### Task 15: Regenerate one consumer schema

**Files:**
- None in this fork. Temporary changes in the consumer repo only, reverted at end of task.

- [ ] **Step 1: Swap consumer to local replace**

```bash
CONSUMER=/home/smoothbrain/dev/matthewsreis/worktrees/main/service-api-go
FORK=/var/home/smoothbrain/dev/matthewsreis/ent/.worktrees/i-h8-ent
cd $CONSUMER
cp go.mod go.mod.bak
go mod edit -replace entgo.io/ent=$FORK
go mod tidy
```

- [ ] **Step 2: Regenerate the full consumer**

```bash
cd $CONSUMER
go generate ./api-graphql/src/...
```

Expected: regeneration succeeds. `where.go` files for all 111 schemas now use compact output.

- [ ] **Step 3: Build and test the consumer**

```bash
cd $CONSUMER/api-graphql/src
go build ./ent/gen/...
go test ./ent/gen/... -count=1
# Plus whatever the consumer's primary test target is:
go test ./...
```

Expected: builds and tests PASS. If anything fails, investigate:

- Compile error about missing imports of `entbuilder` → template import plumbing (Task 14 step 3) is incomplete; fix.
- Test failure → a behavior change slipped in; compare to Task 14 step 5 results and SQL snapshots.

- [ ] **Step 4: Capture post-POC exported API snapshot**

While the consumer is still in post-POC state (local replace, regenerated), capture the API for Task 17's diff check:

```bash
mkdir -p /tmp/api-poc
for pkg in $(go list ./ent/gen/...); do
  go doc -all "$pkg" > /tmp/api-poc/$(echo $pkg | tr / _).txt 2>/dev/null || true
done
```

- [ ] **Step 5: Nothing to commit in consumer (local experiment)**

Do not create commits in the consumer repo. This step produces measurements, not persistent changes. The consumer `go.mod` is still in local-replace state and will be reverted in Task 16 step 3.

---

### Task 16: Measure post-POC

**Files:**
- Modify: `docs/superpowers/results/2026-04-23-poc-where-measurements.md` (fill Post-POC table)

- [ ] **Step 1: Capture the same metrics as Task 11 step 2, against the regenerated consumer**

```bash
CONSUMER=/home/smoothbrain/dev/matthewsreis/worktrees/main/service-api-go
FORK=/var/home/smoothbrain/dev/matthewsreis/ent/.worktrees/i-h8-ent

# LOC
find $CONSUMER/api-graphql/src/ent/gen -name '*.go' -exec cat {} + | wc -l > /tmp/poc-loc.txt
find $CONSUMER/api-graphql/src/ent/gen/*/where.go -exec cat {} + | wc -l > /tmp/poc-where-loc.txt

# Cold build
cd $CONSUMER/api-graphql/src
go clean -cache
/usr/bin/time -v go build -o /dev/null ./ent/gen/... 2> /tmp/poc-build.txt

# Vet
/usr/bin/time -v go vet ./ent/gen/... 2> /tmp/poc-vet.txt
```

- [ ] **Step 2: Fill the Post-POC table in the measurements doc**

Open `$FORK/docs/superpowers/results/2026-04-23-poc-where-measurements.md` and populate the Post-POC section mirroring the Baseline section. Add a third column with deltas (absolute and percentage).

Exact layout:

```markdown
## Post-POC

| Metric | Baseline | Post-POC | Delta |
|---|---|---|---|
| Consumer ent/gen total LOC | 1,624,012 | … | … (-X%) |
| Consumer where.go total LOC | 155,706 | … | … (-X%) |
| Cold `go build ./ent/gen/...` wall time | … | … | … |
| Cold `go build` peak RSS | … | … | … |
| `go vet ./ent/gen/...` wall time | … | … | … |
| `go vet` peak RSS | … | … | … |
```

- [ ] **Step 3: Restore consumer go.mod**

```bash
cd $CONSUMER
mv go.mod.bak go.mod
go mod tidy
```

Expected: consumer's `go.mod` back to pre-POC state. Verify: `git -C $CONSUMER diff -- go.mod` is empty.

- [ ] **Step 4: Commit measurements**

```bash
cd $FORK
git add docs/superpowers/results/2026-04-23-poc-where-measurements.md
git commit -m "docs(results): POC measurements for where.go compaction"
```

---

### Task 17: Go/no-go decision

**Files:**
- Modify: `docs/superpowers/results/2026-04-23-poc-where-measurements.md` (fill Decision section)

Apply the spec's gate criteria against the measurements:

- where.go LOC ≥ 60% reduction?
- `go vet` peak RSS ≥ 30% reduction?
- `go build` wall time ≥ 25% reduction?
- All tests (ent integration + consumer) green?
- Zero diff in exported API of generated code?

- [ ] **Step 1: Diff the exported API snapshots captured in Task 11 and Task 15**

```bash
diff -r /tmp/api-baseline /tmp/api-poc > /tmp/api-diff.txt
wc -l /tmp/api-diff.txt
```

Expected: **empty diff**. Any diff means the refactor broke drop-in compat. Inspect `/tmp/api-diff.txt` — signature-level differences are a NO-GO regardless of other gate results. Only purely cosmetic diffs (e.g., whitespace in godoc output) are acceptable; if unsure, treat as NO-GO.

- [ ] **Step 2: Fill the Decision section**

```markdown
## Decision

**Gate results:**

| Gate | Target | Measured | Met? |
|---|---|---|---|
| where.go LOC reduction | ≥60% | …% | ✓/✗ |
| go vet peak RSS reduction | ≥30% | …% | ✓/✗ |
| go build wall-time reduction | ≥25% | …% | ✓/✗ |
| ent integration tests | green | … | ✓/✗ |
| consumer tests | green | … | ✓/✗ |
| exported API diff | zero | … | ✓/✗ |

**Decision:** [GO / NO-GO]

**Rationale:** (1-2 paragraphs explaining the call based on the numbers)

**Next step:**
- If GO: write Phase 4 implementation plan (`docs/superpowers/plans/2026-XX-XX-ent-code-reduction-phase-4.md`).
- If NO-GO: draft a new spec covering Approach 2 or 3; POC scope changes.
```

- [ ] **Step 3: Commit decision**

```bash
git add docs/superpowers/results/2026-04-23-poc-where-measurements.md
git commit -m "docs(results): record POC decision for ent code reduction"
```

- [ ] **Step 4: Push**

```bash
git push
```

Phase 1-3 complete.

---

## Self-review appendix

This plan was self-reviewed against the spec. Covered items:

- Phase 1 covers user step 3 (coverage audit).
- Phase 2 covers user step 4 (shore up tests).
- Phase 3 covers user step 5 (POC).
- The go/no-go gate mentioned in the spec is executed in Task 17 with concrete numeric thresholds.
- Correctness safeguards from the spec: SQL snapshot harness (Task 9), direct entbuilder unit tests (Tasks 4-8, 12-13), consumer CI validation (Task 15), exported-API diff check (Task 17 step 1).
- Witness-test generator plumbing from the spec is **not** in this plan — it's only needed starting at Phase 4 `mutation.go` work, where descriptor-struct drift risk first appears. Defer to the Phase 4 plan.
- `gql_*.go` explicitly out of scope per the spec; not touched.

Not covered by this plan (deferred to Phase 4 plan, iff Task 17 is GO):

- `create.go`, `update.go`, `delete.go`, `mutation.go`, `client.go`, `entql.go`, `query.go` template rewrites.
- Witness-test generator.
- Full 111-schema regen and measurement run.
