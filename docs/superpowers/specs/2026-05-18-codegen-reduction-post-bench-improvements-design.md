# Codegen Reduction — Post-Bench Improvements Design

**Status:** Design draft, awaiting user review
**Date:** 2026-05-18
**Worktree:** `.claude/worktrees/wiggly-singing-pancake` on branch `worktree-wiggly-singing-pancake`
**Parent spec:** [docs/superpowers/specs/2026-05-15-ent-codegen-reduction-epic.md](2026-05-15-ent-codegen-reduction-epic.md) (epic §8 verdict and §9 follow-up scope)
**Stack tool:** git-spice deferred — all commits stay local until end-of-epic per [[feedback-no-prs-until-end-of-epic]]

## 1. Context

The 7-PR codegen-reduction stack landed locally and the consumer-scale bench (MatthewsREIS/gemini service-api-go, 134 entities) measured the §8 targets ([[project-codegen-reduction-bench-results]]):

| Section | Baseline | Post-PR-6 | Δ wall | Δ RSS |
|---|---|---|---|---|
| GENERATE cold | 5:27, 9.6 GB | 2:43, 7.3 GB | **−50%** | −24% |
| GENERATE warm | 2:30, 8.8 GB | 1:24, 7.3 GB | **−44%** | −17% |
| GQLGEN cold | 4:35, 7.5 GB | 3:33, 5.5 GB | −23% | −27% |
| GQLGEN warm | 0:45, 2.7 GB | 0:36, 2.3 GB | −20% | −15% |
| BUILD cold | 2:15, 10.2 GB | **2:32**, 8.8 GB | **+13%** | −14% |
| BUILD warm | 0:06, 2.5 GB | 0:06, 3.0 GB | ~ | +20% |

The wins on generate and on memory are clear; the build-wall regression and the LOC miss (1.57M / −7%, target ≤850K) are unfinished business.

### 1.1 Diagnostic findings (2026-05-18 session)

**LOC inventory (1.61M total, 2,905 files):**

- `gen/` root package alone: **403K LOC** in one Go package — the dominant compile-time hotspot
  - `gql_*.go` (where_input, mutation_input, pagination, node, edge, collection): 275K
  - `*_facade.go` (per-entity edge helpers + type aliases): 86K
  - `entql.go`, `fake.go`, `indexing.go`, `queryhelpers.go`, `search_*.go`: ~42K
- `gen/internal/` (cycle-break re-exports for entity types): 108K LOC in one package
- Top per-entity subpackages: escrow 88K, property 49K, contact 43K
- Per-entity `create.go`/`update.go`/`where.go` are already structurally minimal (4-line setters, 3-line predicates) — bulk is API surface area, not redundancy
- Per-entity `mutation.go` is **20 lines** (PR-5 mutation collapse verified working at scale)

**Build profile (cold `go build -p=32` with action graph):**

| Package | Compile time |
|---|---|
| `src/graph` (gqlgen `generated.go`) | **89s** ← actual long-pole |
| `src/ent/gen` (root) | **52s** |
| `src/resolvers` | 18s |
| `src/ent/gen/internal` | 16s |
| `src/ent/gen/escrow` (biggest entity subpkg) | 12s |
| Other entity subpkgs | 5-9s each, parallel |

**Critical path:** entity subpkgs (12s parallel) → internal (16s) → gen root (52s) → src/graph (89s) ≈ **169s**. The `src/graph` long-pole is out of scope per epic §9 (gqlgen separate epic) — the realistic build-wall floor with all in-scope improvements is ~110-120s.

**Architectural conclusion:** the gen root package is the highest-leverage in-scope target. Splitting it reverses the +13% regression and goes net negative vs baseline.

## 2. Goals

1. **Reverse the BUILD cold +13% regression** and push wall to ~−10% vs baseline (`2:15 → ~2:00`).
2. **Recover meaningful LOC** through facade-pattern collapse + nibbles: target `−30K to −60K LOC` (1.57M → ~1.52M, ~−10% on top of PR-5).
3. **Reduce build peak RSS** by an additional ~10-15% via package-size split.
4. **Honest re-framing of §8 LOC target.** The −50% / ≤850K goal is structurally unattainable without API-contract changes (drop setter methods, drop comparator predicates per field). Reframe to the achievable LOC win and document.

## 3. Non-goals

- **gqlgen-output reduction.** `src/graph` long-pole stays — epic §9 deliberately scopes it as a separate effort. The build-wall floor from this work is bounded by it.
- **API-contract changes.** Public surface stays compatible. No removal of per-field setters, per-comparator predicate functions, or facade helper names. All splits use type aliases / one-line wrappers to preserve consumer call sites.
- **Migration-tool ergonomics.** [[project-codegen-reduction-pr6-consumer-migration-gap]] already shipped; no new tool work in this design. (If a future PR-6-style migration surfaces new patterns, extend the tool then.)
- **Schema-side template fixes.** The entsearch / entsf / enthubspot template residuals from the bench session are consumer-owned templates; not addressed here.

## 4. Architecture

Three improvements: **B (gen-root split)**, **F (generic edge helpers)**, **N (nibbles)**. B is the urgent build-wall fix; F is the LOC win; N is cleanup.

### 4.1 Lever B — Split gen root into sibling subpackages

**Goal:** drop gen root from 403K LOC to ≤50K LOC by moving bulk into sibling subpackages that compile in parallel.

**Sibling-subpackage architecture:**

```
gen/<entity>/                 ← entity-self-contained code (existing)
gen/internal/                 ← cycle-break re-exports (existing)
gen/edges/                    ← NEW: facade load<Edge> + With/Query/QueryFromQuery bodies
gen/whereinputs/              ← NEW: all *WhereInput types (cross-entity refs OK at sibling level)
gen/mutationinputs/           ← NEW: all *MutationInput types
gen/gqledges/                 ← NEW: all gql_edge_<entity>.go bodies
gen/gqlcollections/           ← NEW: all gql_collection_<entity>.go bodies
gen/                          ← root, ~50K LOC: type aliases re-exporting from siblings, plus entql.go/fake.go/indexing.go/query+search helpers
```

Each sibling subpackage imports all `gen/<entity>/` packages but no other sibling. They compile in parallel after entity subpackages finish.

**Per-entity subpackages also absorb the entity-self-contained gql files:**

```
gen/<entity>/gql_pagination.go    ← moved from gen/gql_pagination_<entity>.go
gen/<entity>/gql_node.go          ← moved from gen/gql_node_<entity>.go
```

**Movable bulk:**

| Source | Target | LOC | Cycle risk |
|---|---|---|---|
| `gen/<entity>_facade.go` load/With/Query bodies | `gen/edges/<entity>.go` | ~70K | none (sibling imports all entity pkgs) |
| `gen/gql_where_input_*.go` | `gen/whereinputs/<entity>.go` | ~150K | none |
| `gen/gql_mutation_input_*.go` | `gen/mutationinputs/<entity>.go` | ~50K | none |
| `gen/gql_edge_*.go` | `gen/gqledges/<entity>.go` | ~40K | none |
| `gen/gql_collection_*.go` | `gen/gqlcollections/<entity>.go` | ~20K | none |
| `gen/gql_pagination_*.go` | `gen/<entity>/gql_pagination.go` | ~70K | none (entity-self-contained) |
| `gen/gql_node_*.go` | `gen/<entity>/gql_node.go` | ~30K | none (entity-self-contained) |

**Root after split:** entql.go (12K), fake.go (9K), indexing.go (7K), queryhelpers.go (3K), search_helpers.go (3K), search_pagination.go (3K), per-entity facade aliases (~10K total after load-body extraction) ≈ **47K LOC**.

**Surface preservation:** root re-exports via type aliases — consumers continue to write `gen.UserWhereInput`, `gen.WithUserCreatedBy(q)`, etc.

```go
// gen/facade_aliases.go (re-exports from whereinputs/, edges/, ...)
type UserWhereInput = whereinputs.UserWhereInput
func WithUserCreatedBy(q *UserQuery, opts ...func(*UserQuery)) *UserQuery {
    return edges.WithUserCreatedBy(q, opts...)
}
```

The function re-exports add one line per edge in root (currently the function body is 10-25 lines per edge), so the root stays small even with full alias coverage.

**Where the codegen changes live:**

- **ent core (this repo):**
  - `entc/gen/template/facade.tmpl` — split into `facade.tmpl` (root: imports + aliases + one-line wrappers) and a new `edges.tmpl` with `SubPackage: true` routing the load/With/Query/QueryFromQuery bodies to `gen/edges/`.
  - `entc/gen/template.go` — register the new `edges` template, wire SubPackage path → `gen/edges/<entity>.go`.
  - Possibly `entc/gen/gen.go` for any path/import resolution that assumes facade lives only in root.

- **contrib/entgql (separate worktree at `/var/home/smoothbrain/dev/matthewsreis/contrib/.claude/worktrees/entgql-collection-subpkg`):**
  - `entgql/extension.go` — extend the existing hook-based per-entity file emission to route to sibling subpackages (`gen/whereinputs/`, `gen/mutationinputs/`, `gen/gqledges/`, `gen/gqlcollections/`) and to per-entity subpackages (`gen/<entity>/gql_pagination.go`, `gen/<entity>/gql_node.go`). The hook pattern is already proven for `mutation_input_subpkg.tmpl`; extend the same shape.
  - Add new wrapper templates as needed: `where_input_subpkg.tmpl`, `pagination_subpkg.tmpl`, etc. — modeled on existing `mutation_input_subpkg.tmpl`.

**Estimated complexity:** 1-2 days per file-type group, ~5-8 days total for full B coverage. Stage as B-1 / B-2 / B-3 for incremental bench validation (see §5).

### 4.2 Lever F — Generic edge helpers in entbuilder

**Goal:** replace identical per-edge boilerplate in facade.tmpl with calls into a new generic runtime helper, saving ~30-50K LOC and reducing template-execution work during `generate`.

**New file: `runtime/entbuilder/edge_methods.go`**

```go
package entbuilder

import (
    "context"
    "entgo.io/ent/dialect/sql"
    "entgo.io/ent/dialect/sql/sqlgraph"
)

// EagerLoader runs the neighbor-fetch + parent.Edges.<Field> assignment for one edge.
// Per-edge load functions remain code-generated (they have M2M/O2M/M2O branch logic
// and structural Edges.<EdgeName> field assignment that generics cannot express).
// This helper covers the identical boilerplate around the load call.
type EagerLoader[P any, C any] func(ctx context.Context, sub *Query[C], parents []*P) error

// RegisterEagerLoad replaces the per-entity With<Node><Edge> function body.
//
// Before (in facade.tmpl, ~10 lines per edge):
//   func WithUserCreatedBy(q *UserQuery, opts ...func(*UserQuery)) *UserQuery {
//       sub := NewUserClient(q.Config).Query()
//       for _, opt := range opts { opt(sub) }
//       return q.StoreEager("created_by", func(ctx context.Context, parents []*User) error {
//           return loadUserCreatedBy(ctx, sub, parents)
//       })
//   }
// After (one line in facade.tmpl):
//   func WithUserCreatedBy(q *UserQuery, opts ...func(*UserQuery)) *UserQuery {
//       return entbuilder.RegisterEagerLoad(q, "created_by", NewUserClient, loadUserCreatedBy, opts)
//   }
func RegisterEagerLoad[P, C any](
    q *Query[P],
    edgeName string,
    newClient func(any) *Client[C],
    loader EagerLoader[P, C],
    opts []func(*Query[C]),
) *Query[P] { /* ... */ }

// QueryEdgeFromNode replaces the per-entity Query<Node><Edge> function body.
func QueryEdgeFromNode[P, C any](
    c *Client[C],
    node *P,
    step sqlgraph.Step,
) *Query[C] { /* ... */ }

// QueryEdgeFromQuery replaces the per-entity Query<Node><Edge>FromQuery body.
func QueryEdgeFromQuery[P, C any](
    q *Query[P],
    newClient func(any) *Client[C],
    step sqlgraph.Step,
) *Query[C] { /* ... */ }

// RegisterNamedEagerLoad replaces WithNamed<Node><Edge> body.
func RegisterNamedEagerLoad[P, C any](
    q *Query[P],
    edgeName string,
    name string,
    newClient func(any) *Client[C],
    loader EagerLoader[P, C], // a loadNamed* variant
    opts []func(*Query[C]),
) *Query[P] { /* ... */ }
```

**Template changes (entc/gen/template/facade.tmpl):**

- Strip the body of `With<Node><Edge>`, `WithNamed<Node><Edge>`, `Query<Node><Edge>`, `Query<Node><Edge>FromQuery` down to a single `return entbuilder.<Helper>(...)` call.
- Keep `load<Node><Edge>` and `loadNamed<Node><Edge>` per-edge — these have M2M/O2M/M2O branching and structural `parent.Edges.<Field> = ...` assignment that cannot be hoisted (Go generics cannot express named field assignment without an interface; introducing an interface is an API contract change).
- After lever B-1 lands (load bodies move to `gen/edges/`), the entire facade per-edge surface becomes one-liner wrappers in root and full bodies in `gen/edges/`.

**LOC math:** 134 entities × ~10 edges average × (4 functions × ~10-25 lines each, condensed to ~5 lines each) ≈ **−40K to −50K LOC**. The big `load<Edge>` and `loadNamed<Edge>` bodies (30-90 lines each, M2M up to 90) remain.

**Why not also genericize `load<Edge>`:** the body assigns to `parent.Edges.<EdgeName>` and `parent.AppendNamed<EdgeName>(name, n)` — Go generics cannot express named struct-field assignment. An interface `type EdgeSetter[C] interface { SetEdge<EdgeName>(C) }` would require per-edge generated methods, defeating the point. A `map[string][]any` Edges representation would be an API contract change. Punt to a future epic.

### 4.3 Lever N — Nibbles

Two small surface-area trims with negligible behavior risk:

- **Skip Cmp predicates for UUID-ID fields.** `IDGT` / `IDGTE` / `IDLT` / `IDLTE` are emitted unconditionally on the ID field. UUIDs aren't meaningfully orderable in business logic. Gate the predicate template on `(not (eq $n.ID.Type "uuid.UUID"))`. **Estimate: −3K to −5K LOC.**
- **Confirm-and-elide string-only predicates on non-string fields.** `EqualFold` / `ContainsFold` / `Contains` / `HasPrefix` / `HasSuffix` should already be conditional on string type. If not, gate them. Needs verification before claiming a saving. **Estimate: up to −5K LOC if currently unconditional.**

## 5. Implementation order and bench gates

The user instruction is **bench numbers gate everything** — improvements without measured impact don't ship. Stage as follows:

1. **B-1 (ent core: facade edge bodies → `gen/edges/`).** Smallest atomic ent-only change. Re-run consumer bench, measure build wall + RSS delta. Required improvement to proceed: any measurable build-wall reduction (even 5%) confirms the architectural direction.
2. **B-2 (entgql contrib: pagination + node per-entity).** No cross-entity risk; smaller change. Re-bench.
3. **B-3 (entgql contrib: where_input + mutation_input + gqledges + gqlcollections siblings).** Larger change; produces the bulk of the build-wall win. Re-bench.
4. **Decision point.** Compare measured B totals against §6 predictions. If build wall hits the §6 range and the user accepts the LOC outcome (B is LOC-neutral, so total LOC stays at ~1.57M minus N), ship the epic. Otherwise proceed to F as the recovery lever.
5. **F (entbuilder edge helpers + facade.tmpl one-liners).** Deferred contingency — held in reserve as the LOC recovery lever (the only in-scope lever that actually shrinks LOC) and as insurance if any B sub-lever underperforms. **Not skipped**; sequenced after the architectural changes prove out so we measure B's contribution cleanly first.
6. **N (nibbles).** Run independently of the B/F decision when there's time — pure cleanup with negligible behavior risk.

Re-bench after each step using the existing `/tmp/bench-pr6-post-uncapped.sh` script and diff against `/var/tmp/bench-pr6/post-pr06-uncapped-20260518-040049.txt`.

**Bench environment constraints** (per [[feedback-bench-build-memory-limits]]):
- `GOMEMLIMIT=8GiB GOGC=25` on all `go build` / `go run entc.go` invocations
- No `GOMAXPROCS` cap on the 32-core/62GB bench host (uncapped is fair-comparison vs baseline)
- Each ent/contrib worktree edit followed by clean regen → migration tool → build in the bench worktree

**Bench worktree note:** the post-PR-6 bench worktree is currently in a working state per [[project-codegen-reduction-pr6-consumer-migration-gap]]. Do not reset it. Use a snapshot or a second bench worktree for any experimentation that mutates `src/ent/gen/`.

## 6. Predicted bench impact

Predictions are falsifiable; re-bench validates each.

| Lever | LOC Δ | BUILD cold wall Δ | BUILD cold RSS Δ |
|---|---|---|---|
| B-1 (facade edges → subpkg) | neutral | −5% to −10% | −5% |
| B-2 (pagination + node per-entity) | neutral | −3% to −5% | −3% |
| B-3 (where/mutation inputs siblings) | neutral | −10% to −15% | −5% to −10% |
| **B total** | **neutral** | **−18% to −25%** vs current | **−10% to −15%** |
| F (generic edge helpers) | **−30K to −50K** | ~0% | ~0% |
| N (nibbles) | −5K to −10K | ~0% | ~0% |

**Composite outcome vs current post-PR-6 (BUILD cold 2:32 / 8.8 GB):**
- BUILD cold: ~1:55 to ~2:05 wall, ~7.5-7.9 GB RSS
- vs baseline (BUILD cold 2:15): **net ~−10% wall**, **~−25% RSS**
- LOC: 1.57M → ~1.51M to ~1.53M (~−10% on top of PR-5)

The §8 −50% LOC target stays missed; that target is structurally unattainable without contract changes.

## 7. Risks and open questions

- **R1: sibling-subpackage compile parallelism may not materialize at predicted scale.** The action-graph profile shows entity subpkgs already parallelize; siblings should behave similarly, but Go's build scheduler is the real test. If B-1 alone doesn't show a build-wall improvement, re-evaluate whether sibling splits help at all.
- **R2: type-alias re-exports in root for non-trivial types** (interface-implementing structs, types with methods). Currently each per-entity facade has `type UserWhereInput = whereinputs.UserWhereInput`. If a type has methods defined via `func (w *UserWhereInput) Foo()`, the method must live in the package where the type is defined (i.e., in `whereinputs/`). Audit needed for which gql_ types carry methods.
- **R3: cross-package imports in `gen/edges/`** — the new sibling needs to import every entity subpackage. Confirm Go's import-cycle detector handles 134 sibling imports cleanly (it should; this is a fan-out, not a cycle).
- **R4: entgql template Option 2 (SubPackage flag in ent core)** vs Option 1 (hook routing in extension.go) is a fork point. Option 1 is the agent's recommendation (~200 LOC, 1-2 days). Option 2 is cleaner but requires ent core changes (~500 LOC, 1-2 weeks) and benefits other extensions. **Decision: pick Option 1 for this design.** Option 2 is a separate future-epic candidate.
- **R5: gqlgen `src/graph` 89s long-pole.** Caps our build-wall improvements at ~110-120s floor. Out of scope per §9 but worth re-acknowledging when reporting numbers.
- **R6: Lever F signatures are illustrative.** Verified: `runtime/entbuilder/query.go` defines `QueryState[P ~func(*sql.Selector)]` (PR-4) — the generic predicate-state struct that per-entity `<Entity>Query` embeds. There is no `Query[T]` or `Client[T]` generic type. The helpers in §4.2 therefore can't take `*Query[P]` directly; actual signatures must thread the per-entity Query type as a constrained type parameter (e.g., `func RegisterEagerLoad[Q hasStoreEager[P], P, C any](q Q, ...) Q`) — interface constraint, not type-parameter substitution. Specifics will land in the implementation plan; the LOC math in §6 holds because the generated call-site shape is the same regardless of helper signature form.

## 8. Verification criteria

A lever is "shipped" only when:
1. Generated code compiles clean in the bench worktree (`./api-graphql/...` builds with no errors after fresh regen + migration tool).
2. Existing in-repo ent integration tests pass (`go test ./...` from ent root).
3. Bench script re-runs and the predicted impact in §6 is met within ±5 percentage points.

If a lever undershoots prediction by >5pp, stop and re-investigate before stacking the next lever.

## 9. Out-of-band items surfaced during diagnostic

- **Bench worktree consumer code outside `./api-graphql/`** (e.g., `internal/oneoff_tasks/`, `cmd/generate/`) has unmigrated call sites — `Contact.QueryCreatedBy`, `Property.QueryOwner`, etc. The migration tool was scoped to `./api-graphql/...` per the resolved gap memo. Either extend tool scope or hand-fix in the bench. Not blocking this design but worth recording.
- **`src/graph` is the actual build long-pole at 89s.** If gqlgen output becomes in-scope later, attacking it produces a bigger build-wall win than anything proposed here. Tag for §9 epic planning.

## 10. Session-resume instructions

If this design's implementation is interrupted:

1. Read [[project-codegen-reduction-epic]] and [[project-codegen-reduction-bench-results]] to recover overall epic state.
2. Read this spec to understand the post-bench design.
3. Check `git log --oneline -20` on `worktree-wiggly-singing-pancake` for in-progress lever commits (typical commit prefix: `feat(entc/gen):` or `refactor(facade):`).
4. Re-run `/tmp/bench-pr6-post-uncapped.sh` and diff against the most recent prior bench result in `/var/tmp/bench-pr6/` to confirm where the work-in-progress puts the numbers.
5. Resume at the next un-shipped lever in §5.

Per [[feedback-no-prs-until-end-of-epic]]: no git pushes, no PRs, no `gs submit` until the user lifts the moratorium.
