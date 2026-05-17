# PR 6 — per-entity packages design

**Status:** Design approved, plan pending
**Date:** 2026-05-16
**Parent epic:** [`2026-05-15-ent-codegen-reduction-epic.md`](2026-05-15-ent-codegen-reduction-epic.md) §4 PR6
**Branch:** `worktree-wiggly-singing-pancake` (local-only per [[feedback-no-prs-until-end-of-epic]])

## 0. Summary

Move the last four per-type templates (`client/type`, `query`, `mutation/type`, `dialect/sql/entql/type`) from root `ent/` into `ent/<entity>/` sub-packages. Root retains a thin per-entity facade emitting type aliases plus free functions for cross-entity edge methods. The bidirectional-edge import cycle that blocked the rolled-back attempt (commit `281578574` / rollback `2a22fb09f`) is broken by hoisting all cross-entity logic to root: sub-packages have zero sibling-entity imports.

Bundles two follow-ups previously listed as out-of-scope:
- `cmd/ent-codegen-migrate` receiver-type bug fix (corrupted 116 schema files on first apply against `service-api-go`).
- Migration-tool extension covering the new PR 6 rewrites *and* the PR 5 typed-edge-accessor regression.

## 1. Motivation

Per the epic spec §1, root `ent/` carries ~235K LOC across 134 entities post-PR5. PRs 3-5 shrank per-entity LOC and moved CUD/where templates to sub-packages, but root still hosts `<entity>_client.go`, `<entity>_query.go`, `<entity>_mutation.go`, and `<entity>_entql.go`. Of those, `<entity>_query.go` is the biggest per-entity contributor at root (~21 KB / 600 LOC per entity in the privacy fixture; aggregated ≈ 80 KB × 134 = ~11 MB).

That root concentration forces Go's type-checker into one giant package — the working-set + symbol-table cost dominates CI wall and RSS even after the LOC reductions of PRs 3-5. Splitting into 134 per-entity packages unlocks parallel compilation and shrinks per-package working sets. Spec §1 calls this "the biggest compile-time win lever."

The rollback in `2a22fb09f` proved the move is non-trivial: the rollback commit message ("generated code still won't compile because Task (model) and TaskMutation types aren't yet available in sub-packages. That requires moving model/mutation to internal or sub-packages") identifies one cycle source, which PRs 4-5 subsequently resolved via the `ent/internal/<entity>_model.go` + `ent/internal/<entity>_mutation.go` pattern. **A second cycle source — typed cross-entity references in `*Query` and `*Client` (eager-loader fields, edge query methods) — was never solved and is the central design concern of this PR.**

## 2. Goals + non-goals

**Goals:**
- Move `client/type`, `query`, `mutation/type`, `entql/type` to sub-packages.
- Sub-package `ent/<entity>/` imports zero sibling entity packages (lint-enforced by regression test).
- Consumer references like `*ent.TaskQuery` keep resolving (type aliases at root facade).
- Migration tool can mechanically rewrite consumer call sites for the API changes this PR introduces, plus clean up the PR 5 typed-edge-accessor regression in one pass.
- Bench at consumer scale (`service-api-go/api-graphql`) shows ≥ 25% reduction in `go build` wall and peak RSS vs. baseline.

**Non-goals:**
- Removing the root facade entirely. Consumers continue using `ent.X` imports.
- Tree-shaking generated code based on consumer usage (separate epic).
- Touching GraphQL-binding files (separate epic).
- Tree-wide consumer migration to use sub-package types directly (e.g., switching `*ent.TaskQuery` → `*task.TaskQuery` throughout). Type aliases keep the existing import surface.

## 3. Architecture

### 3.1 Output layout

Before:
```
ent/
  task_client.go    (~6.5 KB)
  task_query.go     (~21 KB)
  task_mutation.go  (~0.8 KB — already alias)
  task_entql.go     (~3.1 KB)
  task.go           (~0.7 KB — already alias)
  task/             (existing sub-package from PRs 3-5)
    shared.go, create.go, update.go, delete.go, where.go, task.go (meta)
```

After:
```
ent/
  task_facade.go    (~2-3 KB — NEW: type aliases + cross-entity free functions)
  task.go           (unchanged — model type alias)
  task/             (now leaf package, no sibling imports)
    shared.go, create.go, update.go, delete.go, where.go, task.go (meta)
    client.go       (NEW — self-only Client methods)
    query.go        (NEW — self-only Query methods, opaque eager-load storage)
    mutation.go     (NEW — alias to internal.TaskMutation, was task_mutation.go)
    entql.go        (NEW — TaskFilter + Where* methods, predicate-only cross-pkg)
```

Net per-entity LOC delta (privacy fixture, ~660 LOC currently at root per entity):
- Removed from root: ~600 LOC (query + client + mutation alias + entql)
- Added at root: ~50-80 LOC (facade with aliases + edge free functions, scales with edge count)
- Moved to sub-package: ~400-450 LOC (self-only methods of query + client; mutation/entql relocate as-is)
- Net root reduction: **≈ 520-550 LOC per entity** (~70-80 KB per entity at 134-entity scale; ~10 MB aggregate reduction at root).

### 3.2 Cycle break

**Sub-package invariant**: every file in `ent/<entity>/` imports only:
- `entgo.io/ent` standard libraries (`ent`, `dialect/sql`, `dialect/sql/sqlgraph`, `entql`, `runtime/entbuilder`)
- The consumer's `<module>/ent/internal` (for `Config`, `Hook`, `*Mutation`, `*Model`)
- The consumer's `<module>/ent/predicate` (leaf — predicates for all entities share this package)
- The consumer's schema package (for enums + field types defined there)
- Standard library + third-party (`uuid`, `time`, etc.)

**Forbidden**: any import of `<module>/ent/<sibling-entity>` AND any import of the root `<module>/ent` package. Regression-tested. Sub-packages are leaves of the dependency graph — root → sub-packages only, never the reverse.

**Mechanism**: cross-entity references are confined to:
1. Root facade files (`ent/<entity>_facade.go`) — root imports all sub-packages, no other side imports root.
2. The root graph-level files (`ent/client.go`, `ent/ent.go`, `ent/tx.go`, etc.) — already root-only.

Eager-load storage in sub-package `*Query` is opaque:
```go
type TaskQuery struct {
    Config
    entbuilder.QueryState[predicate.Task]
    order        []OrderOption
    eagerLoaders map[string]func(context.Context, []*Task) error
}

func (q *TaskQuery) StoreEager(name string, loader func(context.Context, []*Task) error) *TaskQuery {
    if q.eagerLoaders == nil {
        q.eagerLoaders = make(map[string]func(context.Context, []*Task) error)
    }
    q.eagerLoaders[name] = loader
    return q
}
```

The loader's type signature mentions only `*Task` (self) — no `*TeamQuery` or other sibling appears in the sub-package. The closure body, captured at the root facade, holds the typed `*TeamQuery` reference internally; that internal capture doesn't propagate to the sub-package's type surface.

Sub-package `*Query.All()` iterates `eagerLoaders` after fetching the parent rows:
```go
func (q *TaskQuery) All(ctx context.Context) ([]*Task, error) {
    nodes, err := q.sqlAll(ctx)
    if err != nil { return nil, err }
    for _, loader := range q.eagerLoaders {
        if err := loader(ctx, nodes); err != nil { return nil, err }
    }
    return nodes, nil
}
```

### 3.3 Root facade (per-entity)

One new file per entity, generated from a new template:

```go
// ent/task_facade.go (NEW)
package ent

import (
    "context"

    "{module}/ent/task"
    "{module}/ent/team"
    "{module}/ent/user"
    "entgo.io/ent/dialect/sql"
    "entgo.io/ent/dialect/sql/sqlgraph"
)

// Type aliases — the public consumer-facing names continue to resolve here.
type (
    TaskQuery      = task.TaskQuery
    TaskClient     = task.Client
    TaskMutation   = task.TaskMutation
    TaskCreate     = task.TaskCreate
    TaskCreateBulk = task.TaskCreateBulk
    TaskUpdate     = task.TaskUpdate
    TaskUpdateOne  = task.TaskUpdateOne
    TaskDelete     = task.TaskDelete
    TaskDeleteOne  = task.TaskDeleteOne
    TaskGroupBy    = task.TaskGroupBy
    TaskSelect     = task.TaskSelect
    TaskFilter     = task.TaskFilter
)

// Constructors that other root code needs.
var NewTaskClient = task.NewClient

// Cross-entity edge methods — free functions because attaching them as methods
// to the type alias is illegal in Go and adding them to a wrapper struct would
// force re-emission of all chaining methods.

func WithTaskTeams(q *TaskQuery, opts ...func(*TeamQuery)) *TaskQuery {
    sub := NewTeamClient(q.Config).Query()
    for _, o := range opts { o(sub) }
    return q.StoreEager("teams", func(ctx context.Context, parents []*Task) error {
        return loadTaskTeams(ctx, sub, parents)
    })
}

func WithTaskOwner(q *TaskQuery, opts ...func(*UserQuery)) *TaskQuery {
    sub := NewUserClient(q.Config).Query()
    for _, o := range opts { o(sub) }
    return q.StoreEager("owner", func(ctx context.Context, parents []*Task) error {
        return loadTaskOwner(ctx, sub, parents)
    })
}

func QueryTaskTeams(c *TaskClient, t *Task) *TeamQuery {
    q := NewTeamClient(c.Config).Query()
    q.SetPath(func(ctx context.Context) (*sql.Selector, error) {
        return taskTeamsSelector(t), nil
    })
    return q
}

func QueryTaskOwner(c *TaskClient, t *Task) *UserQuery {
    q := NewUserClient(c.Config).Query()
    q.SetPath(func(ctx context.Context) (*sql.Selector, error) {
        return taskOwnerSelector(t), nil
    })
    return q
}

// loadTaskTeams + loadTaskOwner: the eager-load implementations that today
// live as *TaskQuery.loadTeams/loadOwner methods in task_query.go.
// They move to root because they reference *TeamQuery / *UserQuery internals.
func loadTaskTeams(ctx context.Context, sub *TeamQuery, parents []*Task) error {
    // same body as today's _q.loadTeams(...), parametrized on (sub, parents)
    ...
}
```

### 3.4 Sub-package method surface

**`ent/<entity>/query.go`** — `type TaskQuery struct { ... }` + methods on `*TaskQuery`:
- Where, Limit, Offset, Order, Unique
- Clone (without edge-loader content; eagerLoaders map shallow-copies if non-nil)
- All, First, FirstX, FirstID, FirstIDX, Only, OnlyX, OnlyID, OnlyIDX
- IDs, IDsX, Count, CountX, Exist, ExistX
- GroupBy, Select, Aggregate
- StoreEager (NEW — exposed for root facade)
- prepareQuery, sqlAll, sqlCount, sqlQuery, querySpec
- modifiers + dialect extensions

**`ent/<entity>/client.go`** — `type Client struct { Config }` + self-only methods:
- New + NewClient
- Use, Intercept
- Create, CreateBulk, MapCreateBulk
- Update, UpdateOne, UpdateOneID
- Delete, DeleteOne, DeleteOneID
- Query, Get, GetX
- Hooks, Interceptors
- mutate (internal — used by Client.mutate dispatch in root)

**`ent/<entity>/mutation.go`** — relocated verbatim from `mutation_type.tmpl`. Content unchanged: type alias to `internal.TaskMutation` + opt constructor aliases.

**`ent/<entity>/entql.go`** — TaskFilter struct + AddPredicate, Filter, NewTaskFilterForMutation, Where (generic entql.P), WhereID, WhereField (× fields), WhereHasEdge / WhereHasEdgeWith (× edges). Cycle-safe because the only cross-package reference is `predicate.<Sibling>` and all predicate types live in the leaf `ent/predicate` package.

## 4. Consumer API changes

| Today | After PR 6 |
|---|---|
| `client.Task.Query().WithTeams(func(q *TeamQuery){...})` | `ent.WithTaskTeams(client.Task.Query(), func(q *TeamQuery){...})` |
| `client.Task.QueryTeams(t)` | `ent.QueryTaskTeams(client.Task, t)` |
| `*ent.TaskQuery`, `*ent.TaskClient`, `*ent.TaskMutation`, etc. | unchanged (type aliases) |
| Method-chained edge use: `c.Query().Where(...).WithTeams(...).Limit(10).All(ctx)` | Free-function wrap: `ent.WithTaskTeams(c.Query().Where(...), ...).Limit(10).All(ctx)` |

Self-only chaining (`Where`, `Limit`, `Order`, `All`, etc.) is unchanged — those methods continue to live on the type and the type alias means `*ent.TaskQuery.Where(...)` still works.

The free-function wrap loses a single dot in the chain but otherwise reads identically. The migration tool (§6) rewrites mechanically.

### 4.1 Documented breakage

Consumer code reaching into private/unexported sub-package state directly was never supported and remains unsupported — but the structural move (root → sub-package types) may surface accidental dependencies. Specifically:
- `reflect.TypeOf((*ent.TaskQuery)(nil))` now yields `*task.TaskQuery` (the underlying aliased type). Code matching on type name strings needs updating.
- `go/types`-based static analysis of consumer code sees a different declaration site for the type. Tools that key on `pkg.TaskQuery` may need to follow alias targets.

`MIGRATION.md` documents these.

## 5. Codegen plumbing changes

### 5.1 `entc/gen/template.go`

Flip `SubPackage: true` and update `Format` on four templates:

| Template name | Before format (file path) | After format (file path) |
|---|---|---|
| `client/type` | `task_client.go` | `task/client.go` |
| `query` | `task_query.go` | `task/query.go` |
| `mutation/type` | `task_mutation.go` | `task/mutation.go` |
| `dialect/sql/entql/type` | `task_entql.go` | `task/entql.go` |

Format-function shape: `func(t *Type) string { return fmt.Sprintf("%s/<file>.go", t.PackageDir()) }` (use a small helper if repeated).

All four also set `SubPackage: true` so `graph.go:277` wraps the type in `typeScope{InSubPackage: true}` for template execution.

Add a new per-type template `facade/type` *(not* `SubPackage)*:
- Name: `facade/type`
- Format: `pkgf("%s_facade.go")`
- Cond: `notView` (views skip — no Client/Query/Mutation)
- Emits type aliases + edge free functions; iterates `$.Edges` for the per-edge `With<Entity><Edge>` / `Query<Entity><Edge>` + `load<Entity><Edge>` helpers.

Add deleted-file entries to `deletedTypeTemplates`:
```go
deletedTypeTemplates = []string{
    "%s_create.go", "%s_update.go", "%s_delete.go",   // existing
    "%s/client.go",                                    // existing (rollback artifact)
    "%s_client.go",                                    // NEW — old root file
    "%s_query.go",                                     // NEW
    "%s_mutation.go",                                  // NEW
    "%s_entql.go",                                     // NEW
}
```

### 5.2 `entc/gen/func.go` + `entc/gen/type.go`

`PackageQualifier()` and `BuilderImports()` already handle InSubPackage scope. Verify they cover the query/client templates' symbol references; extend if any new qualification site is uncovered (e.g., `task.OrderOption` referenced from query.tmpl needs to become bare `OrderOption` when InSubPackage).

`SiblingImports()` is currently used by `query` and `client/type` templates (line 14 of both). When InSubPackage, the imports it generates must drop sibling entity packages (since those are now forbidden) — facade picks up the cross-entity import responsibility.

Adjust `Type.SiblingImports()` or add a typeScope-aware override so sub-package contexts emit only the cycle-safe imports (`internal`, `predicate`, `entbuilder`, `sqlgraph`, etc.) and root-facade contexts emit the full sibling list.

### 5.3 Template body changes

**`entc/gen/template/builder/query.tmpl`** (the biggest delta):
- Remove `withTeams *TeamQuery` style fields (replace with `eagerLoaders map[string]func(context.Context, []*<Entity>) error`).
- Remove `WithTeams` / `WithOwner` / etc. methods (they emit from facade template instead).
- Remove `QueryEdge` per-edge methods (those were already on Client, not Query — verify).
- Remove `loadTeams` / `loadOwner` eager-load implementations (move to facade template).
- Adjust All() to dispatch via `eagerLoaders` map.
- Adjust Clone() to shallow-copy the eagerLoaders map.
- Drop the `{{ $.Package }}.` qualifier prefix on same-package symbols (now handled by `PackageQualifier()` returning empty in sub-package scope — already in place).
- Add `StoreEager` method.

**`entc/gen/template/builder/client_type.tmpl`**:
- Remove `Query<Edge>` per-edge methods (move to facade).
- Drop `{{ $q }}` qualifier on builder type names (`task.TaskCreate` → `TaskCreate` since now same-package).
- Drop `{{ $n.Package }}.` qualifier (now same-package).

**`entc/gen/template/builder/mutation_type.tmpl`**:
- No content changes. Relocate via `SubPackage: true`. Verify the `import "{module}/internal"` resolves correctly when InSubPackage (it still imports internal — that's leaf and cycle-safe).

**`entc/gen/template/dialect/sql/entql_type.tmpl`**:
- Drop `{{ $.PackageAlias }} "{{ $.Config.Package }}/{{ $.PackageDir }}"` import (now same-package, removes need for the package-alias import).
- Drop `{{ $n.Package }}.FieldX` qualifier (now `FieldX`).
- Keep `predicate.<Sibling>` references (predicate is leaf, cycle-safe).

**`entc/gen/template/facade.tmpl`** (NEW):
- Header + imports (root-style: pkg `ent`, imports all sub-packages, internal, predicate, sqlgraph).
- Type aliases for all generated per-entity types (TaskQuery, TaskClient, TaskMutation, TaskCreate, TaskCreateBulk, TaskUpdate, TaskUpdateOne, TaskDelete, TaskDeleteOne, TaskGroupBy, TaskSelect, TaskFilter).
- Constructor var aliases (`var NewTaskClient = task.NewClient`).
- For each edge: emit `With<Entity><Edge>`, `Query<Entity><Edge>`, `load<Entity><Edge>` (the cross-entity helper).

### 5.4 graph.go + assets.go

`graph.go:268-289` already invokes templates with `typeScope{InSubPackage: true}` when `tmpl.SubPackage` is set. No core change needed there. Verify `assets.add(...)` correctly handles the new sub-package output paths for the four moved templates.

`cleanOldNodes` (graph.go:328) handles `deletedTypeTemplates` — old root files (`task_query.go` etc.) get cleaned up on regen.

## 6. Migration tool changes

### 6.1 Receiver-type bug fix (follow-up #1, gate-blocking)

**Current bug** (`cmd/ent-codegen-migrate/rewrite_mutation.go:146`): `matchMutationCall` matches purely on method name. When run against `service-api-go`, it rewrote `edge.From("x")` (from `entgo.io/ent/schema/edge`) as `entbuilder.GetField[string](edge, "from").Ref(...)` — corrupting 116 schema files.

**Fix**:
- Use `golang.org/x/tools/go/packages` to load the target Go package with `NeedTypes | NeedTypesInfo | NeedSyntax`.
- In `matchMutationCall`, resolve the receiver expression's static type via `types.Info.TypeOf(recv)`.
- Skip rewrites where the receiver isn't `*ent.*Mutation`, `*ent.*Query`, or `*ent.*Client` (depending on the rewrite pass).
- New unit tests covering:
  - `m.SetX(v)` where `m *ent.TaskMutation` — rewritten ✓
  - `edge.From("x")` where `edge` is the schema-DSL type — skipped ✓
  - `m.SetX(v)` where `m` is an interface type that happens to declare `SetX` — skipped (only concrete `*ent.*Mutation` matches)

### 6.2 New AST passes

**`rewrite_edge_method.go`** (NEW, for PR 6 API change):
- Matches `*ent.<Entity>Query` receivers calling `With<Edge>(...)` → rewrites to `ent.With<Entity><Edge>(q, ...)`
- Matches `*ent.<Entity>Client` receivers calling `Query<Edge>(t)` → rewrites to `ent.Query<Entity><Edge>(c, t)`
- Uses the same receiver-type resolution machinery as the fix.

**`rewrite_typed_edge_accessor.go`** (NEW, for PR 5 follow-up #2):
- Matches `*ent.<Entity>Mutation` receivers calling `<Edge>ID()` / `<Edge>IDs()` (the PR 5 removed accessors).
- Rewrites to `m.EdgeID("<edge>")` / `m.EdgeIDs("<edge>")` (matching the surviving generic API).
- Handles the typed-return-value cast on the read side.

### 6.3 Dispatch update

`cmd/ent-codegen-migrate/main.go` adds the new passes to the dispatch handler chain. Per-PR flags (`--rewrite-mutation`, `--rewrite-predicate`, `--rewrite-edge-method`, `--rewrite-edge-accessor`) for selective application; default `--all` runs every pass.

### 6.4 Tests

- Extend `cmd/ent-codegen-migrate/integration_test.go` against the privacy fixture covering:
  - Receiver-type fix: ensure schema-DSL `edge.From("x")` is preserved.
  - PR 6 edge-method rewrites: `q.WithTeams(...)` → `ent.WithTaskTeams(q, ...)`.
  - PR 5 edge-accessor rewrites: `m.OwnerID()` → `m.EdgeID("owner")` (or whichever runtime API survives).
- New fixture covering a bidirectional cycle (Task ↔ Team ↔ User) if not already present — privacy fixture has Task/Team/User but verify edges are mutually wired.

## 7. Test strategy

### 7.1 `entc/gen` regression tests

All template-output snapshots must regenerate. Expect very large diffs — review template + plumbing changes alongside fixture changes; use the bench tool's fixture-diff summarizer.

### 7.2 New regression: no-sibling-imports lint

`entc/internal/subpackage_imports_test.go` (NEW): walks each integration fixture's `ent/<entity>/` directory; scans every `.go` file for imports of `<module>/ent/<other-entity>`; fails if any are found.

Strict assertion. Catches future template changes that re-introduce cycles.

### 7.3 `entc/integration` tests

Every integration fixture regenerates and re-runs. Spot-checked subset:
- `entc/integration/privacy` (the prompt's reference fixture — bidirectional edges, hooks, policies)
- `entc/integration/edgeschema` (M2M-with-payload — known cycle generator)
- `entc/integration/multischema` (cross-database edges — exercises path generation)
- `entc/integration/migrate` (schema migrations against per-entity output)

If any integration suite fails after the move, the fix lives in this PR — no carryover.

### 7.4 Migration tool tests

Per §6.4.

### 7.5 Bench harness

`/tmp/bench-pr6.sh` against the wiggly worktree at HEAD post-PR6. Compare to baseline (master) and post-PR0-5 (current HEAD before PR6). Bench fixture is `service-api-go/api-graphql` (134 entities). Tracked numbers:
- `task generate` wall + peak RSS
- `go build` wall + peak RSS
- Total generated LOC, root-only LOC, per-package LOC distribution
- Generated file count (expected to RISE — per-entity sub-package files multiply)

Pre-bench requires running the migration tool against the consumer once the tool's fixes land (so the consumer code compiles against the new ent API).

## 8. Acceptance gates

Gate-blocking — none of these can be deferred:

1. `go test ./entc/...` passes with regenerated fixtures.
2. New no-sibling-imports lint passes for every integration fixture.
3. Migration tool's expanded test suite passes (receiver-type fix + new passes covered) AND each pass is idempotent (re-running on already-transformed code produces no diffs — explicit unit test).
4. `go build ./...` passes after running the migration tool against the privacy fixture's hand-written test code (smoke test that the rewrites are syntactically + semantically correct).
5. Consumer-scale bench (`service-api-go/api-graphql`) shows:
   - `go build` wall ≤ 75% of baseline (≥ 25% reduction)
   - `go build` peak RSS ≤ 75% of baseline
   - `task generate` wall does not regress (allowed to be ±10%)
   - Root `ent/` LOC ≤ 30% of baseline root LOC (the structural-move success metric)

Bench gates have to clear before the PR is declared shippable. If the parallelism win is below 25% on build metrics, the design needs revision (e.g., the per-entity sub-package count is too high to amortize per-package overhead; would need a sub-grouping scheme).

## 9. Risks

| Risk | Mitigation |
|---|---|
| AST surgery on consumer code (migration tool) corrupts files (already happened once with the receiver-type bug) | Receiver-type fix is gate-blocking + comprehensive tests + dry-run mode (`--dry-run` prints diffs without writing) |
| 4-template move + facade emission + migration-tool extension is the largest PR of the epic — review surface is huge | Break implementation into 5-7 sequential commits (each with its own tests passing) so reviewers can read the stack incrementally |
| Per-entity sub-package count at consumer scale (134) introduces per-package overhead that erodes the parallelism win | Bench gate (acceptance §8.5) aborts if BUILD wall regresses; fallback option: group entities by schema sub-tree to reduce package count |
| The `eagerLoaders map[string]func(...) error` opaque storage breaks GraphQL bindings or other consumers that expected the typed `withTeams` field | GraphQL bindings access ent through the public methods, not struct fields — verify by running entgql integration tests; if breakage, expose typed accessors as part of the facade |
| `entql_type` template removes the package-alias import — if any reference to the sibling package was hiding there, build breaks | Scan template body before the move; lint via no-sibling-imports test |
| Hand-written code in consumer hooks references `*ent.TaskQuery.WithTeams` style chaining (heavy migration cost) | Migration tool handles the chained case (`q.Where(...).WithTeams(...)`) by wrapping in the free-function form |
| Re-running the receiver-bug-fixed migration tool against service-api-go after the prior corruption is required to bench — corruption is partially-applied so naive re-run would double-transform | (a) Tool requirement: each rewrite pass MUST be idempotent — re-running the fixed tool against already-transformed code produces no further changes (test gate); (b) `MIGRATION.md` documents `git restore` recovery for any consumer that ran the broken version |

## 10. Implementation order

The plan (written separately via `writing-plans`) sequences as:

1. Migration-tool receiver-type fix (independent, test-driven, lands first as foundation).
2. Migration-tool new passes (`rewrite_edge_method.go`, `rewrite_typed_edge_accessor.go`) — built before consuming them.
3. Codegen plumbing: `template.go` flag flips + new `facade/type` template registration + new typeScope helpers.
4. Sub-package template body edits (`query.tmpl`, `client_type.tmpl`, `mutation_type.tmpl`, `entql_type.tmpl`).
5. New `facade.tmpl` template body.
6. Regenerate integration fixtures; iterate on test failures.
7. No-sibling-imports lint test + regression coverage.
8. Consumer-scale bench run; capture numbers in `BENCH_RESULTS.md`.
9. `MIGRATION.md` updates documenting the API change + migration-tool usage.

## 11. Out of scope (explicit deferrals)

- Removing the root facade entirely. The facade is a permanent boundary between the consumer-stable API and the internal package split.
- Migration of GraphQL bindings (1,286 files in `ent/gen/gql_*.go`) — separate epic per spec §9.
- Tree-shaking / dead-code elimination based on consumer usage analysis — v2 per spec §9.
- Sub-grouping entities by schema sub-tree if the per-package count proves too high — fallback plan kept in §9 risks but not implemented preemptively.

## 12. Open questions

None. The two design unknowns (cycle break + scope of bundled follow-ups) were settled during brainstorming; bench gates handle the parallelism-win uncertainty.

## 13. Memory + commit hygiene

- After every implementer subagent dispatch (the per-commit cadence from PRs 3-5), re-verify `pwd && git rev-parse --abbrev-ref HEAD` before each `git commit` per the epic spec §11's recurring failure mode. Three of ~20 subagent dispatches in PRs 0-2 landed on master; zero in PRs 3-5; PR 6 must maintain the streak.
- All commits stay local on `worktree-wiggly-singing-pancake` per [[feedback-no-prs-until-end-of-epic]]. No `git push`, no `gh pr create`, no `gs branch submit`.
- Bench numbers land in `internal/bench/` next to the PR 5 bench file, plus epic spec §0 progress table updates marking PR 6 ⏳ → ✅ when bench gates pass.
