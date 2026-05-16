# Codegen Epic PR 4 — Generic Builders Design

**Status:** Design approved, ready for `writing-plans` skill
**Date:** 2026-05-16
**Worktree:** `.claude/worktrees/wiggly-singing-pancake` on branch `worktree-wiggly-singing-pancake`
**Parent spec:** [docs/superpowers/specs/2026-05-15-ent-codegen-reduction-epic.md §4 PR 4](2026-05-15-ent-codegen-reduction-epic.md)
**Stack tool:** git-spice (deferred — all commits stay local until end-of-epic per [[feedback-no-prs-until-end-of-epic]])

## 1. Goal

Collapse the bulk of generated `<entity>_query.go` (~700 LOC), `<entity>/update.go` (~500 LOC), and `<entity>/delete.go` (~100 LOC) into generic helpers in `runtime/entbuilder`. Each per-entity file becomes a thin struct + chaining methods + entity-specific edge/SQL bits, delegating terminal logic and shared state to generic helpers.

Target at consumer scale (`service-api-go/api-graphql`, 134 entities): ~50-60K LOC reduction across the three file types combined. Bench captures real numbers in the final task.

## 2. Non-goals (deferred to PR 4b or later)

- **`GroupBy(...) *UserGroupBy` and `Select(...) *UserSelect`.** These return entity-specific types with their own state (`UserGroupBy`, `UserSelect`). Generic-ifying them adds significant design surface (return-type-as-parameter generic juggling, scan-target factories). The per-entity wrappers stay unchanged in PR 4. If consumer bench shows the remaining LOC is meaningful, a follow-up PR can address them.
- **Edge methods** (`WithTeams`, `QueryTeams`, `loadTeams`). The `LoadEdgeM2M` / `LoadEdgeO2M` helpers in `runtime/entbuilder/query.go` already factor the loading logic. The per-entity wrappers stay because they reference entity-specific neighbor types (`*TeamQuery`, `*Team`).
- **Gremlin backend.** SQL only. Gremlin codegen retains its existing emission verbatim.
- **Hooks / Interceptors / Privacy.** Stay where they are. The generic helpers preserve hook/interceptor invocation through the existing entity-specific functions.

## 3. Architecture

### 3.1 New types in `runtime/entbuilder`

**`runtime/entbuilder/query.go`** (extend existing file):

```go
// QueryState holds the generic state every <Entity>Query carries.
type QueryState[P ~func(*sql.Selector)] struct {
    Ctx        *QueryContext
    Order      []func(*sql.Selector) // serialized via OrderOption.ToFunc
    Inters     []Interceptor
    Predicates []P
    Sql        *sql.Selector
    Path       func(context.Context) (*sql.Selector, error)
}

// RunFirst, RunOnly, RunFirstID, RunOnlyID, RunAll, RunIDs, RunCount, RunExist
// are package-level generic helpers that take entity-specific function args.
//
// Signature pattern (RunAll shown; others follow the same shape):
func RunAll[T any, P ~func(*sql.Selector)](
    ctx context.Context,
    state *QueryState[P],
    cfg Config,
    inters []Interceptor,             // global inters from Config; merged with state.Inters
    prepareQuery func(context.Context) error,
    sqlAllFn func(context.Context) ([]*T, error),
) ([]*T, error)

// Plus convenience methods on QueryState[P] for the trivially-stateful ops:
func (s *QueryState[P]) AddPredicates(ps ...P) { s.Predicates = append(s.Predicates, ps...) }
func (s *QueryState[P]) SetLimit(n int)        { s.Ctx.Limit = &n }
func (s *QueryState[P]) SetOffset(n int)       { s.Ctx.Offset = &n }
func (s *QueryState[P]) SetUnique(b bool)      { s.Ctx.Unique = &b }
func (s *QueryState[P]) Clone() *QueryState[P] { /* deep copy */ }
```

The package-level `Run*` functions encapsulate the "check ctx → call interceptor chain → call sqlAll → return result" boilerplate that's identical across all generated `All`/`First`/`Count`/etc. methods today.

**`runtime/entbuilder/update.go`** (extend existing file):

```go
type UpdateState[M any] struct {
    Hooks    []Hook
    Mutation M
}

// RunUpdate handles the Save/Exec boilerplate: WithHooks dispatch, mutation
// validation, sqlSave call. Entity-specific work is injected via sqlSaveFn.
func RunUpdate[M any](
    ctx context.Context,
    state *UpdateState[M],
    sqlSaveFn func(context.Context) (int, error),
) (int, error)

// RunUpdateOne for the *UpdateOne variant: same but returns *T.
func RunUpdateOne[T, M any](
    ctx context.Context,
    state *UpdateState[M],
    sqlSaveFn func(context.Context) (*T, error),
) (*T, error)
```

**`runtime/entbuilder/delete.go`** (extend existing file):

```go
type DeleteState[M any] struct {
    Hooks    []Hook
    Mutation M
}

func RunDelete[M any](
    ctx context.Context,
    state *DeleteState[M],
    sqlExecFn func(context.Context) (int, error),
) (int, error)
```

### 3.2 Generated `<entity>_query.go` after refactor

Before (~700 LOC):

```go
type UserQuery struct {
    Config
    ctx        *QueryContext
    order      []user.OrderOption
    inters     []Interceptor
    predicates []predicate.User
    withTeams  *TeamQuery
    withTasks  *TaskQuery
    sql        *sql.Selector
    path       func(context.Context) (*sql.Selector, error)
}

func (_q *UserQuery) Where(ps ...predicate.User) *UserQuery {
    _q.predicates = append(_q.predicates, ps...)
    return _q
}
// ... 35 more methods, each 5-20 LOC ...
```

After (~250 LOC):

```go
type UserQuery struct {
    Config
    entbuilder.QueryState[predicate.User]
    order     []user.OrderOption  // entity-specific: OrderOption type
    withTeams *TeamQuery          // entity-specific: edge target type
    withTasks *TaskQuery          // entity-specific: edge target type
}

// Chaining methods (2-line wrappers, return *UserQuery for chainability):
func (_q *UserQuery) Where(ps ...predicate.User) *UserQuery {
    _q.QueryState.AddPredicates(ps...); return _q
}
func (_q *UserQuery) Limit(n int) *UserQuery  { _q.QueryState.SetLimit(n); return _q }
func (_q *UserQuery) Offset(n int) *UserQuery { _q.QueryState.SetOffset(n); return _q }
func (_q *UserQuery) Unique(b bool) *UserQuery{ _q.QueryState.SetUnique(b); return _q }
func (_q *UserQuery) Order(o ...user.OrderOption) *UserQuery {
    _q.order = append(_q.order, o...); return _q
}
func (_q *UserQuery) Clone() *UserQuery {
    return &UserQuery{ Config: _q.Config, QueryState: *_q.QueryState.Clone(),
                       order: append([]user.OrderOption{}, _q.order...),
                       withTeams: _q.withTeams.Clone(), withTasks: _q.withTasks.Clone() }
}

// Terminal methods (1-3 line delegations to entbuilder.Run*):
func (_q *UserQuery) All(ctx context.Context) ([]*User, error) {
    return entbuilder.RunAll[User](ctx, &_q.QueryState, _q.Config, _q.inters,
                                   _q.prepareQuery, _q.sqlAll)
}
func (_q *UserQuery) AllX(ctx context.Context) []*User {
    nodes, err := _q.All(ctx); if err != nil { panic(err) }; return nodes
}
// ... First, FirstX, FirstID, FirstIDX, Only, OnlyX, OnlyID, OnlyIDX,
//     IDs, IDsX, Count, CountX, Exist, ExistX — all 1-3 lines each

// Entity-specific edge accessors (UNCHANGED):
func (_q *UserQuery) QueryTeams() *TeamQuery { /* ~20 LOC, uses sqlgraph.NewStep */ }
func (_q *UserQuery) WithTeams(opts ...func(*TeamQuery)) *UserQuery { ... }
// ... same for QueryTasks, WithTasks

// Entity-specific SQL builder (UNCHANGED — uses entity column constants):
func (_q *UserQuery) prepareQuery(ctx context.Context) error { ... }
func (_q *UserQuery) sqlAll(ctx context.Context, hooks ...queryHook) ([]*User, error) { ... }
func (_q *UserQuery) loadTeams(ctx, query, nodes, init, assign) error { ... }
func (_q *UserQuery) loadTasks(ctx, query, nodes, init, assign) error { ... }
func (_q *UserQuery) sqlCount(ctx context.Context) (int, error) { ... }
func (_q *UserQuery) querySpec() *sqlgraph.QuerySpec { ... }
func (_q *UserQuery) sqlQuery(ctx context.Context) *sql.Selector { ... }

// GroupBy / Select — UNCHANGED (deferred to PR 4b):
func (_q *UserQuery) GroupBy(...) *UserGroupBy { ... }
func (_q *UserQuery) Select(...) *UserSelect { ... }
func (_q *UserQuery) Aggregate(...) *UserSelect { ... }
```

### 3.3 Generated `<entity>/update.go` and `<entity>/delete.go`

Same shape — embed `entbuilder.UpdateState[*UserMutation]` and `entbuilder.DeleteState[*UserMutation]`, delegate terminal methods, keep SetX/AddX field-setter methods (those drive the mutation, can't be generic; PR 5 attacks them at the mutation level).

## 4. Why embedding over type-alias

The parent spec's PR 4 description hints at `type UserQuery = entbuilder.Query[User, predicate.User]`. That works only for entities with **zero edges and no entity-specific order types**. In real schemas almost every entity has edges (`withTeams`, `withTasks` fields) and an `OrderOption` type that references the entity's field constants.

Embedding `QueryState[P]` in `UserQuery` is the next-best thing:
- All generic state lives in the embedded struct
- Entity-specific fields live on the outer struct
- Chaining methods on `UserQuery` return `*UserQuery` directly (no method-promotion-loses-outer-type problem)
- Terminal methods delegate to package-level `entbuilder.Run*` functions that take the embedded state by pointer

## 5. Why package-level generic Run* functions

Alternative considered: methods on `*entbuilder.Query[T,P]`. Rejected because method promotion in Go returns the embedded type, not the outer type, breaking chainability (`user.Query().Limit(5).WithTeams(...)` would fail because `Limit` would return `*Query[T,P]`, not `*UserQuery`).

Package-level functions sidestep this entirely. The function-arg pattern matches the existing `LoadEdgeM2M[N,E,NID,EID]` style (see `runtime/entbuilder/query.go:60`).

## 6. Stack-relative ordering

PR 4 has no API-breaking impact on consumers if the generated `UserQuery` keeps the same methods with the same signatures. The internal embedding is invisible.

It depends on **PR 3** (predicate-type alias) because the generic `QueryState[P ~func(*sql.Selector)]` parameter needs `predicate.User` to be a type with the underlying type `func(*sql.Selector)` — PR 3 made it a type alias, which gives that property by definition.

It does NOT depend on PR 5 (mutation collapse) — `UpdateState[M]` and `DeleteState[M]` work with whatever mutation type the entity has today.

## 7. Acceptance criteria

- All existing tests pass (`./entc/...`, `./entc/integration/...` for privacy/hooks/edgeschema; broader suite within the same pre-existing-gremlin envelope spec §11 documents)
- `entc/gen` regression fixture tests pass after regeneration
- `internal/bench/pr4.jsonl` shows measurable per-file `_query.go` / `_update.go` / `_delete.go` LOC reduction (eyeball target: ≥30% per file on average across the three in-repo fixtures)
- `go build ./...` clean
- `go vet ./...` clean (modulo pre-existing warnings spec §11 covers)
- `runtime/entbuilder` has unit tests for the new `Run*` functions (using table-driven fixtures with mock query specs)

## 8. Risks

| Risk | Mitigation |
|---|---|
| Generic instantiation cost erodes the LOC win at scale (Go 1.18+ GC-shape stenciling) | Measure at consumer scale; if regression, fall back to per-entity inlining for hot paths |
| Hook/interceptor ordering subtly changes between old code paths and new generic dispatch | Carefully preserve the existing interceptor merge logic in `Run*` functions; integration tests on hook ordering must pass |
| Embedded `QueryState[P]` field name collision with entity-specific field (e.g., entity has its own `Ctx` field) | The embedded struct's field names are namespaced via the embed (`q.QueryState.Ctx`); only ambiguous if the outer struct also has `Ctx` — verify none of ent's reserved field names overlap (`Ctx`, `Order`, `Inters`, `Predicates`, `Sql`, `Path`). Add a name-collision check to `entc/gen/type.go` if needed. |
| Hand-written code reaching into private `predicates` / `order` / `sql` / `path` fields breaks | Those fields are unexported in current generated code; any consumer reaching into them is already on undocumented ground. Document the move to `QueryState` as a migration note. |
| Generated `_query.go` files still large (only ~60-65% LOC reduction, not the spec's 60% target by ITSELF — close but not headline) | This is acknowledged; PR 4 plus future GroupBy/Select work would close the gap |

## 9. Out of scope (defer to PR 4b if ever needed)

- `GroupBy(...) *UserGroupBy` and `Select(...) *UserSelect` generic-ification
- Aggregation funcs (`Sum`, `Count`, etc. — they're already on entbuilder helpers but the per-entity GroupBy returns entity-specific types)
- Gremlin backend support for the new generic helpers
- Cross-entity batched queries (orthogonal optimization)

## 10. Implementation outline (for `writing-plans` consumption)

The plan should cover roughly these tasks:

1. Orient — read existing entbuilder/query.go, entbuilder/update.go, entbuilder/delete.go, generated user_query.go, user/update.go, user/delete.go
2. Add `QueryState[P]` type + Clone/Set helpers to `entbuilder/query.go` with unit tests
3. Add `RunAll[T,P]`, `RunFirst[T,P]`, etc. terminal helpers with unit tests (mock query specs)
4. Add `UpdateState[M]` + `RunUpdate[M]` / `RunUpdateOne[T,M]` to `entbuilder/update.go` with unit tests
5. Add `DeleteState[M]` + `RunDelete[M]` to `entbuilder/delete.go` with unit tests
6. Modify `builder/query.tmpl` to emit the new shape (embedded state + delegating terminal methods)
7. Modify `builder/update.tmpl` and `builder/delete.tmpl` to emit the new shape
8. Regenerate fixtures via `go generate ./entc/integration/...`; verify integration tests pass
9. Run bench harness; commit `internal/bench/pr4.jsonl`
10. Final verification + epic spec §0 progress update (mark PR 4 complete, PR 5 next)

The plan should call out:
- Branch-discipline pre-flight (worktree-wiggly-singing-pancake, no push, master at `7e9d99b1...`)
- The reserved-field-name check (Risk #3 above)
- The hook/interceptor ordering preservation (Risk #2)
- Fixture-regen + bench commit per the same shape as PR 3 Tasks 7 + 8
