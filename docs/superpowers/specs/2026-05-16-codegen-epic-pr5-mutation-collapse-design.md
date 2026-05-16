# Codegen Epic PR 5 — Mutation Collapse Design

**Status:** Design draft, awaiting user review
**Date:** 2026-05-16
**Worktree:** `.claude/worktrees/wiggly-singing-pancake` on branch `worktree-wiggly-singing-pancake`
**Parent spec:** [docs/superpowers/specs/2026-05-15-ent-codegen-reduction-epic.md §4 PR 5](2026-05-15-ent-codegen-reduction-epic.md)
**Stack tool:** git-spice (deferred — all commits stay local until end-of-epic per [[feedback-no-prs-until-end-of-epic]])

## 1. Goal

Collapse per-entity `<entity>_mutation.go` (~640-710 LOC for fixture entities, ~2.6K LOC mean at consumer scale) into a generic `entbuilder.Mutation[T]` backed by a per-entity descriptor. Largest single LOC contributor in the epic — spec §1 estimates ~355K LOC of `internal/<entity>_mutation.go` at consumer scale (134 entities). At ~50 LOC mean per-entity file post-PR, **target ~340K LOC reduction at consumer scale**.

Secondary scope (folded in from PR 3 retroactive cleanup): drop PR 3's `// Deprecated:` 1-line predicate wrappers in `<entity>/where.go` now that we have a migration tool to rewrite call sites. Additional ~7-10K LOC at consumer scale.

## 2. Deviations from parent spec

Two material divergences from epic §4 PR 5 and §5.3, justified by [[feedback-api-contract-changes-ok]]:

1. **No `FeatureTypedMutation` compat shim.** Spec §4 PR 5 planned a one-release opt-in deprecation shim that synthesized typed methods as wrappers. We drop it entirely. Hook authors migrate via the tool below, not via a transition period.
2. **No predicate-wrapper deprecation period.** PR 3's `// Deprecated:` wrappers stop being emitted in this PR. Migration tool rewrites call sites.

## 3. Non-goals

- **Concurrent-safe mutations.** Today's mutation is single-goroutine; the generic version preserves that. No mutex on `m.fields` / `m.edges`.
- **Reducing `ent.Mutation` interface surface.** The interface (`Op() Op`, `Type() string`, `Field(name) (ent.Value, bool)`, `SetField(name, v) error`, `OldField`, `AddedFields`, `AddedField`, `AddField`, `ClearedFields`, `FieldCleared`, `ClearField`, `ResetField`, `AddedEdges`, `RemovedEdges`, `ClearedEdges`, `EdgeCleared`, `ClearEdge`, `ResetEdge`, `AddedIDs`, `RemovedIDs`, `Fields()`, `Where`, `WhereP`, `IDs(ctx)`) is preserved bit-for-bit on `*Mutation[T]`.
- **Gremlin backend.** SQL only. Gremlin retains today's per-entity mutation emission verbatim.
- **Hook / Interceptor / Policy reordering.** These continue to receive `*<Entity>Mutation` (which is now `*Mutation[T]` via type alias) — their dispatch order and semantics are unchanged.
- **Old-value caching scheme change.** The existing `sync.Once`-style caching inside the per-entity `WithXID` option becomes a single `loadOldOnce` on the generic mutation. Same behavior.

## 4. Architecture

### 4.1 New types in `runtime/entbuilder`

```go
// runtime/entbuilder/mutation.go (new file)

package entbuilder

import (
    "context"
    "errors"
    "fmt"
    "reflect"
    "sync"

    "entgo.io/ent/dialect/sql"
)

// Cardinality classifies an edge.
type Cardinality uint8

const (
    O2OUnique Cardinality = iota // unique edge: at most one neighbor
    O2M
    M2O
    M2M
)

// FieldSpec describes a scalar field on an entity.
type FieldSpec struct {
    Type     reflect.Type // expected Go type for SetField validation
    GoName   string       // exported struct field name on the entity type (e.g. "Title")
    Nillable bool         // can be cleared via ClearField
    Numeric  bool         // supports AddField numeric increment
    Default  any          // optional default for ResetField; nil means "leave unset"
}

// EdgeSpec describes an edge on an entity.
type EdgeSpec struct {
    Cardinality  Cardinality
    Target       string       // target entity Name (e.g. "Team")
    TargetIDType reflect.Type
    Inverse      bool
}

// Descriptor is the static, package-init-time descriptor for one entity.
// One instance per entity, shared across all Mutation[T] of that entity.
type Descriptor struct {
    Name   string
    IDType reflect.Type
    Fields map[string]FieldSpec
    Edges  map[string]EdgeSpec

    // OldValueFn fetches the existing entity for OldField support.
    // Returns the entity boxed as `any` (Mutation passes through reflect).
    OldValueFn func(ctx context.Context, c any, id any) (any, error)

    // IDsFn implements IDs(ctx) for Update/Delete mutations.
    IDsFn func(ctx context.Context, c any, preds ...func(*sql.Selector)) ([]any, error)
}

// Mutation is the single generic mutation type used by every entity.
// T is a phantom marker letting callers parameterise on the entity struct
// for typed helpers; no T-typed fields live on the mutation itself.
type Mutation[T any] struct {
    Config any // entity package's Config; opaque to entbuilder

    desc *Descriptor

    op  Op
    id  any
    typ string

    // Field state.
    fields  map[string]any      // set values keyed by schema field name
    cleared map[string]struct{} // cleared fields + cleared edges (shared namespace; collisions impossible since field & edge names live in disjoint sets at the schema level)
    added   map[string]any      // numeric increments

    // Edge state.
    edges        map[string]map[any]struct{} // edge name → neighbor ID set
    removedEdges map[string]map[any]struct{} // M2M only

    // Lifecycle.
    done       bool
    oldValue   func(context.Context) (any, error) // captured at WithXID time
    oldOnce    sync.Once
    oldCached  any
    oldErr     error
    predicates []func(*sql.Selector)
    idsFunc    func(context.Context, ...func(*sql.Selector)) ([]any, error)
}

// Compile-time assertion that the generic mutation satisfies the ent.Mutation interface.
// One concrete instantiation suffices for the compiler — Mutation[any] is fine.
var _ ent.Mutation = (*Mutation[any])(nil)

// NewMutation constructs a generic mutation for an entity.
func NewMutation[T any](c any, op Op, desc *Descriptor, opts ...func(*Mutation[T])) *Mutation[T] {
    m := &Mutation[T]{
        Config:  c,
        desc:    desc,
        op:      op,
        typ:     desc.Name,
        cleared: make(map[string]struct{}),
    }
    for _, opt := range opts { opt(m) }
    return m
}
```

The methods on `*Mutation[T]` implementing every `ent.Mutation` interface call live in `runtime/entbuilder/mutation_methods.go` (split for file-size manageability). Each method dispatches via the descriptor instead of a per-entity switch.

**OldField via reflect** — no per-field extractor closures:

```go
func (m *Mutation[T]) OldField(ctx context.Context, name string) (any, error) {
    spec, ok := m.desc.Fields[name]
    if !ok { return nil, fmt.Errorf("unknown %s field %s", m.desc.Name, name) }
    if !m.op.Is(OpUpdateOne) { return nil, fmt.Errorf("OldField only allowed on UpdateOne") }
    if m.id == nil || m.oldValue == nil { return nil, errors.New("OldField requires ID + loader") }

    m.oldOnce.Do(func() {
        if m.done {
            m.oldErr = errors.New("querying old values post mutation is not allowed")
            return
        }
        m.oldCached, m.oldErr = m.oldValue(ctx)
    })
    if m.oldErr != nil { return nil, m.oldErr }
    return reflect.ValueOf(m.oldCached).Elem().FieldByName(spec.GoName).Interface(), nil
}
```

Reflect read cost: ~50-100 ns/call. Cached behind `sync.Once` per mutation. Hook bench gate (≤100 ns mean for `Field`) is met without reflect on the hot read path (`Field()` returns from `m.fields` map directly).

**Edge methods on the generic mutation** replace today's typed `Add<Edge>IDs`/`Remove<Edge>IDs`/`Clear<Edge>`/etc.:

```go
func (m *Mutation[T]) AddEdgeIDs(edge string, ids ...any) error
func (m *Mutation[T]) RemoveEdgeIDs(edge string, ids ...any) error // M2M only; errors on unique
func (m *Mutation[T]) SetEdgeID(edge string, id any) error          // unique edges
func (m *Mutation[T]) ClearEdge(edge string) error
func (m *Mutation[T]) EdgeID(edge string) (any, bool)               // unique edges
func (m *Mutation[T]) EdgeIDs(edge string) []any                    // any cardinality
func (m *Mutation[T]) RemovedEdgeIDs(edge string) []any             // M2M only
```

The existing `ent.Mutation` interface methods (`AddedEdges()`, `AddedIDs(name)`, `RemovedEdges()`, `RemovedIDs(name)`, `ClearedEdges()`, `EdgeCleared(name)`, `ResetEdge(name)`) stay — they're the iteration contract hooks consume today.

**Typed call-site helpers** in `runtime/entbuilder` for ergonomics:

```go
func GetField[V any](m *Mutation[any], name string) (V, bool)              // unbox m.Field(name)
func OldFieldAs[V any](ctx context.Context, m *Mutation[any], name string) (V, error) // unbox OldField
func EdgeIDsAs[ID any](m *Mutation[any], edge string) []ID                  // unbox EdgeIDs
func EdgeIDAs[ID any](m *Mutation[any], edge string) (ID, bool)             // unbox EdgeID
```

These are package-level generic functions taking `*Mutation[any]` so callers can use them against any entity's mutation without re-stating T.

### 4.2 Generated `<entity>_mutation.go` after refactor

Before (~711 LOC for `task_mutation.go`):

```go
type TaskMutation struct {
    Config
    op            Op
    typ           string
    id            *int
    title         *string
    description   *string
    status        *TaskStatus
    uuid          *uuid.UUID
    clearedFields map[string]struct{}
    teams         map[int]struct{}
    removedteams  map[int]struct{}
    clearedteams  bool
    owner         *int
    clearedowner  bool
    done          bool
    oldValue      func(context.Context) (*Task, error)
    predicates    []predicate.Task
    idsFunc       func(context.Context, ...predicate.Task) ([]int, error)
}
// + 30+ typed methods (SetTitle, Title, OldTitle, ResetTitle, etc.)
// + AddTeamIDs/RemoveTeamIDs/ClearTeams/etc. per edge
// + Field()/SetField()/OldField()/ClearField()/ResetField()/AddedField()/AddField()
//   switch tables (~150 LOC)
```

After (~55 LOC):

```go
// Code generated by ent, DO NOT EDIT.
package internal

import (
    "context"
    "reflect"

    "entgo.io/ent/dialect/sql"
    "entgo.io/ent/entc/integration/privacy/ent/predicate"
    "entgo.io/ent/runtime/entbuilder"

    "github.com/google/uuid"
)

// TaskMutation is the mutation type for the Task entity.
type TaskMutation = entbuilder.Mutation[Task]

// taskDescriptor describes Task's fields and edges. Built once at package init.
var taskDescriptor = &entbuilder.Descriptor{
    Name:   "Task",
    IDType: reflect.TypeFor[int](),
    Fields: map[string]entbuilder.FieldSpec{
        "title":       {Type: reflect.TypeFor[string](),    GoName: "Title"},
        "description": {Type: reflect.TypeFor[string](),    GoName: "Description", Nillable: true},
        "status":      {Type: reflect.TypeFor[TaskStatus](),GoName: "Status"},
        "uuid":        {Type: reflect.TypeFor[uuid.UUID](), GoName: "UUID",        Nillable: true},
    },
    Edges: map[string]entbuilder.EdgeSpec{
        "teams": {Cardinality: entbuilder.M2M,       Target: "Team", TargetIDType: reflect.TypeFor[int]()},
        "owner": {Cardinality: entbuilder.O2OUnique, Target: "User", TargetIDType: reflect.TypeFor[int](), Inverse: true},
    },
    OldValueFn: func(ctx context.Context, c any, id any) (any, error) {
        return c.(*Config).Task.Get(ctx, id.(int))
    },
    IDsFn: func(ctx context.Context, c any, preds ...func(*sql.Selector)) ([]any, error) {
        ids, err := c.(*Config).Task.Query().Where(taskPreds(preds)...).IDs(ctx)
        if err != nil { return nil, err }
        out := make([]any, len(ids))
        for i, id := range ids { out[i] = id }
        return out, nil
    },
}

func taskPreds(ps []func(*sql.Selector)) []predicate.Task {
    out := make([]predicate.Task, len(ps))
    for i, p := range ps { out[i] = p }
    return out
}

// NewTaskMutation creates a new mutation for the Task entity.
func NewTaskMutation(c Config, op Op, opts ...func(*TaskMutation)) *TaskMutation {
    return entbuilder.NewMutation[Task](&c, op, taskDescriptor, opts...)
}

// Where appends predicates to the Task mutation.
func taskWhere(m *TaskMutation, ps ...predicate.Task) {
    s := make([]func(*sql.Selector), len(ps))
    for i, p := range ps { s[i] = p }
    m.WhereP(s...)
}
```

The `WithTaskID` / `WithTask` / `WithTaskIDsFunc` option constructors stay (they capture entity-specific old-value loaders) but become 3-5 LOC each instead of 20 LOC.

### 4.3 CUD builder template updates

Today's generated CUD builders call typed methods on the mutation:

```go
// <entity>_create.go (today)
func (tc *TaskCreate) SetTitle(s string) *TaskCreate {
    tc.mutation.SetTitle(s)
    return tc
}
```

After PR 5:

```go
// <entity>_create.go (after)
func (tc *TaskCreate) SetTitle(s string) *TaskCreate {
    _ = tc.mutation.SetField("title", s) // SetField returns error only on type mismatch; impossible here since types come from the descriptor at codegen time
    return tc
}
```

Public CUD setter signatures unchanged. Internals point at `SetField`. Same for `<entity>_update.go`, `<entity>_delete.go`.

`tc.mutation.<entity-specific-typed-old-value>()` calls in CUD code → use `entbuilder.OldFieldAs[V]`.

### 4.4 PR 3 deprecated wrapper deletion

`entc/gen/template/where.tmpl` stops emitting the `// Deprecated: ...` 1-line wrappers for non-ID fields (and `import.tmpl` drops the `entgo.io/ent/where` import if no remaining call sites need it within the regenerated package — verify at implementation). ID-specific helpers (`IDIn`, `IDEQ`, etc.) stay — they're consumed by generated `<entity>_query.go` loader code (`query.Where(user.IDIn(ids...))`).

Fixtures regenerate cleanly. Migration tool handles all call-site rewrites (see §5).

## 5. Migration tool (`cmd/ent-codegen-migrate`)

New CLI in this fork (importable as `entgo.io/ent/cmd/ent-codegen-migrate`):

```
go run entgo.io/ent/cmd/ent-codegen-migrate ./path/to/consumer/...
```

AST rewrites for two PR transitions in one tool:

**Mutation API (PR 5):**

| Old call | New call |
|---|---|
| `m.SetTitle(v)` | `m.SetField("title", v)` |
| `m.Title()` returning `(string, bool)` | `entbuilder.GetField[string](m, "title")` |
| `m.OldTitle(ctx)` returning `(string, error)` | `entbuilder.OldFieldAs[string](ctx, m, "title")` |
| `m.ResetTitle()` | `m.ResetField("title")` |
| `m.ClearTitle()` | `m.ClearField("title")` |
| `m.TitleCleared()` | `m.FieldCleared("title")` |
| `m.AddTeamIDs(ids...)` | `m.AddEdgeIDs("teams", asAny(ids)...)` |
| `m.RemoveTeamIDs(ids...)` | `m.RemoveEdgeIDs("teams", asAny(ids)...)` |
| `m.TeamsIDs()` | `entbuilder.EdgeIDsAs[int](m, "teams")` |
| `m.ClearTeams()` | `m.ClearEdge("teams")` |
| `m.TeamsCleared()` | `m.EdgeCleared("teams")` |
| `m.SetOwnerID(id)` | `m.SetEdgeID("owner", id)` |
| `m.OwnerID()` returning `(int, bool)` | `entbuilder.EdgeIDAs[int](m, "owner")` |

**Predicate API (PR 3 retro):**

| Old call | New call |
|---|---|
| `user.NameEQ(x)` | `where.EQ(user.FieldName, x)` |
| `user.NameContains(x)` | `where.Contains(user.FieldName, x)` |
| (all other `<entity>.<Field><Op>(...)` per [PR 3 migration guide](2026-05-16-predicate-collapse-migration.md)) | corresponding `where.<Op>(<entity>.Field<Field>, ...)` |

ID-specific helpers (`user.IDIn`, `user.IDEQ`, etc.) untouched — they remain part of the API.

**Tool internals.** Driven by the same `Descriptor` shape PR 5 introduces: tool walks the consumer's generated `*_mutation.go` files (or a `--descriptor` file pointing at the schema), builds the rewrite table, AST-walks consumer source, applies patches, runs `goimports`. Rough size: ~400-600 LOC of Go using `golang.org/x/tools/go/ast/astutil` and `go/types` for type info.

**Tool ships in PR 5.** Acceptance gate: tool transforms the in-repo `entc/integration/{privacy,hooks,edgeschema}/...` test files (excluding generated code) cleanly, all tests still pass after rewrite.

## 6. Why map-backed over per-entity struct

Three candidates were considered:

| Option | Per-entity LOC | Per-field access | unsafe | Generic stencil cost |
|---|---|---|---|---|
| A — `Mutation[T]` with `map[string]any` storage; no per-entity struct | ~55 | ~50-80 ns | none | 134 × small |
| B — Per-entity struct + `unsafe.Offsetof` table | ~250 | ~2 ns | yes | 134 × medium |
| C — Per-entity struct + cached `reflect` indices | ~250 | ~80 ns | none | 134 × medium |

A wins for the epic's primary metric (LOC + compile-time RSS):

- **LOC delta**: A drops ~94K LOC at consumer scale; B/C drop ~67K. A's extra ~27K is per-entity struct field declarations that B/C still emit.
- **Compile-time RSS**: dominated by per-package type-checker symbol-table size; A removes the most symbols.
- **Hook bench gate (≤100 ns mean for `Field`)**: all three pass. B's 2 ns is irrelevant since hooks run at request rate, not loop rate.
- **`unsafe` audit surface**: A and C eliminate.
- **DevX**: roughly equal across all three in steady state. A's minor disadvantage is `reflect.TypeOf(m) == *entbuilder.Mutation[Task]` rather than `*TaskMutation`; documented in MIGRATION.md.

The migration tool (§5) handles call-site fan-out for any of the three, so the choice is unconstrained by consumer-side ergonomics.

## 7. Why drop the compat shim

The parent spec §4 PR 5 and §5.3 planned `FeatureTypedMutation` — an opt-in flag that synthesizes typed methods (`SetTitle`, `Title`, etc.) as 1-line wrappers around the generic API, shipped for one release then removed. We diverge per [[feedback-api-contract-changes-ok]]:

- Migration is one-time and automatable (the tool in §5 makes it a single `go run` command at the consumer)
- Maintaining the shim means emitting ~150 LOC × 134 entities = ~20K LOC of wrapper code in PR 5 only to delete it in PR 5.1
- The shim adds template complexity (gated emission, two code paths to test)
- LOC win is recurring forever; migration cost is one weekend

Shim's only justification would be a consumer whose codebase the tool can't reliably rewrite. The mutation call patterns are mechanical (receiver type + method name + arg list); the AST rewriter handles them deterministically.

## 8. Stack-relative ordering

- **Depends on PR 3** (predicate type alias makes `predicate.Task = func(*sql.Selector)`) — needed for `Mutation.WhereP` and the `taskPreds` adapter.
- **Depends on PR 4** (generic builders) only for thematic consistency — PR 4 introduced the `runtime/entbuilder` package as the home for generic state types. PR 5 extends it with `Mutation[T]`.
- **Does NOT depend on PR 2** (the parent spec linked PR 5 to PR 2 via `FeatureTypedMutation`, which is now dropped).
- **Blocks PR 6** (per-entity packages) less than originally thought — PR 6 splits the `internal/` package into per-entity sub-packages; with PR 5, `internal/` is much smaller, so PR 6's split is easier but optional.

## 9. Acceptance criteria

- **All integration tests pass** for `./entc/integration/privacy/...`, `./entc/integration/hooks/...`, `./entc/integration/edgeschema/...`. Hook ordering and lifecycle semantics preserved.
- **`entc/gen` regression fixtures pass** after regeneration. Fixture diffs reviewed alongside template changes.
- **`internal/bench/pr5.jsonl` shows ≥85% LOC reduction** in `<entity>_mutation.go` files across the three in-repo fixtures (eyeball target — confirmed at bench time).
- **Hook bench gate**: `internal/bench/mutation_hot_path_test.go` runs as `go test -bench BenchmarkMutationField`; `m.Field("title")` ≤ 100 ns/op mean, `m.SetField("title", v)` ≤ 200 ns/op mean. Test fails the build if exceeded.
- **`go build ./...` clean**.
- **`go vet ./...` clean** (modulo any pre-existing warnings).
- **`runtime/entbuilder` has unit tests** for `Mutation[T]`, `Descriptor`, all generic methods, edge cardinality enforcement, type validation, and the four typed call-site helpers.
- **Migration tool unit tests** cover every rewrite rule with table-driven fixtures (input AST → expected output AST).
- **Migration tool integration test**: runs against `entc/integration/edgeschema/ent/`-shape hand-written hooks (added as part of PR 5 as test fixtures), produces compilable output that passes the integration tests.
- **PR 3 deprecated wrappers gone** from regenerated fixtures; no `// Deprecated:` markers remain in `<entity>/where.go`.

## 10. Risks

| Risk | Mitigation |
|---|---|
| Generic instantiation cost (Go 1.18+ GC-shape stenciling) erodes the LOC win at scale | Measure at consumer scale before declaring victory; each entity has one stencil of `Mutation[T]` but the stencils are small (~100 LOC of generic code total). |
| Reflect-based OldField is slow in hot paths | Cached behind `sync.Once`; hook bench gate enforces the budget. If a real consumer hits the gate, fall back to descriptor-attached per-field extractor closures (~5 LOC per field; reversible). |
| Map-allocation cost on every NewMutation hurts request-rate throughput | Each map is created lazily on first use (not in NewMutation); for read-mostly mutations (e.g. update with one field set) only the `fields` map allocates. |
| Hooks reach into deprecated typed methods despite the migration tool | Tool runs in CI as a check; consumer's CI fails until tool has been run. Migration guide describes the one-command fix. |
| `ent.Mutation` interface compliance subtly drifts | Compile-time assertion `var _ ent.Mutation = (*Mutation[any])(nil)`; integration test suite exercises every interface method. |
| `Where(ps ...predicate.Task)` becomes a free function (`taskWhere(m, ps...)`) instead of a method — code that calls `m.Where(...)` won't compile after regen | Migration tool rewrites `m.Where(ps...)` → `taskWhere(m, ps...)`, OR keep a per-entity `Where` method (1 LOC wrapper) on the alias type. Decide at implementation time after benchmarking method-on-alias vs free-function. |
| Reflect type lookups via `reflect.TypeFor[T]()` require Go 1.22+ | Check go.mod; ent already requires 1.23 per fixture bench setup (`go 1.23` in bench/fixture template). Safe. |
| `c.(*Config).Task.Get(...)` in `OldValueFn` creates a circular dep (entbuilder doesn't know `Config`, entity package's `Config` references mutations) | The closure is defined in the per-entity file, NOT in entbuilder. The descriptor stores the closure as `func(ctx, c any, id any) (any, error)`; entbuilder calls it without knowing the concrete `Config` type. The per-entity file imports both entbuilder and the entity's own package without cycle. |
| Migration tool misses unusual call patterns (custom wrappers, embedded mutations) | Tool emits warnings for patterns it can't rewrite (e.g. `f(m.SetTitle)` passing the method as a value); maintainer manually fixes. Tool's pass-rate target: ≥99% of call sites auto-rewritten on the consumer fixture. |

## 11. Out of scope (defer to future PRs if ever needed)

- **Concurrent-safe mutations**: today's single-goroutine model preserved.
- **Reducing `ent.Mutation` interface surface**: preserves the interface bit-for-bit.
- **Gremlin backend support**: SQL only.
- **Cross-entity batched mutations**: orthogonal.
- **Removing per-entity `WithXID` option constructors**: stays as today (3-5 LOC each); not worth the codegen complexity of generic-ifying.

## 12. Implementation outline (for `writing-plans` consumption)

The plan should cover roughly these tasks:

1. **Orient** — read existing `entc/integration/privacy/ent/internal/task_mutation.go`, `runtime/entbuilder/{create,update,delete,query}.go`, `entc/gen/template/dialect/sql/where.tmpl` (for §4.4)
2. **Add `Mutation[T]`, `Descriptor`, `FieldSpec`, `EdgeSpec` to `runtime/entbuilder/mutation.go`** with unit tests covering every method against a synthetic descriptor
3. **Add typed call-site helpers** (`GetField`, `OldFieldAs`, `EdgeIDsAs`, `EdgeIDAs`) with unit tests
4. **Add hook bench gate** at `internal/bench/mutation_hot_path_test.go` — fails build if `Field`/`SetField` exceed budget
5. **Modify mutation template** to emit the new shape (alias + descriptor + constructor + `WithXID` options)
6. **Modify CUD templates** (`<entity>_create.go`, `<entity>_update.go`, `<entity>_delete.go`) to call `m.SetField(...)` internally
7. **Flip `where.tmpl`** to stop emitting deprecated wrappers; ID-specific helpers retained
8. **Regenerate fixtures** via `go generate ./entc/integration/...`; verify integration tests pass for privacy/hooks/edgeschema
9. **Build `cmd/ent-codegen-migrate`** (mutation API + predicate API rewrites) with table-driven unit tests
10. **Add migration-tool integration test** that rewrites in-repo hand-written hooks and runs the integration suite against the rewritten output
11. **Run bench harness**; commit `internal/bench/pr5.jsonl`
12. **Final verification** + epic spec §0 progress update (mark PR 5 complete, PR 6 next); add migration tool usage to `MIGRATION.md`

Critical plan callouts:
- **Branch-discipline pre-flight** on every commit subagent: `pwd && git rev-parse --abbrev-ref HEAD` must show `worktree-wiggly-singing-pancake`; three prior implementer subagents committed to master by accident in PRs 0-2
- **Hook ordering preservation** (Risk #5): integration tests on hook lifecycle must pass
- **Migration tool's failure mode** (Risk #7): warnings for unrewritable patterns; tool exit code documents pass-rate
- **`Where` method vs free function** (Risk #6): benchmark both at implementation; pick the cheaper option
- **Per-PR bench commit** + epic spec §0 progress update follows the same shape as PR 3/PR 4 closing commits
