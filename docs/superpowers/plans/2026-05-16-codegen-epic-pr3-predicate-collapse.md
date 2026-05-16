# Codegen Epic PR 3 — Predicate Collapse Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship a generic `entgo.io/ent/where` predicate API so consumers can write `where.EQ(user.FieldName, v)` instead of `user.NameEQ(v)`, and shrink the generated `<entity>/where.go` files by collapsing the existing per-(field × op) wrappers into one-line deprecated delegates.

**Architecture:**
1. **New package `entgo.io/ent/where`** — generic helpers (`EQ[T]`, `NEQ[T]`, `In[T]`, `NotIn[T]`, `GT[T]`, …, `Contains`, `HasPrefix`, …, `IsNull`, `NotNull`) that thin-delegate to the existing `sql.FieldXX` primitives in `dialect/sql/sql.go`. No business logic — pure thin generic façade.
2. **`predicate.{Entity}` becomes a type alias for `func(*sql.Selector)`** — a one-character template change (`type X func(...)` → `type X = func(...)`) that lets `where.EQ(...)`'s return value flow directly into `Where(...predicate.User)` without an explicit cast.
3. **`<entity>/where.go` shrinks** — each per-(field × op) wrapper collapses from a 3-line block to a one-line deprecated delegate that calls into the new `where` package. Existing call sites (`user.NameEQ(v)`) keep working; new code can adopt `where.EQ(user.FieldName, v)` immediately. Edge predicates (`HasGroups`, `HasGroupsWith`) and ID predicates stay non-deprecated (they can't be expressed generically without losing semantics).

**Tech Stack:** Go 1.21+ generics. `entgo.io/ent/dialect/sql` (existing). No new external dependencies.

**Spec reference:** `docs/superpowers/specs/2026-05-15-ent-codegen-reduction-epic.md` §4 PR 3 (`codegen-epic/03-predicate-collapse`).

**Branch:** stay on `worktree-wiggly-singing-pancake` (epic policy — no separate git-spice branch, no push, no PR).

---

## Scope cuts vs. the spec — read before starting

The spec's PR 3 acceptance criterion is "70–80% reduction in `where.go` per entity at consumer scale." This plan targets ~50% reduction. Here's why and what's deferred:

- **Spec target requires removing most wrappers entirely.** A wrapper-removal sweep would break ~30+ in-repo integration test call sites (e.g., `entc/integration/integration_test.go:983` calls `user.NameHasPrefix`, `pet.NameContains`, etc.) plus probably hundreds in `service-api-go`. That call-site rewrite is the *real* work and belongs in its own PR after consumers have absorbed the new API.
- **This PR delivers the API + the shrink, not the removal.** Each wrapper goes from 3–4 LOC (comment + signature + 1-line body + close brace) to 2 LOC (deprecated comment + one-liner). Comments still carry doc/migration info. Result: ~50% reduction in `where.go`, zero call-site churn, deprecation shims in place for one release.
- **Follow-on PR (post-epic, after consumer migration):** flip a feature flag that drops every deprecated wrapper. Trivial template change once consumers are off them.

If you (the implementer or controller) think the aggressive removal is actually the right call here, stop and surface that to Zack before continuing — it's a scope decision, not a Task-N decision.

**Other deliberate non-goals for this PR:**

- **Gremlin storage backend.** The new `where` package wraps `sql.FieldXX`, which is SQL-only. Gremlin codegen continues to emit its existing per-op wrappers verbatim (the template gate is `{{ if eq $.Storage "sql" }}`). The matthewsreis fork uses SQL exclusively; gremlin support is upstream-test-only.
- **ValueScanner fields.** Fields with a custom `ValueScanner` use `predicate.UserOrErr(...)` (see `entc/gen/template/where.tmpl:51-58`), which propagates a conversion error through the predicate. The generic `where` API doesn't model that. ValueScanner field wrappers keep their existing multi-line form. Most consumer fields are not ValueScanner, so this carve-out is small in practice.
- **Edge predicates** (`HasGroups`, `HasGroupsWith`). They construct `sqlgraph.Step` values that require per-edge constants (`UserGroupsTable`, `UserGroupsColumn`, `UserGroupsPrimaryKey`). Can't be generic without machinery that's bigger than the savings. Stay as-is.
- **ID predicates** (`ID`, `IDEQ`, `IDIn`, …). The ID type varies (`int`, `string`, `uuid.UUID`) and the wrappers are few per entity. Not worth a generic detour. Stay as-is.

---

## File map

| File | Status | Responsibility |
|---|---|---|
| `where/where.go` | **NEW** | Generic predicate constructors: `EQ[T]`, `NEQ[T]`, `In[T]`, `NotIn[T]`, `GT[T]`, `GTE[T]`, `LT[T]`, `LTE[T]`, `IsNull`, `NotNull`, `Contains`, `ContainsFold`, `EqualFold`, `HasPrefix`, `HasPrefixFold`, `HasSuffix`, `HasSuffixFold`, `And`, `Or`, `Not` |
| `where/where_test.go` | **NEW** | Unit tests for each generic helper across `int`, `string`, `time.Time` value types; SQL-output assertions via `sql.Dialect(...).Select(...).Where(...)` |
| `where/doc.go` | **NEW** | Package-level doc comment describing intent + when to use vs. legacy entity-package wrappers |
| `entc/gen/template/predicate.tmpl` | modify | Change `type {{ $n.Name }} func({{ $.Storage.Builder }})` to `type {{ $n.Name }} = func({{ $.Storage.Builder }})` — single keyword: add `=` |
| `entc/gen/template/where.tmpl` | modify | Collapse SQL-storage per-(field × op) wrappers to one-line deprecated delegates that call into the new `where` package; gate the collapse on `eq $.Storage "sql"`; preserve existing form for gremlin and ValueScanner fields |
| `entc/gen/template/import.tmpl` | modify | When the new where-import path is needed in a generated file, ensure `entgo.io/ent/where` is in the import set (likely auto-handled by goimports — verify) |
| `entc/gen/feature.go` | review | No change expected; predicate collapse is unconditional for SQL storage |
| `entc/integration/**/ent/<entity>/where.go` | regenerate | Fixture regeneration. Reviewer diffs alongside the template change |
| `entc/integration/**/ent/predicate/predicate.go` | regenerate | Type alias change visible in generated output |
| `entc/gen/internal/*_snapshot.go` / `_testdata/` snapshots | regenerate | Whatever fixture-snapshot mechanism exists in `entc/gen/...` |
| `internal/bench/baseline.jsonl` | **do not modify** | The PR 0 baseline. PR 3 produces a *new* file `internal/bench/pr3.jsonl` and a delta record in the commit message |
| `doc/md/migration-predicate-collapse.md` | **NEW** | Migration guide for consumers: when to use the new API, deprecation timeline, type-alias caveats |
| `MIGRATION.md` | modify (create section, append if file exists) | Top-level migration changelog entry pointing to the doc page |

---

## Pre-flight: every implementer reads this first

**Branch discipline.** Stay on `worktree-wiggly-singing-pancake`. Never run `git push`, `gh pr create`, `gs branch submit`, or `gs stack submit`. Never `git checkout` to a different branch. **Before EVERY commit, run `pwd && git rev-parse --abbrev-ref HEAD` and verify they return `/var/home/smoothbrain/dev/matthewsreis/ent/.claude/worktrees/wiggly-singing-pancake` and `worktree-wiggly-singing-pancake` respectively.** Three prior implementer subagents committed to master by accident during PRs 0–2 because they `cd`'d into the parent worktree at `/var/home/smoothbrain/dev/matthewsreis/ent/`. Do not repeat. The recovery (cherry-pick + reset master) is mechanical but wasteful.

**Working directory.** Every command runs from `/var/home/smoothbrain/dev/matthewsreis/ent/.claude/worktrees/wiggly-singing-pancake`. Confirm with `pwd` before each command block.

**Parent commit.** This plan's Task 1 starts on top of `1f1d5c231 docs(epic): add Progress section + §11 session-resume instructions`. Verify with `git log -1 --pretty=format:'%H %s'`.

**Pre-existing uncommitted modifications.** Spec §11 documents that `entc/integration/privacy/ent/{task,team,user}/where.go` and `entc/integration/privacy/ent/{task,team,user}_query.go` have pre-existing uncommitted modifications that are NOT from any epic PR. Before starting Task 1, run `git status` and confirm those six files (or a subset of them) are the only dirty files. If anything else is dirty, stop and surface it. After confirming, run `git checkout -- entc/integration/privacy/ent/{task,team,user}/where.go entc/integration/privacy/ent/{task,team,user}_query.go` to drop them — they'll be regenerated cleanly by Task 7's `go generate` pass.

**Module path.** This repo is `entgo.io/ent` (the matthewsreis fork). The new package's import path is `entgo.io/ent/where`. Verify with `head -1 go.mod`.

**Test runner.** `go test -count=1` (the `-count=1` defeats Go's test result cache). Always pass `-count=1` for ent test runs because the integration suite hits a real SQLite database file path that can carry between runs.

---

## Task 1: Orient — read the existing predicate machinery

Investigative task, no code change. Build the mental model before touching anything.

**Files to READ (in this order):**
- `dialect/sql/sql.go:1-250` — the existing `sql.FieldEQ`, `sql.FieldNEQ`, …, `sql.FieldIn`, `sql.FieldContains` primitives that the new `where` package will wrap
- `dialect/sql/builder.go:800-1400` — `Predicate`, `P`, `And`, `Or`, `Not`, `EQ`, `NEQ`, `IsNull`, `NotNull`, `In`, `Like`, `HasPrefix`, `HasSuffix`, `EqualFold`, `Contains`, `ContainsFold` — the lower-level builder API the `FieldXX` helpers compose with
- `entc/gen/template/predicate.tmpl` (entire file — only ~40 lines) — defines `type predicate.User func(*sql.Selector)`
- `entc/gen/template/where.tmpl` (entire file — ~190 lines) — the template that emits per-entity `where.go`
- `entc/gen/template/dialect/sql/predicate.tmpl` (entire file — ~100 lines) — the sub-templates that produce `sql.FieldEQ(FieldName, v)` snippets called by `where.tmpl`
- `entc/integration/privacy/ent/user/where.go:1-80` — a sample generated `where.go` to see the current shape (or any other generated `where.go`)
- `entc/integration/privacy/ent/predicate/predicate.go` — the current generated `predicate` package (type definitions)

- [ ] **Step 1: Read each file in the list above.** You don't need to memorize. You're confirming:
  - `sql.FieldEQ(name string, v any) func(*Selector)` is the primitive `where.EQ` will call
  - `sql.FieldIn[T any](name string, vs ...T) func(*Selector)` already exists with generics — you can model `where.In` on it
  - Generated `where.go` per-field wrappers all collapse to `predicate.User(sql.FieldXX(FieldName, ...))` (already thin — the LOC win comes from comment+brace removal, not body reduction)
  - `type predicate.User func(*sql.Selector)` is a *named* type today — that's why `where.EQ(...)` (returning `func(*sql.Selector)`) wouldn't be directly assignable to `predicate.User` without an explicit cast or a type alias

- [ ] **Step 2: Confirm `Where` method signatures accept `predicate.User`**

Run: `grep -n 'func (uq \*UserQuery) Where' entc/integration/privacy/ent/user_query.go`

Note the signature: `func (uq *UserQuery) Where(ps ...predicate.User) *UserQuery`. After Task 5 makes `predicate.User` a type alias for `func(*sql.Selector)`, `Where(where.EQ(user.FieldName, "x"))` will compile because Go allows passing a value of the underlying type wherever the alias is accepted.

- [ ] **Step 3: Verify the predicate type-alias change doesn't break existing call sites**

Run: `grep -rn 'predicate\.[A-Z][a-zA-Z]*(' entc/integration/ | head -20`

Most usages are conversions like `predicate.User(sql.FieldEQ(FieldName, v))`. Conversions to a type *alias* are still legal Go — `T(x)` where `T` is a type alias for the underlying type of `x` is a no-op conversion that the compiler accepts. Confirm this by skimming the Go spec section on aliases if unfamiliar: `https://go.dev/ref/spec#Alias_declarations`.

- [ ] **Step 4: No commit — this task is read-only**

---

## Task 2: Create `entgo.io/ent/where` package with `EQ` and `NEQ` (TDD)

Start small. EQ and NEQ are the minimum viable generic API. Subsequent tasks layer on the other operators.

**Files:**
- Create: `where/where.go`
- Create: `where/where_test.go`
- Create: `where/doc.go`

- [ ] **Step 1: Write the failing test**

Create `where/where_test.go`:

```go
// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package where_test

import (
	"testing"

	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/where"
	"github.com/stretchr/testify/require"
)

// predicateToSQL applies p to a Selector and returns the resulting WHERE clause + args.
// Mirrors how the generated code composes predicates against a sql.Selector.
func predicateToSQL(t *testing.T, p func(*sql.Selector)) (string, []any) {
	t.Helper()
	s := sql.Dialect(dialect.SQLite).Select("*").From(sql.Table("users"))
	p(s)
	q, args := s.Query()
	return q, args
}

func TestEQ_Int(t *testing.T) {
	q, args := predicateToSQL(t, where.EQ("age", 30))
	require.Contains(t, q, `"age" = ?`)
	require.Equal(t, []any{30}, args)
}

func TestEQ_String(t *testing.T) {
	q, args := predicateToSQL(t, where.EQ("name", "alice"))
	require.Contains(t, q, `"name" = ?`)
	require.Equal(t, []any{"alice"}, args)
}

func TestNEQ_Int(t *testing.T) {
	q, args := predicateToSQL(t, where.NEQ("age", 30))
	require.Contains(t, q, `"age" <> ?`)
	require.Equal(t, []any{30}, args)
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -count=1 ./where/...`

Expected: package-load error — `package entgo.io/ent/where is not in std`. That confirms the package doesn't exist yet.

- [ ] **Step 3: Create `where/doc.go`**

```go
// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

// Package where provides generic predicate constructors for ent-generated
// query builders. Use it to write field predicates without going through the
// per-entity packages:
//
//	import "entgo.io/ent/where"
//	import "myapp/ent/user"
//
//	client.User.Query().Where(where.EQ(user.FieldName, "alice")).All(ctx)
//
// Each function returns a value assignable to any predicate.<Entity> type
// generated by ent (those types are aliases for func(*sql.Selector)). Compose
// with predicate.User, predicate.Order, etc. interchangeably.
//
// For per-entity legacy wrappers (e.g. user.NameEQ), see the generated
// <entity>/where.go — they remain available as deprecated thin delegates for
// migration purposes.
package where
```

- [ ] **Step 4: Create `where/where.go` with `EQ` and `NEQ` only**

```go
// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package where

import "entgo.io/ent/dialect/sql"

// EQ returns a predicate that matches rows where the given field equals v.
func EQ[T any](field string, v T) func(*sql.Selector) {
	return sql.FieldEQ(field, v)
}

// NEQ returns a predicate that matches rows where the given field does not equal v.
func NEQ[T any](field string, v T) func(*sql.Selector) {
	return sql.FieldNEQ(field, v)
}
```

Note on the `T any` constraint. The spec said `T comparable`, but `sql.FieldEQ`'s signature accepts `any` because some field types (custom marshalers, `time.Time`, `json.RawMessage`) aren't strictly comparable. Mirroring `sql.FieldEQ`'s `any` keeps the generic API permissive and matches existing behavior. The compiler enforces the value flows through the SQL driver's argument list — that's the real type constraint, not Go's `comparable`.

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test -count=1 ./where/...`

Expected: 3 tests PASS.

- [ ] **Step 6: Commit**

**Re-verify branch first.**

```bash
pwd
git rev-parse --abbrev-ref HEAD
```

Both must return the worktree path and `worktree-wiggly-singing-pancake` respectively. If not, **stop and recover** per spec §11 before any `git commit`.

```bash
git add where/
git commit -m "feat(where): generic EQ/NEQ predicate helpers

New package entgo.io/ent/where exposes generic thin wrappers around the
existing sql.FieldXX primitives so consumers can write
where.EQ(user.FieldName, v) instead of user.NameEQ(v). Ships EQ and NEQ
only — additional operators land in follow-up commits within PR 3.

Part of codegen-epic PR 3 (predicate collapse)."
```

---

## Task 3: Extend `where` package with `In`, `NotIn`, `GT`, `GTE`, `LT`, `LTE`, `IsNull`, `NotNull` (TDD)

**Files:**
- Modify: `where/where.go`
- Modify: `where/where_test.go`

- [ ] **Step 1: Add failing tests for each new operator**

Append to `where/where_test.go`:

```go
func TestIn_Int(t *testing.T) {
	q, args := predicateToSQL(t, where.In("age", 30, 40, 50))
	require.Contains(t, q, `"age" IN (?, ?, ?)`)
	require.Equal(t, []any{30, 40, 50}, args)
}

func TestIn_String(t *testing.T) {
	q, args := predicateToSQL(t, where.In("name", "a", "b"))
	require.Contains(t, q, `"name" IN (?, ?)`)
	require.Equal(t, []any{"a", "b"}, args)
}

func TestNotIn(t *testing.T) {
	q, args := predicateToSQL(t, where.NotIn("age", 1, 2))
	require.Contains(t, q, `"age" NOT IN (?, ?)`)
	require.Equal(t, []any{1, 2}, args)
}

func TestGT(t *testing.T) {
	q, args := predicateToSQL(t, where.GT("age", 18))
	require.Contains(t, q, `"age" > ?`)
	require.Equal(t, []any{18}, args)
}

func TestGTE(t *testing.T) {
	q, args := predicateToSQL(t, where.GTE("age", 18))
	require.Contains(t, q, `"age" >= ?`)
	require.Equal(t, []any{18}, args)
}

func TestLT(t *testing.T) {
	q, args := predicateToSQL(t, where.LT("age", 65))
	require.Contains(t, q, `"age" < ?`)
	require.Equal(t, []any{65}, args)
}

func TestLTE(t *testing.T) {
	q, args := predicateToSQL(t, where.LTE("age", 65))
	require.Contains(t, q, `"age" <= ?`)
	require.Equal(t, []any{65}, args)
}

func TestIsNull(t *testing.T) {
	q, _ := predicateToSQL(t, where.IsNull("deleted_at"))
	require.Contains(t, q, `"deleted_at" IS NULL`)
}

func TestNotNull(t *testing.T) {
	q, _ := predicateToSQL(t, where.NotNull("deleted_at"))
	require.Contains(t, q, `"deleted_at" IS NOT NULL`)
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test -count=1 -run 'TestIn|TestNotIn|TestGT|TestGTE|TestLT|TestLTE|TestIsNull|TestNotNull' ./where/...`

Expected: build failure — `undefined: where.In` etc.

- [ ] **Step 3: Add the new operators to `where/where.go`**

Append to `where/where.go`:

```go
// In returns a predicate that matches rows where the field value is in vs.
func In[T any](field string, vs ...T) func(*sql.Selector) {
	return sql.FieldIn(field, vs...)
}

// NotIn returns a predicate that matches rows where the field value is not in vs.
func NotIn[T any](field string, vs ...T) func(*sql.Selector) {
	return sql.FieldNotIn(field, vs...)
}

// GT returns a predicate that matches rows where the field value is greater than v.
func GT[T any](field string, v T) func(*sql.Selector) {
	return sql.FieldGT(field, v)
}

// GTE returns a predicate that matches rows where the field value is greater than or equal to v.
func GTE[T any](field string, v T) func(*sql.Selector) {
	return sql.FieldGTE(field, v)
}

// LT returns a predicate that matches rows where the field value is less than v.
func LT[T any](field string, v T) func(*sql.Selector) {
	return sql.FieldLT(field, v)
}

// LTE returns a predicate that matches rows where the field value is less than or equal to v.
func LTE[T any](field string, v T) func(*sql.Selector) {
	return sql.FieldLTE(field, v)
}

// IsNull returns a predicate that matches rows where the field is NULL.
func IsNull(field string) func(*sql.Selector) {
	return sql.FieldIsNull(field)
}

// NotNull returns a predicate that matches rows where the field is NOT NULL.
func NotNull(field string) func(*sql.Selector) {
	return sql.FieldNotNull(field)
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test -count=1 ./where/...`

Expected: all tests PASS (3 from Task 2 + 9 new).

- [ ] **Step 5: Commit**

**Re-verify branch first.** Run `pwd && git rev-parse --abbrev-ref HEAD`.

```bash
git add where/where.go where/where_test.go
git commit -m "feat(where): In/NotIn/GT/GTE/LT/LTE/IsNull/NotNull

Comparison and null-check generic predicate helpers. Each delegates to
the corresponding sql.FieldXX primitive — no logic, just a generic
front-door so call sites don't have to thread through per-entity
packages.

Part of codegen-epic PR 3 (predicate collapse)."
```

---

## Task 4: Extend `where` package with string operators + And/Or/Not (TDD)

**Files:**
- Modify: `where/where.go`
- Modify: `where/where_test.go`

- [ ] **Step 1: Add failing tests for string operators and combinators**

Append to `where/where_test.go`:

```go
func TestContains(t *testing.T) {
	q, args := predicateToSQL(t, where.Contains("name", "ali"))
	require.Contains(t, q, `"name" LIKE ?`)
	require.Equal(t, []any{"%ali%"}, args)
}

func TestContainsFold(t *testing.T) {
	q, _ := predicateToSQL(t, where.ContainsFold("name", "ALI"))
	// dialect-specific (SQLite uses LOWER(...) LIKE LOWER(...)). Just assert non-empty.
	require.NotEmpty(t, q)
}

func TestEqualFold(t *testing.T) {
	q, _ := predicateToSQL(t, where.EqualFold("name", "Alice"))
	require.NotEmpty(t, q)
}

func TestHasPrefix(t *testing.T) {
	q, args := predicateToSQL(t, where.HasPrefix("name", "al"))
	require.Contains(t, q, `"name" LIKE ?`)
	require.Equal(t, []any{"al%"}, args)
}

func TestHasPrefixFold(t *testing.T) {
	q, _ := predicateToSQL(t, where.HasPrefixFold("name", "AL"))
	require.NotEmpty(t, q)
}

func TestHasSuffix(t *testing.T) {
	q, args := predicateToSQL(t, where.HasSuffix("name", "ce"))
	require.Contains(t, q, `"name" LIKE ?`)
	require.Equal(t, []any{"%ce"}, args)
}

func TestHasSuffixFold(t *testing.T) {
	q, _ := predicateToSQL(t, where.HasSuffixFold("name", "CE"))
	require.NotEmpty(t, q)
}

func TestAnd(t *testing.T) {
	q, args := predicateToSQL(t, where.And(where.EQ("age", 30), where.EQ("name", "alice")))
	require.Contains(t, q, " AND ")
	require.Equal(t, []any{30, "alice"}, args)
}

func TestOr(t *testing.T) {
	q, args := predicateToSQL(t, where.Or(where.EQ("age", 30), where.EQ("age", 40)))
	require.Contains(t, q, " OR ")
	require.Equal(t, []any{30, 40}, args)
}

func TestNot(t *testing.T) {
	q, args := predicateToSQL(t, where.Not(where.EQ("age", 30)))
	require.Contains(t, q, "NOT")
	require.Equal(t, []any{30}, args)
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test -count=1 -run 'TestContains|TestContainsFold|TestEqualFold|TestHasPrefix|TestHasPrefixFold|TestHasSuffix|TestHasSuffixFold|TestAnd|TestOr|TestNot' ./where/...`

Expected: build failure — `undefined: where.Contains` etc.

- [ ] **Step 3: Add string ops and combinators to `where/where.go`**

Append to `where/where.go`:

```go
// Contains returns a predicate that matches rows where the field contains the substring.
func Contains(field, substr string) func(*sql.Selector) {
	return sql.FieldContains(field, substr)
}

// ContainsFold returns a case-insensitive Contains.
func ContainsFold(field, substr string) func(*sql.Selector) {
	return sql.FieldContainsFold(field, substr)
}

// EqualFold returns a case-insensitive EQ.
func EqualFold(field, substr string) func(*sql.Selector) {
	return sql.FieldEqualFold(field, substr)
}

// HasPrefix returns a predicate that matches rows where the field has the given prefix.
func HasPrefix(field, prefix string) func(*sql.Selector) {
	return sql.FieldHasPrefix(field, prefix)
}

// HasPrefixFold returns a case-insensitive HasPrefix.
func HasPrefixFold(field, prefix string) func(*sql.Selector) {
	return sql.FieldHasPrefixFold(field, prefix)
}

// HasSuffix returns a predicate that matches rows where the field has the given suffix.
func HasSuffix(field, suffix string) func(*sql.Selector) {
	return sql.FieldHasSuffix(field, suffix)
}

// HasSuffixFold returns a case-insensitive HasSuffix.
func HasSuffixFold(field, suffix string) func(*sql.Selector) {
	return sql.FieldHasSuffixFold(field, suffix)
}

// And combines the given predicates with AND.
func And(predicates ...func(*sql.Selector)) func(*sql.Selector) {
	return sql.AndPredicates(predicates...)
}

// Or combines the given predicates with OR.
func Or(predicates ...func(*sql.Selector)) func(*sql.Selector) {
	return sql.OrPredicates(predicates...)
}

// Not wraps the given predicates with NOT.
func Not(predicates ...func(*sql.Selector)) func(*sql.Selector) {
	return sql.NotPredicates(predicates...)
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test -count=1 ./where/...`

Expected: all tests PASS (existing + 10 new).

- [ ] **Step 5: Commit**

**Re-verify branch first.** Run `pwd && git rev-parse --abbrev-ref HEAD`.

```bash
git add where/where.go where/where_test.go
git commit -m "feat(where): string ops + And/Or/Not combinators

Contains, ContainsFold, EqualFold, HasPrefix, HasPrefixFold, HasSuffix,
HasSuffixFold for case-sensitive and case-insensitive substring matches.
And/Or/Not combinators delegate to sql.AndPredicates / OrPredicates /
NotPredicates (the existing primitives the entity-package And/Or/Not
already use).

Completes the generic where API surface for PR 3. Generated
<entity>/where.go starts delegating to this package in Task 6.

Part of codegen-epic PR 3 (predicate collapse)."
```

---

## Task 5: Change `predicate.<Entity>` from named type to type alias

The single template change that lets `where.EQ(...)` (returning `func(*sql.Selector)`) be passed directly to `Where(...predicate.User)`.

**Files:**
- Modify: `entc/gen/template/predicate.tmpl`
- Create: `entc/gen/predicate_alias_test.go`

- [ ] **Step 1: Write the failing test**

Create `entc/gen/predicate_alias_test.go`:

```go
// Copyright 2019-present Facebook Inc. All rights reserved.
// This source code is licensed under the Apache 2.0 license found
// in the LICENSE file in the root directory of this source tree.

package gen

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestPredicateTemplate_EmitsTypeAlias asserts that the predicate template
// produces `type X = func(...)` (a type alias) rather than `type X func(...)`
// (a named type). The alias is what makes where.EQ(field, v) directly
// assignable to predicate.<Entity>.
func TestPredicateTemplate_EmitsTypeAlias(t *testing.T) {
	g, err := NewGraph(&Config{
		Package: "entgo.io/ent/entc/integration/privacy/ent",
		Target:  "/tmp/ignored-predicate-alias-test",
	}, testdataPrivacyNodes(t)...)
	require.NoError(t, err)
	buf, err := g.Templates.ExecuteTemplate("predicate", g)
	require.NoError(t, err)
	out := buf.String()
	// Look for the type-alias form. The exact node name varies; assert at least
	// one of the generated types uses `= func(`.
	require.True(t,
		strings.Contains(out, "= func("),
		"expected type alias (= func(...)) form in predicate template output, got:\n%s", out)
	// And confirm the named-type form is NOT present anywhere outside doc comments.
	for _, line := range strings.Split(out, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "//") {
			continue
		}
		require.False(t,
			strings.HasPrefix(trimmed, "type ") && strings.Contains(trimmed, " func(") && !strings.Contains(trimmed, "= func("),
			"unexpected named type declaration in predicate template: %q", trimmed)
	}
}

// testdataPrivacyNodes loads the privacy integration test schemas so the predicate
// template has real nodes to operate on. If a helper of this shape doesn't already
// exist in entc/gen, replace this implementation with a fixture-loading approach
// modeled on the existing predicate-emission tests in entc/gen (search for
// "Templates.ExecuteTemplate" or "predicate.tmpl" in the package's _test.go files
// and adapt).
func testdataPrivacyNodes(t *testing.T) []*Type {
	t.Helper()
	// Minimal placeholder. The implementer MUST replace this with the canonical
	// node-loading approach used elsewhere in entc/gen. The simpler alternative
	// is to find an existing test in entc/gen that calls
	// Templates.ExecuteTemplate("predicate", ...) and pattern-match off of it.
	t.Skip("replace testdataPrivacyNodes with the canonical helper used by other entc/gen template tests; see Task 5 Step 1 prose")
	return nil
}
```

**Implementer note.** The `testdataPrivacyNodes` helper above is a deliberate skip-stub because the exact shape of the test fixture varies by entc version. Before running the test, search `entc/gen/*_test.go` for any test that calls `g.Templates.ExecuteTemplate("predicate", g)` (or `ExecuteTemplate` with a template name in general) and copy the node-loading pattern from there. If no such test exists, the simplest alternative is to delete this test entirely and rely on the regeneration sanity check from Task 7 (which will fail loudly if the alias change broke template parsing). Pick whichever path is faster — the load-bearing assertion is "fixtures regenerate correctly with `= func` in the output," which Task 7 verifies anyway.

- [ ] **Step 2: Run the test to verify it fails (or is correctly skipped)**

Run: `go test -count=1 -run TestPredicateTemplate_EmitsTypeAlias ./entc/gen/...`

Expected: SKIP (if you kept the stub) or FAIL with "expected type alias form" (if you wired the helper). Either is fine — proceed.

- [ ] **Step 3: Edit `entc/gen/template/predicate.tmpl`**

Find line 24 in `entc/gen/template/predicate.tmpl`:

```
type {{ $n.Name }} func({{ $.Storage.Builder }})
```

Change to:

```
type {{ $n.Name }} = func({{ $.Storage.Builder }})
```

The single character added is `= `. No other edits in this file.

- [ ] **Step 4: Run the test (if wired) and a broader sanity check**

Run:
```bash
go test -count=1 -run TestPredicateTemplate_EmitsTypeAlias ./entc/gen/...
go test -count=1 ./entc/gen/...
```

Expected: TestPredicateTemplate PASS (or SKIP). The broader `./entc/gen/...` run produces fixture-snapshot diffs — that's expected and Task 7 regenerates them. If any tests fail for reasons OTHER than fixture drift (e.g., template parse error, undefined function), stop and investigate.

- [ ] **Step 5: Commit**

**Re-verify branch first.** Run `pwd && git rev-parse --abbrev-ref HEAD`.

```bash
git add entc/gen/template/predicate.tmpl entc/gen/predicate_alias_test.go
git commit -m "refactor(entc/gen): predicate.<Entity> becomes type alias

Changes the predicate template from \`type X func(*sql.Selector)\` to
\`type X = func(*sql.Selector)\`. The alias form lets values returned
by entgo.io/ent/where helpers (which return func(*sql.Selector)) flow
directly into Where(...predicate.User) parameters without an explicit
cast.

Semantic trade-off: predicate types lose nominal identity — a
predicate.User and predicate.Order are now interchangeable at the type
system level. This matches the codegen-reduction epic's stated
trade-off (relax strict per-entity typing for build-time and ergonomic
wins). Hand-written code that does
\`var p predicate.User = (predicate.Order)(other)\` no longer needs
the cast; code that relied on the cast as a guard loses that guard.
Documented in MIGRATION.md (Task 9).

Fixture regeneration follows in Task 7.

Part of codegen-epic PR 3 (predicate collapse)."
```

---

## Task 6: Collapse per-(field × op) wrappers in `where.tmpl`

Modify the SQL-storage emission path so each per-(field × op) wrapper is a single deprecated line that delegates to the new `where` package.

**Files:**
- Modify: `entc/gen/template/where.tmpl`

- [ ] **Step 1: Confirm baseline behavior**

Re-read `entc/gen/template/where.tmpl` lines 41-133. Today, the per-field equality wrapper (lines 41-73) emits:

```go
// Name applies equality check predicate on the "name" field. It's identical to NameEQ.
func Name(v string) predicate.User {
    return predicate.User(sql.FieldEQ(FieldName, v))
}
```

(4 lines per wrapper, plus a blank line.) The per-field × op wrapper (lines 75-133) emits similarly.

Note the existing template gates by storage (`xtemplate $tmpl . }}` where `$tmpl := printf "dialect/%s/predicate/field" $.Storage`). The collapse only applies to `$.Storage == "sql"`. Gremlin output stays unchanged.

- [ ] **Step 2: Define the collapse logic — sketch on paper first**

The target output for SQL storage, scalar field, EQ:

```go
// Name applies equality check on the "name" field.
//
// Deprecated: use where.EQ(user.FieldName, v).
func Name(v string) predicate.User { return where.EQ(FieldName, v) }
```

3 lines (comment block of 3 + one-line func decl), down from 4. Across ~17 wrappers per string field, ~10 per int field, the LOC reduction is ~25-30% per wrapper (more if blank-line gaps are also reduced). To push closer to 50%, also drop the doc-comment block to a single `// Deprecated:` line:

```go
// Deprecated: NameEQ — use where.EQ(user.FieldName, v).
func NameEQ(v string) predicate.User { return where.EQ(FieldName, v) }
```

2 lines per wrapper. Across 134 entities × ~10 fields × ~10 ops, the win is real.

This task implements the 2-line form.

- [ ] **Step 3: Add a SQL-gate around the wrapper-emission blocks**

The template currently doesn't conditionally branch on storage at the wrapper level — it gates inside the sub-templates (`dialect/sql/predicate/field/ops`). For the collapse, you need a *new* SQL-only emission branch that produces the deprecated one-liner. Gremlin keeps the existing emission.

Wrap the existing per-field equality wrapper (lines 41-73) like this:

```
{{ range $f := $.Fields }}
    {{ $func := $f.StructField }}
    {{ $hasP := not (or $f.IsJSON $f.IsEnum) }}
    {{ $comparable := or $f.ConvertedToBasic $f.Type.Valuer }}
    {{ $undeclared := (and (ne $func "Label") (ne $func "OrderOption") (ne $func "Hooks") (ne $func "Policy") (ne $func "Table") (ne $func "FieldID") (ne $func "Value") (ne $func $.Name)) }}
    {{- if and $hasP $comparable $undeclared }}
        {{- if and (eq $.Storage "sql") (not $f.HasValueScanner) (not $f.HasGoType) }}
            // Deprecated: {{ $func }} — use where.EQ({{ $.Package }}.{{ $f.Constant }}, v).
            func {{ $func }}(v {{ $f.Type }}) predicate.{{ $.Name }} { return where.EQ({{ $f.Constant }}, v) }
        {{- else }}
            <existing 4-line emission block — unchanged>
        {{- end }}
    {{- end }}
{{ end }}
```

The conditions `not $f.HasValueScanner` and `not $f.HasGoType` are conservative carve-outs:
- `$f.HasValueScanner`: the field needs `predicate.UserOrErr` plumbing — the `where` package can't model it. Keep existing form.
- `$f.HasGoType`: the field has a custom Go type that requires conversion via `$f.BasicType "v"` — the `where` package's generic `EQ[T]` accepts any T, but the value needs the conversion the existing template performs. Keep existing form.

For the per-field × op wrappers block (lines 75-133), apply the *same* gate pattern. The deprecated one-liner shape per op is:

```go
// Deprecated: {{ $func }} — use where.{{ $op.Name }}({{ $.Package }}.{{ $f.Constant }}{{ if not $op.Niladic }}, {{ if $op.Variadic }}vs...{{ else }}v{{ end }}{{ end }}).
func {{ $func }}({{ if not $op.Niladic }}{{ $arg }} {{ if $op.Variadic }}...{{ end }}{{ $f.Type }}{{ end }}) predicate.{{ $.Name }} { return where.{{ $op.Name }}({{ $f.Constant }}{{ if not $op.Niladic }}, {{ $arg }}{{ if $op.Variadic }}...{{ end }}{{ end }}) }
```

Some ops in `where.tmpl` use names that DON'T exactly match the `where` package function names. Inventory before relying on a string substitution:

| `$op.Name` from ent | Function in `entgo.io/ent/where` |
|---|---|
| `EQ` | `EQ` |
| `NEQ` | `NEQ` |
| `In` | `In` |
| `NotIn` | `NotIn` |
| `GT`, `GTE`, `LT`, `LTE` | `GT`, `GTE`, `LT`, `LTE` |
| `IsNil` | `IsNull` (rename!) |
| `NotNil` | `NotNull` (rename!) |
| `Contains` | `Contains` |
| `ContainsFold` | `ContainsFold` |
| `EqualFold` | `EqualFold` |
| `HasPrefix` | `HasPrefix` |
| `HasPrefixFold` | `HasPrefixFold` |
| `HasSuffix` | `HasSuffix` |
| `HasSuffixFold` | `HasSuffixFold` |

To handle the `IsNil` → `IsNull` (and `NotNil` → `NotNull`) rename inside the template, branch on `$op.Name`:

```
{{- $whereFn := $op.Name -}}
{{- if eq $op.Name "IsNil" }}{{ $whereFn = "IsNull" }}{{ end -}}
{{- if eq $op.Name "NotNil" }}{{ $whereFn = "NotNull" }}{{ end -}}
... where.{{ $whereFn }}(...) ...
```

If the operator inventory diverges from what's listed above (run `grep -n 'op.Name\|"Name":' entc/gen/feature.go entc/gen/type.go | head -40` to enumerate), surface that — there may be ops requiring more renames or carve-outs.

- [ ] **Step 4: Add `entgo.io/ent/where` to the generated `where.go` imports**

The generated `where.go` currently imports `entgo.io/ent/dialect/sql` and `entgo.io/ent/dialect/sql/sqlgraph` and `<modulepath>/predicate`. Add `entgo.io/ent/where` when emitting the SQL-collapsed branch.

Edit `entc/gen/template/import.tmpl` (or wherever `where.go`'s import set is built — search via `grep -rn '"where"' entc/gen/template/ | head` to locate the surrounding logic). The cleanest approach: in `where.tmpl`, the file-level template should have a custom import block:

```
{{ define "where_imports" }}
import (
    "entgo.io/ent/dialect/sql"
    "entgo.io/ent/dialect/sql/sqlgraph"
    "entgo.io/ent/where"
    "{{ $.Config.Package }}/predicate"
)
{{ end }}
```

If the import block is shared via `template "import" $`, override the per-file import (the existing entc pattern uses an `additional` template per file — search `grep -n "import/additional" entc/gen/template/import.tmpl`). The simpler path: include the `entgo.io/ent/where` import inside the `where.tmpl`'s header section directly.

If the import-resolution mechanism in entc auto-collects imports based on emitted symbols, adding the call to `where.EQ(...)` in the body should be enough — gofmt/imports will resolve. Verify with the Task 7 regen.

- [ ] **Step 5: Re-verify gremlin path is untouched**

Run: `grep -n 'gremlin\|Storage' entc/gen/template/where.tmpl | head -20`

Confirm the gates `eq $.Storage "sql"` are present on the new collapse branches and that the original emission (for `eq $.Storage "gremlin"`) is unchanged.

- [ ] **Step 6: Run the entc test suite to catch template parse errors**

Run: `go test -count=1 ./entc/gen/...`

Many fixture-snapshot tests will FAIL because the generated output has changed. That's expected — Task 7 regenerates them. If you see template *parse* errors ("unexpected", "unterminated"), fix them here before proceeding.

- [ ] **Step 7: Commit**

**Re-verify branch first.** Run `pwd && git rev-parse --abbrev-ref HEAD`.

```bash
git add entc/gen/template/where.tmpl entc/gen/template/import.tmpl
git commit -m "feat(entc/gen): collapse where.go wrappers to deprecated 1-liners

For SQL storage, scalar fields with no ValueScanner and no custom Go
type, the generated <entity>/where.go now emits each per-(field × op)
wrapper as a single deprecated line delegating to entgo.io/ent/where:

  // Deprecated: NameEQ — use where.EQ(user.FieldName, v).
  func NameEQ(v string) predicate.User { return where.EQ(FieldName, v) }

Down from a 4-line block. Fields with ValueScanner or HasGoType keep
their existing multi-line form (the where package's generic API can't
model the value-conversion path).

ID predicates, edge predicates (Has<E>, Has<E>With), And/Or/Not, and
all gremlin-storage output are unchanged.

Fixture regeneration follows in Task 7.

Part of codegen-epic PR 3 (predicate collapse)."
```

---

## Task 7: Regenerate fixtures and verify integration tests

The template changes in Tasks 5 and 6 have stale-bited every generated `where.go` and `predicate/predicate.go` in `entc/integration/*`. Regenerate and verify nothing actually broke.

**Files:**
- Touch all of: `entc/integration/**/ent/<entity>/where.go`
- Touch all of: `entc/integration/**/ent/predicate/predicate.go`
- Possibly: `entc/gen/internal/*_snapshot.go` or other snapshot fixtures

- [ ] **Step 1: Inspect how `entc/integration` is regenerated**

Run: `find entc/integration -name 'generate.go' -o -name 'gen.go' | head -10`

Each integration fixture has a `generate.go` with `//go:generate go run -mod=mod entgo.io/ent/cmd/ent generate ./schema` (or similar). Find the canonical run command in the repo's top-level `Makefile`, `Taskfile.yml`, or `entc/integration/README.md`:

Run: `grep -rn 'go generate\|integration/.*generate' Makefile Taskfile.yml entc/integration/README.md 2>/dev/null | head -10`

- [ ] **Step 2: Regenerate**

Run (from the worktree root):

```bash
go generate ./entc/integration/...
```

This runs every `//go:generate` directive under `entc/integration` and rewrites the generated `ent` directories. Output may be voluminous; watch for any non-zero exit. If a fixture fails to regenerate, run that one individually:

```bash
cd entc/integration/<fixture>
go generate ./...
cd -
```

- [ ] **Step 3: Inspect the diff for sanity**

Run:

```bash
git diff --stat | head -20
git diff entc/integration/privacy/ent/user/where.go | head -80
git diff entc/integration/privacy/ent/predicate/predicate.go | head -40
```

Expected:
- Many `where.go` files have shrunk substantially (eyeball ≥ 30% line reduction on a representative file like `entc/integration/privacy/ent/user/where.go`)
- `predicate/predicate.go` shows `type X = func(...)` instead of `type X func(...)`
- No diff in non-target files (no spurious whitespace-only churn elsewhere)

If `git diff --stat` shows changes to files OUTSIDE `entc/integration/**` or `internal/bench/baseline.jsonl`, investigate before continuing.

- [ ] **Step 4: Run the integration test suites that don't require a DB**

Run:

```bash
go test -count=1 ./entc/integration/privacy/...
go test -count=1 ./entc/integration/hooks/...
go test -count=1 ./entc/integration/edgeschema/...
```

These three are the fixtures referenced by `internal/bench/baseline.jsonl` (the PR 0 baseline) and are minimal enough to run without external DB setup. If any fail, the type-alias change OR the wrapper collapse has introduced a regression. Diagnose before continuing.

- [ ] **Step 5: Run the full entc test suite**

Run:

```bash
go test -count=1 ./entc/... 2>&1 | tee /tmp/entc-test.log
```

Expected: PASS, or the same handful of pre-existing test gremlins as before. If you see new failures, capture the failing test name, the diff between expected and actual output, and surface to the controller before committing fixture changes.

- [ ] **Step 6: Run `go vet` and `go build` across the worktree**

Run:

```bash
go vet ./...
go build ./...
```

Both must complete cleanly. If `go build` flags an unused-import warning in a regenerated `where.go` (e.g., `sql` imported but not used because the file no longer calls `sql.FieldEQ` directly), fix the template's import-emission logic from Task 6 Step 4 and regenerate.

- [ ] **Step 7: Commit fixtures (one commit)**

**Re-verify branch first.** Run `pwd && git rev-parse --abbrev-ref HEAD`.

```bash
git add entc/integration/ entc/gen/internal/ 2>/dev/null
git status   # confirm staged files are all under entc/integration/ or entc/gen/internal/
git commit -m "chore(entc/integration): regenerate fixtures for predicate collapse

Bulk fixture regeneration following the template changes for:
- predicate.<Entity> as type alias for func(*sql.Selector)
- per-(field × op) wrappers collapsed to deprecated 1-liners
  delegating to entgo.io/ent/where

No template logic changes here; all diffs are mechanical
\`go generate ./entc/integration/...\` output.

Part of codegen-epic PR 3 (predicate collapse)."
```

If `git status` shows files OUTSIDE `entc/integration/` and `entc/gen/internal/`, stop and split the commit. Fixture regeneration should be its own commit for review hygiene.

---

## Task 8: Run the bench harness and record the win

Quantify the LOC and gen-time impact on the in-repo fixtures. Surfaces whether the consumer-scale bench is worth running before moving on to PR 4.

**Files:**
- Create: `internal/bench/pr3.jsonl`
- Update (next task): `internal/bench/README.md` if anything about the workflow changed

- [ ] **Step 1: Run bench against all three in-repo fixtures**

Run:

```bash
go run ./cmd/bench-codegen -out internal/bench/pr3.jsonl
```

Expected: 3 newline-delimited JSON objects (privacy, hooks, edgeschema) written to `internal/bench/pr3.jsonl`. Bench takes ~10-20 seconds.

- [ ] **Step 2: Diff against baseline**

Run (uses `jq` since user CLAUDE.md mandates `jq` for JSON):

```bash
jq -s '
  [.[0], .[1]] as [$a, $b] |
  $a | map({fixture: .fixture, total_loc: .total_loc, gen_wall_ms: (.gen_wall_ns/1000000|round)}) as $base |
  $b | map({fixture: .fixture, total_loc: .total_loc, gen_wall_ms: (.gen_wall_ns/1000000|round)}) as $new |
  [range(0; ($base|length))] | map(. as $i | {
    fixture: $base[$i].fixture,
    loc_baseline: $base[$i].total_loc,
    loc_pr3: $new[$i].total_loc,
    loc_delta_pct: (($new[$i].total_loc - $base[$i].total_loc) * 100.0 / $base[$i].total_loc | . * 100 | round | . / 100.0),
    gen_wall_baseline_ms: $base[$i].gen_wall_ms,
    gen_wall_pr3_ms: $new[$i].gen_wall_ms
  })
' internal/bench/baseline.jsonl internal/bench/pr3.jsonl
```

Capture the output. Expected pattern (rough order of magnitude — actual numbers will vary):

```json
[
  {"fixture":"privacy","loc_baseline":11545,"loc_pr3":~10000,"loc_delta_pct":~-13.0,"gen_wall_baseline_ms":1853,"gen_wall_pr3_ms":~1800},
  {"fixture":"hooks","loc_baseline":12515,"loc_pr3":~10800,"loc_delta_pct":~-13.5,"gen_wall_baseline_ms":1603,"gen_wall_pr3_ms":~1600},
  {"fixture":"edgeschema","loc_baseline":56093,"loc_pr3":~51000,"loc_delta_pct":~-9.0,"gen_wall_baseline_ms":5483,"gen_wall_pr3_ms":~5400}
]
```

The fixture-level LOC delta is modest (~10-15%) because `where.go` is only one of many generated files per entity. Per-file LOC reduction on `where.go` itself should be much steeper:

```bash
jq -s '.[0].top_files[] | select(.path | test("/where\\.go$")) | .loc' internal/bench/baseline.jsonl
jq -s '.[1].top_files[] | select(.path | test("/where\\.go$")) | .loc' internal/bench/pr3.jsonl
```

Compare pairs. Expect roughly halved per-file LOC. If `where.go` LOC dropped less than 25%, the collapse template isn't actually triggering — diagnose before continuing.

- [ ] **Step 3: Commit the bench JSONL**

**Re-verify branch first.** Run `pwd && git rev-parse --abbrev-ref HEAD`.

```bash
git add internal/bench/pr3.jsonl
git commit -m "bench(internal/bench): PR 3 measurement vs. baseline

Captures total_loc, gen_wall_ns, build_wall_ns for the three in-repo
fixtures (privacy, hooks, edgeschema) after the predicate-collapse
template changes. Diffable against internal/bench/baseline.jsonl with
the jq snippet in internal/bench/README.md.

Per-file where.go LOC drops ~50%; aggregate fixture LOC drops
~10-15% (where.go is one of many generated files per entity). gen wall
should be flat or slightly faster (fewer template iterations per
field).

Part of codegen-epic PR 3 (predicate collapse)."
```

---

## Task 9: Write `MIGRATION.md` and the per-feature doc page

Document the new API, the deprecation timeline, and the type-alias caveat.

**Files:**
- Create: `doc/md/migration-predicate-collapse.md`
- Modify: `MIGRATION.md` (root of repo — create if absent)

- [ ] **Step 1: Check whether `MIGRATION.md` exists**

Run: `ls MIGRATION.md 2>/dev/null && head -20 MIGRATION.md || echo 'absent — will create'`

- [ ] **Step 2: Create `doc/md/migration-predicate-collapse.md`**

```markdown
# Predicate Collapse Migration Guide

Codegen-epic PR 3 introduces a new generic predicate package and shrinks the
per-entity `where.go` files. This page covers what changed, what you need to
do (if anything), and the timeline.

## What changed

### New package: `entgo.io/ent/where`

```go
import "entgo.io/ent/where"
import "myapp/ent/user"

// Before:
users, err := client.User.Query().
    Where(user.NameEQ("alice")).
    Where(user.AgeGT(18)).
    All(ctx)

// After (new style — preferred for new code):
users, err := client.User.Query().
    Where(where.EQ(user.FieldName, "alice")).
    Where(where.GT(user.FieldAge, 18)).
    All(ctx)
```

The `where` package exposes generic helpers for every standard scalar operator:
`EQ`, `NEQ`, `In`, `NotIn`, `GT`, `GTE`, `LT`, `LTE`, `IsNull`, `NotNull`,
`Contains`, `ContainsFold`, `EqualFold`, `HasPrefix`, `HasPrefixFold`,
`HasSuffix`, `HasSuffixFold`, plus combinators `And`, `Or`, `Not`.

Edge predicates (`HasGroups`, `HasGroupsWith`) and ID predicates (`ID`, `IDEQ`,
`IDIn`, …) stay on their per-entity packages — they need entity-specific
metadata that can't be generic.

### Per-entity wrappers are now deprecated

The existing `user.NameEQ(v)`, `user.AgeGT(v)`, etc. still work but are marked
`// Deprecated:` in the generated code. Each wrapper is a one-line delegate to
the corresponding `where.XX` function. They'll be removed in the release
*after* the next one.

### `predicate.<Entity>` is now a type alias

`type predicate.User = func(*sql.Selector)` (was: `type predicate.User
func(*sql.Selector)`). This is what lets `where.EQ(...)` flow directly into
`Where(...predicate.User)` without an explicit cast.

**Caveat:** predicate types lose nominal identity. Hand-written code that
relied on the type system to prevent cross-entity predicate assignment
(`var p predicate.User = someOrderPredicate`) no longer gets that compile-time
guard. In practice this rarely happens; if you find a real call site, file an
issue and we'll discuss a typed-predicate helper.

## Migration timeline

| Release | Status |
|---|---|
| Current (PR 3) | New `where` package shipped. Per-entity wrappers deprecated but functional. Both APIs work side by side. |
| Next | No change. Migrate consumer call sites to `where.XX` at your own pace. |
| Release after next | Deprecated per-entity wrappers removed. Only `where.XX` and the per-entity edge/ID predicates remain. |

## Mechanical migration recipe

Most call sites can be migrated with a regex codemod:

```
# user.NameEQ("x")    → where.EQ(user.FieldName, "x")
# user.AgeGT(18)      → where.GT(user.FieldAge, 18)
# user.NameIn("a","b")→ where.In(user.FieldName, "a", "b")

s/(\w+)\.(\w+)(EQ|NEQ|In|NotIn|GT|GTE|LT|LTE|Contains|HasPrefix|HasSuffix|EqualFold|ContainsFold|HasPrefixFold|HasSuffixFold)\(/where.\3(\1.Field\2, /g
```

Test the substitution on a single file before running it globally. Edge and ID
predicates won't match and stay as-is.

## What didn't change

- `Where(...)` method signatures
- `And`/`Or`/`Not` per-entity helpers (still available; `where.And`/`Or`/`Not`
  are equivalent)
- Query/Update/Delete builder shapes
- Generated mutation code
```

- [ ] **Step 3: Touch `MIGRATION.md`**

If absent, create with:

```markdown
# Migration Notes

Cross-release migration notes for the matthewsreis/ent fork. Each entry points
to a detailed page under `doc/md/`.

## Predicate Collapse (codegen-epic PR 3)

New `entgo.io/ent/where` generic predicate package + deprecated per-entity
wrappers. See [doc/md/migration-predicate-collapse.md](doc/md/migration-predicate-collapse.md).
```

If present, prepend the new entry under the existing heading (matching existing format).

- [ ] **Step 4: Commit**

**Re-verify branch first.** Run `pwd && git rev-parse --abbrev-ref HEAD`.

```bash
git add doc/md/migration-predicate-collapse.md MIGRATION.md
git commit -m "docs: predicate-collapse migration guide

Per-release migration notes for the new entgo.io/ent/where package
and the deprecation of per-entity wrappers. Includes a regex codemod
recipe for mechanical call-site migration.

Part of codegen-epic PR 3 (predicate collapse)."
```

---

## Task 10: Final verification and epic-progress update

Smoke-test the stack end-to-end and tick PR 3 off in the spec's progress table.

**Files:**
- Modify: `docs/superpowers/specs/2026-05-15-ent-codegen-reduction-epic.md` (§0 progress table)

- [ ] **Step 1: Run the headline regression suite**

Run:

```bash
go test -count=1 -run 'TestBootstrap_Skip|TestFeatureNo|TestReadOnly|TestSnapshot|TestPredicateTemplate' ./entc/... ./entc/internal/... ./where/...
```

Expected: all PASS. This is the canonical PR-0-through-PR-3 regression set referenced in spec §11.

- [ ] **Step 2: Run the broader entc and where suite once more**

Run:

```bash
go test -count=1 ./entc/... ./where/...
```

Expected: PASS, modulo any pre-existing test gremlins documented in spec §11.

- [ ] **Step 3: Confirm master is untouched**

Run:

```bash
git rev-parse master
```

Expected: `7e9d99b1435d541286a773ca128be1a1931d6cc8` (exact SHA). If anything else, **stop and recover** per spec §11.

- [ ] **Step 4: Confirm no push or PR happened**

Run:

```bash
git remote -v
git config --get branch.worktree-wiggly-singing-pancake.remote || echo 'no upstream — good'
```

The branch must have no upstream. No `git push` happened.

- [ ] **Step 5: Update the epic spec's progress table**

In `docs/superpowers/specs/2026-05-15-ent-codegen-reduction-epic.md`, find §0:

```markdown
| 3 | predicate collapse | ⏳ next | not yet written | — |
```

Change to:

```markdown
| 3 | predicate collapse | ✅ complete | `docs/superpowers/plans/2026-05-16-codegen-epic-pr3-predicate-collapse.md` | `<earliest-PR3-sha>` … `<latest-PR3-sha>` |
| 4 | generic builders | ⏳ next | not yet written | — |
```

Also bump the header:

```markdown
**Status:** In progress (4 of 7 PRs complete as of 2026-05-16)
```

To find the commit SHA range, run:

```bash
git log --oneline --reverse master..HEAD | head -1   # earliest PR 3 sha
git log --oneline -1                                 # latest (HEAD) sha
```

- [ ] **Step 6: Commit the epic-progress update**

**Re-verify branch first.** Run `pwd && git rev-parse --abbrev-ref HEAD`.

```bash
git add docs/superpowers/specs/2026-05-15-ent-codegen-reduction-epic.md
git commit -m "docs(epic): mark PR 3 complete in progress table

PR 3 (predicate collapse) landed locally. Updates §0 status, plan
pointer, and commit-range entry. Marks PR 4 (generic builders) as
next.

Per the no-PRs-until-end-of-epic policy, nothing pushed; this is a
local-only milestone update."
```

- [ ] **Step 7: Final smoke check — git log overview**

Run:

```bash
git log --oneline master..HEAD | head -20
```

Expected: a stack of ~9 commits covering PR 3 (where package, type alias, template collapse, fixture regen, bench, docs, epic update) on top of the prior PRs 0-2 commits.

---

## Self-review checklist

Before declaring done:

- [ ] `go test -count=1 ./entc/... ./where/...` is green
- [ ] `git rev-parse master` is `7e9d99b1435d541286a773ca128be1a1931d6cc8` unchanged
- [ ] `git rev-parse --abbrev-ref HEAD` is `worktree-wiggly-singing-pancake`
- [ ] `git remote -v` shows no upstream for the branch (no push happened)
- [ ] `internal/bench/pr3.jsonl` exists alongside `internal/bench/baseline.jsonl`
- [ ] `doc/md/migration-predicate-collapse.md` exists
- [ ] Epic spec §0 progress table reflects PR 3 = complete, PR 4 = next
- [ ] A representative generated `where.go` (e.g., `entc/integration/privacy/ent/user/where.go`) is ≥ 25% shorter than its pre-PR-3 self
