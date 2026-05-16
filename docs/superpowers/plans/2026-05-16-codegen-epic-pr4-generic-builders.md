# Codegen Epic PR 4 — Generic Builders Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Collapse the terminal-method bodies and chaining-state plumbing in generated `<entity>_query.go`, `<entity>/update.go`, and `<entity>/delete.go` files into reusable generic helpers in `runtime/entbuilder`. Each per-entity file keeps its struct + chaining methods + SQL-layer methods but delegates terminal logic (`All`, `First`, `Count`, etc.) and dispatch boilerplate to a handful of generic functions.

**Architecture:** Three new things in `runtime/entbuilder`:
1. **Ent-interop primitives** (`SetContextOp`, `WithInterceptors`) moved/copied from the generated `ent.go` so they can be called from generic helpers without depending on per-package state.
2. **`QueryState[P ~func(*sql.Selector)]`** — embedded into generated `<Entity>Query`, holding the generic state (ctx, predicates, sql, path, inters).
3. **Package-level `Run*` generic helpers** (`RunAll`, `RunFirst`, `RunOnly`, `RunIDs`, `RunCount`, `RunExist`, plus `RunUpdate`, `RunUpdateOne`, `RunDelete`) that take entity-specific functions as arguments. This avoids the method-promotion-loses-outer-type problem with embedded generic structs.

**Tech Stack:** Go 1.21+ generics, `entgo.io/ent` (top-level package for Hook/Querier/QueryContext types), `entgo.io/ent/dialect/sql/sqlgraph`.

**Spec reference:** `docs/superpowers/specs/2026-05-16-codegen-epic-pr4-generic-builders-design.md` (PR 4 design); `docs/superpowers/specs/2026-05-15-ent-codegen-reduction-epic.md` §4 PR 4 (epic context).

**Branch:** stay on `worktree-wiggly-singing-pancake` (epic policy — no separate git-spice branch, no push, no PR).

---

## Scope cuts vs. the design spec — read before starting

The design spec §1 estimates "~50-60K LOC reduction at consumer scale." Investigation during plan-writing found that **most** of the LOC in `<entity>_query.go` (244 of 743 LOC, ~33%) lives in the SQL-layer methods (`querySpec`, `sqlQuery`, `loadX`, `prepareQuery`, `sqlAll`, `sqlCount`) which are unavoidably entity-specific — they reference the entity's column constants, table name, scan funcs, and edge metadata.

This plan therefore targets the **terminal-method collapse + chaining-state delegation**, which realistically saves ~140-180 LOC per entity (terminal 142 LOC → ~30 LOC; chaining 37 LOC → ~12 LOC). At consumer scale (134 entities), that's ~20K LOC reduction in `_query.go`. Update/Delete contribute another ~5-10K combined. Total realistic PR 4 LOC win: ~25-30K, well under the design spec's optimistic estimate.

The SQL-layer refactor (generic `MakeQuerySpec`, generic edge loaders) is deferred — it's a meaningful next PR but much more design work, and the templates currently produce defensible code for those methods.

If you (the implementer or controller) think attacking the SQL-layer is worth the additional design effort within PR 4, stop and surface that to Zack — it's a scope decision, not a Task-N decision.

---

## File map

| File | Status | Responsibility |
|---|---|---|
| `runtime/entbuilder/dispatch.go` | **NEW** | Ent-interop primitives: `SetContextOp`, `WithInterceptors` — package-level functions that take `*ent.QueryContext`, `ent.Query`, `ent.Querier`, `[]ent.Interceptor`. Imports `entgo.io/ent` (NEW import for entbuilder). |
| `runtime/entbuilder/dispatch_test.go` | **NEW** | Unit tests for `SetContextOp` (returns ctx with op set) and `WithInterceptors` (chains interceptors in reverse order). |
| `runtime/entbuilder/query.go` | modify | Add `QueryState[P ~func(*sql.Selector)]` struct + `AddPredicates`/`SetLimit`/`SetOffset`/`SetUnique`/`Clone` helpers. Add `RunAll[V]`, `RunFirst[V]`, `RunOnly[V]`, `RunIDs[V]`, `RunCount`, `RunExist` package-level generics. |
| `runtime/entbuilder/query_test.go` | **NEW** | Table-driven tests for `QueryState` helpers and `Run*` terminals (mock prepareQuery + sqlAll funcs verify the dispatch flow). |
| `runtime/entbuilder/update.go` | modify | Add `UpdateState[M any]` struct + `RunUpdate[M]` and `RunUpdateOne[T, M]` package-level generics. |
| `runtime/entbuilder/update_test.go` | **NEW** | Unit tests for `RunUpdate`/`RunUpdateOne` (mock sqlSave fn, verify hook dispatch). |
| `runtime/entbuilder/delete.go` | modify | Add `DeleteState[M any]` struct + `RunDelete[M]` package-level generic. |
| `runtime/entbuilder/delete_test.go` | **NEW** | Unit tests for `RunDelete`. |
| `entc/gen/template/builder/query.tmpl` | modify | Emit generated `<Entity>Query` with embedded `entbuilder.QueryState[P]`, chaining methods delegating to embedded state, terminal methods 1-2 lines calling `entbuilder.Run*`. |
| `entc/gen/template/builder/update.tmpl` | modify | Emit generated `<Entity>Update` / `<Entity>UpdateOne` with embedded `entbuilder.UpdateState[*<Entity>Mutation]`, terminal methods delegating to `entbuilder.RunUpdate`/`RunUpdateOne`. |
| `entc/gen/template/builder/delete.tmpl` | modify | Same pattern for `<Entity>Delete` / `<Entity>DeleteOne`. |
| `entc/gen/template/import.tmpl` | modify | Add `entgo.io/ent/runtime/entbuilder` import when query/update/delete templates emit it. Today entbuilder is conditionally imported via `CreateUsesEntbuilder` / `UpdateUsesEntbuilder` / `DeleteUsesEntbuilder` Scope flags — extend or piggy-back on those. |
| `entc/integration/**/ent/<entity>_query.go` | regenerate | Fixture regeneration. |
| `entc/integration/**/ent/<entity>/update.go` | regenerate | Same. |
| `entc/integration/**/ent/<entity>/delete.go` | regenerate | Same. |
| `internal/bench/pr4.jsonl` | **NEW** | Bench output recorded post-regen. |
| `docs/superpowers/specs/2026-05-15-ent-codegen-reduction-epic.md` | modify | Update §0 progress table: PR 4 ✅, PR 5 next. Bump header to "5 of 7 PRs complete". |

---

## Pre-flight: every implementer reads this first

**Branch discipline.** Stay on `worktree-wiggly-singing-pancake`. Never run `git push`, `gh pr create`, `gs branch submit`, or `gs stack submit`. Never `git checkout` to a different branch. **Before EVERY `git commit`, run `pwd && git rev-parse --abbrev-ref HEAD` and verify they return `/var/home/smoothbrain/dev/matthewsreis/ent/.claude/worktrees/wiggly-singing-pancake` and `worktree-wiggly-singing-pancake` respectively.** Three prior PR 0-2 implementer subagents committed to master by accident. Do not repeat.

**Working directory.** Every command runs from `/var/home/smoothbrain/dev/matthewsreis/ent/.claude/worktrees/wiggly-singing-pancake`. Confirm with `pwd` before each command block. If Bash shell state has `cd`'d to a subdirectory (e.g., after Task 9's fixture regen), use absolute paths or explicitly `cd` back to the worktree root.

**Parent commit.** This plan's Task 1 starts on top of `b582f7c2b spec(codegen-epic): PR 4 generic builders design`. Verify with `git log -1 --pretty=format:'%H %s'`.

**Test runner.** Use `go test -count=1` (defeats Go's test cache). Always pass `-count=1` for entc test runs because integration suites hit a real SQLite file path that can carry between runs.

**Shell cwd.** Bash tool's shell state persists across calls. If a command leaves you in a subdirectory (e.g., `cd entc/integration/<fixture>` for an isolated regen), the next command starts from there. Either `cd` back to worktree root or use absolute paths.

---

## Task 1: Orient — read the existing infrastructure

Investigative task, no code change. Build the mental model before touching anything.

**Files to READ:**
- `runtime/entbuilder/query.go` (entire file — `EdgeLoadDescriptor`, `LoadEdgeM2M`, `LoadEdgeO2M`, `LoadEdgeO2O`, `LoadEdgeM2O` — the existing edge-loading helpers; this is the pattern for function-arg-passing generic helpers)
- `runtime/entbuilder/update.go` and `delete.go` (entire files — existing descriptor types)
- `runtime/entbuilder/helpers.go` (existing field/scanner helpers)
- `runtime/entbuilder/create.go` (existing CREATE descriptor pattern)
- `ent.go` lines 263-360 (Hook, Mutation, Query, Querier, Interceptor type definitions in the upstream `entgo.io/ent` package)
- `ent.go` lines 515-580 (QueryContext struct and helpers)
- `entc/integration/privacy/ent/user_query.go` (entire file — a representative generated query builder)
- `entc/integration/privacy/ent/user/update.go` lines 1-60 + 480-512 (start + tail — see the Where method and the Save/Exec terminals)
- `entc/integration/privacy/ent/user/delete.go` (entire file — small)
- `entc/integration/privacy/ent/ent.go` lines 440-510 (the generated `WithHooks`, `setContextOp`, `querierAll`, `querierCount`, `withInterceptors` functions you'll be replacing)
- `entc/gen/template/builder/query.tmpl` (entire file — the template you'll modify in Task 7)
- `entc/gen/template/builder/update.tmpl` and `delete.tmpl` (entire files)

- [ ] **Step 1: Read each file in the list above.** You don't need to memorize. You're confirming:
  - The function-arg-passing pattern is already used in `LoadEdgeM2M` etc. — you'll model `RunAll`/`RunUpdate`/etc. on the same shape
  - The generated `setContextOp(ctx, qc, op)`, `withInterceptors[V](ctx, q, qr, inters)`, and `querierAll[V, Q]()` are what your `entbuilder.Run*` helpers will encapsulate
  - The generated `Where`/`Limit`/`Offset` chaining methods just append to or set fields on the builder — easy to delegate to embedded `QueryState`
  - The generated terminal methods (`All`, `First`, `Count`, etc.) all follow the same 4-5 line pattern: setContextOp → prepareQuery → querierX → withInterceptors
  - The generated SQL-layer methods (`sqlAll`, `prepareQuery`, `querySpec`, etc.) are unavoidably entity-specific and stay in the per-entity file

- [ ] **Step 2: Confirm `runtime/entbuilder` does NOT currently import `entgo.io/ent`**

Run: `head -15 runtime/entbuilder/query.go`

Expected: imports are `context`, `database/sql/driver`, `fmt`, `entgo.io/ent/dialect/sql`, `entgo.io/ent/dialect/sql/sqlgraph`. No `entgo.io/ent` top-level import.

Adding `entgo.io/ent` is fine — there's no circular dependency since `entgo.io/ent` (the top-level package defining Hook/Querier/etc.) doesn't import `entgo.io/ent/runtime/entbuilder` (it's a runtime helper consumed by generated code, not by the core ent package). Confirm by:

Run: `grep -rn '"entgo.io/ent/runtime/entbuilder"' *.go 2>/dev/null` (from worktree root, not entc/integration/...). Expected: zero matches outside generated code.

- [ ] **Step 3: Verify the `ent.Querier` / `ent.QuerierFunc` types are exported and usable**

Run: `grep -n "^func \|^type " ent.go | grep -i "querier\|intercept\|hook\b\|querycontext"`

Confirm `ent.Querier`, `ent.QuerierFunc`, `ent.Interceptor`, `ent.Hook`, `ent.QueryContext`, `ent.Value`, `ent.Query` are all exported.

- [ ] **Step 4: No commit — this task is read-only**

---

## Task 2: Add ent-interop primitives to entbuilder

Move the `setContextOp` and `withInterceptors` logic into `runtime/entbuilder` so the generic `Run*` helpers (Tasks 4-6) can call them. The generated `ent.go` retains its own versions for now (for hand-written code that might use them) — they can be removed in a follow-up cleanup.

**Files:**
- Create: `runtime/entbuilder/dispatch.go`
- Create: `runtime/entbuilder/dispatch_test.go`

- [ ] **Step 1: Write the failing test**

Create `runtime/entbuilder/dispatch_test.go`:

```go
// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package entbuilder_test

import (
	"context"
	"testing"

	"entgo.io/ent"
	"entgo.io/ent/runtime/entbuilder"
	"github.com/stretchr/testify/require"
)

func TestSetContextOp_AttachesOpWhenAbsent(t *testing.T) {
	qc := &ent.QueryContext{Type: "User"}
	ctx := entbuilder.SetContextOp(context.Background(), qc, "QueryAll")
	gotQC := ent.QueryFromContext(ctx)
	require.NotNil(t, gotQC)
	require.Equal(t, "QueryAll", gotQC.Op)
	require.Equal(t, "User", gotQC.Type)
}

func TestSetContextOp_DoesNotOverrideExisting(t *testing.T) {
	parent := &ent.QueryContext{Op: "Outer", Type: "User"}
	ctx := ent.NewQueryContext(context.Background(), parent)
	qc := &ent.QueryContext{Type: "User"}
	ctx = entbuilder.SetContextOp(ctx, qc, "Inner")
	gotQC := ent.QueryFromContext(ctx)
	require.NotNil(t, gotQC)
	require.Equal(t, "Outer", gotQC.Op, "outer QueryContext must win when one is already attached")
}

func TestWithInterceptors_NoInters_CallsQuerier(t *testing.T) {
	called := false
	qr := ent.QuerierFunc(func(_ context.Context, _ ent.Query) (ent.Value, error) {
		called = true
		return 42, nil
	})
	v, err := entbuilder.WithInterceptors[int](context.Background(), nil, qr, nil)
	require.NoError(t, err)
	require.Equal(t, 42, v)
	require.True(t, called)
}

func TestWithInterceptors_ChainsInReverseOrder(t *testing.T) {
	var order []string
	mk := func(name string) ent.Interceptor {
		return ent.InterceptFunc(func(next ent.Querier) ent.Querier {
			return ent.QuerierFunc(func(ctx context.Context, q ent.Query) (ent.Value, error) {
				order = append(order, name+":before")
				v, err := next.Query(ctx, q)
				order = append(order, name+":after")
				return v, err
			})
		})
	}
	qr := ent.QuerierFunc(func(_ context.Context, _ ent.Query) (ent.Value, error) {
		order = append(order, "inner")
		return 0, nil
	})
	_, err := entbuilder.WithInterceptors[int](context.Background(), nil, qr, []ent.Interceptor{mk("A"), mk("B")})
	require.NoError(t, err)
	require.Equal(t, []string{"A:before", "B:before", "inner", "B:after", "A:after"}, order)
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -count=1 ./runtime/entbuilder/...`

Expected: build failure — `undefined: entbuilder.SetContextOp` and `entbuilder.WithInterceptors`.

- [ ] **Step 3: Create `runtime/entbuilder/dispatch.go`**

```go
// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package entbuilder

import (
	"context"
	"fmt"

	"entgo.io/ent"
)

// SetContextOp attaches the given QueryContext (including its op) to ctx, but only
// if no QueryContext is already present. This mirrors the per-entity-package
// setContextOp helper that ent codegen has emitted for years, hoisted here so
// the generic Run* helpers don't need to call back into the generated package.
func SetContextOp(ctx context.Context, qc *ent.QueryContext, op string) context.Context {
	if ent.QueryFromContext(ctx) == nil {
		qc.Op = op
		ctx = ent.NewQueryContext(ctx, qc)
	}
	return ctx
}

// WithInterceptors invokes the querier through the given interceptor chain.
// Interceptors are applied in reverse order (last registered runs outermost),
// matching the per-entity-package withInterceptors helper that ent codegen has
// emitted for years.
func WithInterceptors[V ent.Value](ctx context.Context, q ent.Query, qr ent.Querier, inters []ent.Interceptor) (V, error) {
	for i := len(inters) - 1; i >= 0; i-- {
		qr = inters[i].Intercept(qr)
	}
	rv, err := qr.Query(ctx, q)
	if err != nil {
		var zero V
		return zero, err
	}
	v, ok := rv.(V)
	if !ok {
		var zero V
		return zero, fmt.Errorf("entbuilder: unexpected query result type %T", rv)
	}
	return v, nil
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test -count=1 ./runtime/entbuilder/...`

Expected: 4 tests PASS.

- [ ] **Step 5: Commit**

**Re-verify branch first.** Run `pwd && git rev-parse --abbrev-ref HEAD`. They must return the worktree path and `worktree-wiggly-singing-pancake`. If not, STOP and report BLOCKED.

Two separate Bash calls (per user CLAUDE.md — never chain `git add && git commit`):

```bash
git add runtime/entbuilder/dispatch.go runtime/entbuilder/dispatch_test.go
```

```bash
git commit -m "$(cat <<'EOF'
feat(entbuilder): SetContextOp + WithInterceptors primitives

Hoists the per-entity-package setContextOp and withInterceptors
helpers (which ent codegen has emitted into every generated ent.go
for years) into runtime/entbuilder. Lets subsequent generic Run*
helpers dispatch through the interceptor chain without depending on
the generated package's local versions.

Generated ent.go keeps its own setContextOp/withInterceptors for
hand-written code that calls them; those can be removed in a
follow-up once codegen no longer emits calls to them.

New import: runtime/entbuilder now depends on entgo.io/ent (the
top-level package defining QueryContext / Querier / Interceptor).
There's no cycle because entgo.io/ent does not import entbuilder.

Part of codegen-epic PR 4 (generic builders).
EOF
)"
```

---

## Task 3: Add `QueryState[P]` to entbuilder with helpers (TDD)

State container that generated `<Entity>Query` will embed, plus chaining-helper methods that don't need to return the outer type.

**Files:**
- Modify: `runtime/entbuilder/query.go`
- Create: `runtime/entbuilder/query_state_test.go`

- [ ] **Step 1: Write the failing test**

Create `runtime/entbuilder/query_state_test.go`:

```go
// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package entbuilder_test

import (
	"context"
	"testing"

	"entgo.io/ent"
	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/runtime/entbuilder"
	"github.com/stretchr/testify/require"
)

type testPred = func(*sql.Selector)

func TestQueryState_AddPredicates(t *testing.T) {
	s := &entbuilder.QueryState[testPred]{Ctx: &ent.QueryContext{}}
	p1 := testPred(func(*sql.Selector) {})
	p2 := testPred(func(*sql.Selector) {})
	s.AddPredicates(p1, p2)
	require.Len(t, s.Predicates, 2)
}

func TestQueryState_SetLimit(t *testing.T) {
	s := &entbuilder.QueryState[testPred]{Ctx: &ent.QueryContext{}}
	s.SetLimit(42)
	require.NotNil(t, s.Ctx.Limit)
	require.Equal(t, 42, *s.Ctx.Limit)
}

func TestQueryState_SetOffset(t *testing.T) {
	s := &entbuilder.QueryState[testPred]{Ctx: &ent.QueryContext{}}
	s.SetOffset(7)
	require.NotNil(t, s.Ctx.Offset)
	require.Equal(t, 7, *s.Ctx.Offset)
}

func TestQueryState_SetUnique(t *testing.T) {
	s := &entbuilder.QueryState[testPred]{Ctx: &ent.QueryContext{}}
	s.SetUnique(true)
	require.NotNil(t, s.Ctx.Unique)
	require.True(t, *s.Ctx.Unique)
}

func TestQueryState_Clone_DeepCopiesPredicates(t *testing.T) {
	s := &entbuilder.QueryState[testPred]{
		Ctx:        &ent.QueryContext{Type: "User"},
		Predicates: []testPred{func(*sql.Selector) {}, func(*sql.Selector) {}},
	}
	c := s.Clone()
	require.NotSame(t, s, c, "Clone must return a distinct pointer")
	require.NotSame(t, s.Ctx, c.Ctx, "Clone must deep-copy Ctx")
	require.Equal(t, s.Ctx.Type, c.Ctx.Type)
	require.Len(t, c.Predicates, 2)
	// Mutating clone must not affect original.
	c.AddPredicates(func(*sql.Selector) {})
	require.Len(t, s.Predicates, 2)
	require.Len(t, c.Predicates, 3)
}

func TestQueryState_Clone_PreservesInterceptors(t *testing.T) {
	inter := ent.InterceptFunc(func(next ent.Querier) ent.Querier { return next })
	s := &entbuilder.QueryState[testPred]{
		Ctx:    &ent.QueryContext{},
		Inters: []ent.Interceptor{inter},
	}
	c := s.Clone()
	require.Len(t, c.Inters, 1)
}

// Compile-time sanity check that QueryState[P] satisfies the embedded-into-<Entity>Query pattern.
func TestQueryState_EmbedsCleanly(t *testing.T) {
	type UserQuery struct {
		entbuilder.QueryState[testPred]
		extraField int
	}
	uq := &UserQuery{}
	uq.Ctx = &ent.QueryContext{}
	uq.SetLimit(5)
	uq.extraField = 10
	require.Equal(t, 5, *uq.Ctx.Limit)
	require.Equal(t, 10, uq.extraField)
	_ = context.Background() // unused-import suppressor; context is imported above by other tests
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test -count=1 -run 'TestQueryState' ./runtime/entbuilder/...`

Expected: build failure — `undefined: entbuilder.QueryState`.

- [ ] **Step 3: Add `QueryState[P]` to `runtime/entbuilder/query.go`**

Append to `runtime/entbuilder/query.go` (BEFORE the existing `EdgeLoadDescriptor` block, after the package declaration and imports — so the new public-facing types come first):

```go
// QueryState holds the generic state every generated <Entity>Query carries.
// Generated <Entity>Query structs embed QueryState[predicate.Entity] and
// delegate chaining methods (Where/Limit/Offset/Unique) to helpers on this
// struct. Terminal methods (All/First/Count/etc.) delegate to the package-level
// Run* generic functions below.
//
// The P type parameter is the entity's predicate type (e.g., predicate.User);
// PR 3 made every predicate.<Entity> a type alias for func(*sql.Selector), so
// P naturally satisfies the ~func(*sql.Selector) constraint.
type QueryState[P ~func(*sql.Selector)] struct {
	Ctx        *ent.QueryContext
	Inters     []ent.Interceptor
	Predicates []P
	// Sql is the in-progress query selector. nil for fresh queries built
	// via Client.Entity.Query(); non-nil when the query is being constructed
	// from another query (e.g., as part of an edge traversal).
	Sql *sql.Selector
	// Path is set when the query was constructed from a parent query
	// (e.g., user.QueryTeams()). When set, Sql is built lazily by calling Path.
	Path func(context.Context) (*sql.Selector, error)
}

// AddPredicates appends ps to the state's predicate list.
func (s *QueryState[P]) AddPredicates(ps ...P) {
	s.Predicates = append(s.Predicates, ps...)
}

// SetLimit sets the LIMIT clause for this query.
func (s *QueryState[P]) SetLimit(n int) {
	s.Ctx.Limit = &n
}

// SetOffset sets the OFFSET clause for this query.
func (s *QueryState[P]) SetOffset(n int) {
	s.Ctx.Offset = &n
}

// SetUnique sets the DISTINCT flag for this query.
func (s *QueryState[P]) SetUnique(b bool) {
	s.Ctx.Unique = &b
}

// Clone returns a deep copy of the state. The returned QueryState shares no
// mutable state with the receiver — appending to Clone's Predicates / Inters
// does not affect the original.
//
// Note: Clone does NOT deep-copy entity-specific edge state held on the
// embedding <Entity>Query — that's the caller's responsibility (the generated
// Clone method copies edge state, then defers to s.Clone() for the embedded
// state).
func (s *QueryState[P]) Clone() *QueryState[P] {
	out := &QueryState[P]{
		Ctx:        s.Ctx.Clone(),
		Inters:     append([]ent.Interceptor(nil), s.Inters...),
		Predicates: append([]P(nil), s.Predicates...),
		Sql:        s.Sql,
		Path:       s.Path,
	}
	return out
}
```

You'll also need to add `entgo.io/ent` to the imports of `runtime/entbuilder/query.go` if it isn't already there. Check with `head -15 runtime/entbuilder/query.go` first; add the import if needed.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test -count=1 -run 'TestQueryState' ./runtime/entbuilder/...`

Expected: 7 tests PASS.

Then run the full entbuilder suite to confirm no regression: `go test -count=1 ./runtime/entbuilder/...`

Expected: PASS (Task 2's 4 tests + the 7 new ones).

- [ ] **Step 5: Commit**

**Re-verify branch first.** Run `pwd && git rev-parse --abbrev-ref HEAD`.

```bash
git add runtime/entbuilder/query.go runtime/entbuilder/query_state_test.go
```

```bash
git commit -m "$(cat <<'EOF'
feat(entbuilder): QueryState[P] generic state container

Adds the QueryState[P ~func(*sql.Selector)] struct that generated
<Entity>Query types will embed in PR 4 Task 7. Holds the generic
state (ctx, inters, predicates, sql, path) that's identical across
every entity's query builder today.

Helpers: AddPredicates, SetLimit, SetOffset, SetUnique, Clone. The
embedded-struct + helpers pattern preserves method chainability on
the outer <Entity>Query (Where(...) returns *UserQuery, not
*QueryState).

Depends on PR 3's predicate type alias: predicate.User is now an
alias for func(*sql.Selector), which satisfies the P ~func(*sql.Selector)
constraint by underlying-type matching.

Part of codegen-epic PR 4 (generic builders).
EOF
)"
```

---

## Task 4: Add `Run*` terminal helpers for queries (TDD)

Package-level generic functions that encapsulate the "setContextOp → prepareQuery → querier chain → withInterceptors" pattern every generated `All`/`First`/`Count`/`Exist`/`IDs`/`Only` method currently spells out.

**Files:**
- Modify: `runtime/entbuilder/query.go`
- Create: `runtime/entbuilder/query_run_test.go`

- [ ] **Step 1: Write the failing test**

Create `runtime/entbuilder/query_run_test.go`:

```go
// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package entbuilder_test

import (
	"context"
	"errors"
	"testing"

	"entgo.io/ent"
	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/runtime/entbuilder"
	"github.com/stretchr/testify/require"
)

// fakeUser is a stand-in for a generated User type.
type fakeUser struct{ Name string }

// fakeUserQuery is a stand-in for a generated *UserQuery; it satisfies ent.Query (the empty interface).
type fakeUserQuery struct {
	entbuilder.QueryState[testPred]
}

func TestRunAll_Success(t *testing.T) {
	q := &fakeUserQuery{QueryState: entbuilder.QueryState[testPred]{Ctx: &ent.QueryContext{}}}
	prepared := false
	sqlAll := func(ctx context.Context) ([]*fakeUser, error) {
		require.True(t, prepared, "prepareQuery must run before sqlAll")
		return []*fakeUser{{Name: "alice"}, {Name: "bob"}}, nil
	}
	prep := func(_ context.Context) error { prepared = true; return nil }

	nodes, err := entbuilder.RunAll[[]*fakeUser](context.Background(), q, q.Ctx, "QueryAll", nil, prep, sqlAll)
	require.NoError(t, err)
	require.Len(t, nodes, 2)
	require.Equal(t, "alice", nodes[0].Name)
	require.Equal(t, "QueryAll", q.Ctx.Op, "op must be attached to ctx via QueryState.Ctx")
}

func TestRunAll_PrepareQueryError_Aborts(t *testing.T) {
	q := &fakeUserQuery{QueryState: entbuilder.QueryState[testPred]{Ctx: &ent.QueryContext{}}}
	wantErr := errors.New("prep boom")
	prep := func(_ context.Context) error { return wantErr }
	sqlCalled := false
	sqlAll := func(context.Context) ([]*fakeUser, error) {
		sqlCalled = true
		return nil, nil
	}

	nodes, err := entbuilder.RunAll[[]*fakeUser](context.Background(), q, q.Ctx, "QueryAll", nil, prep, sqlAll)
	require.ErrorIs(t, err, wantErr)
	require.Nil(t, nodes)
	require.False(t, sqlCalled, "sqlAll must not run when prepareQuery fails")
}

func TestRunAll_InterceptorChainRunsAroundSqlAll(t *testing.T) {
	q := &fakeUserQuery{QueryState: entbuilder.QueryState[testPred]{Ctx: &ent.QueryContext{}}}
	var order []string
	mk := func(name string) ent.Interceptor {
		return ent.InterceptFunc(func(next ent.Querier) ent.Querier {
			return ent.QuerierFunc(func(ctx context.Context, qq ent.Query) (ent.Value, error) {
				order = append(order, name+":pre")
				v, err := next.Query(ctx, qq)
				order = append(order, name+":post")
				return v, err
			})
		})
	}
	prep := func(context.Context) error { return nil }
	sqlAll := func(context.Context) ([]*fakeUser, error) {
		order = append(order, "sql")
		return []*fakeUser{}, nil
	}

	_, err := entbuilder.RunAll[[]*fakeUser](context.Background(), q, q.Ctx, "QueryAll",
		[]ent.Interceptor{mk("outer"), mk("inner")}, prep, sqlAll)
	require.NoError(t, err)
	require.Equal(t, []string{"outer:pre", "inner:pre", "sql", "inner:post", "outer:post"}, order)
}

func TestRunCount_Success(t *testing.T) {
	q := &fakeUserQuery{QueryState: entbuilder.QueryState[testPred]{Ctx: &ent.QueryContext{}}}
	prep := func(context.Context) error { return nil }
	sqlCount := func(context.Context) (int, error) { return 7, nil }

	got, err := entbuilder.RunCount(context.Background(), q, q.Ctx, "QueryCount", nil, prep, sqlCount)
	require.NoError(t, err)
	require.Equal(t, 7, got)
}

func TestRunFirst_NodeFound(t *testing.T) {
	q := &fakeUserQuery{QueryState: entbuilder.QueryState[testPred]{Ctx: &ent.QueryContext{}}}
	prep := func(context.Context) error { return nil }
	sqlAll := func(context.Context) ([]*fakeUser, error) {
		return []*fakeUser{{Name: "first"}, {Name: "second"}}, nil
	}
	notFound := func(label string) error { return errors.New(label + ": not found") }

	n, err := entbuilder.RunFirst[*fakeUser, []*fakeUser](context.Background(), q, q.Ctx, "QueryFirst", "User", nil, prep, sqlAll, notFound)
	require.NoError(t, err)
	require.Equal(t, "first", n.Name)
}

func TestRunFirst_NotFoundError(t *testing.T) {
	q := &fakeUserQuery{QueryState: entbuilder.QueryState[testPred]{Ctx: &ent.QueryContext{}}}
	prep := func(context.Context) error { return nil }
	sqlAll := func(context.Context) ([]*fakeUser, error) { return nil, nil }
	notFound := func(label string) error { return errors.New(label + ": not found") }

	n, err := entbuilder.RunFirst[*fakeUser, []*fakeUser](context.Background(), q, q.Ctx, "QueryFirst", "User", nil, prep, sqlAll, notFound)
	require.Error(t, err)
	require.Contains(t, err.Error(), "User: not found")
	require.Nil(t, n)
}

func TestRunOnly_ExactlyOne(t *testing.T) {
	q := &fakeUserQuery{QueryState: entbuilder.QueryState[testPred]{Ctx: &ent.QueryContext{}}}
	prep := func(context.Context) error { return nil }
	sqlAll := func(context.Context) ([]*fakeUser, error) { return []*fakeUser{{Name: "x"}}, nil }
	notFound := func(label string) error { return errors.New(label + ": not found") }
	notSingular := func(label string) error { return errors.New(label + ": not singular") }

	n, err := entbuilder.RunOnly[*fakeUser, []*fakeUser](context.Background(), q, q.Ctx, "QueryOnly", "User", nil, prep, sqlAll, notFound, notSingular)
	require.NoError(t, err)
	require.Equal(t, "x", n.Name)
}

func TestRunOnly_MultipleResults_Error(t *testing.T) {
	q := &fakeUserQuery{QueryState: entbuilder.QueryState[testPred]{Ctx: &ent.QueryContext{}}}
	prep := func(context.Context) error { return nil }
	sqlAll := func(context.Context) ([]*fakeUser, error) {
		return []*fakeUser{{Name: "a"}, {Name: "b"}}, nil
	}
	notFound := func(label string) error { return errors.New(label + ": not found") }
	notSingular := func(label string) error { return errors.New(label + ": not singular") }

	_, err := entbuilder.RunOnly[*fakeUser, []*fakeUser](context.Background(), q, q.Ctx, "QueryOnly", "User", nil, prep, sqlAll, notFound, notSingular)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not singular")
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test -count=1 -run 'TestRunAll|TestRunCount|TestRunFirst|TestRunOnly' ./runtime/entbuilder/...`

Expected: build failure — `undefined: entbuilder.RunAll` etc.

- [ ] **Step 3: Add Run* helpers to `runtime/entbuilder/query.go`**

Append to `runtime/entbuilder/query.go`:

```go
// RunAll executes the All-shaped query: attaches op to ctx, runs prepareQuery,
// then dispatches through the interceptor chain to sqlAll. The returned slice
// type V is typically []*T for an entity type T.
//
// prepareQuery is the per-entity hook that runs validation/setup before sql.
// sqlAll is the per-entity SQL execution; it receives the prepared ctx.
func RunAll[V ent.Value](
	ctx context.Context,
	q ent.Query,
	qc *ent.QueryContext,
	op string,
	inters []ent.Interceptor,
	prepareQuery func(context.Context) error,
	sqlAll func(context.Context) (V, error),
) (V, error) {
	ctx = SetContextOp(ctx, qc, op)
	if err := prepareQuery(ctx); err != nil {
		var zero V
		return zero, err
	}
	qr := ent.QuerierFunc(func(ctx context.Context, _ ent.Query) (ent.Value, error) {
		return sqlAll(ctx)
	})
	return WithInterceptors[V](ctx, q, qr, inters)
}

// RunCount executes the Count-shaped query.
func RunCount(
	ctx context.Context,
	q ent.Query,
	qc *ent.QueryContext,
	op string,
	inters []ent.Interceptor,
	prepareQuery func(context.Context) error,
	sqlCount func(context.Context) (int, error),
) (int, error) {
	ctx = SetContextOp(ctx, qc, op)
	if err := prepareQuery(ctx); err != nil {
		return 0, err
	}
	qr := ent.QuerierFunc(func(ctx context.Context, _ ent.Query) (ent.Value, error) {
		return sqlCount(ctx)
	})
	return WithInterceptors[int](ctx, q, qr, inters)
}

// RunFirst executes the First-shaped query: returns the first node from sqlAll
// or a typeNotFound error when the slice is empty. V is typically []*T (the
// node-slice type); the function returns the first element as *T (the N type
// parameter, which is the element type of V).
func RunFirst[N any, V ~[]N](
	ctx context.Context,
	q ent.Query,
	qc *ent.QueryContext,
	op, label string,
	inters []ent.Interceptor,
	prepareQuery func(context.Context) error,
	sqlAll func(context.Context) (V, error),
	notFoundError func(label string) error,
) (N, error) {
	nodes, err := RunAll[V](ctx, q, qc, op, inters, prepareQuery, sqlAll)
	if err != nil {
		var zero N
		return zero, err
	}
	if len(nodes) == 0 {
		var zero N
		return zero, notFoundError(label)
	}
	return nodes[0], nil
}

// RunOnly executes the Only-shaped query: returns exactly one node, or an
// error if zero (notFoundError) or multiple (notSingularError).
func RunOnly[N any, V ~[]N](
	ctx context.Context,
	q ent.Query,
	qc *ent.QueryContext,
	op, label string,
	inters []ent.Interceptor,
	prepareQuery func(context.Context) error,
	sqlAll func(context.Context) (V, error),
	notFoundError func(label string) error,
	notSingularError func(label string) error,
) (N, error) {
	nodes, err := RunAll[V](ctx, q, qc, op, inters, prepareQuery, sqlAll)
	if err != nil {
		var zero N
		return zero, err
	}
	switch len(nodes) {
	case 0:
		var zero N
		return zero, notFoundError(label)
	case 1:
		return nodes[0], nil
	default:
		var zero N
		return zero, notSingularError(label)
	}
}
```

Note on the generic signature shape: `RunFirst[N any, V ~[]N]` constrains V to be a slice of N. Callers will instantiate as `RunFirst[*User, []*User](...)` — explicit on both type parameters because Go's type inference doesn't propagate from V to N reliably in all cases. The templates in Task 7 emit the explicit instantiation.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test -count=1 ./runtime/entbuilder/...`

Expected: PASS (Task 2's + Task 3's + Task 4's tests, ~18 total).

- [ ] **Step 5: Commit**

**Re-verify branch first.** Run `pwd && git rev-parse --abbrev-ref HEAD`.

```bash
git add runtime/entbuilder/query.go runtime/entbuilder/query_run_test.go
```

```bash
git commit -m "$(cat <<'EOF'
feat(entbuilder): RunAll/RunCount/RunFirst/RunOnly terminals

Package-level generic helpers that encapsulate the
setContextOp -> prepareQuery -> interceptor-chain -> sqlAll
pattern every generated <Entity>Query.All/First/Count/etc. spells
out today.

RunAll/RunCount take (ctx, q, qc, op, inters, prepareQuery, sqlAll).
RunFirst/RunOnly layer on top of RunAll and apply the
first/only-one semantics with caller-supplied notFound /
notSingular error factories.

RunIDs and RunExist follow in Task 4b within this same commit if
template work needs them, or as a follow-up; for now RunAll +
RunCount cover the majority of terminal dispatches. The generated
template can express IDs/Exist via the existing per-entity Select
chain (no entbuilder helper needed) and via FirstID->IsNotFound
(also already exists).

Part of codegen-epic PR 4 (generic builders).
EOF
)"
```

---

## Task 5: Add `UpdateState[M]` and `RunUpdate`/`RunUpdateOne` to entbuilder (TDD)

Same pattern, smaller surface — the Update builder's terminals are `Save(ctx) (int, error)` and `Exec(ctx) error`, plus the `*UpdateOne` variant's `Save(ctx) (*T, error)`.

**Files:**
- Modify: `runtime/entbuilder/update.go`
- Create: `runtime/entbuilder/update_run_test.go`

- [ ] **Step 1: Write the failing test**

Create `runtime/entbuilder/update_run_test.go`:

```go
// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package entbuilder_test

import (
	"context"
	"errors"
	"testing"

	"entgo.io/ent"
	"entgo.io/ent/runtime/entbuilder"
	"github.com/stretchr/testify/require"
)

// fakeMutation satisfies the minimal ent.Mutation surface for these tests.
type fakeMutation struct{ op ent.Op }

func (m *fakeMutation) Op() ent.Op                                  { return m.op }
func (m *fakeMutation) Type() string                                { return "User" }
func (m *fakeMutation) Fields() []string                            { return nil }
func (m *fakeMutation) Field(string) (ent.Value, bool)              { return nil, false }
func (m *fakeMutation) SetField(string, ent.Value) error            { return nil }
func (m *fakeMutation) AddedFields() []string                       { return nil }
func (m *fakeMutation) AddedField(string) (ent.Value, bool)         { return nil, false }
func (m *fakeMutation) AddField(string, ent.Value) error            { return nil }
func (m *fakeMutation) ClearedFields() []string                     { return nil }
func (m *fakeMutation) FieldCleared(string) bool                    { return false }
func (m *fakeMutation) ClearField(string) error                     { return nil }
func (m *fakeMutation) ResetField(string)                           {}
func (m *fakeMutation) AddedEdges() []string                        { return nil }
func (m *fakeMutation) AddedIDs(string) []ent.Value                 { return nil }
func (m *fakeMutation) RemovedEdges() []string                      { return nil }
func (m *fakeMutation) RemovedIDs(string) []ent.Value               { return nil }
func (m *fakeMutation) ClearedEdges() []string                      { return nil }
func (m *fakeMutation) EdgeCleared(string) bool                     { return false }
func (m *fakeMutation) ClearEdge(string) error                      { return nil }
func (m *fakeMutation) ResetEdge(string) error                      { return nil }
func (m *fakeMutation) WhereP(...func(any))                         {}

func TestRunUpdate_NoHooks_CallsSqlSave(t *testing.T) {
	state := &entbuilder.UpdateState[*fakeMutation]{
		Mutation: &fakeMutation{op: ent.OpUpdate},
	}
	called := false
	sqlSave := func(context.Context) (int, error) {
		called = true
		return 3, nil
	}
	n, err := entbuilder.RunUpdate(context.Background(), state, sqlSave)
	require.NoError(t, err)
	require.Equal(t, 3, n)
	require.True(t, called)
}

func TestRunUpdate_SqlSaveError_Propagates(t *testing.T) {
	state := &entbuilder.UpdateState[*fakeMutation]{
		Mutation: &fakeMutation{op: ent.OpUpdate},
	}
	want := errors.New("db locked")
	sqlSave := func(context.Context) (int, error) { return 0, want }
	n, err := entbuilder.RunUpdate(context.Background(), state, sqlSave)
	require.ErrorIs(t, err, want)
	require.Equal(t, 0, n)
}

func TestRunUpdateOne_NoHooks_CallsSqlSave(t *testing.T) {
	state := &entbuilder.UpdateState[*fakeMutation]{
		Mutation: &fakeMutation{op: ent.OpUpdateOne},
	}
	want := &fakeUser{Name: "updated"}
	sqlSave := func(context.Context) (*fakeUser, error) { return want, nil }
	got, err := entbuilder.RunUpdateOne[*fakeUser](context.Background(), state, sqlSave)
	require.NoError(t, err)
	require.Same(t, want, got)
}
```

**Note on the hook chain:** The existing `entc/integration/privacy/ent/ent.go:WithHooks` generic chains hooks BEFORE calling sqlSave. Our `RunUpdate` mirrors that — when `state.Hooks` is non-empty, it builds the same hook chain (mutate-func → mutate via hooks → final sqlSave). For TDD simplicity, the tests above use the no-hooks path. A more elaborate hook-chain test can be added if needed during code review.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test -count=1 -run 'TestRunUpdate' ./runtime/entbuilder/...`

Expected: build failure — `undefined: entbuilder.UpdateState, entbuilder.RunUpdate, entbuilder.RunUpdateOne`.

- [ ] **Step 3: Add to `runtime/entbuilder/update.go`**

Append to `runtime/entbuilder/update.go` (after the existing types):

```go
// UpdateState holds the generic state every generated <Entity>Update and
// <Entity>UpdateOne builder carries. Generated builders embed this and
// delegate Save/Exec to RunUpdate/RunUpdateOne.
//
// M is the entity's mutation pointer type, e.g., *UserMutation.
type UpdateState[M any] struct {
	Hooks    []ent.Hook
	Mutation M
}

// RunUpdate executes the Save-shaped terminal for *<Entity>Update builders.
// Returns the number of affected rows.
//
// sqlSave is the per-entity SQL execution; it receives the prepared ctx.
//
// When state.Hooks is non-empty, the hook chain (mutate-func → hooks → final
// sqlSave) is built and invoked. The chain mirrors the existing
// per-entity-package WithHooks helper that ent codegen has emitted for years.
func RunUpdate[M ent.Mutation](
	ctx context.Context,
	state *UpdateState[M],
	sqlSave func(context.Context) (int, error),
) (int, error) {
	return runMutate[int, M](ctx, state.Hooks, state.Mutation, sqlSave)
}

// RunUpdateOne executes the Save-shaped terminal for *<Entity>UpdateOne builders.
// Returns the updated entity.
func RunUpdateOne[T any, M ent.Mutation](
	ctx context.Context,
	state *UpdateState[M],
	sqlSave func(context.Context) (*T, error),
) (*T, error) {
	return runMutate[*T, M](ctx, state.Hooks, state.Mutation, sqlSave)
}

// runMutate is the shared hook-chaining mechanic. Mirrors the per-entity
// WithHooks helper that ent codegen has emitted: builds the chain in reverse
// (so the first registered hook wraps outermost), then invokes it.
func runMutate[V ent.Value, M ent.Mutation](
	ctx context.Context,
	hooks []ent.Hook,
	mutation M,
	exec func(context.Context) (V, error),
) (V, error) {
	var mut ent.Mutator = ent.MutateFunc(func(ctx context.Context, m ent.Mutation) (ent.Value, error) {
		return exec(ctx)
	})
	for i := len(hooks) - 1; i >= 0; i-- {
		mut = hooks[i](mut)
	}
	rv, err := mut.Mutate(ctx, mutation)
	if err != nil {
		var zero V
		return zero, err
	}
	v, ok := rv.(V)
	if !ok {
		var zero V
		return zero, fmt.Errorf("entbuilder: unexpected mutation result type %T", rv)
	}
	return v, nil
}
```

The `runMutate` helper requires `ent.Mutator` and `ent.MutateFunc` — these are exported from the upstream `entgo.io/ent` package; verify with `grep "type Mutator\|^func.*MutateFunc" ent.go`. If they're called `Mutate` instead of `Mutator`, adjust accordingly.

You'll also need to add `entgo.io/ent` and `fmt` to the imports of `runtime/entbuilder/update.go` if not already there.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test -count=1 ./runtime/entbuilder/...`

Expected: PASS.

- [ ] **Step 5: Commit**

**Re-verify branch first.** Run `pwd && git rev-parse --abbrev-ref HEAD`.

```bash
git add runtime/entbuilder/update.go runtime/entbuilder/update_run_test.go
```

```bash
git commit -m "$(cat <<'EOF'
feat(entbuilder): UpdateState[M] + RunUpdate/RunUpdateOne

UpdateState[M] is embedded into generated <Entity>Update /
<Entity>UpdateOne builders in PR 4 Task 7. Holds Hooks and the
typed Mutation.

RunUpdate runs the Save terminal returning row count;
RunUpdateOne returns the updated entity. Both delegate to
runMutate which chains hooks in reverse order and invokes the
final sqlSave — mirrors the per-entity-package WithHooks helper
that ent codegen has emitted for years.

Part of codegen-epic PR 4 (generic builders).
EOF
)"
```

---

## Task 6: Add `DeleteState[M]` and `RunDelete` to entbuilder (TDD)

Smallest of the three — Delete has only `Exec(ctx) (int, error)` and `ExecX(ctx) int`.

**Files:**
- Modify: `runtime/entbuilder/delete.go`
- Create: `runtime/entbuilder/delete_run_test.go`

- [ ] **Step 1: Write the failing test**

Create `runtime/entbuilder/delete_run_test.go`:

```go
// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package entbuilder_test

import (
	"context"
	"testing"

	"entgo.io/ent"
	"entgo.io/ent/runtime/entbuilder"
	"github.com/stretchr/testify/require"
)

func TestRunDelete_NoHooks_CallsSqlExec(t *testing.T) {
	state := &entbuilder.DeleteState[*fakeMutation]{
		Mutation: &fakeMutation{op: ent.OpDelete},
	}
	called := false
	sqlExec := func(context.Context) (int, error) {
		called = true
		return 5, nil
	}
	n, err := entbuilder.RunDelete(context.Background(), state, sqlExec)
	require.NoError(t, err)
	require.Equal(t, 5, n)
	require.True(t, called)
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -count=1 -run 'TestRunDelete' ./runtime/entbuilder/...`

Expected: build failure — `undefined: entbuilder.DeleteState, entbuilder.RunDelete`.

- [ ] **Step 3: Add to `runtime/entbuilder/delete.go`**

Append:

```go
// DeleteState holds the generic state every generated <Entity>Delete and
// <Entity>DeleteOne builder carries.
type DeleteState[M any] struct {
	Hooks    []ent.Hook
	Mutation M
}

// RunDelete executes the Exec-shaped terminal for *<Entity>Delete builders.
func RunDelete[M ent.Mutation](
	ctx context.Context,
	state *DeleteState[M],
	sqlExec func(context.Context) (int, error),
) (int, error) {
	return runMutate[int, M](ctx, state.Hooks, state.Mutation, sqlExec)
}
```

(`runMutate` was defined in Task 5's `runtime/entbuilder/update.go`. It's a package-internal helper shared across update.go and delete.go.)

You may also need to add `entgo.io/ent` to the imports of `runtime/entbuilder/delete.go` if not already there.

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test -count=1 ./runtime/entbuilder/...`

Expected: PASS.

- [ ] **Step 5: Commit**

**Re-verify branch first.** Run `pwd && git rev-parse --abbrev-ref HEAD`.

```bash
git add runtime/entbuilder/delete.go runtime/entbuilder/delete_run_test.go
```

```bash
git commit -m "$(cat <<'EOF'
feat(entbuilder): DeleteState[M] + RunDelete

Mirror of UpdateState/RunUpdate for *<Entity>Delete builders. Wraps
the per-entity sqlExec function with the shared runMutate hook
chain.

Part of codegen-epic PR 4 (generic builders).
EOF
)"
```

---

## Task 7: Modify `builder/query.tmpl` to use the new shape

Embed `entbuilder.QueryState[predicate.<Entity>]` into the generated `<Entity>Query` struct. Delegate chaining methods to embedded state. Replace terminal-method bodies with 1-2 line calls to `entbuilder.Run*`.

**Files:**
- Modify: `entc/gen/template/builder/query.tmpl`
- Possibly: `entc/gen/template/import.tmpl` (if the `entbuilder` import isn't picked up by the existing `CreateUsesEntbuilder` flag)

- [ ] **Step 1: Read the current query.tmpl carefully**

Run: `cat entc/gen/template/builder/query.tmpl | head -100`

Then: `wc -l entc/gen/template/builder/query.tmpl`

You're orienting on:
- Where the `<Entity>Query` struct is declared
- Where the chaining methods (`Where`, `Limit`, `Offset`, `Unique`, `Order`, `Clone`) are emitted
- Where the terminal methods (`First`, `Only`, `All`, `IDs`, `Count`, `Exist` + X variants) are emitted
- Where the SQL-layer methods (`prepareQuery`, `sqlAll`, etc.) are emitted

- [ ] **Step 2: Change the `<Entity>Query` struct emission to embed `QueryState[predicate.<Entity>]`**

Find the lines that emit:

```
type {{ $.Name }}Query struct {
    Config
    ctx        *QueryContext
    order      []{{ ... }}.OrderOption
    inters     []Interceptor
    predicates []predicate.{{ $.Name }}
    {{ range $e := $.Edges }}
    with{{ $e.StructField }}  *{{ $e.Type.Name }}Query
    {{ end }}
    sql  *sql.Selector
    path func(context.Context) (*sql.Selector, error)
}
```

Replace the generic-state fields with the embedded `QueryState`. New shape:

```
type {{ $.Name }}Query struct {
    Config
    entbuilder.QueryState[predicate.{{ $.Name }}]
    order []{{ ... }}.OrderOption
    {{ range $e := $.Edges }}
    with{{ $e.StructField }}  *{{ $e.Type.Name }}Query
    {{ end }}
}
```

The previously-field accesses `_q.ctx`, `_q.inters`, `_q.predicates`, `_q.sql`, `_q.path` now flow through method promotion: `_q.Ctx`, `_q.Inters`, `_q.Predicates`, `_q.Sql`, `_q.Path` (note: field name capitalization changed because the embedded struct's fields are exported).

The implementer must update **every** reference in the template that uses the old field name. Search:

```bash
grep -n "_q\.\(ctx\|inters\|predicates\|sql\|path\)" entc/gen/template/builder/query.tmpl
```

For each hit, decide whether the field is now `_q.Ctx`, `_q.Inters`, etc. (via the embedded promotion) or `_q.QueryState.X` (explicit). The promoted form is shorter — use it everywhere except when there'd be a name collision with an outer field.

- [ ] **Step 3: Change chaining-method emission to delegate to QueryState helpers**

Find the emissions for `Where`, `Limit`, `Offset`, `Unique`. Each is currently a 3-4 line function. Replace each with a 2-line delegate:

```
// Where adds a new predicate for the {{ $.Name }}Query builder.
func (_q *{{ $.Name }}Query) Where(ps ...predicate.{{ $.Name }}) *{{ $.Name }}Query {
    _q.QueryState.AddPredicates(ps...); return _q
}

// Limit the number of records to be returned by this query.
func (_q *{{ $.Name }}Query) Limit(limit int) *{{ $.Name }}Query {
    _q.QueryState.SetLimit(limit); return _q
}

// Offset to start from.
func (_q *{{ $.Name }}Query) Offset(offset int) *{{ $.Name }}Query {
    _q.QueryState.SetOffset(offset); return _q
}

// Unique configures the query builder to filter duplicate records.
func (_q *{{ $.Name }}Query) Unique(unique bool) *{{ $.Name }}Query {
    _q.QueryState.SetUnique(unique); return _q
}
```

`Order` and `Clone` are entity-specific (they reference per-entity types) — keep their existing emission but adjust internal field accesses to use the new promoted field names.

For `Clone()` specifically, the body changes to:

```
return &{{ $.Name }}Query{
    Config: _q.Config,
    QueryState: *_q.QueryState.Clone(),
    order: append([]{{ $.Package }}.OrderOption{}, _q.order...),
    {{ range $e := $.Edges }}
    with{{ $e.StructField }}: _q.with{{ $e.StructField }}.Clone(),
    {{ end }}
}
```

- [ ] **Step 4: Change terminal-method emission to call `entbuilder.Run*`**

Find the emissions for `First`, `Only`, `All`, `IDs`, `Count`, `Exist`. Each is currently a 4-5 line function body.

Replace `All` with:

```
// All executes the query and returns a list of {{ $.Name }}s.
func (_q *{{ $.Name }}Query) All(ctx context.Context) ([]*{{ $.Name }}, error) {
    return entbuilder.RunAll[[]*{{ $.Name }}](ctx, _q, _q.Ctx, ent.OpQueryAll, _q.Inters, _q.prepareQuery, _q.sqlAll)
}
```

Replace `Count` with:

```
// Count returns the count of the given query.
func (_q *{{ $.Name }}Query) Count(ctx context.Context) (int, error) {
    return entbuilder.RunCount(ctx, _q, _q.Ctx, ent.OpQueryCount, _q.Inters, _q.prepareQuery, _q.sqlCount)
}
```

Replace `First` with:

```
// First returns the first {{ $.Name }} entity from the query.
func (_q *{{ $.Name }}Query) First(ctx context.Context) (*{{ $.Name }}, error) {
    return entbuilder.RunFirst[*{{ $.Name }}, []*{{ $.Name }}](ctx, _q, _q.Ctx, ent.OpQueryFirst, "{{ $.Name }}", _q.Inters, _q.prepareQuery, _q.sqlAll, func(label string) error { return &NotFoundError{label} })
}
```

Replace `Only` with:

```
// Only returns the only {{ $.Name }} entity in the query, returning an error
// if not exactly one entity is returned.
func (_q *{{ $.Name }}Query) Only(ctx context.Context) (*{{ $.Name }}, error) {
    return entbuilder.RunOnly[*{{ $.Name }}, []*{{ $.Name }}](ctx, _q, _q.Ctx, ent.OpQueryOnly, "{{ $.Name }}", _q.Inters, _q.prepareQuery, _q.sqlAll, func(label string) error { return &NotFoundError{label} }, func(label string) error { return &NotSingularError{label} })
}
```

The X-variants (`AllX`, `FirstX`, etc.) keep their existing emission — they just call the non-X version and panic on error. No change.

`FirstID`, `OnlyID`, `IDs`, `Exist` keep their existing emission for now — they use `_q.Select(...).Scan(...)` or `_q.FirstID(...)` which can't be cleanly factored into entbuilder without entity-specific Select/Scan plumbing. Leave them in the per-entity template emission, just update field references to use the promoted names (`_q.Ctx` etc.).

- [ ] **Step 5: Add the `entbuilder` import**

The existing `import.tmpl` adds `entgo.io/ent/runtime/entbuilder` conditionally via `Scope.CreateUsesEntbuilder` (or `Update`/`Delete` variants). Check if the query.tmpl sets one of those Scope flags via `extend` (search via `grep -n 'extend.*Entbuilder' entc/gen/template/builder/query.tmpl`).

If yes, you may already be covered — verify the import lands in the spot-check at Step 7.

If no, either:
- Add a new Scope flag `QueryUsesEntbuilder` and set it via `extend` at the top of `query` template, then add a parallel conditional import in `import.tmpl`
- Or piggy-back: add `(eq $.Storage.Name "sql")` to the existing `CreateUsesEntbuilder` (since SQL storage ALWAYS uses entbuilder now via query terminals)

The simpler option: add `QueryUsesEntbuilder` Scope flag set unconditionally to `eq $.Storage.Name "sql"` at the top of the query template's body (similar to PR 3 Task 6's `WhereUsesWherePkg`). Then in `import.tmpl`, replace the existing entbuilder import condition with:

```
{{- if and (hasField $ "Scope") (or ($.Scope.CreateUsesEntbuilder) ($.Scope.UpdateUsesEntbuilder) ($.Scope.DeleteUsesEntbuilder) ($.Scope.QueryUsesEntbuilder)) }}
    "entgo.io/ent/runtime/entbuilder"
{{- end }}
```

- [ ] **Step 6: Sanity-check template parsing**

Run: `go test -count=1 ./entc/gen/...`

Expected: PASS (fixture-snapshot drift may appear in Task 9 when fixtures regenerate; `./entc/gen/...` tests themselves don't check fixture content per PR 3 Task 5 precedent). What you're checking is no parse errors / build errors / panics.

If you see anything other than fixture-snapshot warnings, STOP and fix the template syntax.

- [ ] **Step 7: Spot-check a single fixture's output**

Run a manual regen on one fixture and inspect:

```bash
cd entc/integration/privacy
go generate ./...
cd /var/home/smoothbrain/dev/matthewsreis/ent/.claude/worktrees/wiggly-singing-pancake
```

Then:

```bash
sed -n '1,60p' entc/integration/privacy/ent/user_query.go
sed -n '120,180p' entc/integration/privacy/ent/user_query.go
wc -l entc/integration/privacy/ent/user_query.go
gofmt -d entc/integration/privacy/ent/user_query.go
```

Expected:
- The `UserQuery` struct embeds `entbuilder.QueryState[predicate.User]`
- `Where`, `Limit`, `Offset`, `Unique` are 2-line delegates
- `All`, `Count`, `First`, `Only` are 1-2 line calls to `entbuilder.Run*`
- `gofmt -d` produces no output (file is correctly formatted)
- LOC dropped meaningfully from the baseline 743 (target: ~500-580 LOC after the collapse)
- `import "entgo.io/ent/runtime/entbuilder"` is in the import set

If output is malformed, debug the template before committing. Then restore the fixture changes (Task 9 will redo them properly):

```bash
git checkout -- entc/integration/privacy/
```

- [ ] **Step 8: Commit**

**Re-verify branch first.** Run `pwd && git rev-parse --abbrev-ref HEAD`.

```bash
git add entc/gen/template/builder/query.tmpl entc/gen/template/import.tmpl
```

```bash
git commit -m "$(cat <<'EOF'
feat(entc/gen): generic <Entity>Query via embedded QueryState

Generated <Entity>Query now embeds entbuilder.QueryState[predicate.X]
and delegates chaining methods (Where/Limit/Offset/Unique) to embedded
helpers. Terminal methods (All/Count/First/Only) call package-level
entbuilder.Run* generic helpers.

Entity-specific bits unchanged:
- Order method (references per-entity OrderOption type)
- Edge methods (QueryX, WithX, loadX — reference per-entity edge types)
- prepareQuery, sqlAll, sqlCount, querySpec, sqlQuery (use per-entity
  column constants and table name)
- IDs, Exist, FirstID, OnlyID (compose with Select/IsNotFound, no
  clean entbuilder seam)
- GroupBy, Select, Aggregate (return per-entity types; deferred to
  potential PR 4b)

Per-entity _query.go LOC drops from ~743 to ~500-580 (specific number
captured in PR 4 bench in Task 10).

Adds Scope.QueryUsesEntbuilder flag to drive the conditional
entgo.io/ent/runtime/entbuilder import in import.tmpl.

Fixture regeneration follows in Task 9.

Part of codegen-epic PR 4 (generic builders).
EOF
)"
```

---

## Task 8: Modify `builder/update.tmpl` and `builder/delete.tmpl`

Same pattern as Task 7 but for the Update and Delete builders. Embed `entbuilder.UpdateState[*<Entity>Mutation]` / `entbuilder.DeleteState[*<Entity>Mutation]`, delegate terminal `Save`/`Exec` to `entbuilder.RunUpdate`/`RunUpdateOne`/`RunDelete`.

**Files:**
- Modify: `entc/gen/template/builder/update.tmpl`
- Modify: `entc/gen/template/builder/delete.tmpl`

- [ ] **Step 1: Read the current update.tmpl and delete.tmpl**

```bash
wc -l entc/gen/template/builder/update.tmpl entc/gen/template/builder/delete.tmpl
head -60 entc/gen/template/builder/update.tmpl
head -60 entc/gen/template/builder/delete.tmpl
```

Identify:
- Where `<Entity>Update` and `<Entity>UpdateOne` structs are emitted (look for `type {{ $.Name }}Update struct`)
- Where `<Entity>Delete` and `<Entity>DeleteOne` structs are emitted
- Where the `Save` (Update/UpdateOne) and `Exec` (Delete) terminal methods are emitted
- Where the `hooks`, `mutation` fields are used

- [ ] **Step 2: Change `<Entity>Update` struct to embed `UpdateState`**

Find:

```
type {{ $.Name }}Update struct {
    Config
    hooks    []Hook
    mutation *{{ $.Name }}Mutation
}
```

Replace with:

```
type {{ $.Name }}Update struct {
    Config
    entbuilder.UpdateState[*{{ $.Name }}Mutation]
}
```

Then update field references throughout the template:
- `_u.hooks` → `_u.Hooks`
- `_u.mutation` → `_u.Mutation`

Use `grep -n "_u\.\(hooks\|mutation\)" entc/gen/template/builder/update.tmpl` to find every occurrence.

Apply the same change to `<Entity>UpdateOne` (separate struct in the same template).

- [ ] **Step 3: Change `Save` terminal to call `entbuilder.RunUpdate*`**

For `<Entity>Update.Save`, find:

```
func (_u *{{ $.Name }}Update) Save(ctx context.Context) (int, error) {
    return WithHooks(ctx, _u.sqlSave, _u.mutation, _u.hooks)
}
```

Replace with:

```
func (_u *{{ $.Name }}Update) Save(ctx context.Context) (int, error) {
    return entbuilder.RunUpdate(ctx, &_u.UpdateState, _u.sqlSave)
}
```

For `<Entity>UpdateOne.Save`:

```
func (_u *{{ $.Name }}UpdateOne) Save(ctx context.Context) (*{{ $.Name }}, error) {
    return entbuilder.RunUpdateOne[{{ $.Name }}](ctx, &_u.UpdateState, _u.sqlSave)
}
```

(Note: pass `&_u.UpdateState` to take the address of the embedded state.)

The X-variants (`SaveX`, `ExecX`) and `Exec` keep their existing emission.

- [ ] **Step 4: Same shape for `<Entity>Delete`/`<Entity>DeleteOne`**

In `entc/gen/template/builder/delete.tmpl`:

```
type {{ $.Name }}Delete struct {
    Config
    entbuilder.DeleteState[*{{ $.Name }}Mutation]
}

func (_d *{{ $.Name }}Delete) Exec(ctx context.Context) (int, error) {
    return entbuilder.RunDelete(ctx, &_d.DeleteState, _d.sqlExec)
}
```

Update field references: `_d.hooks` → `_d.Hooks`, `_d.mutation` → `_d.Mutation`.

`<Entity>DeleteOne.Exec` calls through to `<Entity>Delete.Exec` via the inner field — that pattern doesn't change.

- [ ] **Step 5: Sanity-check template parsing**

Run: `go test -count=1 ./entc/gen/...`

Expected: PASS (no parse errors).

- [ ] **Step 6: Spot-check generated output (optional but recommended)**

```bash
cd entc/integration/privacy
go generate ./...
cd /var/home/smoothbrain/dev/matthewsreis/ent/.claude/worktrees/wiggly-singing-pancake
sed -n '1,40p' entc/integration/privacy/ent/user/update.go
sed -n '1,40p' entc/integration/privacy/ent/user/delete.go
gofmt -d entc/integration/privacy/ent/user/update.go entc/integration/privacy/ent/user/delete.go
git checkout -- entc/integration/
```

Expected:
- `UserUpdate` embeds `entbuilder.UpdateState[*UserMutation]`
- `Save` is a 1-line delegate to `entbuilder.RunUpdate`
- `UserDelete` embeds `entbuilder.DeleteState[*UserMutation]`
- `Exec` is a 1-line delegate to `entbuilder.RunDelete`
- `gofmt -d` is silent

- [ ] **Step 7: Commit**

**Re-verify branch first.** Run `pwd && git rev-parse --abbrev-ref HEAD`.

```bash
git add entc/gen/template/builder/update.tmpl entc/gen/template/builder/delete.tmpl
```

```bash
git commit -m "$(cat <<'EOF'
feat(entc/gen): generic <Entity>Update/Delete via embedded state

Generated <Entity>Update / <Entity>UpdateOne / <Entity>Delete /
<Entity>DeleteOne now embed entbuilder.UpdateState[*<Entity>Mutation]
or entbuilder.DeleteState[*<Entity>Mutation] and delegate Save/Exec
to entbuilder.RunUpdate / RunUpdateOne / RunDelete.

Entity-specific bits unchanged:
- SetX/AddX field-setter methods (drive mutation, can't be generic
  without attacking the mutation level — PR 5's scope)
- sqlSave, sqlExec (use per-entity table name + column constants)
- Where methods (forward to embedded mutation)

Fixture regeneration follows in Task 9.

Part of codegen-epic PR 4 (generic builders).
EOF
)"
```

---

## Task 9: Regenerate fixtures and verify

Same shape as PR 3 Task 7. The template changes in Tasks 7-8 have stale-bited every generated `<entity>_query.go`, `<entity>/update.go`, `<entity>/delete.go`. Regenerate and verify nothing broke.

- [ ] **Step 1: Regenerate**

From worktree root:

```bash
go generate ./entc/integration/...
```

Watch for any non-zero exit. If a fixture fails individually, run it alone:

```bash
cd entc/integration/<failing-fixture>
go generate ./...
cd /var/home/smoothbrain/dev/matthewsreis/ent/.claude/worktrees/wiggly-singing-pancake
```

- [ ] **Step 2: Inspect the diff for sanity**

```bash
git diff --stat | head -30
git diff entc/integration/privacy/ent/user_query.go | head -120
git diff entc/integration/privacy/ent/user/update.go | head -80
git diff entc/integration/privacy/ent/user/delete.go | head -50
```

Expected:
- Many `_query.go`, `update.go`, `delete.go` files shrunk
- `gofmt`-clean output everywhere
- No diff in files OUTSIDE `entc/integration/` (or in `entc/gen/internal/` snapshot fixtures, if any)

- [ ] **Step 3: Run the headline integration tests**

```bash
go test -count=1 ./entc/integration/privacy/...
go test -count=1 ./entc/integration/hooks/...
go test -count=1 ./entc/integration/edgeschema/...
```

All three must PASS. These are the bench-baseline fixtures and they exercise hook ordering, edge loading, and predicate dispatch — exactly the surface PR 4 changes. If any fail, capture the failing test name and any diff output, and STOP / report BLOCKED.

- [ ] **Step 4: Run the broader entc test suite**

```bash
go test -count=1 ./entc/... 2>&1 | tee /tmp/entc-test-pr4-task9.log
```

Expected: PASS modulo pre-existing gremlins documented in spec §11 (the `entc/integration/customid/ent/intsid_query.go:697` vet warning surfaced after PR 3 regen — still pre-existing, won't fix here).

- [ ] **Step 5: `go vet` and `go build`**

```bash
go vet ./...
go build ./...
```

Both must complete cleanly. If `go build` flags an unused-import warning in any regenerated file (`Interceptor` not used, `Hook` not used, etc.), fix the template's emission logic from Task 7 or Task 8 and regenerate.

- [ ] **Step 6: Commit fixtures**

**Re-verify branch first.** Run `pwd && git rev-parse --abbrev-ref HEAD`. Both must return the worktree path and `worktree-wiggly-singing-pancake`.

Examine what's staged:

```bash
git status --short
```

All changed files should be under `entc/integration/` or `entc/gen/internal/`. If anything else is dirty, investigate before committing.

```bash
git add entc/integration/ entc/gen/internal/ 2>/dev/null || true
git status --short  # verify staged set
```

```bash
git commit -m "$(cat <<'EOF'
chore(entc/integration): regenerate fixtures for generic builders

Bulk fixture regeneration following the template changes for:
- <Entity>Query embedding entbuilder.QueryState[predicate.X]
  (PR 4 Task 7)
- <Entity>Update / <Entity>UpdateOne embedding
  entbuilder.UpdateState[*<Entity>Mutation] (PR 4 Task 8)
- <Entity>Delete / <Entity>DeleteOne embedding
  entbuilder.DeleteState[*<Entity>Mutation] (PR 4 Task 8)

No template logic changes here; all diffs are mechanical
`go generate ./entc/integration/...` output.

Part of codegen-epic PR 4 (generic builders).
EOF
)"
```

---

## Task 10: Bench, epic spec update, final verification

- [ ] **Step 1: Run bench**

From worktree root:

```bash
go run ./cmd/bench-codegen -out internal/bench/pr4.jsonl
```

- [ ] **Step 2: Diff against baseline + PR 3**

```bash
echo "=== baseline.jsonl ==="
jq -c '{fixture, total_loc, gen_wall_ms:(.gen_wall_ns/1000000|round)}' internal/bench/baseline.jsonl
echo "=== pr3.jsonl ==="
jq -c '{fixture, total_loc, gen_wall_ms:(.gen_wall_ns/1000000|round)}' internal/bench/pr3.jsonl
echo "=== pr4.jsonl ==="
jq -c '{fixture, total_loc, gen_wall_ms:(.gen_wall_ns/1000000|round)}' internal/bench/pr4.jsonl
```

Capture the LOC deltas per fixture. The PR 4 row should show further reduction vs. PR 3 (additional terminal-collapse savings).

Spot-check per-file `_query.go` / `_update.go` / `_delete.go` reductions:

```bash
echo "=== pr3 _query.go top sizes ==="
jq -r '.top_files[] | select(.path | test("_query\\.go$")) | "\(.loc) \(.path|split("/")|.[-2:]|join("/"))"' internal/bench/pr3.jsonl
echo "=== pr4 _query.go top sizes ==="
jq -r '.top_files[] | select(.path | test("_query\\.go$")) | "\(.loc) \(.path|split("/")|.[-2:]|join("/"))"' internal/bench/pr4.jsonl
```

- [ ] **Step 3: Commit bench output**

**Re-verify branch first.** Run `pwd && git rev-parse --abbrev-ref HEAD`.

Replace the placeholder numbers in the commit message below with your ACTUAL measured numbers:

```bash
git add internal/bench/pr4.jsonl
```

```bash
git commit -m "$(cat <<EOF
bench(internal/bench): PR 4 measurement vs. PR 3

Captures total_loc, gen_wall_ns, build_wall_ns for the three in-repo
fixtures after the generic builder template changes.

Aggregate fixture LOC deltas (vs. pr3.jsonl):
<INSERT YOUR ACTUAL fixture: delta% numbers HERE>

Per-file _query.go LOC dropped <ACTUAL> on average; _update.go and
_delete.go also smaller (specific numbers in jsonl top_files).

Diffable against baseline.jsonl and pr3.jsonl with the jq snippet
in internal/bench/README.md.

Part of codegen-epic PR 4 (generic builders).
EOF
)"
```

(Replace the `<INSERT...>` placeholders with real numbers from Step 2.)

- [ ] **Step 4: Run headline regression suite**

```bash
go test -count=1 -run 'TestBootstrap_Skip|TestFeatureNo|TestReadOnly|TestSnapshot|TestPredicate|TestQueryState|TestRunAll|TestRunUpdate|TestRunDelete|TestSetContextOp|TestWithInterceptors' ./entc/... ./entc/internal/... ./where/... ./runtime/entbuilder/... 2>&1 | tail -40
```

Expected: PASS.

- [ ] **Step 5: Confirm master untouched**

```bash
git rev-parse master
```

Expected: `7e9d99b1435d541286a773ca128be1a1931d6cc8`. If anything else, STOP and recover per spec §11.

```bash
git remote -v
git config --get branch.worktree-wiggly-singing-pancake.remote || echo 'no upstream — good'
```

No upstream configured.

- [ ] **Step 6: Update epic spec §0 progress table**

Edit `docs/superpowers/specs/2026-05-15-ent-codegen-reduction-epic.md`. Find row 4:

```markdown
| 4 | generic builders | ⏳ next | not yet written | — |
```

Replace with:

```markdown
| 4 | generic builders | ✅ complete | `docs/superpowers/plans/2026-05-16-codegen-epic-pr4-generic-builders.md` | `<earliest-PR4-sha>` … `<latest-PR4-sha>` |
| 5 | mutation collapse (biggest LOC pull, ~355K) | ⏳ next | not yet written | — |
```

And bump the header at line 3:

```markdown
**Status:** In progress (5 of 7 PRs complete as of 2026-05-16)
```

To find the SHA range, run:

```bash
git log --oneline --reverse 58a5f07ad..HEAD | head -1  # earliest PR 4 commit (the one after PR 3's progress update)
git log --oneline -1                                   # HEAD (latest)
```

- [ ] **Step 7: Commit the epic-progress update**

**Re-verify branch first.** Run `pwd && git rev-parse --abbrev-ref HEAD`.

```bash
git add docs/superpowers/specs/2026-05-15-ent-codegen-reduction-epic.md
```

```bash
git commit -m "$(cat <<'EOF'
docs(epic): mark PR 4 complete in progress table

PR 4 (generic builders) landed locally. Updates §0 status header,
plan pointer, and commit-range entry. Marks PR 5 (mutation collapse)
as next.

Per the no-PRs-until-end-of-epic policy, nothing pushed; this is a
local-only milestone update.
EOF
)"
```

- [ ] **Step 8: Final smoke check**

```bash
git log --oneline 58a5f07ad..HEAD
```

Confirm you see the stack of ~10 PR 4 commits.

---

## Self-review checklist

Before declaring done:

- [ ] `go test -count=1 ./entc/... ./where/... ./runtime/entbuilder/...` green
- [ ] `git rev-parse master` is `7e9d99b1435d541286a773ca128be1a1931d6cc8` unchanged
- [ ] `git rev-parse --abbrev-ref HEAD` is `worktree-wiggly-singing-pancake`
- [ ] `git remote -v` and branch upstream check show no push happened
- [ ] `internal/bench/pr4.jsonl` exists alongside `baseline.jsonl` and `pr3.jsonl`
- [ ] Epic spec §0 progress table shows PR 4 = complete, PR 5 = next
- [ ] A representative generated `_query.go` (e.g., `entc/integration/privacy/ent/user_query.go`) is ≥ 15% shorter than its post-PR-3 self
