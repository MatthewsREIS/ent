# ent Code Reduction Phase 4B — Lean-Generics Templates for Create / Update / Delete / Query

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extend the Phase 3 `where.tmpl` lean-generics pattern to the remaining scalar-body templates: `builder/setter.tmpl`, `builder/create.tmpl`, `builder/update.tmpl`, `builder/delete.tmpl`, and `builder/query.tmpl`. Add the minimal `runtime/entbuilder` helpers needed, route generated bodies through them, regenerate every `entc/integration/*` scenario green, and capture a consumer dry-run measurement that compares against the Phase 3 baseline.

**Architecture:** Three new helpers (`BSet`, `BClear`, `Must`) in `runtime/entbuilder` collapse the three repetitive builder-method shapes. Each affected builder template gains a `$isSQL` gate and, for each simple-body method, emits a 1-line call routed through the helper on the SQL storage path. Value-scanner and GoType-conversion branches preserve the current multi-line emission, as Phase 3 did. `import.tmpl` gains a `QueryUsesEntbuilder` scope flag (the other three already exist). The pre-existing `runtime/entbuilder/{create,update,delete,query}.go` descriptor-driven helpers are untouched — they are used by the `dialect/sql/*.tmpl` templates and are out of scope for this phase. Phase 4C productionizes the descriptor-driven approach for `mutation`, `client`, and `entql` and will reuse them then.

**Tech Stack:** Go ≥1.22, `entc` codegen, Go templates, standard-library `testing`, `go vet`, `go build -o /dev/null`, shell tools (`wc`, `/usr/bin/time -v`).

**Spec:** `docs/superpowers/specs/2026-04-24-ent-code-reduction-phase-4-hybrid.md`
**Predecessors:**
- `docs/superpowers/plans/2026-04-23-ent-code-reduction-phase-1-3.md` (Phase 3 — `where.tmpl` pattern this plan extends)
- `docs/superpowers/plans/2026-04-24-ent-code-reduction-phase-4a-spike.md` (Phase 4A — validated descriptor-driven mutation for Phase 4C)
- `docs/superpowers/results/2026-04-23-poc-where-measurements.md` (Phase 3 baseline: 1,635,044 LOC / 9,646,432 kB peak RSS / 1:33.50 cold build)
- `docs/superpowers/results/2026-04-24-phase4a-spike-measurements.md` (Phase 4A GO decision)

**Successor:** Phase 4C plan (descriptor-driven `mutation` / `client` / `entql` templates + witness-test generator). Not in this plan.

**Downstream consumer:** `/home/smoothbrain/dev/matthewsreis/worktrees/main/service-api-go` — uses `replace entgo.io/ent => github.com/MatthewsREIS/ent …` in its `go.mod`. We'll temporarily swap to a local path for measurement, same as Phase 3.

---

## Out-of-scope for this plan

- Descriptor-driven rewrites for `mutation.go`, `client.go`, `entql.go` (Phase 4C).
- Witness-test generator emission (Phase 4C).
- Any changes to `entc/gen/template/dialect/sql/*.tmpl` (the dialect-body templates). Only the `builder/*.tmpl` templates (the outer wrappers) are rewritten here.
- Any changes to `entc/gen/template/dialect/gremlin/*.tmpl` — the `$isSQL` gate preserves the existing multi-line path for non-SQL storage, identical to Phase 3's approach.
- Any changes to `runtime/entbuilder/create.go`, `update.go`, `delete.go`, `query.go` — these are pre-existing descriptor-driven helpers used by dialect templates, not by this plan.
- Consumer (`service-api-go`) commits of any kind. Dry-run measurement only.
- Upstream merges.

---

## File Structure

**Files created by this plan:**

```
runtime/entbuilder/
  builder_setter.go           # Task 1-4 — BSet, BClear
  builder_setter_test.go      # Task 1-4 — unit tests
  builder_query.go            # Task 5-6 — Must
  builder_query_test.go       # Task 5-6 — unit tests

docs/superpowers/results/
  2026-04-24-phase4b-measurements.md  # Task 19-21 — baseline + post-POC + decision
```

**Files modified by this plan:**

```
entc/gen/template/import.tmpl         # Task 7 — add QueryUsesEntbuilder flag
entc/gen/template/builder/setter.tmpl # Task 9  — route simple setters through BSet/BClear
entc/gen/template/builder/create.tmpl # Task 11 — route SaveX/ExecX + propagate scope flag
entc/gen/template/builder/update.tmpl # Task 13 — route SaveX/ExecX, update/edges Clear + propagate scope flag
entc/gen/template/builder/delete.tmpl # Task 14 — route ExecX + propagate scope flag
entc/gen/template/builder/query.tmpl  # Task 15 — route X-wrappers + propagate scope flag

entc/integration/*/ent/**             # Tasks 10, 12, 14, 15, 16 — regenerated artifacts, committed as diffs
```

**Files read but NOT modified (reference):**

```
entc/gen/template/where.tmpl                       # Phase 3's precedent — the pattern this plan extends
runtime/entbuilder/predicate.go                    # Phase 3 helpers — reference only
runtime/entbuilder/helpers.go                      # existing helpers — reference only
runtime/entbuilder/create.go / update.go / delete.go / query.go
                                                   # descriptor-driven helpers — reference only, NOT modified
entc/gen/template/dialect/sql/{create,update,delete,query}.tmpl
                                                   # dialect bodies — NOT modified
entc/integration/hooks/spike/                      # Phase 4A spike — reference only
docs/superpowers/specs/2026-04-24-ent-code-reduction-phase-4-hybrid.md
docs/superpowers/plans/2026-04-23-ent-code-reduction-phase-1-3.md (Tasks 11–17)
docs/superpowers/results/2026-04-23-poc-where-measurements.md
```

---

## Phase B-1 — Runtime helpers (TDD)

Add the minimum-viable generics needed to route Phase 4B's simple-body templates. All helpers live in `runtime/entbuilder` alongside the Phase 3 `predicate.go` file. Three helpers cover the high-volume method shapes.

### Task 1: `BSet` — write the failing test

**Files:**
- Create: `runtime/entbuilder/builder_setter_test.go`

- [ ] **Step 1: Write the failing test**

Create `runtime/entbuilder/builder_setter_test.go`:

```go
package entbuilder

import "testing"

// fakeBuilder stands in for a generated *XxxCreate / *XxxUpdate type.
// The real builders hold config + hooks + mutation; for the helper tests
// we only need an addressable struct and a method-value-like setter.
type fakeBuilder struct {
	setCalls int
	lastInt  int
	lastStr  string
}

func (b *fakeBuilder) setInt(v int) {
	b.setCalls++
	b.lastInt = v
}

func (b *fakeBuilder) setStr(v string) {
	b.setCalls++
	b.lastStr = v
}

func TestBSet_ReturnsSameBuilder(t *testing.T) {
	b := &fakeBuilder{}
	got := BSet(b, b.setInt, 42)
	if got != b {
		t.Fatalf("BSet must return the builder pointer it was given")
	}
	if b.setCalls != 1 {
		t.Fatalf("setter must be called exactly once; got %d", b.setCalls)
	}
	if b.lastInt != 42 {
		t.Fatalf("setter must receive v=42; got %d", b.lastInt)
	}
}

func TestBSet_Str(t *testing.T) {
	b := &fakeBuilder{}
	got := BSet(b, b.setStr, "hello")
	if got != b || b.lastStr != "hello" {
		t.Fatalf("BSet[string] failed: got=%p lastStr=%q", got, b.lastStr)
	}
}

func TestBSet_InstantiatesForMultipleBuilderTypes(t *testing.T) {
	// Ensure generic is instantiable for distinct builder types, matching
	// the 111-schemas-per-consumer pattern.
	type builderA struct{ n int }
	type builderB struct{ n int }
	a := &builderA{}
	b := &builderB{}
	_ = BSet(a, func(v int) { a.n = v }, 1)
	_ = BSet(b, func(v int) { b.n = v }, 2)
	if a.n != 1 || b.n != 2 {
		t.Fatalf("separate instantiations interfered: a.n=%d b.n=%d", a.n, b.n)
	}
}
```

- [ ] **Step 2: Run — expect compile failure**

Run: `go test ./runtime/entbuilder/ -run TestBSet -v`
Expected: `undefined: BSet`.

- [ ] **Step 3: Commit the failing test**

```bash
git add runtime/entbuilder/builder_setter_test.go
```

```bash
git commit -m "test(entbuilder): failing tests for BSet builder helper"
```

---

### Task 2: `BSet` — implement

**Files:**
- Create: `runtime/entbuilder/builder_setter.go`

- [ ] **Step 1: Implement**

Create `runtime/entbuilder/builder_setter.go`:

```go
package entbuilder

// BSet is the generic shape for a generated *XxxCreate / *XxxUpdate setter
// that wraps a call into the mutation and returns the builder. Generated
// code of the form
//
//	func (uc *UserCreate) SetName(v string) *UserCreate {
//	    uc.mutation.SetName(v)
//	    return uc
//	}
//
// collapses to
//
//	func (uc *UserCreate) SetName(v string) *UserCreate { return entbuilder.BSet(uc, uc.mutation.SetName, v) }
//
// The type parameter B is the builder's struct type (call site passes *B);
// V is the value type being set. Instantiation cost scales with
// (# distinct builder types) × (# distinct field types).
func BSet[B any, V any](b *B, set func(V), v V) *B {
	set(v)
	return b
}
```

- [ ] **Step 2: Run**

Run: `go test ./runtime/entbuilder/ -run TestBSet -v`
Expected: 3 PASS.

- [ ] **Step 3: Commit**

```bash
git add runtime/entbuilder/builder_setter.go
```

```bash
git commit -m "feat(entbuilder): add BSet generic builder helper"
```

---

### Task 3: `BClear` — write failing test

**Files:**
- Modify: `runtime/entbuilder/builder_setter_test.go`

- [ ] **Step 1: Append test**

Append to `runtime/entbuilder/builder_setter_test.go`:

```go

func TestBClear_ReturnsSameBuilder(t *testing.T) {
	b := &fakeBuilder{}
	var clearCalls int
	got := BClear(b, func() { clearCalls++ })
	if got != b {
		t.Fatalf("BClear must return the builder pointer it was given")
	}
	if clearCalls != 1 {
		t.Fatalf("clear func must be called exactly once; got %d", clearCalls)
	}
}
```

- [ ] **Step 2: Run — expect compile failure**

Run: `go test ./runtime/entbuilder/ -run TestBClear -v`
Expected: `undefined: BClear`.

- [ ] **Step 3: Commit the failing test**

```bash
git add runtime/entbuilder/builder_setter_test.go
```

```bash
git commit -m "test(entbuilder): failing test for BClear builder helper"
```

---

### Task 4: `BClear` — implement

**Files:**
- Modify: `runtime/entbuilder/builder_setter.go`

- [ ] **Step 1: Append**

Append to `runtime/entbuilder/builder_setter.go`:

```go

// BClear is the generic shape for a generated *XxxUpdate / *XxxCreate
// Clear-method that wraps a call into the mutation and returns the builder.
// Generated code of the form
//
//	func (uu *UserUpdate) ClearBio() *UserUpdate {
//	    uu.mutation.ClearBio()
//	    return uu
//	}
//
// collapses to
//
//	func (uu *UserUpdate) ClearBio() *UserUpdate { return entbuilder.BClear(uu, uu.mutation.ClearBio) }
func BClear[B any](b *B, clear func()) *B {
	clear()
	return b
}
```

- [ ] **Step 2: Run**

Run: `go test ./runtime/entbuilder/ -run TestBClear -v`
Expected: 1 PASS.

- [ ] **Step 3: Commit**

```bash
git add runtime/entbuilder/builder_setter.go
```

```bash
git commit -m "feat(entbuilder): add BClear generic builder helper"
```

---

### Task 5: `Must` — write failing test

**Files:**
- Create: `runtime/entbuilder/builder_query_test.go`

- [ ] **Step 1: Write the failing test**

Create `runtime/entbuilder/builder_query_test.go`:

```go
package entbuilder

import (
	"errors"
	"testing"
)

func TestMust_ReturnsValueOnNilErr(t *testing.T) {
	got := Must(42, nil)
	if got != 42 {
		t.Fatalf("Must should return v when err is nil; got %d", got)
	}
}

func TestMust_PanicsOnErr(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("Must must panic when err is non-nil")
		}
		err, ok := r.(error)
		if !ok {
			t.Fatalf("Must must panic with the err value; got %T", r)
		}
		if err.Error() != "boom" {
			t.Fatalf("panic payload must be the err; got %v", err)
		}
	}()
	_ = Must(0, errors.New("boom"))
}

func TestMust_ZeroValueTypeInference(t *testing.T) {
	// Ensure T is inferred from the first arg across return types that matter
	// for query builders: slices, pointers, primitives.
	type node struct{ ID int }
	var zeroSlice []*node
	got := Must(zeroSlice, nil)
	if got != nil {
		t.Fatalf("zero slice must round-trip through Must; got %v", got)
	}

	var zeroPtr *node
	gotPtr := Must(zeroPtr, nil)
	if gotPtr != nil {
		t.Fatalf("nil *node must round-trip through Must; got %v", gotPtr)
	}
}
```

- [ ] **Step 2: Run — expect compile failure**

Run: `go test ./runtime/entbuilder/ -run TestMust -v`
Expected: `undefined: Must`.

- [ ] **Step 3: Commit the failing test**

```bash
git add runtime/entbuilder/builder_query_test.go
```

```bash
git commit -m "test(entbuilder): failing tests for Must query helper"
```

---

### Task 6: `Must` — implement

**Files:**
- Create: `runtime/entbuilder/builder_query.go`

- [ ] **Step 1: Implement**

Create `runtime/entbuilder/builder_query.go`:

```go
package entbuilder

// Must is the generic shape for a generated *XxxQuery XxxX wrapper that
// panics on error and returns the value. Generated code of the form
//
//	func (uq *UserQuery) AllX(ctx context.Context) []*User {
//	    nodes, err := uq.All(ctx)
//	    if err != nil {
//	        panic(err)
//	    }
//	    return nodes
//	}
//
// collapses to
//
//	func (uq *UserQuery) AllX(ctx context.Context) []*User { return entbuilder.Must(uq.All(ctx)) }
//
// NOTE: the NotFound-tolerant wrappers (FirstX, FirstIDX) cannot use Must;
// they are emitted single-line-collapsed but still multi-statement because
// IsNotFound lives in the generated per-schema package and can't be
// referenced from runtime/entbuilder without an import cycle.
func Must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}
```

- [ ] **Step 2: Run**

Run: `go test ./runtime/entbuilder/ -run TestMust -v`
Expected: 3 PASS.

- [ ] **Step 3: Sanity-check the full entbuilder test suite**

Run: `go test ./runtime/entbuilder/ -v`
Expected: all existing tests (Phase 2, Phase 3, Phase 4A) plus the new ones — all PASS.

- [ ] **Step 4: Commit**

```bash
git add runtime/entbuilder/builder_query.go
```

```bash
git commit -m "feat(entbuilder): add Must generic query-wrapper helper"
```

---

## Phase B-2 — Template plumbing

### Task 7: Add `QueryUsesEntbuilder` flag to `import.tmpl`

**Files:**
- Modify: `entc/gen/template/import.tmpl` (line 46)

The other three flags (Create/Update/Delete/WhereUsesEntbuilder) already gate the `"entgo.io/ent/runtime/entbuilder"` import. `query.tmpl` needs the same pathway.

- [ ] **Step 1: Edit `import.tmpl`**

Find the existing conditional at line 46 of `entc/gen/template/import.tmpl`:

```gotemplate
{{- if and (hasField $ "Scope") (or ($.Scope.CreateUsesEntbuilder) ($.Scope.UpdateUsesEntbuilder) ($.Scope.DeleteUsesEntbuilder) ($.Scope.WhereUsesEntbuilder)) }}
    "entgo.io/ent/runtime/entbuilder"
{{- end }}
```

Replace with:

```gotemplate
{{- if and (hasField $ "Scope") (or ($.Scope.CreateUsesEntbuilder) ($.Scope.UpdateUsesEntbuilder) ($.Scope.DeleteUsesEntbuilder) ($.Scope.WhereUsesEntbuilder) ($.Scope.QueryUsesEntbuilder)) }}
    "entgo.io/ent/runtime/entbuilder"
{{- end }}
```

- [ ] **Step 2: Verify templates still parse**

Run: `go build ./entc/gen/...`
Expected: zero errors. Template-parse errors would show as build errors in `gen_test.go` regeneration helpers.

- [ ] **Step 3: Commit**

```bash
git add entc/gen/template/import.tmpl
```

```bash
git commit -m "gen: add QueryUsesEntbuilder scope flag to import template"
```

---

### Task 8: Record regenerator incantations

**Files:**
- None (reference only for the next template tasks)

- [ ] **Step 1: Understand the regen surface**

The `entc/integration/*/` directory contains one sub-directory per scenario, each with its own `ent/` output and a `generate.go` tool hook. To regenerate one scenario:

```bash
cd /var/home/smoothbrain/dev/matthewsreis/ent/.worktrees/i-h8-ent
go generate ./entc/integration/<scenario>/...
```

To regenerate everything under the integration module:

```bash
cd /var/home/smoothbrain/dev/matthewsreis/ent/.worktrees/i-h8-ent
go generate ./entc/integration/...
```

Because `entc/integration` is a separate Go module with a `replace entgo.io/ent => ../..`, regeneration picks up template changes in the parent module's `entc/gen/template/` automatically.

To run integration tests:

```bash
cd /var/home/smoothbrain/dev/matthewsreis/ent/.worktrees/i-h8-ent/entc/integration
go test ./...
```

(run from inside `entc/integration/` because it is a separate module — not from the repo root.)

To verify a single scenario quickly:

```bash
cd /var/home/smoothbrain/dev/matthewsreis/ent/.worktrees/i-h8-ent
go generate ./entc/integration/edgeschema/...
(cd entc/integration && go test ./edgeschema/...)
```

The Phase 3 precedent used `edgeschema/` because it exercises every edge kind. Phase 4B must also regenerate and run `hooks/`, `customid/`, `multischema/`, `json/`, and `privacy/` because the template changes touch all CUD categories, each with scenario-specific peculiarities.

- [ ] **Step 2: No commit (reference-only task)**

Proceed to Task 9.

---

### Task 9: Rewrite `builder/setter.tmpl` for 1-line SetX / ClearX / AddX

**Files:**
- Modify: `entc/gen/template/builder/setter.tmpl`

This is the highest-volume emission point: one `SetX` per field × N schemas. Phase 3's precedent (`where.tmpl`) showed that collapsing the 3-line body to a 1-line routed call meaningfully reduces build wall time without blowing up the type graph. Apply the same pattern here. The existing `SetNillableX` and variadic edge helpers (`AddXIDs`) are collapsed via line compression only (no generic) because their shapes resist clean generic instantiation (nillable deref, variadic method value).

- [ ] **Step 1: Read the current template**

Open `entc/gen/template/builder/setter.tmpl`. The current emission for each scalar-field setter (lines 24-35) is:

```gotemplate
{{ range $f := $fields }}
    {{ $func := print "Set" $f.StructField }}
    {{ $ft := $f.Type.String }}{{ if and $.Scope.InSubPackage $f.IsEnum }}{{ $ft = trimPackage $f.Type.String $.Package }}{{ end }}
    // {{ $func }} sets the "{{ $f.Name }}" field.
    func ({{ $receiver }} *{{ $builder }}) {{ $func }}(v {{ $ft }}) *{{ $builder }} {
        {{- /* setting numeric type override previous calls to Add. */}}
        {{- if and $updater $f.SupportsMutationAdd }}
            {{ $receiver }}.mutation.{{ $f.MutationReset }}()
        {{- end }}
        {{ $receiver }}.mutation.{{ $f.MutationSet }}(v)
        return {{ $receiver }}
    }
    ...
```

Goal shape (SQL storage, non-Add-supporting scalar field):

```go
// SetName sets the "name" field.
func (uc *UserCreate) SetName(v string) *UserCreate { return entbuilder.BSet(uc, uc.mutation.SetName, v) }
```

Updater with `SupportsMutationAdd` keeps its Reset call; collapse to single line via inline composition (no new helper):

```go
// SetCount sets the "count" field.
func (uu *UserUpdate) SetCount(v int) *UserUpdate { uu.mutation.ResetCount(); uu.mutation.SetCount(v); return uu }
```

- [ ] **Step 2: Replace the setter block**

In `entc/gen/template/builder/setter.tmpl`, replace the `SetX` emission block (lines 24-35) with the following. Keep the surrounding `range $f := $fields` intact; modify only the body of the generated function:

```gotemplate
{{ $isSQL := eq $.Storage.Name "sql" }}

{{ range $f := $fields }}
    {{ $func := print "Set" $f.StructField }}
    {{ $ft := $f.Type.String }}{{ if and $.Scope.InSubPackage $f.IsEnum }}{{ $ft = trimPackage $f.Type.String $.Package }}{{ end }}
    // {{ $func }} sets the "{{ $f.Name }}" field.
    {{- if and $updater $f.SupportsMutationAdd }}
        {{- /* Numeric update setter: Reset then Set (two mutation calls).
               Generic wrapper doesn't cleanly express two calls; emit a
               compact 1-line multi-statement body. */}}
        func ({{ $receiver }} *{{ $builder }}) {{ $func }}(v {{ $ft }}) *{{ $builder }} { {{ $receiver }}.mutation.{{ $f.MutationReset }}(); {{ $receiver }}.mutation.{{ $f.MutationSet }}(v); return {{ $receiver }} }
    {{- else if $isSQL }}
        {{- /* SQL simple scalar setter: route through entbuilder.BSet. */}}
        func ({{ $receiver }} *{{ $builder }}) {{ $func }}(v {{ $ft }}) *{{ $builder }} { return entbuilder.BSet({{ $receiver }}, {{ $receiver }}.mutation.{{ $f.MutationSet }}, v) }
    {{- else }}
        {{- /* Non-SQL (e.g. gremlin) fallback: preserve original multi-line body. */}}
        func ({{ $receiver }} *{{ $builder }}) {{ $func }}(v {{ $ft }}) *{{ $builder }} {
            {{ $receiver }}.mutation.{{ $f.MutationSet }}(v)
            return {{ $receiver }}
        }
    {{- end }}
```

- [ ] **Step 3: Collapse `SetNillableX`**

In `setter.tmpl`, the current `SetNillableX` block (around lines 42-49) is:

```gotemplate
// {{ $nillableFunc }} sets the "{{ $f.Name }}" field if the given value is not nil.
func ({{ $receiver }} *{{ $builder }}) {{ $nillableFunc }}(v *{{ $ft }}) *{{ $builder }} {
    if v != nil {
        {{ $receiver }}.{{ $func }}(*v)
    }
    return {{ $receiver }}
}
```

Replace with a single-line emission (applies to both SQL and non-SQL — the body has no sql/gremlin split):

```gotemplate
// {{ $nillableFunc }} sets the "{{ $f.Name }}" field if the given value is not nil.
func ({{ $receiver }} *{{ $builder }}) {{ $nillableFunc }}(v *{{ $ft }}) *{{ $builder }} { if v != nil { {{ $receiver }}.{{ $func }}(*v) }; return {{ $receiver }} }
```

- [ ] **Step 4: Collapse `AddX`**

Current `AddX` block (around lines 52-57):

```gotemplate
// {{ $f.MutationAdd }} adds value to the "{{ $f.Name }}" field.
func ({{ $receiver }} *{{ $builder }}) {{ $f.MutationAdd }}(v {{ $f.SignedType }}) *{{ $builder }} {
    {{ $receiver }}.mutation.{{ $f.MutationAdd }}(v)
    return {{ $receiver }}
}
```

Replace with (SQL → route through BSet; non-SQL → collapse):

```gotemplate
// {{ $f.MutationAdd }} adds value to the "{{ $f.Name }}" field.
{{- if $isSQL }}
    func ({{ $receiver }} *{{ $builder }}) {{ $f.MutationAdd }}(v {{ $f.SignedType }}) *{{ $builder }} { return entbuilder.BSet({{ $receiver }}, {{ $receiver }}.mutation.{{ $f.MutationAdd }}, v) }
{{- else }}
    func ({{ $receiver }} *{{ $builder }}) {{ $f.MutationAdd }}(v {{ $f.SignedType }}) *{{ $builder }} { {{ $receiver }}.mutation.{{ $f.MutationAdd }}(v); return {{ $receiver }} }
{{- end }}
```

- [ ] **Step 5: Collapse `AppendX`**

Current `AppendX` block (around lines 60-65):

```gotemplate
// {{ $f.MutationAppend }} appends value to the "{{ $f.Name }}" field.
func ({{ $receiver }} *{{ $builder }}) {{ $f.MutationAppend }}(v {{ $f.Type }}) *{{ $builder }} {
    {{ $receiver }}.mutation.{{ $f.MutationAppend }}(v)
    return {{ $receiver }}
}
```

Replace with:

```gotemplate
// {{ $f.MutationAppend }} appends value to the "{{ $f.Name }}" field.
{{- if $isSQL }}
    func ({{ $receiver }} *{{ $builder }}) {{ $f.MutationAppend }}(v {{ $f.Type }}) *{{ $builder }} { return entbuilder.BSet({{ $receiver }}, {{ $receiver }}.mutation.{{ $f.MutationAppend }}, v) }
{{- else }}
    func ({{ $receiver }} *{{ $builder }}) {{ $f.MutationAppend }}(v {{ $f.Type }}) *{{ $builder }} { {{ $receiver }}.mutation.{{ $f.MutationAppend }}(v); return {{ $receiver }} }
{{- end }}
```

- [ ] **Step 6: Collapse `ClearX`**

Current `ClearX` block (around lines 69-74):

```gotemplate
// {{ $func }} clears the value of the "{{ $f.Name }}" field.
func ({{ $receiver }} *{{ $builder }}) {{ $func }}() *{{ $builder }} {
    {{ $receiver }}.mutation.{{ $func }}()
    return {{ $receiver }}
}
```

Replace with:

```gotemplate
// {{ $func }} clears the value of the "{{ $f.Name }}" field.
{{- if $isSQL }}
    func ({{ $receiver }} *{{ $builder }}) {{ $func }}() *{{ $builder }} { return entbuilder.BClear({{ $receiver }}, {{ $receiver }}.mutation.{{ $func }}) }
{{- else }}
    func ({{ $receiver }} *{{ $builder }}) {{ $func }}() *{{ $builder }} { {{ $receiver }}.mutation.{{ $func }}(); return {{ $receiver }} }
{{- end }}
```

- [ ] **Step 7: Collapse edge `AddXIDs` / `SetXID` / `SetNillableXID`**

Current edge-add block (around lines 87-91) is:

```gotemplate
// {{ $idsFunc }} {{ $op }}s the "{{ $e.Name }}" edge to the {{ $e.Type.Name }} entity by ID{{ if not $e.Unique }}s{{ end }}.
func ({{ $receiver }} *{{ $builder }}) {{ $idsFunc }}({{ if $e.Unique }}id{{ else }}ids ...{{ end }} {{ $e.Type.ID.Type }}) *{{ $builder }} {
    {{ $receiver }}.mutation.{{ $idsFunc }}({{ if $e.Unique }}id{{ else }}ids ...{{ end }})
    return {{ $receiver }}
}
```

Replace with (single-line collapse — variadic and unique paths both fit on one line; do not use BSet here because variadic method-value inference trips the compiler):

```gotemplate
// {{ $idsFunc }} {{ $op }}s the "{{ $e.Name }}" edge to the {{ $e.Type.Name }} entity by ID{{ if not $e.Unique }}s{{ end }}.
{{- if $e.Unique }}
    {{- if $isSQL }}
        func ({{ $receiver }} *{{ $builder }}) {{ $idsFunc }}(id {{ $e.Type.ID.Type }}) *{{ $builder }} { return entbuilder.BSet({{ $receiver }}, {{ $receiver }}.mutation.{{ $idsFunc }}, id) }
    {{- else }}
        func ({{ $receiver }} *{{ $builder }}) {{ $idsFunc }}(id {{ $e.Type.ID.Type }}) *{{ $builder }} { {{ $receiver }}.mutation.{{ $idsFunc }}(id); return {{ $receiver }} }
    {{- end }}
{{- else }}
    func ({{ $receiver }} *{{ $builder }}) {{ $idsFunc }}(ids ...{{ $e.Type.ID.Type }}) *{{ $builder }} { {{ $receiver }}.mutation.{{ $idsFunc }}(ids...); return {{ $receiver }} }
{{- end }}
```

Current `SetNillableXID` block (around lines 94-101) is:

```gotemplate
// {{ $nillableIDsFunc }} sets the "{{ $e.Name }}" edge to the {{ $e.Type.Name }} entity by ID if the given value is not nil.
func ({{ $receiver }} *{{ $builder }}) {{ $nillableIDsFunc }}(id *{{ $e.Type.ID.Type }}) *{{ $builder }} {
    if id != nil {
        {{ $receiver}} = {{ $receiver }}.{{ $idsFunc }}(*id)
    }
    return {{ $receiver }}
}
```

Replace with single-line collapse (keep the receiver-reassignment because `{{ $idsFunc }}` returns `*Builder` and is conventionally chained):

```gotemplate
// {{ $nillableIDsFunc }} sets the "{{ $e.Name }}" edge to the {{ $e.Type.Name }} entity by ID if the given value is not nil.
func ({{ $receiver }} *{{ $builder }}) {{ $nillableIDsFunc }}(id *{{ $e.Type.ID.Type }}) *{{ $builder }} { if id != nil { {{ $receiver }} = {{ $receiver }}.{{ $idsFunc }}(*id) }; return {{ $receiver }} }
```

- [ ] **Step 8: Collapse the trailing `Mutation()` getter**

At line 106-108:

```gotemplate
// Mutation returns the {{ $.MutationName }} object of the builder.
func ({{ $receiver }} *{{ $builder }}) Mutation() *{{ $.MutationName }} {
    return {{ $receiver }}.mutation
}
```

Replace with:

```gotemplate
// Mutation returns the {{ $.MutationName }} object of the builder.
func ({{ $receiver }} *{{ $builder }}) Mutation() *{{ $.MutationName }} { return {{ $receiver }}.mutation }
```

(No `$isSQL` gate — one-line getter works for all storages.)

- [ ] **Step 9: Verify the template parses and produces compilable Go**

At this point the template change still has no consumer — create.tmpl and update.tmpl haven't yet been updated to set the `CreateUsesEntbuilder`/`UpdateUsesEntbuilder` scope flags. But regeneration of the `edgeschema` scenario will exercise this template via the still-existing scope flags from Phase 3's work.

Run: `go build ./entc/gen/...`
Expected: zero errors.

- [ ] **Step 10: Do not commit yet**

The next task regenerates `entc/integration/*` and validates. If the template has a bug, the regen will fail or the tests will fail, and we'll fix the template before committing. Keep the working change in the index.

---

### Task 10: Regenerate + validate after `setter.tmpl` rewrite

**Files:**
- Modify: many files under `entc/integration/*/ent/**`

- [ ] **Step 1: Regenerate the scenarios that exercise the setter template**

Run:

```bash
cd /var/home/smoothbrain/dev/matthewsreis/ent/.worktrees/i-h8-ent
go generate ./entc/integration/edgeschema/...
go generate ./entc/integration/hooks/...
go generate ./entc/integration/customid/...
go generate ./entc/integration/json/...
go generate ./entc/integration/multischema/...
```

Expected: all five regens complete without errors. Inspect one representative file to confirm 1-line bodies:

```bash
grep -A1 'func (\w\+ \*\w\+Create) Set\w\+' entc/integration/edgeschema/ent/tweet_create.go | head -20
```

Expected: most non-numeric setters are on a single line containing `entbuilder.BSet(...)`.

- [ ] **Step 2: Build the integration module**

Run:

```bash
cd /var/home/smoothbrain/dev/matthewsreis/ent/.worktrees/i-h8-ent/entc/integration
go build ./...
```

Expected: zero errors. If you see "undefined: entbuilder", the `CreateUsesEntbuilder` / `UpdateUsesEntbuilder` scope flags are not yet wired in create.tmpl / update.tmpl. That's expected — Task 11 / 13 handle it. The fix for *this* task is: because setter.tmpl is a shared sub-template invoked from both create.tmpl and update.tmpl, it cannot rely on the parent template's scope flag until Tasks 11 and 13 run. Temporarily, the non-SQL fallback path (no `entbuilder.*` references) will be emitted for anything where the parent hasn't set the flag. Re-run Step 1 to confirm. Alternatively, if this is a blocker, reorder: run Tasks 11 and 13 first, then come back to Task 10.

(Implementation note: because `setter.tmpl` is invoked via `{{ template "setter" . }}` with the outer scope extended, the `$isSQL` computation inside setter.tmpl uses `$.Storage`, which is always defined, so the SQL routing path will emit `entbuilder.*` for SQL storage regardless of create/update scope. The `entbuilder` import is then supplied by the `Create/UpdateUsesEntbuilder` flags set in Tasks 11 / 13. Until those flags are in place, regen compile of generated SQL-storage scenarios will fail with missing imports. Proceed to Tasks 11 and 13 to resolve.)

- [ ] **Step 3: Proceed directly to Task 11**

Do not commit or revert yet; the integration build won't be green until Tasks 11-15 are done. Task 16 is the integrated commit point for setter.tmpl + create.tmpl + update.tmpl + delete.tmpl + query.tmpl + the regenerated diffs.

---

### Task 11: Rewrite `builder/create.tmpl` — propagate `CreateUsesEntbuilder` + collapse Save/SaveX/Exec/ExecX

**Files:**
- Modify: `entc/gen/template/builder/create.tmpl`

- [ ] **Step 1: Set the scope flag on the import invocation**

In `entc/gen/template/builder/create.tmpl`, the import block is at lines 17-19:

```gotemplate
{{ with extend $ "Imports" $.BuilderImports }}
    {{ template "import" . }}
{{ end }}
```

Replace with:

```gotemplate
{{ with extend $ "Imports" $.BuilderImports "CreateUsesEntbuilder" true }}
    {{ template "import" . }}
{{ end }}
```

This adds the `"entgo.io/ent/runtime/entbuilder"` import to the generated `xxx_create.go` file whenever the storage is SQL (the import itself is emitted only when `CreateUsesEntbuilder` is true AND the generated code actually uses an `entbuilder.*` symbol; the import is a no-op if no symbol is used).

- [ ] **Step 2: Collapse `SaveX`**

Current `SaveX` (lines 61-67):

```gotemplate
// SaveX calls Save and panics if Save returns an error.
func ({{ $receiver }} *{{ $builder }}) SaveX(ctx context.Context) *{{ $.Name }} {
    v, err := {{ $receiver }}.Save(ctx)
    if err != nil {
        panic(err)
    }
    return v
}
```

Replace with:

```gotemplate
// SaveX calls Save and panics if Save returns an error.
func ({{ $receiver }} *{{ $builder }}) SaveX(ctx context.Context) *{{ $.Name }} { return entbuilder.Must({{ $receiver }}.Save(ctx)) }
```

- [ ] **Step 3: Collapse `Exec`**

Current `Exec` (lines 69-72):

```gotemplate
// Exec executes the query.
func ({{ $receiver }} *{{ $builder }}) Exec(ctx context.Context) error {
    _, err := {{ $receiver }}.Save(ctx)
    return err
}
```

Replace with:

```gotemplate
// Exec executes the query.
func ({{ $receiver }} *{{ $builder }}) Exec(ctx context.Context) error { _, err := {{ $receiver }}.Save(ctx); return err }
```

(No generic is a good fit here — the return is `error`, not a value; just collapse.)

- [ ] **Step 4: Collapse `ExecX`**

Current `ExecX` (lines 75-79):

```gotemplate
// ExecX is like Exec, but panics if an error occurs.
func ({{ $receiver }} *{{ $builder }}) ExecX(ctx context.Context) {
    if err := {{ $receiver }}.Exec(ctx); err != nil {
        panic(err)
    }
}
```

Replace with:

```gotemplate
// ExecX is like Exec, but panics if an error occurs.
func ({{ $receiver }} *{{ $builder }}) ExecX(ctx context.Context) { if err := {{ $receiver }}.Exec(ctx); err != nil { panic(err) } }
```

- [ ] **Step 5: Leave `defaults()` and `check()` multi-line**

The bodies have conditional logic per field (default-func check, validator branches, enum checks). Collapsing them hurts readability without a meaningful LOC win. Preserve current emission.

- [ ] **Step 6: Do not commit yet**

Same rationale as Task 10 — all builder templates should land together in Task 16.

- [ ] **Step 7: Do not regen yet**

We'll regen and test after all four builder templates + import plumbing are updated.

---

### Task 12: Verify create.tmpl parses

**Files:**
- None (build-check only)

- [ ] **Step 1: Build `entc/gen`**

Run: `go build ./entc/gen/...`
Expected: zero errors.

- [ ] **Step 2: No commit**

---

### Task 13: Rewrite `builder/update.tmpl` — propagate flag + collapse Save/SaveX/Exec/ExecX + update/edges

**Files:**
- Modify: `entc/gen/template/builder/update.tmpl`

- [ ] **Step 1: Set the scope flag on the import invocation**

In `entc/gen/template/builder/update.tmpl`, the import block is at lines 16-18:

```gotemplate
{{ with extend $ "Imports" $.BuilderImports }}
    {{ template "import" . }}
{{ end }}
```

Replace with:

```gotemplate
{{ with extend $ "Imports" $.BuilderImports "UpdateUsesEntbuilder" true }}
    {{ template "import" . }}
{{ end }}
```

- [ ] **Step 2: Collapse `SaveX` (both `Update` and `UpdateOne` builders)**

`UpdateName` builder (lines 65-71):

```gotemplate
// SaveX is like Save, but panics if an error occurs.
func ({{ $receiver }} *{{ $builder }}) SaveX(ctx context.Context) int {
    affected, err := {{ $receiver }}.Save(ctx)
    if err != nil {
        panic(err)
    }
    return affected
}
```

Replace with:

```gotemplate
// SaveX is like Save, but panics if an error occurs.
func ({{ $receiver }} *{{ $builder }}) SaveX(ctx context.Context) int { return entbuilder.Must({{ $receiver }}.Save(ctx)) }
```

`UpdateOne` builder (lines 148-154):

```gotemplate
// SaveX is like Save, but panics if an error occurs.
func ({{ $receiver }} *{{ $onebuilder }}) SaveX(ctx context.Context) *{{ $.Name }} {
    node, err := {{ $receiver }}.Save(ctx)
    if err != nil {
        panic(err)
    }
    return node
}
```

Replace with:

```gotemplate
// SaveX is like Save, but panics if an error occurs.
func ({{ $receiver }} *{{ $onebuilder }}) SaveX(ctx context.Context) *{{ $.Name }} { return entbuilder.Must({{ $receiver }}.Save(ctx)) }
```

- [ ] **Step 3: Collapse `Exec` and `ExecX` in both builders**

`Update` builder Exec (lines 74-77):

```gotemplate
// Exec executes the query.
func ({{ $receiver }} *{{ $builder }}) Exec(ctx context.Context) error {
    _, err := {{ $receiver }}.Save(ctx)
    return err
}
```

Replace with:

```gotemplate
// Exec executes the query.
func ({{ $receiver }} *{{ $builder }}) Exec(ctx context.Context) error { _, err := {{ $receiver }}.Save(ctx); return err }
```

`Update` builder ExecX (lines 80-84):

```gotemplate
// ExecX is like Exec, but panics if an error occurs.
func ({{ $receiver }} *{{ $builder }}) ExecX(ctx context.Context) {
    if err := {{ $receiver }}.Exec(ctx); err != nil {
        panic(err)
    }
}
```

Replace with:

```gotemplate
// ExecX is like Exec, but panics if an error occurs.
func ({{ $receiver }} *{{ $builder }}) ExecX(ctx context.Context) { if err := {{ $receiver }}.Exec(ctx); err != nil { panic(err) } }
```

Apply the same two replacements in the `UpdateOne` builder (lines 157-167).

- [ ] **Step 4: Collapse `Where` in both builders**

`Update` builder Where (lines 37-40):

```gotemplate
// Where appends a list predicates to the {{ $builder }} builder.
func ({{ $receiver}} *{{ $builder }}) Where(ps ...predicate.{{ $.Name }}) *{{ $builder }} {
    {{ $mutation }}.Where(ps...)
    return {{ $receiver }}
}
```

Replace with:

```gotemplate
// Where appends a list predicates to the {{ $builder }} builder.
func ({{ $receiver}} *{{ $builder }}) Where(ps ...predicate.{{ $.Name }}) *{{ $builder }} { {{ $mutation }}.Where(ps...); return {{ $receiver }} }
```

`UpdateOne` builder Where (lines 121-124):

```gotemplate
// Where appends a list predicates to the {{ $builder }} builder.
func ({{ $receiver }} *{{ $onebuilder }}) Where(ps ...predicate.{{ $.Name }}) *{{ $onebuilder }} {
    {{ $mutation }}.Where(ps...)
    return {{ $receiver }}
}
```

Replace with:

```gotemplate
// Where appends a list predicates to the {{ $builder }} builder.
func ({{ $receiver }} *{{ $onebuilder }}) Where(ps ...predicate.{{ $.Name }}) *{{ $onebuilder }} { {{ $mutation }}.Where(ps...); return {{ $receiver }} }
```

- [ ] **Step 5: Collapse `Select` on UpdateOne**

Lines 128-131:

```gotemplate
// Select allows selecting one or more fields (columns) of the returned entity.
// The default is selecting all fields defined in the entity schema.
func ({{ $receiver }} *{{ $onebuilder }}) Select(field string, fields ...string) *{{ $onebuilder }} {
    {{ $receiver }}.fields = append([]string{field}, fields...)
    return {{ $receiver }}
}
```

Replace with:

```gotemplate
// Select allows selecting one or more fields (columns) of the returned entity.
// The default is selecting all fields defined in the entity schema.
func ({{ $receiver }} *{{ $onebuilder }}) Select(field string, fields ...string) *{{ $onebuilder }} { {{ $receiver }}.fields = append([]string{field}, fields...); return {{ $receiver }} }
```

- [ ] **Step 6: Collapse `update/edges` Clear + RemoveXIDs**

Inside the `define "update/edges"` block (lines 199-223), current emission is:

```gotemplate
{{ range $e := $.EdgesWithID }}
    {{ if $e.Immutable }}
        {{continue}}
    {{ end }}
    {{ $func := $e.MutationClear }}
    // {{ $func }} clears {{ if $e.Unique }}the "{{ $e.Name }}" edge{{ else }}all "{{ $e.Name }}" edges{{ end }} to the {{ $e.Type.Name }} entity.
    func ({{ $receiver }} *{{ $builder }}) {{ $func }}() *{{ $builder }} {
        {{ $mutation }}.{{ $func }}()
        return {{ $receiver }}
    }
    {{ if not $e.Unique }}
        {{ $idsFunc := print "Remove" (singular $e.Name | pascal) "IDs" }}
        // {{ $idsFunc }} removes the "{{ $e.Name }}" edge to {{ $e.Type.Name }} entities by IDs.
        func ({{ $receiver }} *{{ $builder }}) {{ $idsFunc }}(ids ...{{ $e.Type.ID.Type }}) *{{ $builder }} {
            {{ $mutation }}.{{ $idsFunc }}(ids...)
            return {{ $receiver }}
        }
    {{ end }}
{{ end }}
```

Replace with (introduce an `$isSQL` gate at the top of the define block, then route Clear through `BClear` on SQL; collapse RemoveXIDs variadic inline):

```gotemplate
{{ $isSQL := eq $.Storage.Name "sql" }}
{{ range $e := $.EdgesWithID }}
    {{ if $e.Immutable }}
        {{continue}}
    {{ end }}
    {{ $func := $e.MutationClear }}
    // {{ $func }} clears {{ if $e.Unique }}the "{{ $e.Name }}" edge{{ else }}all "{{ $e.Name }}" edges{{ end }} to the {{ $e.Type.Name }} entity.
    {{- if $isSQL }}
        func ({{ $receiver }} *{{ $builder }}) {{ $func }}() *{{ $builder }} { return entbuilder.BClear({{ $receiver }}, {{ $mutation }}.{{ $func }}) }
    {{- else }}
        func ({{ $receiver }} *{{ $builder }}) {{ $func }}() *{{ $builder }} { {{ $mutation }}.{{ $func }}(); return {{ $receiver }} }
    {{- end }}
    {{ if not $e.Unique }}
        {{ $idsFunc := print "Remove" (singular $e.Name | pascal) "IDs" }}
        // {{ $idsFunc }} removes the "{{ $e.Name }}" edge to {{ $e.Type.Name }} entities by IDs.
        func ({{ $receiver }} *{{ $builder }}) {{ $idsFunc }}(ids ...{{ $e.Type.ID.Type }}) *{{ $builder }} { {{ $mutation }}.{{ $idsFunc }}(ids...); return {{ $receiver }} }
    {{ end }}
{{ end }}
```

- [ ] **Step 7: Leave `defaults()` and `check()` multi-line**

Same rationale as create.tmpl: conditional logic per field, no clean generic fit, minimal volume.

- [ ] **Step 8: Verify template parses**

Run: `go build ./entc/gen/...`
Expected: zero errors.

- [ ] **Step 9: Do not commit yet**

---

### Task 14: Rewrite `builder/delete.tmpl` — propagate flag + collapse ExecX, DeleteOne.ExecX, Where

**Files:**
- Modify: `entc/gen/template/builder/delete.tmpl`

- [ ] **Step 1: Set the scope flag on the import invocation**

In `entc/gen/template/builder/delete.tmpl` line 16:

```gotemplate
{{ template "import" $ }}
```

Replace with:

```gotemplate
{{ with extend $ "DeleteUsesEntbuilder" true }}
    {{ template "import" . }}
{{ end }}
```

- [ ] **Step 2: Collapse `Where`**

Lines 35-38:

```gotemplate
// Where appends a list predicates to the {{ $builder }} builder.
func ({{ $receiver }} *{{ $builder }}) Where(ps ...predicate.{{ $.Name }}) *{{ $builder }} {
    {{ $mutation }}.Where(ps...)
    return {{ $receiver }}
}
```

Replace with:

```gotemplate
// Where appends a list predicates to the {{ $builder }} builder.
func ({{ $receiver }} *{{ $builder }}) Where(ps ...predicate.{{ $.Name }}) *{{ $builder }} { {{ $mutation }}.Where(ps...); return {{ $receiver }} }
```

- [ ] **Step 3: Collapse `ExecX` (delete builder)**

Lines 46-52:

```gotemplate
// ExecX is like Exec, but panics if an error occurs.
func ({{ $receiver }} *{{ $builder }}) ExecX(ctx context.Context) int {
    n, err := {{ $receiver }}.Exec(ctx)
    if err != nil {
        panic(err)
    }
    return n
}
```

Replace with:

```gotemplate
// ExecX is like Exec, but panics if an error occurs.
func ({{ $receiver }} *{{ $builder }}) ExecX(ctx context.Context) int { return entbuilder.Must({{ $receiver }}.Exec(ctx)) }
```

- [ ] **Step 4: Collapse `DeleteOne` helpers**

Lines 79-83 (Where on DeleteOne):

```gotemplate
// Where appends a list predicates to the {{ $builder }} builder.
func ({{ $oneReceiver }} *{{ $onebuilder }}) Where(ps ...predicate.{{ $.Name }}) *{{ $onebuilder }} {
    {{ $oneReceiver }}.{{ $mutation }}.Where(ps...)
    return {{ $oneReceiver }}
}
```

Replace with:

```gotemplate
// Where appends a list predicates to the {{ $builder }} builder.
func ({{ $oneReceiver }} *{{ $onebuilder }}) Where(ps ...predicate.{{ $.Name }}) *{{ $onebuilder }} { {{ $oneReceiver }}.{{ $mutation }}.Where(ps...); return {{ $oneReceiver }} }
```

`DeleteOne.Exec` (lines 86-96) has NotFound-classification logic and is not cleanly single-line collapsible. Leave multi-line.

`DeleteOne.ExecX` (lines 99-103):

```gotemplate
// ExecX is like Exec, but panics if an error occurs.
func ({{ $oneReceiver }} *{{ $onebuilder }}) ExecX(ctx context.Context) {
    if err := {{ $oneReceiver }}.Exec(ctx); err != nil {
        panic(err)
    }
}
```

Replace with:

```gotemplate
// ExecX is like Exec, but panics if an error occurs.
func ({{ $oneReceiver }} *{{ $onebuilder }}) ExecX(ctx context.Context) { if err := {{ $oneReceiver }}.Exec(ctx); err != nil { panic(err) } }
```

- [ ] **Step 5: Verify template parses**

Run: `go build ./entc/gen/...`
Expected: zero errors.

- [ ] **Step 6: Do not commit yet**

---

### Task 15: Rewrite `builder/query.tmpl` — propagate flag + collapse X-wrappers

**Files:**
- Modify: `entc/gen/template/builder/query.tmpl`

- [ ] **Step 1: Set the scope flag on the import invocation**

In `entc/gen/template/builder/query.tmpl` lines 14-16:

```gotemplate
{{ with extend $ "Imports" $.SiblingImports }}
    {{ template "import" . }}
{{ end }}
```

Replace with:

```gotemplate
{{ with extend $ "Imports" $.SiblingImports "QueryUsesEntbuilder" true }}
    {{ template "import" . }}
{{ end }}
```

- [ ] **Step 2: Collapse `FirstX`**

Lines 107-113:

```gotemplate
// FirstX is like First, but panics if an error occurs.
func ({{ $receiver }} *{{ $builder }}) FirstX(ctx context.Context) *{{ $.Name }} {
    node, err := {{ $receiver }}.First(ctx)
    if err != nil && !IsNotFound(err) {
        panic(err)
    }
    return node
}
```

This has NotFound-tolerant semantics that Must doesn't cover. Collapse inline (no generic):

```gotemplate
// FirstX is like First, but panics if an error occurs.
func ({{ $receiver }} *{{ $builder }}) FirstX(ctx context.Context) *{{ $.Name }} { node, err := {{ $receiver }}.First(ctx); if err != nil && !IsNotFound(err) { panic(err) }; return node }
```

- [ ] **Step 3: Collapse `FirstIDX`**

Lines 131-137:

```gotemplate
// FirstIDX is like FirstID, but panics if an error occurs.
func ({{ $receiver }} *{{ $builder }}) FirstIDX(ctx context.Context) {{ $.ID.Type }} {
    id, err := {{ $receiver }}.FirstID(ctx)
    if err != nil && !IsNotFound(err) {
        panic(err)
    }
    return id
}
```

Replace with:

```gotemplate
// FirstIDX is like FirstID, but panics if an error occurs.
func ({{ $receiver }} *{{ $builder }}) FirstIDX(ctx context.Context) {{ $.ID.Type }} { id, err := {{ $receiver }}.FirstID(ctx); if err != nil && !IsNotFound(err) { panic(err) }; return id }
```

- [ ] **Step 4: Collapse `OnlyX`**

Lines 159-165:

```gotemplate
// OnlyX is like Only, but panics if an error occurs.
func ({{ $receiver }} *{{ $builder }}) OnlyX(ctx context.Context) *{{ $.Name }} {
    node, err := {{ $receiver }}.Only(ctx)
    if err != nil {
        panic(err)
    }
    return node
}
```

Replace with:

```gotemplate
// OnlyX is like Only, but panics if an error occurs.
func ({{ $receiver }} *{{ $builder }}) OnlyX(ctx context.Context) *{{ $.Name }} { return entbuilder.Must({{ $receiver }}.Only(ctx)) }
```

- [ ] **Step 5: Collapse `OnlyIDX`**

Lines 188-194:

```gotemplate
// OnlyIDX is like OnlyID, but panics if an error occurs.
func ({{ $receiver }} *{{ $builder }}) OnlyIDX(ctx context.Context) {{ $.ID.Type }} {
    id, err := {{ $receiver }}.OnlyID(ctx)
    if err != nil {
        panic(err)
    }
    return id
}
```

Replace with:

```gotemplate
// OnlyIDX is like OnlyID, but panics if an error occurs.
func ({{ $receiver }} *{{ $builder }}) OnlyIDX(ctx context.Context) {{ $.ID.Type }} { return entbuilder.Must({{ $receiver }}.OnlyID(ctx)) }
```

- [ ] **Step 6: Collapse `AllX`**

Lines 208-214:

```gotemplate
// AllX is like All, but panics if an error occurs.
func ({{ $receiver }} *{{ $builder }}) AllX(ctx context.Context) []*{{ $.Name }} {
    nodes, err := {{ $receiver }}.All(ctx)
    if err != nil {
        panic(err)
    }
    return nodes
}
```

Replace with:

```gotemplate
// AllX is like All, but panics if an error occurs.
func ({{ $receiver }} *{{ $builder }}) AllX(ctx context.Context) []*{{ $.Name }} { return entbuilder.Must({{ $receiver }}.All(ctx)) }
```

- [ ] **Step 7: Collapse `IDsX`**

Lines 231-237:

```gotemplate
// IDsX is like IDs, but panics if an error occurs.
func ({{ $receiver }} *{{ $builder }}) IDsX(ctx context.Context) []{{ $.ID.Type }} {
    ids, err := {{ $receiver }}.IDs(ctx)
    if err != nil {
        panic(err)
    }
    return ids
}
```

Replace with:

```gotemplate
// IDsX is like IDs, but panics if an error occurs.
func ({{ $receiver }} *{{ $builder }}) IDsX(ctx context.Context) []{{ $.ID.Type }} { return entbuilder.Must({{ $receiver }}.IDs(ctx)) }
```

- [ ] **Step 8: Collapse `CountX`**

Lines 250-256:

```gotemplate
// CountX is like Count, but panics if an error occurs.
func ({{ $receiver }} *{{ $builder }}) CountX(ctx context.Context) int {
    count, err := {{ $receiver }}.Count(ctx)
    if err != nil {
        panic(err)
    }
    return count
}
```

Replace with:

```gotemplate
// CountX is like Count, but panics if an error occurs.
func ({{ $receiver }} *{{ $builder }}) CountX(ctx context.Context) int { return entbuilder.Must({{ $receiver }}.Count(ctx)) }
```

- [ ] **Step 9: Collapse `ExistX`**

Lines 272-278:

```gotemplate
// ExistX is like Exist, but panics if an error occurs.
func ({{ $receiver }} *{{ $builder }}) ExistX(ctx context.Context) bool {
    exist, err := {{ $receiver }}.Exist(ctx)
    if err != nil {
        panic(err)
    }
    return exist
}
```

Replace with:

```gotemplate
// ExistX is like Exist, but panics if an error occurs.
func ({{ $receiver }} *{{ $builder }}) ExistX(ctx context.Context) bool { return entbuilder.Must({{ $receiver }}.Exist(ctx)) }
```

- [ ] **Step 10: Collapse `Where`, `Limit`, `Offset`, `Unique`, `Order`**

Current bodies (lines 43-71) are small two-statement setters returning the receiver. Each one has:

```gotemplate
func ({{ $receiver }} *{{ $builder }}) Limit(limit int) *{{ $builder }} {
    {{ $receiver }}.ctx.Limit = &limit
    return {{ $receiver }}
}
```

These don't fit BSet cleanly (there's no method-value to pass; it's a field assignment). Just collapse to single-line:

```gotemplate
func ({{ $receiver }} *{{ $builder }}) Limit(limit int) *{{ $builder }} { {{ $receiver }}.ctx.Limit = &limit; return {{ $receiver }} }
```

Apply the same single-line collapse to Where (line 43), Limit (line 49), Offset (line 55), Unique (line 62), Order (line 68).

- [ ] **Step 11: Leave remaining multi-body methods alone**

`First`, `FirstID`, `Only`, `OnlyID`, `All`, `IDs`, `Count`, `Exist`, `Clone`, `QueryX`, `WithX`, `GroupBy`, `Select`, `Aggregate`, `prepareQuery` — all have bodies with >2 statements of non-trivial logic (interceptor setup, context construction, conditional branches, clone copying). Leave multi-line.

- [ ] **Step 12: Verify template parses**

Run: `go build ./entc/gen/...`
Expected: zero errors.

- [ ] **Step 13: Do not commit yet — proceed to Task 16**

---

### Task 16: Regenerate every integration scenario + run full integration suite

**Files:**
- Modify: many files under `entc/integration/*/ent/**`

- [ ] **Step 1: Regenerate all integration scenarios**

```bash
cd /var/home/smoothbrain/dev/matthewsreis/ent/.worktrees/i-h8-ent
go generate ./entc/integration/...
```

Expected: completes with zero errors. If regen fails due to a template bug, inspect the error:
- "undefined: entbuilder" in a generated file → a builder template isn't setting its scope flag (check Tasks 11, 13, 14, 15 step 1). Fix the template, re-run regen.
- Template-parse errors → fix the template syntax, re-run regen.
- "scope flag not found" → ensure `hasField $ "Scope"` guard is in place in import.tmpl for the new flag; the Phase 3 guard should still apply.

- [ ] **Step 2: Spot-check emitted code**

```bash
grep -c 'entbuilder\.BSet\b' entc/integration/edgeschema/ent/tweet_create.go
grep -c 'entbuilder\.BClear\b' entc/integration/edgeschema/ent/tweet_update.go
grep -c 'entbuilder\.Must\b' entc/integration/edgeschema/ent/tweet_query.go
grep -c 'entbuilder\.FieldEQ\b' entc/integration/edgeschema/ent/tweet/where.go
```

Expected: each count > 0. The last (`FieldEQ`) should match the Phase 3 expectation — the `where.go` emission is unchanged.

- [ ] **Step 3: Build the integration module**

```bash
cd /var/home/smoothbrain/dev/matthewsreis/ent/.worktrees/i-h8-ent/entc/integration
go build ./...
```

Expected: zero errors. If you see "imported and not used: entgo.io/ent/runtime/entbuilder" for a gremlin-only scenario, the `$isSQL` gate in one of the templates failed to fall through to the non-SQL multi-line path. Inspect and fix.

- [ ] **Step 4: Run the full integration test suite**

```bash
cd /var/home/smoothbrain/dev/matthewsreis/ent/.worktrees/i-h8-ent/entc/integration
go test ./... -count=1
```

Expected: all scenarios PASS. This is the critical correctness gate — the rewrites must produce behaviorally identical code. Total runtime: ~2-5 minutes across all scenarios.

- [ ] **Step 5: Run the SQL snapshot regression harness**

```bash
cd /var/home/smoothbrain/dev/matthewsreis/ent/.worktrees/i-h8-ent
go test ./entc/gen/ -run TestSnapshot -v
```

Expected: all snapshot tests PASS. If any fail with a diff, inspect it:
- Pure whitespace diff → template change introduced tab/space differences; update snapshots with `-update-sql-snapshots` only if it's cosmetic.
- Semantic SQL diff → a real behavior change; fix the template.

- [ ] **Step 6: Commit the combined template + regen work**

```bash
git add entc/gen/template/builder/setter.tmpl entc/gen/template/builder/create.tmpl entc/gen/template/builder/update.tmpl entc/gen/template/builder/delete.tmpl entc/gen/template/builder/query.tmpl entc/integration/
```

```bash
git commit -m "gen: compact create/update/delete/query builder templates through entbuilder

Route scalar SetX/AddX/AppendX/ClearX and the error-panic X-wrappers through
entbuilder.BSet / BClear / Must when the storage is SQL. Preserve multi-line
emission for defaults(), check(), hook-setup, interceptor logic, and non-SQL
(gremlin) fallbacks.

Phase 3 established the pattern in where.tmpl; this lands it across the
remaining scalar-body builder templates. entc/integration/* regenerated
and green on go test ./... from entc/integration/. SQL snapshots pass.
Descriptor-driven rewrites for mutation/client/entql land in Phase 4C."
```

---

### Task 17: Regression-check the non-SQL (gremlin) path

**Files:**
- Inspect only.

- [ ] **Step 1: Verify gremlin scenarios stayed on the multi-line emission path**

```bash
grep -c 'entbuilder\.' entc/integration/gremlin/ 2>/dev/null || echo "0"
```

Expected: either no such directory (the gremlin integration tests live under a different path) or zero matches. To be sure, check any storage-gremlin output file directly:

```bash
find entc/integration -path '*gremlin*' -name '*_create.go' -o -path '*gremlin*' -name '*_update.go' | head -3 | xargs grep -l 'entbuilder\.' 2>/dev/null || echo "no gremlin files reference entbuilder (expected)"
```

Expected: the echo fallback fires — no gremlin files reference entbuilder.

- [ ] **Step 2: No commit (sanity check only)**

If any gremlin file references `entbuilder`, the `$isSQL` gate is wrong in one of the templates. Fix, regen, re-run Task 16 steps 4-6, then amend or add a commit with the fix.

---

## Phase B-3 — Consumer dry-run measurement

Mirror Phase 3's Tasks 11, 15, 16, 17. The consumer (service-api-go) is regenerated under a local replace, measured, then restored. No commits land in the consumer.

### Task 18: Capture Phase 4B baseline against current i-h8-ent HEAD

**Files:**
- Create: `docs/superpowers/results/2026-04-24-phase4b-measurements.md` (skeleton)

The "baseline" here is the state with Phase 3's `where.tmpl` change but *before* Phase 4B's builder template changes. We achieve this by measuring against the commit immediately preceding Task 16's commit.

- [ ] **Step 1: Record the pre-Phase-4B SHA**

Run:

```bash
cd /var/home/smoothbrain/dev/matthewsreis/ent/.worktrees/i-h8-ent
# The parent of the current HEAD (the Phase 4B combined commit) = the baseline.
BASELINE_SHA=$(git rev-parse HEAD~1)
CURRENT_SHA=$(git rev-parse HEAD)
echo "Baseline (pre-4B): $BASELINE_SHA"
echo "Current (post-4B):   $CURRENT_SHA"
```

- [ ] **Step 2: Create a scratch worktree at the baseline SHA for consumer regen**

```bash
cd /var/home/smoothbrain/dev/matthewsreis/ent
git worktree add .worktrees/phase4b-baseline $BASELINE_SHA
```

- [ ] **Step 3: Regenerate the consumer against the baseline**

```bash
CONSUMER=/home/smoothbrain/dev/matthewsreis/worktrees/main/service-api-go
BASELINE_FORK=/var/home/smoothbrain/dev/matthewsreis/ent/.worktrees/phase4b-baseline

cd $CONSUMER
cp go.mod go.mod.bak
GOWORK=off go mod edit -replace entgo.io/ent=$BASELINE_FORK
GOWORK=off go mod tidy
GOWORK=off go generate ./...
```

Expected: regeneration succeeds. `go.mod` shows `replace entgo.io/ent => /var/home/smoothbrain/dev/matthewsreis/ent/.worktrees/phase4b-baseline`.

- [ ] **Step 4: Capture baseline metrics**

```bash
# LOC totals
find $CONSUMER/api-graphql/src/ent/gen -name '*.go' -exec cat {} + | wc -l > /tmp/phase4b-baseline-loc.txt

# Per-category LOC
for cat in where create update delete query; do
  find $CONSUMER/api-graphql/src/ent/gen -type f \( -name "${cat}.go" -o -name "*_${cat}.go" \) -exec cat {} + 2>/dev/null | wc -l > /tmp/phase4b-baseline-loc-${cat}.txt
done

# Cold build
cd $CONSUMER/api-graphql/src
GOWORK=off go clean -cache
GOWORK=off /usr/bin/time -v go build -o /dev/null ./ent/gen/... 2> /tmp/phase4b-baseline-build.txt

# Vet
GOWORK=off /usr/bin/time -v go vet ./ent/gen/... 2> /tmp/phase4b-baseline-vet.txt

# Exported API snapshot
mkdir -p /tmp/phase4b-api-baseline
for pkg in $(GOWORK=off go list ./ent/gen/...); do
  GOWORK=off go doc -all "$pkg" > /tmp/phase4b-api-baseline/$(echo $pkg | tr / _).txt 2>/dev/null || true
done
```

Expected: all files populated. Note peak RSS values from the .txt files — these are the gate metrics.

- [ ] **Step 5: Create the measurements doc skeleton**

Create `docs/superpowers/results/2026-04-24-phase4b-measurements.md` (in the i-h8-ent worktree, not the scratch baseline worktree):

```markdown
# Phase 4B Measurements — Lean-Generics create / update / delete / query

**Date:** 2026-04-24
**Target spec:** docs/superpowers/specs/2026-04-24-ent-code-reduction-phase-4-hybrid.md
**Plan:** docs/superpowers/plans/2026-04-24-ent-code-reduction-phase-4b.md
**Go version:** (fill from `go version`)
**Consumer SHA:** (fill from `git -C $CONSUMER rev-parse HEAD`)
**Baseline fork SHA (pre-4B, = Phase 3 state):** (fill from $BASELINE_SHA above)
**Post-4B fork SHA:** (fill from $CURRENT_SHA above)

## Methodology

Measurements captured against the downstream consumer `service-api-go` via
`replace entgo.io/ent => <local-fork-path>`. Baseline pointed at
`.worktrees/phase4b-baseline` (pre-4B). Post-4B pointed at
`.worktrees/i-h8-ent`. Between runs, `go clean -cache` and
`go mod tidy` are re-issued.

LOC from `wc -l` over generated Go files. Build/vet numbers from
`/usr/bin/time -v` on cold builds. Exported-API snapshot from
`go doc -all` per package for cross-referencing.

Consumer uses `GOWORK=off` for all commands (it has a `go.work` parent).

## Baseline (pre-Phase-4B, = Phase 3 state)

| Metric | Value |
|---|---|
| Consumer ent/gen total LOC | (from /tmp/phase4b-baseline-loc.txt) |
| where.go LOC | (from /tmp/phase4b-baseline-loc-where.txt) |
| create.go LOC (<schema>_create.go) | (from /tmp/phase4b-baseline-loc-create.txt) |
| update.go LOC | (from /tmp/phase4b-baseline-loc-update.txt) |
| delete.go LOC | (from /tmp/phase4b-baseline-loc-delete.txt) |
| query.go LOC | (from /tmp/phase4b-baseline-loc-query.txt) |
| Cold `go build ./ent/gen/...` wall time | (from /tmp/phase4b-baseline-build.txt) |
| Cold `go build` peak RSS (kB) | (from /tmp/phase4b-baseline-build.txt) |
| `go vet ./ent/gen/...` wall time | (from /tmp/phase4b-baseline-vet.txt) |
| `go vet` peak RSS (kB) | (from /tmp/phase4b-baseline-vet.txt) |

## Post-4B

(filled in at Task 20)

## Decision

(filled in at Task 21)
```

Fill in the baseline column from `/tmp/phase4b-baseline-*.txt`.

- [ ] **Step 6: Restore consumer go.mod and commit skeleton**

```bash
cd $CONSUMER
mv go.mod.bak go.mod
GOWORK=off go mod tidy

cd /var/home/smoothbrain/dev/matthewsreis/ent/.worktrees/i-h8-ent
git add docs/superpowers/results/2026-04-24-phase4b-measurements.md
```

```bash
git commit -m "docs(results): Phase 4B baseline measurements"
```

---

### Task 19: Regenerate consumer against post-4B, measure

**Files:**
- Modify: `docs/superpowers/results/2026-04-24-phase4b-measurements.md` (fill Post-4B section)

- [ ] **Step 1: Swap to post-4B replace, regen**

```bash
CONSUMER=/home/smoothbrain/dev/matthewsreis/worktrees/main/service-api-go
FORK=/var/home/smoothbrain/dev/matthewsreis/ent/.worktrees/i-h8-ent

cd $CONSUMER
cp go.mod go.mod.bak
GOWORK=off go mod edit -replace entgo.io/ent=$FORK
GOWORK=off go mod tidy
GOWORK=off go generate ./...
```

Expected: regen succeeds. If it fails, the template bug slipped past Task 16's integration tests (111-schema breadth catches things that the integration suite's handful of schemas misses). Fix the template and regen.

- [ ] **Step 2: Capture post-4B metrics**

```bash
# LOC totals
find $CONSUMER/api-graphql/src/ent/gen -name '*.go' -exec cat {} + | wc -l > /tmp/phase4b-post-loc.txt

# Per-category LOC
for cat in where create update delete query; do
  find $CONSUMER/api-graphql/src/ent/gen -type f \( -name "${cat}.go" -o -name "*_${cat}.go" \) -exec cat {} + 2>/dev/null | wc -l > /tmp/phase4b-post-loc-${cat}.txt
done

# Cold build
cd $CONSUMER/api-graphql/src
GOWORK=off go clean -cache
GOWORK=off /usr/bin/time -v go build -o /dev/null ./ent/gen/... 2> /tmp/phase4b-post-build.txt

# Vet
GOWORK=off /usr/bin/time -v go vet ./ent/gen/... 2> /tmp/phase4b-post-vet.txt

# Post-POC API snapshot for API diff
mkdir -p /tmp/phase4b-api-post
for pkg in $(GOWORK=off go list ./ent/gen/...); do
  GOWORK=off go doc -all "$pkg" > /tmp/phase4b-api-post/$(echo $pkg | tr / _).txt 2>/dev/null || true
done
```

Expected: all files populated.

- [ ] **Step 3: Fill the Post-4B section of the measurements doc**

Open `docs/superpowers/results/2026-04-24-phase4b-measurements.md` and populate:

```markdown
## Post-4B

Captured 2026-04-24. Consumer regenerated against i-h8-ent HEAD (Task 16's combined commit).

| Metric | Baseline | Post-4B | Delta |
|---|---|---|---|
| Consumer ent/gen total LOC | (from baseline) | (from /tmp/phase4b-post-loc.txt) | diff (-X%) |
| where.go LOC | … | … | … |
| create.go LOC | … | … | … |
| update.go LOC | … | … | … |
| delete.go LOC | … | … | … |
| query.go LOC | … | … | … |
| Cold `go build ./ent/gen/...` wall time | … | … | … |
| Cold `go build` peak RSS (kB) | … | … | … (-X% or +X%) |
| `go vet ./ent/gen/...` wall time | … | … | … |
| `go vet` peak RSS (kB) | … | … | … |
```

Compute deltas. For each metric: `delta = post - baseline`, `%change = delta / baseline * 100`.

- [ ] **Step 4: Restore consumer go.mod**

```bash
cd $CONSUMER
mv go.mod.bak go.mod
GOWORK=off go mod tidy
git -C $CONSUMER diff -- go.mod
```

Expected: `git diff` on consumer's go.mod is empty. Do not commit in the consumer.

- [ ] **Step 5: Commit measurements**

```bash
cd /var/home/smoothbrain/dev/matthewsreis/ent/.worktrees/i-h8-ent
git add docs/superpowers/results/2026-04-24-phase4b-measurements.md
```

```bash
git commit -m "docs(results): Phase 4B post-4B consumer measurements"
```

---

### Task 20: API-diff check

**Files:**
- Modify: `docs/superpowers/results/2026-04-24-phase4b-measurements.md` (add diff note)

- [ ] **Step 1: Diff the exported-API snapshots**

```bash
diff -r /tmp/phase4b-api-baseline /tmp/phase4b-api-post > /tmp/phase4b-api-diff.txt
wc -l /tmp/phase4b-api-diff.txt
```

Expected: **empty diff** (zero lines). Any diff means the refactor broke drop-in compat. Inspect `/tmp/phase4b-api-diff.txt`:
- Pure godoc whitespace/formatting diff → cosmetic, acceptable; note it.
- Signature or type-identity diff → NO-GO; fix before proceeding.

- [ ] **Step 2: Record result in the measurements doc**

Append to the Post-4B section:

```markdown
### Exported API diff

Diff of `go doc -all` across all 127 generated packages, baseline vs post-4B:

- Total diff lines: `(from wc -l)`
- Verdict: (ZERO / COSMETIC ONLY / SIGNIFICANT)
- If non-zero: attach an excerpt here showing the first few diverging lines.
```

- [ ] **Step 3: Commit if non-empty updates happen**

Only commit if the doc changed:

```bash
git status
```

If `docs/superpowers/results/2026-04-24-phase4b-measurements.md` shows modified, commit. Otherwise skip.

```bash
git add docs/superpowers/results/2026-04-24-phase4b-measurements.md
```

```bash
git commit -m "docs(results): Phase 4B exported-API diff verification"
```

---

### Task 21: Go/no-go decision for Phase 4C

**Files:**
- Modify: `docs/superpowers/results/2026-04-24-phase4b-measurements.md` (fill Decision section)

Apply the revised gates from the hybrid spec — but evaluate against the *incremental* change over Phase 3's baseline, not the full-phase targets, because Phase 4C's descriptor-driven work is what closes the remaining gap.

- [ ] **Step 1: Apply the gate criteria**

The hybrid-spec gates for the full refactor (Phase 4 overall) are `≥40% LOC reduction`, `≤5.5 GB peak RSS`, `≥40% faster cold build`. Phase 4B is expected to deliver only part of that — the descriptor-driven mutation/client/entql rewrites (Phase 4C) carry the heaviest type-graph collapse.

Phase 4B's specific gates:

| Gate | Target | Rationale |
|---|---|---|
| Incremental LOC reduction vs. baseline | ≥ 5% on total, ≥ 15% on create+update+delete+query combined | setter.tmpl alone touches ~35% of generated LOC |
| Cold `go build` wall time | No regression (≤ baseline) | Generic instantiation cost shouldn't outweigh the body collapse |
| Cold `go build` peak RSS | No regression (≤ baseline + 5% noise) | Phase 3 showed generics don't blow up RSS |
| `go vet` peak RSS | No regression | Same as above |
| ent integration suite | 100% green, tests unchanged | Task 16 confirms |
| Consumer build | passes `go build ./ent/gen/...` | Task 19 step 1 confirms |
| Exported API diff | zero signature change | Task 20 confirms |

- [ ] **Step 2: Fill the Decision section**

Append to `docs/superpowers/results/2026-04-24-phase4b-measurements.md`:

```markdown
## Decision

**Gate results:**

| Gate | Target | Measured | Met? |
|---|---|---|---|
| Total LOC reduction | ≥ 5% | …% | ✓/✗ |
| create+update+delete+query combined LOC reduction | ≥ 15% | …% | ✓/✗ |
| `go build` wall time | no regression | … | ✓/✗ |
| `go build` peak RSS | no regression (± 5%) | … | ✓/✗ |
| `go vet` peak RSS | no regression (± 5%) | … | ✓/✗ |
| ent integration tests | green | … | ✓/✗ |
| Consumer build | green | … | ✓/✗ |
| Exported API diff | zero | … | ✓/✗ |

**Decision:** [GO-to-Phase-4C / NO-GO-reconsider / PARTIAL-needs-tuning]

**Rationale:** (1-2 paragraphs on the numbers)

**Per-LOC efficiency observation:**

Phase 3 saw 1.83% LOC cut → 10.4% wall-time improvement (a 5× multiplier).
Extrapolate here: if Phase 4B's LOC cut is Y% and wall-time improvement is X%,
the multiplier is X/Y. Document whether the Phase 3 pattern held, degraded, or
amplified.

**Path forward:**

- If GO: write Phase 4C implementation plan covering descriptor-driven
  mutation/client/entql templates + witness-test generator.
- If PARTIAL: document which categories over-/under-performed; refine the
  Phase 4C plan to absorb/compensate.
- If NO-GO: the lean-generics approach is capped earlier than expected;
  escalate to pure Approach 2 (descriptor-driven across all categories)
  via a new spec.
```

- [ ] **Step 3: Commit decision**

```bash
git add docs/superpowers/results/2026-04-24-phase4b-measurements.md
```

```bash
git commit -m "docs(results): Phase 4B go/no-go decision"
```

- [ ] **Step 4: Clean up the scratch baseline worktree**

```bash
cd /var/home/smoothbrain/dev/matthewsreis/ent
git worktree remove .worktrees/phase4b-baseline
```

Expected: worktree removed; the branch (if any) at that SHA is preserved in reflog.

- [ ] **Step 5: Push**

```bash
cd /var/home/smoothbrain/dev/matthewsreis/ent/.worktrees/i-h8-ent
git push
```

Phase 4B complete. If GO, Phase 4C plan is the next deliverable.

---

## Self-review appendix

This plan was self-reviewed against the hybrid spec and the Phase 4A results doc.

**Covered:**

- Spec §"The hybrid split" rows marked "Lean generics" for create/update/delete/query/where — Phase 3 covered where; this plan covers create (Task 11), update (Task 13), delete (Task 14), query (Task 15), and the shared setter sub-template (Task 9).
- Spec §"Non-goals" — runtime throughput unchanged, gql_*.go untouched (no template under `gql_*` is modified).
- Spec §"Correctness safeguards" — consumer-CI gating is executed via Task 19 step 1 (full consumer regen + build); SQL snapshot regression check is Task 16 step 5.
- Spec §"Risks / Generic instantiation blowup" — the Phase 3 measurement already answered this for predicate generics; Phase 4B repeats the validation via Task 19 step 2 (RSS + wall time against the 111-schema consumer).
- Phase 4A results §"Path forward / Phase 4B" — this plan implements exactly what that section anticipated.
- User's handoff gotchas 1-6 — all accounted for:
  - 1 (module boundary): Task 16 step 4 runs tests from `entc/integration/`, not the repo root.
  - 2 (consumer GOWORK=off + restore go.mod): Tasks 18, 19 use `GOWORK=off` and restore via `go.mod.bak`; Task 19 step 4 verifies empty diff.
  - 3 (consumer taskfile uses `-gcflags='...=-N -l'`; POC measures plain `go build`): Tasks 18 step 4, 19 step 2 use plain `go build -o /dev/null` for comparability with Phase 3.
  - 4 (revised gates — not original 30% RSS / 60% LOC): Task 21 step 1 uses Phase-4B-specific gates that are incremental over the Phase-3 baseline.
  - 5 (gopls lag): implicit — plan references `go test` / `go build` as the source of truth, never LSP diagnostics.
  - 6 (CardMutation type alias): not a Phase 4B concern — this plan never touches mutation or client templates. Flagged for Phase 4C planning.
- Out-of-scope items from the spec §"Out of scope" — gql_*.go, schema authoring, upstream merge, runtime throughput improvements — none touched.

**Not covered (deferred to Phase 4C, iff Task 21 is GO):**

- Descriptor-driven `internal_mutation.tmpl` rewrite (mutation category).
- Descriptor-driven `client.tmpl` rewrite.
- Descriptor-driven `entql.tmpl` rewrite (file is absent from this plan — entql template lives at `entc/gen/template/*.tmpl`; search during Phase 4C planning).
- Witness-test generator integration into `entc/gen`.
- Through-table edge state in runtime `Mutation[T]`.
- Final 111-schema measurement with the full hybrid success criteria.

**Deliberate choices on helper scope:**

- `BSet` / `BClear` / `Must` are the only new helpers. `BSetNillable` and variadic edge helpers were considered and rejected because the nillable-pointer deref and variadic method-value shapes don't fit cleanly into Go generics without introducing costly instantiations for negligible LOC win. Those paths are handled via line-compression only.
- `MustOrNotFound` was considered for `FirstX` / `FirstIDX` but rejected because `IsNotFound` is defined in the generated per-schema package — routing it through a runtime helper would require passing a classifier callback, which pays syntactic cost the compiler's inlining would have eliminated. Line-compression only for those wrappers.

**Non-placeholder guarantees:**

- Every code block is literal content ready to paste into the template/file.
- Every step has the exact `git add`/`git commit` pair to run.
- No "TBD" / "fill in later" / "add error handling" markers.
- Line numbers in template-edit instructions are copied from the current file state and are stable (files haven't changed since the Read of this session).
- Type references are consistent: `BSet[B any, V any]` in Task 2 matches the usage in Tasks 9, 11, 13, 14, 15. `BClear[B any]` in Task 4 matches the usage in Tasks 9 and 13. `Must[T any]` in Task 6 matches the usage in Tasks 11, 13, 14, 15.
